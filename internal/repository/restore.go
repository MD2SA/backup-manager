package repository

import (
	"context"

	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/jackc/pgx/v5/pgtype"
)

type RestoreRepository interface {
	CreateRestore(ctx context.Context, arg db.CreateRestoreParams) (db.Restore, error)
	UpdateRestore(ctx context.Context, arg db.UpdateRestoreParams) (db.Restore, error)
	GetRestore(ctx context.Context, id pgtype.UUID) (db.Restore, error)
	ListRestores(ctx context.Context) ([]db.Restore, error)
	ListRestoresByExecution(ctx context.Context, executionID pgtype.UUID) ([]db.Restore, error)
	GetActiveRestoreByExecution(ctx context.Context, executionID pgtype.UUID) (db.Restore, error)
	ReconcileInterruptedRestores(ctx context.Context) (int64, error)
}

func (r *Postgres) CreateRestore(ctx context.Context, arg db.CreateRestoreParams) (db.Restore, error) {
	return r.queries.CreateRestore(ctx, arg)
}

func (r *Postgres) UpdateRestore(ctx context.Context, arg db.UpdateRestoreParams) (db.Restore, error) {
	return r.queries.UpdateRestore(ctx, arg)
}

func (r *Postgres) GetRestore(ctx context.Context, id pgtype.UUID) (db.Restore, error) {
	return r.queries.GetRestore(ctx, id)
}

func (r *Postgres) ListRestores(ctx context.Context) ([]db.Restore, error) {
	return r.queries.ListRestores(ctx)
}

func (r *Postgres) ListRestoresByExecution(ctx context.Context, executionID pgtype.UUID) ([]db.Restore, error) {
	return r.queries.ListRestoresByExecution(ctx, executionID)
}

func (r *Postgres) GetActiveRestoreByExecution(ctx context.Context, executionID pgtype.UUID) (db.Restore, error) {
	return r.queries.GetActiveRestoreByExecution(ctx, executionID)
}

func (r *Postgres) ReconcileInterruptedRestores(ctx context.Context) (int64, error) {
	return r.queries.ReconcileInterruptedRestores(ctx)
}
