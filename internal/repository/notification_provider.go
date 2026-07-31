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
	return r.queries.GetNotificationProvider(ctx, id)
}

func (r *Postgres) ListNotificationProviders(ctx context.Context) ([]db.NotificationProvider, error) {
	return r.queries.ListNotificationProviders(ctx)
}

func (r *Postgres) CreateNotificationProvider(
	ctx context.Context,
	arg db.CreateNotificationProviderParams,
) (db.NotificationProvider, error) {
	return r.queries.CreateNotificationProvider(ctx, arg)
}

func (r *Postgres) UpdateNotificationProvider(
	ctx context.Context,
	arg db.UpdateNotificationProviderParams,
) (db.NotificationProvider, error) {
	return r.queries.UpdateNotificationProvider(ctx, arg)
}

func (r *Postgres) DeleteNotificationProvider(ctx context.Context, id pgtype.UUID) error {
	return r.queries.DeleteNotificationProvider(ctx, id)
}
