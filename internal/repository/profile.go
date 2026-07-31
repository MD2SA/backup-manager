package repository

import (
	"context"

	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/jackc/pgx/v5/pgtype"
)

type ProfileRepository interface {
	GetProfile(ctx context.Context, id pgtype.UUID) (db.Profile, error)
	ListProfiles(ctx context.Context) ([]db.Profile, error)
	CreateProfile(ctx context.Context, params db.CreateProfileParams) (db.Profile, error)
	UpdateProfile(ctx context.Context, params db.UpdateProfileParams) (db.Profile, error)
	DeleteProfile(ctx context.Context, id pgtype.UUID) error
	ActivateProfile(ctx context.Context, id pgtype.UUID) error
}

func (r *Postgres) GetProfile(ctx context.Context, id pgtype.UUID) (db.Profile, error) {
	return r.queries.GetProfile(ctx, id)
}

func (r *Postgres) ListProfiles(ctx context.Context) ([]db.Profile, error) {
	return r.queries.ListProfiles(ctx)
}

func (r *Postgres) CreateProfile(ctx context.Context, params db.CreateProfileParams) (db.Profile, error) {
	return r.queries.CreateProfile(ctx, params)
}

func (r *Postgres) UpdateProfile(ctx context.Context, params db.UpdateProfileParams) (db.Profile, error) {
	return r.queries.UpdateProfile(ctx, params)
}

func (r *Postgres) DeleteProfile(ctx context.Context, id pgtype.UUID) error {
	return r.queries.DeleteProfile(ctx, id)
}

func (r *Postgres) ActivateProfile(ctx context.Context, id pgtype.UUID) error {
	return r.queries.ActivateProfile(ctx, id)
}
