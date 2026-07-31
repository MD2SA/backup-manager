package repository

import (
	"context"

	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/jackc/pgx/v5/pgtype"
)

type RetentionPolicyRepository interface {
	GetRetentionPolicy(ctx context.Context, id pgtype.UUID) (db.RetentionPolicy, error)
	ListRetentionPolicies(ctx context.Context) ([]db.RetentionPolicy, error)
	CreateRetentionPolicy(ctx context.Context, arg db.CreateRetentionPolicyParams) (db.RetentionPolicy, error)
	UpdateRetentionPolicy(ctx context.Context, arg db.UpdateRetentionPolicyParams) (db.RetentionPolicy, error)
	DeleteRetentionPolicy(ctx context.Context, id pgtype.UUID) error
}

func (r *Postgres) GetRetentionPolicy(ctx context.Context, id pgtype.UUID) (db.RetentionPolicy, error) {
	return r.queries.GetRetentionPolicy(ctx, id)
}

func (r *Postgres) ListRetentionPolicies(ctx context.Context) ([]db.RetentionPolicy, error) {
	return r.queries.ListRetentionPolicies(ctx)
}

func (r *Postgres) CreateRetentionPolicy(ctx context.Context, arg db.CreateRetentionPolicyParams) (db.RetentionPolicy, error) {
	return r.queries.CreateRetentionPolicy(ctx, arg)
}

func (r *Postgres) UpdateRetentionPolicy(ctx context.Context, arg db.UpdateRetentionPolicyParams) (db.RetentionPolicy, error) {
	return r.queries.UpdateRetentionPolicy(ctx, arg)
}

func (r *Postgres) DeleteRetentionPolicy(ctx context.Context, id pgtype.UUID) error {
	return r.queries.DeleteRetentionPolicy(ctx, id)
}
