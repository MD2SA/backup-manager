package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// bootstrapAdvisoryLock serializes first-boot admin creation across instances.
const bootstrapAdvisoryLock int64 = 0x41555448 // "AUTH"

// BootstrapAdmin atomically creates the first admin user and its bootstrap API
// key. It returns false when users already exist (including when another
// instance won the race).
func (r *Postgres) BootstrapAdmin(ctx context.Context, keyHash []byte, keyPrefix string) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", bootstrapAdvisoryLock); err != nil {
		return false, err
	}

	var count int64
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&count); err != nil {
		return false, err
	}
	if count > 0 {
		return false, tx.Commit(ctx)
	}

	var userID pgtype.UUID
	if err := tx.QueryRow(ctx,
		"INSERT INTO users (email, name) VALUES ('admin@localhost', 'Admin') RETURNING id",
	).Scan(&userID); err != nil {
		return false, err
	}

	if _, err := tx.Exec(ctx,
		"INSERT INTO api_keys (user_id, name, key_prefix, key_hash) VALUES ($1, 'bootstrap', $2, $3)",
		userID, keyPrefix, keyHash,
	); err != nil {
		return false, err
	}

	return true, tx.Commit(ctx)
}
