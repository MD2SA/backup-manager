package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/MD2SA/backup-manager/internal/api/router"
	"github.com/MD2SA/backup-manager/internal/backup"
	"github.com/MD2SA/backup-manager/internal/config"
	"github.com/MD2SA/backup-manager/internal/database"
	"github.com/MD2SA/backup-manager/internal/logger"
	"github.com/MD2SA/backup-manager/internal/monitor"
	"github.com/MD2SA/backup-manager/internal/pkg/crypto"
	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/MD2SA/backup-manager/internal/repository/db"
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
	ProviderService *service.ProviderService
	MetadataBackup  *selfbackup.Service
}

func New(ctx context.Context) (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("configuration error: %w", err)
	}
	log := logger.New(cfg.LogLevel)

	dbPool, err := database.New(ctx, cfg.MetadataDB)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	repo := repository.NewPostgres(dbPool, configKeyForRepo(cfg))
	retentionEngine := retention.New(repo)
	monitorService := monitor.New(repo)
	providerService := service.NewProviderService(repo, cfg)
	backupService := service.NewBackupService(repo, providerService, retentionEngine, monitorService, cfg, log)

	a := &App{
		Config:          cfg,
		Logger:          log,
		DB:              dbPool,
		Repo:            repo,
		Retention:       retentionEngine,
		Monitor:         monitorService,
		BackupService:   backupService,
		ProviderService: providerService,
	}

	a.logInfrastructureStatus()

	// Initialize Backup Runner (Serial execution)
	a.Runner = backup.NewBackupRunner(log, a.BackupService.ExecuteBackup)

	if a.Config.AdminKey == "" {
		log.Warn("SECURITY WARNING: No APP_ADMIN_KEY set. The API is open to anyone with network access.")
	}

	trustedProxies, err := cfg.TrustedProxyPrefixes()
	if err != nil {
		return nil, fmt.Errorf("invalid trusted proxies: %w", err)
	}

	// Initialize Scheduler
	a.Scheduler = scheduler.New(log, func(profileID pgtype.UUID) {
		_ = a.Runner.Enqueue(profileID)
	})

	r := router.New(log, repo, monitorService, a.Config.AdminKey, a.Config.RateLimitRequests, a.Config.RateLimitWindow, a.Config.CORSOrigins, trustedProxies, func(profileID pgtype.UUID) error {
		return a.Runner.Enqueue(profileID)
	}, a.BackupService.ExecuteRestore, func(p db.Profile) {
		if err := a.Scheduler.SetActiveJob(p.ID, p.Schedule); err != nil {
			log.Error("Failed to schedule activated profile", "profile_id", p.ID, "error", err)
		}
	})
	a.Router = r

	// Initialize metadata backup service (scheduled pg_dump of internal state).
	a.MetadataBackup = selfbackup.New(log, cfg)

	return a, nil
}

func (a *App) Start(ctx context.Context) {
	a.Runner.Start(ctx)
	a.Scheduler.Start()
	a.MetadataBackup.Start()

	// Load the currently enabled profile into scheduler
	profiles, err := a.Repo.ListProfiles(ctx)
	if err != nil {
		a.Logger.Error("Failed to list profiles for scheduler", "error", err)
		return
	}

	for _, p := range profiles {
		if p.Enabled {
			if err := a.Scheduler.SetActiveJob(p.ID, p.Schedule); err != nil {
				a.Logger.Error("Failed to add profile to scheduler", "profile_id", p.ID, "error", err)
			}
			break // Only one should be enabled
		}
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
