package repository

import (
	"context"

	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/jackc/pgx/v5/pgtype"
)

type APIKeyRepository interface {
	CreateAPIKey(ctx context.Context, arg db.CreateAPIKeyParams) (db.ApiKey, error)
	GetAPIKey(ctx context.Context, id pgtype.UUID) (db.ApiKey, error)
	GetAPIKeyByHash(ctx context.Context, keyHash []byte) (db.ApiKey, error)
	ListAPIKeysByUser(ctx context.Context, userID pgtype.UUID) ([]db.ApiKey, error)
	ListAPIKeys(ctx context.Context) ([]db.ApiKey, error)
	RevokeAPIKey(ctx context.Context, id pgtype.UUID) (db.ApiKey, error)
	RevokeAllUserAPIKeys(ctx context.Context, userID pgtype.UUID) error
	TouchAPIKey(ctx context.Context, id pgtype.UUID) error
}

func (r *Postgres) CreateAPIKey(ctx context.Context, arg db.CreateAPIKeyParams) (db.ApiKey, error) {
	return r.queries.CreateAPIKey(ctx, arg)
}

func (r *Postgres) GetAPIKey(ctx context.Context, id pgtype.UUID) (db.ApiKey, error) {
	return r.queries.GetAPIKey(ctx, id)
}

func (r *Postgres) GetAPIKeyByHash(ctx context.Context, keyHash []byte) (db.ApiKey, error) {
	return r.queries.GetAPIKeyByHash(ctx, keyHash)
}

func (r *Postgres) ListAPIKeysByUser(ctx context.Context, userID pgtype.UUID) ([]db.ApiKey, error) {
	return r.queries.ListAPIKeysByUser(ctx, userID)
}

func (r *Postgres) ListAPIKeys(ctx context.Context) ([]db.ApiKey, error) {
	return r.queries.ListAPIKeys(ctx)
}

func (r *Postgres) RevokeAPIKey(ctx context.Context, id pgtype.UUID) (db.ApiKey, error) {
	return r.queries.RevokeAPIKey(ctx, id)
}

func (r *Postgres) RevokeAllUserAPIKeys(ctx context.Context, userID pgtype.UUID) error {
	return r.queries.RevokeAllUserAPIKeys(ctx, userID)
}

func (r *Postgres) TouchAPIKey(ctx context.Context, id pgtype.UUID) error {
	return r.queries.TouchAPIKey(ctx, id)
}
