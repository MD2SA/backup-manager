package repository

import (
	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Postgres struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	encKey  []byte
}

// NewPostgres creates a repository. configEncryptionKey is the AES key used to
// encrypt/decrypt provider configurations at rest (may be nil in dev mode).
func NewPostgres(pool *pgxpool.Pool, configEncryptionKey []byte) *Postgres {
	if len(configEncryptionKey) == 0 {
		configEncryptionKey = nil
	}
	return &Postgres{
		pool:    pool,
		queries: db.New(pool),
		encKey:  configEncryptionKey,
	}
}
