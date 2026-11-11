package repository

import (
	"context"

	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/jackc/pgx/v5/pgtype"
)

type AuditRepository interface {
	CreateAuditEntry(ctx context.Context, arg db.CreateAuditEntryParams) (db.AuditLog, error)
	ListAuditEntries(ctx context.Context, arg db.ListAuditEntriesParams) ([]db.AuditLog, error)
	ListAuditEntriesPage(ctx context.Context, arg db.ListAuditEntriesPageParams) ([]db.AuditLog, error)
	DeleteOldAuditEntries(ctx context.Context, createdAt pgtype.Timestamptz) (int64, error)
}

func (r *Postgres) CreateAuditEntry(ctx context.Context, arg db.CreateAuditEntryParams) (db.AuditLog, error) {
	return r.queries.CreateAuditEntry(ctx, arg)
}

func (r *Postgres) ListAuditEntries(ctx context.Context, arg db.ListAuditEntriesParams) ([]db.AuditLog, error) {
	return r.queries.ListAuditEntries(ctx, arg)
}

func (r *Postgres) ListAuditEntriesPage(ctx context.Context, arg db.ListAuditEntriesPageParams) ([]db.AuditLog, error) {
	return r.queries.ListAuditEntriesPage(ctx, arg)
}

func (r *Postgres) DeleteOldAuditEntries(ctx context.Context, createdAt pgtype.Timestamptz) (int64, error) {
	return r.queries.DeleteOldAuditEntries(ctx, createdAt)
}
