package service

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/MD2SA/backup-manager/internal/backup"
	"github.com/MD2SA/backup-manager/internal/config"
	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type fakeRestoreRepo struct {
	repository.Repository

	execution db.Execution
	profile   repository.ProfileFull
	records   []db.ExecutionStorage
	activeErr error
	created   *db.CreateRestoreParams
	updates   []db.UpdateRestoreParams
	restore   db.Restore
	createErr error
}

func (f *fakeRestoreRepo) GetExecution(context.Context, pgtype.UUID) (db.Execution, error) {
	return f.execution, nil
}

func (f *fakeRestoreRepo) GetProfile(context.Context, pgtype.UUID) (repository.ProfileFull, error) {
	return f.profile, nil
}

func (f *fakeRestoreRepo) ListExecutionStorage(context.Context, pgtype.UUID) ([]db.ExecutionStorage, error) {
	return f.records, nil
}

func (f *fakeRestoreRepo) GetActiveRestoreByExecution(context.Context, pgtype.UUID) (db.Restore, error) {
	if f.activeErr != nil {
		return db.Restore{}, f.activeErr
	}
	return db.Restore{ID: pgtype.UUID{Bytes: [16]byte{9}, Valid: true}, Status: "running"}, nil
}

func (f *fakeRestoreRepo) CreateRestore(_ context.Context, arg db.CreateRestoreParams) (db.Restore, error) {
	f.created = &arg
	if f.createErr != nil {
		return db.Restore{}, f.createErr
	}
	return f.restore, nil
}

func (f *fakeRestoreRepo) UpdateRestore(_ context.Context, arg db.UpdateRestoreParams) (db.Restore, error) {
	f.updates = append(f.updates, arg)
	return db.Restore{ID: arg.ID, Status: arg.Status, ErrorMessage: arg.ErrorMessage}, nil
}

func restoreTestService(repo *fakeRestoreRepo, cfg config.Config) *RestoreService {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := NewRestoreService(repo, nil, cfg, logger, nil, nil)
	service.checkTarget = func(context.Context, backup.RestoreTarget) error { return nil }
	return service
}

func configuredTarget() config.Config {
	return config.Config{
		TempDir: "/tmp",
		TargetDB: config.DatabaseConfig{
			Host: "target", Port: "5432", User: "u", Password: "p", DBName: "db", SSLMode: "disable",
		},
	}
}

func testRestoreRepo() (*fakeRestoreRepo, pgtype.UUID, pgtype.UUID) {
	executionID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	providerID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	return &fakeRestoreRepo{
		execution: db.Execution{
			ID:        executionID,
			ProfileID: pgtype.UUID{Bytes: [16]byte{3}, Valid: true},
			Status:    "success",
		},
		profile: repository.ProfileFull{
			StorageProviderIDs: []pgtype.UUID{providerID},
		},
		records: []db.ExecutionStorage{
			{ExecutionID: executionID, ProviderID: providerID, StoragePath: "profile/backup.sql"},
		},
		activeErr: pgx.ErrNoRows,
		restore:   db.Restore{ID: pgtype.UUID{Bytes: [16]byte{4}, Valid: true}, Status: "pending"},
	}, executionID, providerID
}

func TestRestoreService_EnqueueRequiresTarget(t *testing.T) {
	repo, executionID, _ := testRestoreRepo()
	service := restoreTestService(repo, config.Config{TempDir: "/tmp"})

	_, err := service.Enqueue(context.Background(), executionID, RestoreOptions{SingleTransaction: true})
	if err != ErrTargetNotConfigured {
		t.Fatalf("expected ErrTargetNotConfigured, got %v", err)
	}
}

func TestRestoreService_EnqueueNoArtifact(t *testing.T) {
	repo, executionID, _ := testRestoreRepo()
	repo.records = nil
	service := restoreTestService(repo, configuredTarget())

	_, err := service.Enqueue(context.Background(), executionID, RestoreOptions{SingleTransaction: true})
	if err != ErrRestoreNoArtifact {
		t.Fatalf("expected ErrRestoreNoArtifact, got %v", err)
	}
}

func TestRestoreService_EnqueueProviderNotLinked(t *testing.T) {
	repo, executionID, _ := testRestoreRepo()
	repo.profile.StorageProviderIDs = nil
	repo.records = nil
	repo.execution.StoragePath = pgtype.Text{String: "legacy.sql", Valid: true}
	service := restoreTestService(repo, configuredTarget())

	_, err := service.Enqueue(context.Background(), executionID, RestoreOptions{
		ProviderID:        pgtype.UUID{Bytes: [16]byte{8}, Valid: true},
		SingleTransaction: true,
	})
	if err != ErrRestoreProvider {
		t.Fatalf("expected ErrRestoreProvider, got %v", err)
	}
}

func TestRestoreService_EnqueueConflict(t *testing.T) {
	repo, executionID, _ := testRestoreRepo()
	repo.activeErr = nil
	service := restoreTestService(repo, configuredTarget())

	_, err := service.Enqueue(context.Background(), executionID, RestoreOptions{SingleTransaction: true})
	if err != ErrRestoreConflict {
		t.Fatalf("expected ErrRestoreConflict, got %v", err)
	}
}

func TestRestoreService_EnqueueQueues(t *testing.T) {
	repo, executionID, _ := testRestoreRepo()
	service := restoreTestService(repo, configuredTarget())

	var queued pgtype.UUID
	service.SetQueue(func(id pgtype.UUID) error {
		queued = id
		return nil
	})

	restore, err := service.Enqueue(context.Background(), executionID, RestoreOptions{SingleTransaction: true})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if queued != repo.restore.ID {
		t.Fatalf("queued %v, want %v", queued, repo.restore.ID)
	}
	if repo.created == nil || repo.created.Status != "pending" {
		t.Fatalf("restore row was not created as pending: %+v", repo.created)
	}
	if restore.ID != repo.restore.ID {
		t.Fatalf("unexpected restore: %+v", restore)
	}
}

func TestRestoreService_EnqueueQueueFullMarksFailed(t *testing.T) {
	repo, executionID, _ := testRestoreRepo()
	service := restoreTestService(repo, configuredTarget())
	service.SetQueue(func(pgtype.UUID) error { return backup.ErrRestoreQueueFull })

	_, err := service.Enqueue(context.Background(), executionID, RestoreOptions{SingleTransaction: true})
	if err != backup.ErrRestoreQueueFull {
		t.Fatalf("expected ErrRestoreQueueFull, got %v", err)
	}
	if len(repo.updates) == 0 || repo.updates[len(repo.updates)-1].Status != "failed" {
		t.Fatalf("queue failure should mark the restore failed, updates=%+v", repo.updates)
	}
}
