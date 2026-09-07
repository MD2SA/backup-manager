package repository

import (
	"context"

	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/jackc/pgx/v5/pgtype"
)

type NotificationProviderRepository interface {
	GetNotificationProvider(ctx context.Context, id pgtype.UUID) (db.NotificationProvider, error)
	ListNotificationProviders(ctx context.Context) ([]db.NotificationProvider, error)
	CreateNotificationProvider(ctx context.Context, arg db.CreateNotificationProviderParams) (db.NotificationProvider, error)
	UpdateNotificationProvider(ctx context.Context, arg db.UpdateNotificationProviderParams) (db.NotificationProvider, error)
	DeleteNotificationProvider(ctx context.Context, id pgtype.UUID) error
}

func (r *Postgres) GetNotificationProvider(ctx context.Context, id pgtype.UUID) (db.NotificationProvider, error) {
	provider, err := r.queries.GetNotificationProvider(ctx, id)
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

func (r *Postgres) ListNotificationProviders(ctx context.Context) ([]db.NotificationProvider, error) {
	providers, err := r.queries.ListNotificationProviders(ctx)
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

func (r *Postgres) CreateNotificationProvider(
	ctx context.Context,
	arg db.CreateNotificationProviderParams,
) (db.NotificationProvider, error) {
	config, err := r.encryptProviderConfig(arg.Config)
	if err != nil {
		return db.NotificationProvider{}, err
	}
	arg.Config = config

	provider, err := r.queries.CreateNotificationProvider(ctx, arg)
	if err != nil {
		return db.NotificationProvider{}, err
	}

	config, err = r.decryptProviderConfig(provider.Config)
	if err != nil {
		return provider, err
	}
	provider.Config = config
	return provider, nil
}

func (r *Postgres) UpdateNotificationProvider(
	ctx context.Context,
	arg db.UpdateNotificationProviderParams,
) (db.NotificationProvider, error) {
	config, err := r.encryptProviderConfig(arg.Config)
	if err != nil {
		return db.NotificationProvider{}, err
	}
	arg.Config = config

	provider, err := r.queries.UpdateNotificationProvider(ctx, arg)
	if err != nil {
		return db.NotificationProvider{}, err
	}

	config, err = r.decryptProviderConfig(provider.Config)
	if err != nil {
		return provider, err
	}
	provider.Config = config
	return provider, nil
}

func (r *Postgres) DeleteNotificationProvider(ctx context.Context, id pgtype.UUID) error {
	return r.queries.DeleteNotificationProvider(ctx, id)
}
