package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/MD2SA/backup-manager/internal/config"
	"github.com/jackc/pgx/v5"
)

var ErrInstanceLocked = errors.New("another Backup Manager instance already holds the scheduler lock")

const instanceLockKey int64 = 0x424B4D47 // "BKMG"

// InstanceLock keeps a dedicated metadata-database connection open for the
// lifetime of the process. PostgreSQL advisory locks are session-scoped, so the
// lock is released automatically if the process or connection dies.
type InstanceLock struct {
	conn *pgx.Conn
	key  int64
}

func AcquireInstanceLock(ctx context.Context, cfg config.DatabaseConfig) (*InstanceLock, error) {
	conn, err := pgx.Connect(ctx, cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("connect for instance lock: %w", err)
	}

	var acquired bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", instanceLockKey).Scan(&acquired); err != nil {
		_ = conn.Close(context.Background())
		return nil, fmt.Errorf("acquire instance lock: %w", err)
	}
	if !acquired {
		_ = conn.Close(context.Background())
		return nil, ErrInstanceLocked
	}

	return &InstanceLock{conn: conn, key: instanceLockKey}, nil
}

func (l *InstanceLock) Release(ctx context.Context) {
	if l == nil || l.conn == nil {
		return
	}
	_, _ = l.conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", l.key)
	_ = l.conn.Close(context.Background())
	l.conn = nil
}
