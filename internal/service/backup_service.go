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

	s.notify(ctx, p.NotificationProviderID, notification.EventStarted, fmt.Sprintf("Backup started for profile: %s", p.Name), nil)

	storageProvider, err := s.providerService.ResolveStorageProvider(ctx, p.StorageProviderID)
	if err != nil {
		return fmt.Errorf("failed to resolve storage provider: %w", err)
	}

	pipeline := backup.NewPipeline(
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
		&backup.StorageStage{Provider: storageProvider},
		&backup.RetentionStage{Engine: s.retentionEngine, Repo: s.repo, Provider: storageProvider},
		&backup.CleanupStage{},
	)

	execCtx := &backup.ExecutionContext{
		Context:     ctx,
		ExecutionID: execID,
		ProfileID:   profileID,
		TempDir:     s.config.TempDir,
	}

	err = pipeline.Run(execCtx)

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
		s.notify(ctx, p.NotificationProviderID, notification.EventFailed, fmt.Sprintf("Backup failed for profile: %s. Error: %v", p.Name, err), nil)
	} else {
		s.notify(ctx, p.NotificationProviderID, notification.EventSuccess, fmt.Sprintf("Backup completed successfully for profile: %s", p.Name), map[string]interface{}{
			"size":     execCtx.Size,
			"duration": execCtx.EndTime.Sub(execCtx.StartTime).String(),
		})
	}

	_, updateErr := s.repo.UpdateExecution(ctx, updateParams)
	return updateErr
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

	storageProvider, err := s.providerService.ResolveStorageProvider(ctx, profile.StorageProviderID)
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
