package repository

import (
	"context"

	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/jackc/pgx/v5/pgtype"
)

type ExecutionRepository interface {
	CreateExecution(ctx context.Context, arg db.CreateExecutionParams) (db.Execution, error)
	UpdateExecution(ctx context.Context, arg db.UpdateExecutionParams) (db.Execution, error)
	GetExecution(ctx context.Context, id pgtype.UUID) (db.Execution, error)
	ListExecutionsByProfile(ctx context.Context, profileID pgtype.UUID) ([]db.Execution, error)
	DeleteExecution(ctx context.Context, id pgtype.UUID) error
	SetExecutionPinned(ctx context.Context, arg db.SetExecutionPinnedParams) error
	GetLatestExecution(ctx context.Context, profileID pgtype.UUID) (db.Execution, error)
}

func (r *Postgres) CreateExecution(ctx context.Context, arg db.CreateExecutionParams) (db.Execution, error) {
	return r.queries.CreateExecution(ctx, arg)
}

func (r *Postgres) UpdateExecution(ctx context.Context, arg db.UpdateExecutionParams) (db.Execution, error) {
	return r.queries.UpdateExecution(ctx, arg)
}

func (r *Postgres) GetExecution(ctx context.Context, id pgtype.UUID) (db.Execution, error) {
	return r.queries.GetExecution(ctx, id)
}

func (r *Postgres) ListExecutionsByProfile(ctx context.Context, profileID pgtype.UUID) ([]db.Execution, error) {
	return r.queries.ListExecutionsByProfile(ctx, profileID)
}

func (r *Postgres) DeleteExecution(ctx context.Context, id pgtype.UUID) error {
	return r.queries.DeleteExecution(ctx, id)
}

func (r *Postgres) SetExecutionPinned(ctx context.Context, arg db.SetExecutionPinnedParams) error {
	return r.queries.SetExecutionPinned(ctx, arg)
}

func (r *Postgres) GetLatestExecution(ctx context.Context, profileID pgtype.UUID) (db.Execution, error) {
	return r.queries.GetLatestExecution(ctx, profileID)
}
