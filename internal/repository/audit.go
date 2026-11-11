package repository

import (
	"context"

	"github.com/MD2SA/backup-manager/internal/repository/db"
)

type AuditRepository interface {
	CreateAuditEntry(ctx context.Context, arg db.CreateAuditEntryParams) (db.AuditLog, error)
	ListAuditEntries(ctx context.Context, arg db.ListAuditEntriesParams) ([]db.AuditLog, error)
}

func (r *Postgres) CreateAuditEntry(ctx context.Context, arg db.CreateAuditEntryParams) (db.AuditLog, error) {
	return r.queries.CreateAuditEntry(ctx, arg)
}

func (r *Postgres) ListAuditEntries(ctx context.Context, arg db.ListAuditEntriesParams) ([]db.AuditLog, error) {
	return r.queries.ListAuditEntries(ctx, arg)
}
