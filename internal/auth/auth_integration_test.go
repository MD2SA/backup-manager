package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/MD2SA/backup-manager/internal/config"
	"github.com/MD2SA/backup-manager/internal/database"
	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TestAuthIntegration_Postgres is opt-in. Set BACKUP_MANAGER_TEST_DSN to a
// database the test may use to CREATE/DROP temporary databases.
func TestAuthIntegration_Postgres(t *testing.T) {
	adminDSN := os.Getenv("BACKUP_MANAGER_TEST_DSN")
	if adminDSN == "" {
		t.Skip("BACKUP_MANAGER_TEST_DSN is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("connect admin: %v", err)
	}
	defer func() { _ = admin.Close(context.Background()) }()

	dbName := "bm_auth_" + randomSuffix(t)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{dbName}.Sanitize()); err != nil {
		t.Fatalf("create database: %v", err)
	}
	defer dropTestDatabase(t, adminDSN, dbName)

	dsn := replaceDatabase(t, adminDSN, dbName)
	cfg := databaseConfig(t, dsn)
	migrationsDir := filepath.Join("..", "..", "sql", "migrations")
	if err := database.Migrate(ctx, cfg, migrationsDir, 30*time.Second); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	pool, err := database.New(ctx, cfg)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := NewService(repository.NewPostgres(pool, nil), logger)

	const bootstrapKey = "bm_bootstrap_integration_key"
	if err := service.Bootstrap(ctx, bootstrapKey, false); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	adminPrincipal, err := service.Authenticate(ctx, bootstrapKey)
	if err != nil {
		t.Fatalf("authenticate bootstrap key: %v", err)
	}
	if adminPrincipal.Email != "admin@localhost" {
		t.Fatalf("unexpected bootstrap email: %q", adminPrincipal.Email)
	}

	user, err := service.CreateUser(ctx, "ops@example.com", "Ops")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	key, raw, err := service.CreateAPIKey(ctx, user.ID, "integration", pgtype.Timestamptz{})
	if err != nil {
		t.Fatalf("create key: %v", err)
	}

	principal, err := service.Authenticate(ctx, raw)
	if err != nil {
		t.Fatalf("authenticate user key: %v", err)
	}
	if principal.UserID != user.ID || principal.KeyID != key.ID {
		t.Fatalf("unexpected principal: %+v", principal)
	}

	if _, err := service.RevokeAPIKey(ctx, key.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := service.Authenticate(ctx, raw); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("revoked key should be unauthorized, got %v", err)
	}

	if err := service.RecordAudit(ctx, AuditEvent{
		Principal:    adminPrincipal,
		Action:       "POST /api/v1/users",
		ResourceType: "users",
		RequestID:    "req-1",
		Metadata:     map[string]any{"status": 201},
	}); err != nil {
		t.Fatalf("audit: %v", err)
	}
	entries, err := service.ListAuditEntries(ctx, 10, 0)
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(entries) != 1 || entries[0].Action != "POST /api/v1/users" {
		t.Fatalf("unexpected audit entries: %+v", entries)
	}
}

func databaseConfig(t *testing.T, dsn string) config.DatabaseConfig {
	t.Helper()
	parsed, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	return config.DatabaseConfig{
		Host:     parsed.Host,
		Port:     strconv.Itoa(int(parsed.Port)),
		User:     parsed.User,
		Password: parsed.Password,
		DBName:   parsed.Database,
		SSLMode:  sslMode(dsn),
	}
}

func randomSuffix(t *testing.T) string {
	t.Helper()
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(buf)
}

func replaceDatabase(t *testing.T, dsn, database string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	u.Path = "/" + database
	return u.String()
}

func sslMode(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return "disable"
	}
	if mode := u.Query().Get("sslmode"); mode != "" {
		return mode
	}
	return "disable"
}

func dropTestDatabase(t *testing.T, adminDSN, name string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Logf("drop database %s: connect: %v", name, err)
		return
	}
	defer func() { _ = admin.Close(context.Background()) }()

	_, _ = admin.Exec(ctx, `SELECT pg_terminate_backend(pid)
		FROM pg_stat_activity
		WHERE datname = $1 AND pid <> pg_backend_pid()`, name)
	if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Logf("drop database %s: %v", name, err)
	}
}
