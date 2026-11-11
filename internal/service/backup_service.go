package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/MD2SA/backup-manager/internal/backup"
	"github.com/MD2SA/backup-manager/internal/config"
	"github.com/MD2SA/backup-manager/internal/metrics"
	"github.com/MD2SA/backup-manager/internal/monitor"
	"github.com/MD2SA/backup-manager/internal/notification"
	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/MD2SA/backup-manager/internal/retention"
	"github.com/MD2SA/backup-manager/internal/storage"
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
	gate            *backup.Gate
	notifier        *Notifier
	metrics         *metrics.Metrics
}

func NewBackupService(
	repo repository.Repository,
	providerService *ProviderService,
	retentionEngine *retention.Engine,
	monitorService *monitor.Service,
	cfg config.Config,
	logger *slog.Logger,
	gate *backup.Gate,
	notifier *Notifier,
	metricsService *metrics.Metrics,
) *BackupService {
	return &BackupService{
		repo:            repo,
		providerService: providerService,
		retentionEngine: retentionEngine,
		monitorService:  monitorService,
		config:          cfg,
		logger:          logger,
		gate:            gate,
		notifier:        notifier,
		metrics:         metricsService,
	}
}

func (s *BackupService) ExecuteBackup(ctx context.Context, profileID pgtype.UUID) error {
	if err := s.ensureTargetDB("Backup"); err != nil {
		return err
	}

	if s.gate != nil {
		if err := s.gate.Acquire(ctx); err != nil {
			return err
		}
		defer s.gate.Release()
	}

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
		ProfileName: pgutil.Slugify(p.Name),
		TempDir:     s.config.TempDir,
		StartTime:   time.Now(),
	}

	initialStages := []backup.Stage{
		&backup.PostgresDumpStage{
			Host:             s.config.TargetDB.Host,
			Port:             s.config.TargetDB.Port,
			User:             s.config.TargetDB.User,
			Password:         s.config.TargetDB.Password,
			DBName:           s.config.TargetDB.DBName,
			SSLMode:          s.config.TargetDB.SSLMode,
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

	if p.EncryptionEnabled {
		if s.config.AgePublicKey == "" && s.config.EncryptionPassphrase == "" {
			err = errors.New("profile encryption is enabled but no Age public key or encryption passphrase is configured")
		} else {
			initialStages = append(initialStages, &backup.AgeEncryptionStage{
				PublicKey:  s.config.AgePublicKey,
				Passphrase: s.config.EncryptionPassphrase,
			})
		}
	}

	dumpOK := false
	if err == nil {
		pipeline := backup.NewPipeline(initialStages...)
		err = pipeline.Run(execCtx)
		dumpOK = err == nil
	}

	providers := make(map[pgtype.UUID]storage.StorageProvider)
	storedCopies := 0

	if err == nil {
		if len(p.StorageProviderIDs) == 0 {
			err = errors.New("no storage providers linked to profile")
		}

		for _, providerID := range p.StorageProviderIDs {
			storageProvider, resolveErr := s.providerService.ResolveStorageProvider(ctx, providerID)
			if resolveErr != nil {
				execCtx.Log(fmt.Sprintf("Failed to resolve storage provider %s: %v", pgutil.UUIDToString(providerID), resolveErr))
				err = resolveErr
				continue
			}
			providers[providerID] = storageProvider

			upload := &backup.StorageStage{Provider: storageProvider}
			execCtx.Log(fmt.Sprintf("Entering stage: %s for provider %s", upload.Name(), pgutil.UUIDToString(providerID)))
			if uploadErr := upload.Execute(execCtx); uploadErr != nil {
				execCtx.Log(fmt.Sprintf("Stage %s failed for provider %s: %v", upload.Name(), pgutil.UUIDToString(providerID), uploadErr))
				err = uploadErr
				continue
			}

			if recordErr := s.repo.AddExecutionStorage(ctx, db.AddExecutionStorageParams{
				ExecutionID: execID,
				ProviderID:  providerID,
				StoragePath: execCtx.BackupPath,
				Size:        execCtx.Size,
			}); recordErr != nil {
				execCtx.Log(fmt.Sprintf("Failed to record storage copy for provider %s: %v", pgutil.UUIDToString(providerID), recordErr))
				err = recordErr
				continue
			}

			storedCopies++
			execCtx.Log(fmt.Sprintf("Stage %s completed successfully for provider %s", upload.Name(), pgutil.UUIDToString(providerID)))
		}
	}

	// Apply retention once, after every provider has had a chance to receive
	// the new backup. Retention failures are operational warnings, not backup
	// failures: the new copy is already safely stored.
	if err == nil && p.RetentionPolicyID.Valid && storedCopies > 0 {
		// Mark this execution successful before retention so the engine sees
		// the new backup as the latest one instead of a running row. The final
		// update below overwrites the interim end time and logs.
		interimEnd := time.Now()
		interimLogs, _ := json.Marshal(execCtx.Logs)
		if _, updateErr := s.repo.UpdateExecution(ctx, db.UpdateExecutionParams{
			ID:          execID,
			Status:      string(backup.StatusSuccess),
			StartTime:   pgutil.ToTimestamptz(execCtx.StartTime),
			EndTime:     pgutil.ToTimestamptz(interimEnd),
			Duration:    pgutil.ToInterval(interimEnd.Sub(execCtx.StartTime)),
			Size:        pgutil.ToInt8(execCtx.Size),
			Checksum:    pgutil.ToText(execCtx.Checksum),
			StoragePath: pgutil.ToText(execCtx.BackupPath),
			Logs:        interimLogs,
			IsEncrypted: execCtx.IsEncrypted,
		}); updateErr != nil {
			execCtx.Log(fmt.Sprintf("Could not mark execution successful before retention: %v", updateErr))
		} else {
			rp, retentionErr := s.repo.GetRetentionPolicy(ctx, p.RetentionPolicyID)
			if retentionErr != nil {
				execCtx.Log(fmt.Sprintf("Failed to load retention policy: %v", retentionErr))
			} else {
				policy := retention.Policy{
					KeepHourly:  int(rp.KeepHourly),
					KeepDaily:   int(rp.KeepDaily),
					KeepWeekly:  int(rp.KeepWeekly),
					KeepMonthly: int(rp.KeepMonthly),
					KeepYearly:  int(rp.KeepYearly),
					YearlyMonth: int(rp.YearlyMonth),
				}
				if applyErr := s.retentionEngine.Apply(ctx, profileID, policy, providers); applyErr != nil {
					execCtx.Log(fmt.Sprintf("Retention failed: %v", applyErr))
				}
			}
		}
	}

	execCtx.EndTime = time.Now()

	cleanup := &backup.CleanupStage{}
	if cleanupErr := cleanup.Execute(execCtx); cleanupErr != nil {
		execCtx.Log(fmt.Sprintf("Cleanup failed: %v", cleanupErr))
	}

	logsJSON, _ := json.Marshal(execCtx.Logs)

	updateParams := db.UpdateExecutionParams{
		ID:          execID,
		Status:      string(backup.StatusSuccess),
		StartTime:   pgutil.ToTimestamptz(execCtx.StartTime),
		EndTime:     pgutil.ToTimestamptz(execCtx.EndTime),
		Duration:    pgutil.ToInterval(execCtx.EndTime.Sub(execCtx.StartTime)),
		Size:        pgutil.ToInt8(execCtx.Size),
		Checksum:    pgutil.ToText(execCtx.Checksum),
		StoragePath: pgutil.ToText(execCtx.BackupPath),
		Logs:        logsJSON,
		IsEncrypted: execCtx.IsEncrypted,
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

	// Embedded metadata snapshots are best-effort and never fail the backup.
	// They are taken after the execution row is finalized so a recovered
	// metadata database contains the accurate status, checksum and storage path
	// of the backup that carried the snapshot.
	if s.config.MetadataBackupEmbed && dumpOK {
		s.embedMetadata(ctx, p)
	}

	if s.metrics != nil {
		profileIDText := pgutil.UUIDToString(profileID)
		status := "success"
		if err != nil {
			status = "failed"
		}
		s.metrics.BackupsTotal.WithLabelValues(profileIDText, p.Name, status).Inc()
		s.metrics.BackupDuration.WithLabelValues(profileIDText, p.Name).Observe(execCtx.EndTime.Sub(execCtx.StartTime).Seconds())
		if err == nil {
			s.metrics.BackupBytes.WithLabelValues(profileIDText, p.Name).Set(float64(execCtx.Size))
			s.metrics.LastSuccess.WithLabelValues(profileIDText, p.Name).SetToCurrentTime()
		}
	}

	return updateErr
}

func (s *BackupService) notifyAll(ctx context.Context, providerIDs []pgtype.UUID, event notification.Event, message string, metadata map[string]interface{}) {
	if s.notifier != nil {
		s.notifier.All(ctx, providerIDs, event, message, metadata)
	}
}

func (s *BackupService) ensureTargetDB(op string) error {
	if err := s.config.ValidateTargetDB(); err != nil {
		s.logger.Error(fmt.Sprintf("%s aborted: Target Database is not configured", op), "error", err)
		return fmt.Errorf("infrastructure error: %w", err)
	}
	return nil
}
