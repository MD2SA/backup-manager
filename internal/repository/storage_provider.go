package repository

import (
	"context"
	"fmt"

	"github.com/MD2SA/backup-manager/internal/pkg/crypto"
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
	provider, err := r.queries.GetStorageProvider(ctx, id)
	if err != nil {
		return provider, err
	}
	config, err := r.decryptProviderConfig(provider.Config)
	if err != nil {
		return provider, err
	}
	provider.Config = config
	return provider, nil
}

func (r *Postgres) ListStorageProviders(ctx context.Context) ([]db.StorageProvider, error) {
	providers, err := r.queries.ListStorageProviders(ctx)
	if err != nil {
		return nil, err
	}
	for i := range providers {
		config, err := r.decryptProviderConfig(providers[i].Config)
		if err != nil {
			return nil, err
		}
		providers[i].Config = config
	}
	return providers, nil
}

func (r *Postgres) CreateStorageProvider(ctx context.Context, arg db.CreateStorageProviderParams) (db.StorageProvider, error) {
	config, err := r.encryptProviderConfig(arg.Config)
	if err != nil {
		return db.StorageProvider{}, err
	}
	arg.Config = config

	provider, err := r.queries.CreateStorageProvider(ctx, arg)
	if err != nil {
		return db.StorageProvider{}, err
	}

	config, err = r.decryptProviderConfig(provider.Config)
	if err != nil {
		return provider, err
	}
	provider.Config = config
	return provider, nil
}

func (r *Postgres) UpdateStorageProvider(ctx context.Context, arg db.UpdateStorageProviderParams) (db.StorageProvider, error) {
	config, err := r.encryptProviderConfig(arg.Config)
	if err != nil {
		return db.StorageProvider{}, err
	}
	arg.Config = config

	provider, err := r.queries.UpdateStorageProvider(ctx, arg)
	if err != nil {
		return db.StorageProvider{}, err
	}

	config, err = r.decryptProviderConfig(provider.Config)
	if err != nil {
		return provider, err
	}
	provider.Config = config
	return provider, nil
}

func (r *Postgres) DeleteStorageProvider(ctx context.Context, id pgtype.UUID) error {
	return r.queries.DeleteStorageProvider(ctx, id)
}

func (r *Postgres) encryptProviderConfig(raw []byte) ([]byte, error) {
	encoded, err := crypto.EncodeProviderConfig(r.encKey, raw)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt provider config: %w", err)
	}
	return encoded, nil
}

func (r *Postgres) decryptProviderConfig(data []byte) ([]byte, error) {
	decoded, err := crypto.DecodeProviderConfig(r.encKey, data)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt provider config: %w", err)
	}
	return decoded, nil
}
