package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/MD2SA/backup-manager/internal/config"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Migrate applies the SQL migrations in dir to the metadata database. It waits
// for the database to accept connections for up to timeout, which makes the
// container entrypoint independent from shell-based readiness checks.
func Migrate(ctx context.Context, cfg config.DatabaseConfig, dir string, timeout time.Duration) error {
	if dir == "" {
		dir = "sql/migrations"
	}
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("migrations directory %s: %w", dir, err)
	}
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	db, err := sql.Open("pgx", cfg.DSN())
	if err != nil {
		return fmt.Errorf("open metadata database: %w", err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)

	deadline := time.Now().Add(timeout)
	for {
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		pingErr := db.PingContext(pingCtx)
		cancel()
		if pingErr == nil {
			break
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("metadata database not reachable after %s: %w", timeout, pingErr)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("configure migration dialect: %w", err)
	}
	if err := goose.UpContext(ctx, db, dir); err != nil {
		return fmt.Errorf("apply migrations from %s: %w", dir, err)
	}
	return nil
}
