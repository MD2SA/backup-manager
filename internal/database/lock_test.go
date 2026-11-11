package database

import (
	"context"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/MD2SA/backup-manager/internal/config"
	"github.com/jackc/pgx/v5"
)

func TestInstanceLockIntegration(t *testing.T) {
	dsn := os.Getenv("BACKUP_MANAGER_TEST_DSN")
	if dsn == "" {
		t.Skip("BACKUP_MANAGER_TEST_DSN is not set")
	}
	cfg := lockTestConfig(t, dsn)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	first, err := AcquireInstanceLock(ctx, cfg)
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}
	defer first.Release(context.Background())

	if _, err := AcquireInstanceLock(ctx, cfg); err != ErrInstanceLocked {
		t.Fatalf("expected ErrInstanceLocked, got %v", err)
	}

	first.Release(context.Background())
	second, err := AcquireInstanceLock(ctx, cfg)
	if err != nil {
		t.Fatalf("lock after release: %v", err)
	}
	second.Release(context.Background())
}

func lockTestConfig(t *testing.T, dsn string) config.DatabaseConfig {
	t.Helper()
	parsed, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	ssl := "disable"
	if u, parseErr := url.Parse(dsn); parseErr == nil {
		if mode := u.Query().Get("sslmode"); mode != "" {
			ssl = mode
		}
	}
	port := int(parsed.Port)
	if port == 0 {
		port = 5432
	}
	return config.DatabaseConfig{
		Host:     parsed.Host,
		Port:     strconv.Itoa(port),
		User:     parsed.User,
		Password: parsed.Password,
		DBName:   parsed.Database,
		SSLMode:  ssl,
	}
}
