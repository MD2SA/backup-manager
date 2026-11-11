package repository

import (
	"context"

	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/jackc/pgx/v5/pgtype"
)

type ExecutionStorageRepository interface {
	AddExecutionStorage(ctx context.Context, arg db.AddExecutionStorageParams) error
	ListExecutionStorage(ctx context.Context, executionID pgtype.UUID) ([]db.ExecutionStorage, error)
	GetExecutionStorageForProvider(ctx context.Context, arg db.GetExecutionStorageForProviderParams) (db.ExecutionStorage, error)
	DeleteExecutionStorage(ctx context.Context, arg db.DeleteExecutionStorageParams) error
	CountExecutionStorage(ctx context.Context, executionID pgtype.UUID) (int64, error)
	DeleteExecutionIfNoStorage(ctx context.Context, id pgtype.UUID) (int64, error)
}

func (r *Postgres) AddExecutionStorage(ctx context.Context, arg db.AddExecutionStorageParams) error {
	return r.queries.AddExecutionStorage(ctx, arg)
}

func (r *Postgres) ListExecutionStorage(ctx context.Context, executionID pgtype.UUID) ([]db.ExecutionStorage, error) {
	return r.queries.ListExecutionStorage(ctx, executionID)
}

func (r *Postgres) GetExecutionStorageForProvider(ctx context.Context, arg db.GetExecutionStorageForProviderParams) (db.ExecutionStorage, error) {
	return r.queries.GetExecutionStorageForProvider(ctx, arg)
}

func (r *Postgres) DeleteExecutionStorage(ctx context.Context, arg db.DeleteExecutionStorageParams) error {
	return r.queries.DeleteExecutionStorage(ctx, arg)
}

func (r *Postgres) CountExecutionStorage(ctx context.Context, executionID pgtype.UUID) (int64, error) {
	return r.queries.CountExecutionStorage(ctx, executionID)
}

func (r *Postgres) DeleteExecutionIfNoStorage(ctx context.Context, id pgtype.UUID) (int64, error) {
	return r.queries.DeleteExecutionIfNoStorage(ctx, id)
}
