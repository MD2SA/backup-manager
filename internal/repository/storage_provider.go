package repository

import (
	"context"

	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/jackc/pgx/v5/pgtype"
)

type StorageProviderRepository interface {
	GetStorageProvider(ctx context.Context, id pgtype.UUID) (db.StorageProvider, error)
	ListStorageProviders(ctx context.Context) ([]db.StorageProvider, error)
	CreateStorageProvider(ctx context.Context, arg db.CreateStorageProviderParams) (db.StorageProvider, error)
	UpdateStorageProvider(ctx context.Context, arg db.UpdateStorageProviderParams) (db.StorageProvider, error)
	DeleteStorageProvider(ctx context.Context, id pgtype.UUID) error
}

func (r *Postgres) GetStorageProvider(ctx context.Context, id pgtype.UUID) (db.StorageProvider, error) {
	return r.queries.GetStorageProvider(ctx, id)
}

func (r *Postgres) ListStorageProviders(ctx context.Context) ([]db.StorageProvider, error) {
	return r.queries.ListStorageProviders(ctx)
}

func (r *Postgres) CreateStorageProvider(ctx context.Context, arg db.CreateStorageProviderParams) (db.StorageProvider, error) {
	return r.queries.CreateStorageProvider(ctx, arg)
}

func (r *Postgres) UpdateStorageProvider(ctx context.Context, arg db.UpdateStorageProviderParams) (db.StorageProvider, error) {
	return r.queries.UpdateStorageProvider(ctx, arg)
}

func (r *Postgres) DeleteStorageProvider(ctx context.Context, id pgtype.UUID) error {
	return r.queries.DeleteStorageProvider(ctx, id)
}
