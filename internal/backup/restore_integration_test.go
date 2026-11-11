package backup

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	appcrypto "github.com/MD2SA/backup-manager/internal/pkg/crypto"
	"github.com/jackc/pgx/v5"
)

// TestRestoreIntegration_Postgres is opt-in because it needs a real PostgreSQL
// server and the PostgreSQL client binaries. Set BACKUP_MANAGER_TEST_DSN to a
// database the test may use to CREATE/DROP temporary databases, for example:
//
//	BACKUP_MANAGER_TEST_DSN='postgres://postgres:postgres@127.0.0.1:5432/postgres?sslmode=disable' \
//	go test ./internal/backup -run TestRestoreIntegration_Postgres -v
func TestRestoreIntegration_Postgres(t *testing.T) {
	adminDSN := os.Getenv("BACKUP_MANAGER_TEST_DSN")
	if adminDSN == "" {
		t.Skip("BACKUP_MANAGER_TEST_DSN is not set")
	}
	if _, err := exec.LookPath("pg_dump"); err != nil {
		t.Skip("pg_dump is not installed")
	}
	if _, err := exec.LookPath("psql"); err != nil {
		t.Skip("psql is not installed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("connect admin database: %v", err)
	}
	defer func() { _ = admin.Close(context.Background()) }()

	suffix := randomSuffix(t)
	sourceDB := "bm_src_" + suffix
	targetDB := "bm_dst_" + suffix
	createDatabase(t, ctx, admin, sourceDB)
	createDatabase(t, ctx, admin, targetDB)
	defer dropDatabase(t, adminDSN, sourceDB)
	defer dropDatabase(t, adminDSN, targetDB)

	sourceDSN, targetDSN := replaceDatabase(t, adminDSN, sourceDB), replaceDatabase(t, adminDSN, targetDB)
	source, err := pgx.Connect(ctx, sourceDSN)
	if err != nil {
		t.Fatalf("connect source: %v", err)
	}
	if _, err := source.Exec(ctx, `CREATE TABLE widgets (id integer PRIMARY KEY, name text NOT NULL);
		INSERT INTO widgets (id, name) VALUES (1, 'one'), (2, 'two'), (3, 'three');`); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	_ = source.Close(context.Background())

	dir := t.TempDir()
	dumpPath := filepath.Join(dir, "backup.sql.gz")
	cmd := exec.CommandContext(ctx, "pg_dump", "--no-owner", "--no-acl", "-Z", "6", "-f", dumpPath, "-d", sourceDSN)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("pg_dump: %v: %s", err, out)
	}

	plain, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatal(err)
	}
	checksum := sha256Hex(plain)

	encPath := dumpPath + ".age"
	if err := appcrypto.EncryptWithPassphrase(dumpPath, encPath, "integration-passphrase"); err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	encrypted, err := os.ReadFile(encPath)
	if err != nil {
		t.Fatal(err)
	}

	store := newMemoryStorage()
	store.data["backup.sql.gz.age"] = encrypted

	parsed, err := pgx.ParseConfig(targetDSN)
	if err != nil {
		t.Fatal(err)
	}
	target := RestoreTarget{
		Host:     parsed.Host,
		Port:     fmtPort(parsed.Port),
		User:     parsed.User,
		Password: parsed.Password,
		DBName:   targetDB,
		SSLMode:  sslModeFromDSN(adminDSN),
	}

	// 1. A checksum mismatch must abort before touching the target database.
	if _, err := PrepareArtifact(ctx, store, "backup.sql.gz.age", t.TempDir(), sha256Hex([]byte("wrong")), true, "integration-passphrase", "", nil); err == nil {
		t.Fatal("expected checksum mismatch")
	}
	if err := target.Preflight(ctx, false); err != nil {
		t.Fatalf("target should still be empty: %v", err)
	}

	// 2. The real artifact must restore successfully.
	artifact, err := PrepareArtifact(ctx, store, "backup.sql.gz.age", t.TempDir(), checksum, true, "integration-passphrase", "", t.Logf)
	if err != nil {
		t.Fatalf("PrepareArtifact: %v", err)
	}
	defer artifact.Cleanup()

	if err := target.Preflight(ctx, false); err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	output, err := ApplyArtifact(ctx, artifact, target, true)
	if err != nil {
		t.Fatalf("ApplyArtifact: %v\n%s", err, output)
	}

	restored, err := pgx.Connect(ctx, targetDSN)
	if err != nil {
		t.Fatalf("connect target: %v", err)
	}
	defer func() { _ = restored.Close(context.Background()) }()

	var count int
	if err := restored.QueryRow(ctx, `SELECT count(*) FROM widgets`).Scan(&count); err != nil {
		t.Fatalf("query restored table: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 restored rows, got %d", count)
	}

	var name string
	if err := restored.QueryRow(ctx, `SELECT name FROM widgets WHERE id = 2`).Scan(&name); err != nil {
		t.Fatalf("query restored row: %v", err)
	}
	if name != "two" {
		t.Fatalf("unexpected restored value: %q", name)
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

func createDatabase(t *testing.T, ctx context.Context, admin *pgx.Conn, name string) {
	t.Helper()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create database %s: %v", name, err)
	}
}

func dropDatabase(t *testing.T, adminDSN, name string) {
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

func replaceDatabase(t *testing.T, dsn, database string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	u.Path = "/" + database
	return u.String()
}

func fmtPort(port uint16) string {
	if port == 0 {
		return "5432"
	}
	return strconv.Itoa(int(port))
}

func sslModeFromDSN(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return "disable"
	}
	if mode := u.Query().Get("sslmode"); mode != "" {
		return mode
	}
	return "disable"
}
