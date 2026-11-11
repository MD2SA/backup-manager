package repository

import (
	"context"

	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/jackc/pgx/v5/pgtype"
)

type UserRepository interface {
	CreateUser(ctx context.Context, arg db.CreateUserParams) (db.User, error)
	GetUser(ctx context.Context, id pgtype.UUID) (db.User, error)
	GetUserByEmail(ctx context.Context, email string) (db.User, error)
	ListUsers(ctx context.Context) ([]db.User, error)
	ListUsersPage(ctx context.Context, arg db.ListUsersPageParams) ([]db.User, error)
	UpdateUser(ctx context.Context, arg db.UpdateUserParams) (db.User, error)
	DeactivateUser(ctx context.Context, id pgtype.UUID) (db.User, error)
	CountUsers(ctx context.Context) (int64, error)
	CountActiveUsers(ctx context.Context) (int64, error)
	BootstrapAdmin(ctx context.Context, keyHash []byte, keyPrefix string) (bool, error)
}

func (r *Postgres) CreateUser(ctx context.Context, arg db.CreateUserParams) (db.User, error) {
	return r.queries.CreateUser(ctx, arg)
}

func (r *Postgres) GetUser(ctx context.Context, id pgtype.UUID) (db.User, error) {
	return r.queries.GetUser(ctx, id)
}

func (r *Postgres) GetUserByEmail(ctx context.Context, email string) (db.User, error) {
	return r.queries.GetUserByEmail(ctx, email)
}

func (r *Postgres) ListUsers(ctx context.Context) ([]db.User, error) {
	return r.queries.ListUsers(ctx)
}

func (r *Postgres) ListUsersPage(ctx context.Context, arg db.ListUsersPageParams) ([]db.User, error) {
	return r.queries.ListUsersPage(ctx, arg)
}

func (r *Postgres) UpdateUser(ctx context.Context, arg db.UpdateUserParams) (db.User, error) {
	return r.queries.UpdateUser(ctx, arg)
}

func (r *Postgres) DeactivateUser(ctx context.Context, id pgtype.UUID) (db.User, error) {
	return r.queries.DeactivateUser(ctx, id)
}

func (r *Postgres) CountUsers(ctx context.Context) (int64, error) {
	return r.queries.CountUsers(ctx)
}

func (r *Postgres) CountActiveUsers(ctx context.Context) (int64, error) {
	return r.queries.CountActiveUsers(ctx)
}
