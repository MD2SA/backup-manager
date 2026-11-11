package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/MD2SA/backup-manager/internal/api/router"
	"github.com/MD2SA/backup-manager/internal/auth"
	"github.com/MD2SA/backup-manager/internal/backup"
	"github.com/MD2SA/backup-manager/internal/config"
	"github.com/MD2SA/backup-manager/internal/database"
	"github.com/MD2SA/backup-manager/internal/logger"
	"github.com/MD2SA/backup-manager/internal/monitor"
	"github.com/MD2SA/backup-manager/internal/pkg/crypto"
	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/MD2SA/backup-manager/internal/retention"
	"github.com/MD2SA/backup-manager/internal/scheduler"
	"github.com/MD2SA/backup-manager/internal/selfbackup"
	"github.com/MD2SA/backup-manager/internal/service"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type App struct {
	Config          config.Config
	Logger          *slog.Logger
	DB              *pgxpool.Pool
	Router          http.Handler
	Repo            repository.Repository
	Scheduler       *scheduler.Scheduler
	Runner          *backup.BackupRunner
	Retention       *retention.Engine
	Monitor         *monitor.Service
	BackupService   *service.BackupService
	RestoreService  *service.RestoreService
	AuthService     *auth.Service
	RestoreRunner   *backup.RestoreRunner
	ProviderService *service.ProviderService
	MetadataBackup  *selfbackup.Service
}

func New(ctx context.Context, cfg config.Config) (*App, error) {
	log := logger.New(cfg.LogLevel)

	dbPool, err := database.New(ctx, cfg.MetadataDB)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	repo := repository.NewPostgres(dbPool, configKeyForRepo(cfg))
	authService := auth.NewService(repo, log)
	if err := authService.Bootstrap(ctx, cfg.AdminKey, !cfg.IsProduction()); err != nil {
		return nil, fmt.Errorf("auth bootstrap: %w", err)
	}
	retentionEngine := retention.New(repo, log)
	monitorService := monitor.New(repo)
	providerService := service.NewProviderService(repo, cfg)
	notifier := service.NewNotifier(providerService, log)
	gate := backup.NewGate()
	backupService := service.NewBackupService(repo, providerService, retentionEngine, monitorService, cfg, log, gate, notifier)
	restoreService := service.NewRestoreService(repo, providerService, cfg, log, gate, notifier)

	a := &App{
		Config:          cfg,
		Logger:          log,
		DB:              dbPool,
		Repo:            repo,
		Retention:       retentionEngine,
		Monitor:         monitorService,
		BackupService:   backupService,
		RestoreService:  restoreService,
		AuthService:     authService,
		ProviderService: providerService,
	}

	a.logInfrastructureStatus()

	// Initialize runners. Backups and restores share a target database gate so
	// a dump and a restore never overlap.
	a.Runner = backup.NewBackupRunner(log, a.BackupService.ExecuteBackup)
	a.RestoreRunner = backup.NewRestoreRunner(log, a.RestoreService.Execute)
	a.RestoreService.SetQueue(a.RestoreRunner.Enqueue)

	if a.Config.AdminKey == "" {
		log.Warn("SECURITY WARNING: No APP_ADMIN_KEY set. The API is open to anyone with network access.")
	}

	trustedProxies, err := cfg.TrustedProxyPrefixes()
	if err != nil {
		return nil, fmt.Errorf("invalid trusted proxies: %w", err)
	}

	// Initialize Scheduler
	a.Scheduler = scheduler.New(log, func(profileID pgtype.UUID) {
		if err := a.Runner.Enqueue(profileID); err != nil {
			log.Error("Failed to enqueue scheduled backup", "profile_id", profileID, "error", err)
		}
	})

	r := router.New(log, repo, monitorService, authService, a.Config.RateLimitRequests, a.Config.RateLimitWindow, a.Config.CORSOrigins, trustedProxies, func(ctx context.Context) error {
		return dbPool.Ping(ctx)
	}, func(profileID pgtype.UUID) error {
		return a.Runner.Enqueue(profileID)
	}, a.RestoreService.Enqueue, a.syncScheduler)
	a.Router = r

	// Initialize metadata backup service (scheduled pg_dump of internal state).
	a.MetadataBackup = selfbackup.New(log, cfg)

	return a, nil
}

func (a *App) Start(ctx context.Context) {
	if reconciled, err := a.Repo.ReconcileInterruptedRestores(ctx); err != nil {
		a.Logger.Error("Failed to reconcile interrupted restores", "error", err)
	} else if reconciled > 0 {
		a.Logger.Warn("Marked interrupted restores as failed", "count", reconciled)
	}

	a.Runner.Start(ctx)
	a.RestoreRunner.Start(ctx)
	a.Scheduler.Start()
	a.MetadataBackup.Start()

	if err := a.syncScheduler(ctx); err != nil {
		a.Logger.Error("Failed to synchronize scheduler", "error", err)
	}
}

// syncScheduler reconciles the in-memory cron jobs with every enabled profile.
func (a *App) syncScheduler(ctx context.Context) error {
	profiles, err := a.Repo.ListProfiles(ctx)
	if err != nil {
		return err
	}

	jobs := make([]scheduler.Job, 0)
	for _, profile := range profiles {
		if profile.Enabled {
			jobs = append(jobs, scheduler.Job{
				ProfileID: profile.ID,
				Schedule:  profile.Schedule,
			})
		}
	}

	if err := a.Scheduler.Sync(jobs); err != nil {
		return err
	}
	a.Logger.Info("Scheduler synchronized", "jobs", len(jobs))
	return nil
}

// Stop halts background schedulers. It must be called before closing the
// database pool during graceful shutdown.
func (a *App) Stop() {
	if a.MetadataBackup != nil {
		a.MetadataBackup.Stop()
	}
	if a.Scheduler != nil {
		a.Scheduler.Stop()
	}
	done := make(chan struct{})
	go func() {
		if a.Runner != nil {
			a.Runner.Wait()
		}
		if a.RestoreRunner != nil {
			a.RestoreRunner.Wait()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		a.Logger.Warn("Timed out waiting for background runners to stop")
	}
}

func (a *App) logInfrastructureStatus() {
	a.Logger.Info("Infrastructure status",
		"metadata_db", "configured",
		"target_db", a.formatStatus(a.Config.IsTargetDBConfigured(), "configured", "MISSING (Backups will fail)"),
		"storage_path", a.Config.StoragePath,
		"encryption", a.formatStatus(a.Config.AgePublicKey != "" || a.Config.EncryptionPassphrase != "", "enabled", "disabled"),
	)

	if !a.Config.IsTargetDBConfigured() {
		a.Logger.Warn("Target Database is not configured. Any backup or restore attempt will fail.")
	}
}

func (a *App) formatStatus(ok bool, success, failure string) string {
	if ok {
		return success
	}
	return failure
}

// configKeyForRepo derives the AES key for provider-config encryption at rest.
// With no key configured, provider configs are stored in plaintext (dev mode).
func configKeyForRepo(cfg config.Config) []byte {
	if cfg.ConfigEncryptionKey == "" {
		return nil
	}
	return crypto.DeriveConfigKey(cfg.ConfigEncryptionKey)
}
