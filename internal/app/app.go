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
	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/MD2SA/backup-manager/internal/retention"
	"github.com/MD2SA/backup-manager/internal/scheduler"
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
}

func New(ctx context.Context) (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("Configuration error: %w", err)
	}
	log := logger.New(cfg.LogLevel)

	dbPool, err := database.New(ctx, cfg.MetadataDB)
	if err != nil {
		return nil, fmt.Errorf("Failed to initialize database: %w", err)
	}

	repo := repository.NewPostgres(dbPool)
	retentionEngine := retention.New(repo)
	monitorService := monitor.New(repo)
	providerService := service.NewProviderService(repo)
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

	// Initialize Backup Runner (Serial execution)
	a.Runner = backup.NewBackupRunner(log, a.BackupService.ExecuteBackup)

	// Initialize Scheduler
	a.Scheduler = scheduler.New(log, func(profileID pgtype.UUID) {
		_ = a.Runner.Enqueue(profileID)
	})

	r := router.New(log, repo, monitorService, func(profileID pgtype.UUID) error {
		return a.Runner.Enqueue(profileID)
	}, a.BackupService.ExecuteRestore, func(p db.Profile) {
		if err := a.Scheduler.SetActiveJob(p.ID, p.Schedule); err != nil {
			log.Error("Failed to schedule activated profile", "profile_id", p.ID, "error", err)
		}
	})
	a.Router = r

	return a, nil
}

func (a *App) Start(ctx context.Context) {
	a.Runner.Start(ctx)
	a.Scheduler.Start()

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
