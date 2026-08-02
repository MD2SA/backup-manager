package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/MD2SA/backup-manager/internal/backup"
	"github.com/MD2SA/backup-manager/internal/config"
	"github.com/MD2SA/backup-manager/internal/monitor"
	"github.com/MD2SA/backup-manager/internal/notification"
	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/MD2SA/backup-manager/internal/retention"
	"github.com/MD2SA/backup-manager/internal/verification"
	"github.com/jackc/pgx/v5/pgtype"
)

type BackupService struct {
	repo            repository.Repository
	providerService *ProviderService
	retentionEngine *retention.Engine
	monitorService  *monitor.Service
	config          config.Config
	logger          *slog.Logger
}

func NewBackupService(
	repo repository.Repository,
	providerService *ProviderService,
	retentionEngine *retention.Engine,
	monitorService *monitor.Service,
	cfg config.Config,
	logger *slog.Logger,
) *BackupService {
	return &BackupService{
		repo:            repo,
		providerService: providerService,
		retentionEngine: retentionEngine,
		monitorService:  monitorService,
		config:          cfg,
		logger:          logger,
	}
}

func (s *BackupService) ExecuteBackup(ctx context.Context, profileID pgtype.UUID) error {
	p, err := s.repo.GetProfile(ctx, profileID)
	if err != nil {
		return err
	}

	createParams := db.CreateExecutionParams{
		ProfileID: profileID,
		Status:    string(backup.StatusRunning),
	}

	execution, err := s.repo.CreateExecution(ctx, createParams)
	if err != nil {
		return err
	}

	execID := execution.ID

	s.notifyAll(ctx, p.NotificationProviderIDs, notification.EventStarted, fmt.Sprintf("Backup started for profile: %s", p.Name), nil)

	execCtx := &backup.ExecutionContext{
		Context:     ctx,
		ExecutionID: execID,
		ProfileID:   profileID,
		TempDir:     s.config.TempDir,
	}

	// Stages that run only once
	initialStages := []backup.Stage{
		&backup.PostgresDumpStage{
			Host:             s.config.TargetDB.Host,
			Port:             s.config.TargetDB.Port,
			User:             s.config.TargetDB.User,
			Password:         s.config.TargetDB.Password,
			DBName:           s.config.TargetDB.DBName,
			CompressionType:  p.CompressionType,
			CompressionLevel: int(p.CompressionLevel),
		},
		&backup.VerificationStage{
			Strategies: []verification.VerificationStrategy{
				&verification.ArchiveStrategy{},
				&verification.ChecksumStrategy{},
			},
		},
	}

	pipeline := backup.NewPipeline(initialStages...)
	err = pipeline.Run(execCtx)

	if err == nil {
		// Run storage and retention stages for each provider
		for _, sID := range p.StorageProviderIDs {
			storageProvider, sErr := s.providerService.ResolveStorageProvider(ctx, sID)
			if sErr != nil {
				execCtx.Log(fmt.Sprintf("Failed to resolve storage provider %s: %v", pgutil.UUIDToString(sID), sErr))
				err = sErr
				continue
			}

			storageStages := []backup.Stage{
				&backup.StorageStage{Provider: storageProvider},
				&backup.RetentionStage{Engine: s.retentionEngine, Repo: s.repo, Provider: storageProvider},
			}

			for _, stage := range storageStages {
				execCtx.Log(fmt.Sprintf("Entering stage: %s for provider %s", stage.Name(), pgutil.UUIDToString(sID)))
				if stageErr := stage.Execute(execCtx); stageErr != nil {
					execCtx.Log(fmt.Sprintf("Stage %s failed for provider %s: %v", stage.Name(), pgutil.UUIDToString(sID), stageErr))
					err = stageErr
					break
				}
				execCtx.Log(fmt.Sprintf("Stage %s completed successfully for provider %s", stage.Name(), pgutil.UUIDToString(sID)))
			}
		}
	}

	// Always cleanup
	cleanup := &backup.CleanupStage{}
	if cErr := cleanup.Execute(execCtx); cErr != nil {
		execCtx.Log(fmt.Sprintf("Cleanup failed: %v", cErr))
	}

	// Update execution status
	updateParams := db.UpdateExecutionParams{
		ID:          execID,
		Status:      string(backup.StatusSuccess),
		StartTime:   pgutil.ToTimestamptz(execCtx.StartTime),
		EndTime:     pgutil.ToTimestamptz(execCtx.EndTime),
		Duration:    pgutil.ToInterval(execCtx.EndTime.Sub(execCtx.StartTime)),
		Size:        pgutil.ToInt8(execCtx.Size),
		Checksum:    pgutil.ToText(execCtx.Checksum),
		StoragePath: pgutil.ToText(execCtx.BackupPath),
		Logs:        []byte(strings.Join(execCtx.Logs, "\n")),
	}

	if err != nil {
		updateParams.Status = string(backup.StatusFailed)
		updateParams.ErrorMessage = pgutil.ToText(err.Error())
		s.notifyAll(ctx, p.NotificationProviderIDs, notification.EventFailed, fmt.Sprintf("Backup failed for profile: %s. Error: %v", p.Name, err), nil)
	} else {
		s.notifyAll(ctx, p.NotificationProviderIDs, notification.EventSuccess, fmt.Sprintf("Backup completed successfully for profile: %s", p.Name), map[string]interface{}{
			"size":     execCtx.Size,
			"duration": execCtx.EndTime.Sub(execCtx.StartTime).String(),
		})
	}

	_, updateErr := s.repo.UpdateExecution(ctx, updateParams)
	return updateErr
}

func (s *BackupService) notifyAll(ctx context.Context, providerIDs []pgtype.UUID, event notification.Event, message string, metadata map[string]interface{}) {
	for _, id := range providerIDs {
		s.notify(ctx, id, event, message, metadata)
	}
}

func (s *BackupService) notify(ctx context.Context, providerID pgtype.UUID, event notification.Event, message string, metadata map[string]interface{}) {
	if !providerID.Valid {
		return
	}

	provider, err := s.providerService.ResolveNotificationProvider(ctx, providerID)
	if err != nil || provider == nil {
		s.logger.Error("Failed to resolve notification provider", "error", err)
		return
	}

	if err := provider.Send(ctx, event, message, metadata); err != nil {
		s.logger.Error("Failed to send notification", "error", err)
	}
}

func (s *BackupService) ExecuteRestore(ctx context.Context, executionID pgtype.UUID) error {
	execution, err := s.repo.GetExecution(ctx, executionID)
	if err != nil {
		return err
	}

	if !execution.StoragePath.Valid {
		return fmt.Errorf("execution has no storage path")
	}

	profile, err := s.repo.GetProfile(ctx, execution.ProfileID)
	if err != nil {
		return err
	}

	// For restore, we currently just use the first storage provider linked to the profile?
	// Actually, the execution record has the storage path. We need to know which provider it was on.
	// But the current schema doesn't store provider_id in execution.
	// This might be an issue for restore if different providers have different path formats.
	// However, for now, we'll try to use the first one available or the one that works.

	if len(profile.StorageProviderIDs) == 0 {
		return fmt.Errorf("no storage providers linked to profile")
	}

	storageProvider, err := s.providerService.ResolveStorageProvider(ctx, profile.StorageProviderIDs[0])
	if err != nil {
		return fmt.Errorf("failed to resolve storage provider for restore: %w", err)
	}

	pipeline := &backup.RestorePipeline{
		Storage: storageProvider,
		TempDir: s.config.TempDir,
	}
	pipeline.DBConfig.Host = s.config.TargetDB.Host
	pipeline.DBConfig.Port = s.config.TargetDB.Port
	pipeline.DBConfig.User = s.config.TargetDB.User
	pipeline.DBConfig.Password = s.config.TargetDB.Password
	pipeline.DBConfig.DBName = s.config.TargetDB.DBName

	return pipeline.Run(ctx, execution.StoragePath.String)
}
