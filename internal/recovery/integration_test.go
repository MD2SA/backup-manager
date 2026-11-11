package recovery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/MD2SA/backup-manager/internal/config"
	"github.com/MD2SA/backup-manager/internal/database"
	"github.com/MD2SA/backup-manager/internal/pkg/crypto"
	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/MD2SA/backup-manager/internal/service"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const integrationSchema = `
CREATE TABLE users (
  id bigserial PRIMARY KEY,
  email text NOT NULL UNIQUE,
  name text NOT NULL,
  metadata jsonb NOT NULL,
  created_at timestamptz NOT NULL
);
CREATE TABLE orders (
  id bigserial PRIMARY KEY,
  user_id bigint NOT NULL REFERENCES users(id),
  total numeric(12,2) NOT NULL,
  status text NOT NULL,
  created_at timestamptz NOT NULL
);
CREATE TABLE events (
  id bigserial PRIMARY KEY,
  user_id bigint,
  kind text NOT NULL,
  payload jsonb NOT NULL,
  created_at timestamptz NOT NULL
);
CREATE INDEX idx_orders_user ON orders(user_id);
CREATE INDEX idx_events_kind_created ON events(kind, created_at);
INSERT INTO users (email, name, metadata, created_at)
SELECT 'user'||g||'@example.test', 'User '||g,
       jsonb_build_object('source','test','flags',jsonb_build_array(g%2=0)),
       now() - (g || ' minutes')::interval
FROM generate_series(1, 200) g;
INSERT INTO orders (user_id, total, status, created_at)
SELECT (g % 200) + 1, round((random()*1000)::numeric,2), 'paid', now() - (g || ' minutes')::interval
FROM generate_series(1, 1000) g;
INSERT INTO events (user_id, kind, payload, created_at)
SELECT (g % 200) + 1, 'view', jsonb_build_object('path','/p/'||(g%20)), now() - (g || ' minutes')::interval
FROM generate_series(1, 2000) g;
`

const checksumQuery = `
SELECT 'events', count(*), md5(string_agg(md5(t::text), '' ORDER BY id)) FROM events t
UNION ALL SELECT 'orders', count(*), md5(string_agg(md5(t::text), '' ORDER BY id)) FROM orders t
UNION ALL SELECT 'users', count(*), md5(string_agg(md5(t::text), '' ORDER BY id)) FROM users t
ORDER BY 1;`

func TestRecoveryIntegration_Postgres(t *testing.T) {
	adminDSN := os.Getenv("BACKUP_MANAGER_TEST_DSN")
	if adminDSN == "" {
		t.Skip("BACKUP_MANAGER_TEST_DSN is not set")
	}
	for _, binary := range []string{"pg_dump", "pg_restore", "psql"} {
		if _, err := exec.LookPath(binary); err != nil {
			t.Skipf("%s not found in PATH", binary)
		}
	}

	modes := []struct {
		name      string
		configure func(*testing.T, *config.Config)
	}{
		{
			name: "profile-passphrase",
			configure: func(_ *testing.T, cfg *config.Config) {
				cfg.EncryptionPassphrase = "profile-secret"
			},
		},
		{
			name: "age-identity",
			configure: func(t *testing.T, cfg *config.Config) {
				publicKey, privateKey, err := crypto.GenerateX25519KeyPair()
				if err != nil {
					t.Fatal(err)
				}
				cfg.AgePublicKey = publicKey
				cfg.AgePrivateKey = privateKey
			},
		},
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			runRecoveryIntegration(t, adminDSN, func(cfg *config.Config) {
				mode.configure(t, cfg)
			})
		})
	}
}

func runRecoveryIntegration(t *testing.T, adminDSN string, configure func(*config.Config)) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	metaName := createTestDatabase(t, adminDSN, "bm_it_meta")
	targetName := createTestDatabase(t, adminDSN, "bm_it_target")

	cfg := config.Config{
		LogLevel:                "error",
		TempDir:                 t.TempDir(),
		StoragePath:             t.TempDir(),
		ConfigEncryptionKey:     "test-config-encryption-key",
		MetadataBackupEmbed:     true,
		MetadataBackupRetention: 14,
		MetadataDB:              databaseConfigFromDSN(t, adminDSN, metaName),
		TargetDB:                databaseConfigFromDSN(t, adminDSN, targetName),
	}
	configure(&cfg)

	if err := database.Migrate(ctx, cfg.MetadataDB, filepath.Join("..", "..", "sql", "migrations"), 30*time.Second); err != nil {
		t.Fatalf("migrate metadata: %v", err)
	}
	seedTarget(t, cfg.TargetDB)
	sourceChecksums := queryChecksums(t, cfg.TargetDB)
	sourceSchema := querySchema(t, cfg.TargetDB)
	sourceSequences := querySequences(t, cfg.TargetDB)

	pool, err := database.New(ctx, cfg.MetadataDB)
	if err != nil {
		t.Fatal(err)
	}
	encKey := crypto.DeriveConfigKey(cfg.ConfigEncryptionKey)
	repo := repository.NewPostgres(pool, encKey)
	providers := service.NewProviderService(repo, cfg)

	provider, err := repo.CreateStorageProvider(ctx, db.CreateStorageProviderParams{
		Name:   "local",
		Type:   "local",
		Config: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := repo.CreateProfile(ctx, db.CreateProfileParams{
		Name:              "integration",
		Enabled:           false,
		Schedule:          "0 0 * * *",
		CompressionType:   "gzip",
		CompressionLevel:  6,
		EncryptionEnabled: true,
	}, []pgtype.UUID{provider.ID}, nil)
	if err != nil {
		t.Fatal(err)
	}

	backupService := service.NewBackupService(
		repo,
		providers,
		nil,
		nil,
		cfg,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		nil,
		nil,
		nil,
	)
	if err := backupService.ExecuteBackup(ctx, profile.ID); err != nil {
		t.Fatalf("backup: %v", err)
	}

	executions, err := repo.ListExecutionsByProfile(ctx, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(executions) != 1 || executions[0].Status != "success" {
		t.Fatalf("unexpected executions: %+v", executions)
	}
	execution := executions[0]
	if !execution.Checksum.Valid || !execution.StoragePath.Valid {
		t.Fatalf("execution not finalized: %+v", execution)
	}
	profileKey := pgutil.UUIDToString(profile.ID)
	assertMetadataSnapshotExists(t, filepath.Join(cfg.StoragePath, "meta", profileKey))
	pool.Close()

	// Disaster: both the metadata database and the target database are gone.
	dropTestDatabase(t, adminDSN, metaName)
	dropTestDatabase(t, adminDSN, targetName)
	targetName = createTestDatabase(t, adminDSN, "bm_it_target_restored")
	newMetaName := createTestDatabase(t, adminDSN, "bm_it_meta_restored")

	cfg.MetadataDB = databaseConfigFromDSN(t, adminDSN, newMetaName)
	cfg.TargetDB = databaseConfigFromDSN(t, adminDSN, targetName)

	if _, err := Metadata(ctx, cfg, MetadataOptions{StoragePath: cfg.StoragePath}); err != nil {
		t.Fatalf("recover metadata: %v", err)
	}

	restoredPool, err := database.New(ctx, cfg.MetadataDB)
	if err != nil {
		t.Fatal(err)
	}
	defer restoredPool.Close()
	restoredRepo := repository.NewPostgres(restoredPool, encKey)
	restoredExecutions, err := restoredRepo.ListExecutionsByProfile(ctx, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(restoredExecutions) != 1 || restoredExecutions[0].Status != "success" ||
		!restoredExecutions[0].Checksum.Valid || !restoredExecutions[0].StoragePath.Valid {
		t.Fatalf("restored metadata is stale: %+v", restoredExecutions)
	}

	if _, err := Data(ctx, cfg, DataOptions{
		ProfileID:  pgutil.UUIDToString(profile.ID),
		VerifyOnly: true,
	}); err != nil {
		t.Fatalf("verify recovery: %v", err)
	}
	if got := tableCount(t, cfg.TargetDB); got != 0 {
		t.Fatalf("verify-only wrote %d tables into the target", got)
	}

	if _, err := Data(ctx, cfg, DataOptions{
		ProfileID: pgutil.UUIDToString(profile.ID),
		Confirm:   true,
	}); err != nil {
		t.Fatalf("recover data: %v", err)
	}
	if got, want := queryChecksums(t, cfg.TargetDB), sourceChecksums; got != want {
		t.Fatalf("checksums differ:\nwant:\n%s\ngot:\n%s", want, got)
	}
	if got, want := querySchema(t, cfg.TargetDB), sourceSchema; got != want {
		t.Fatalf("schema differs:\nwant:\n%s\ngot:\n%s", want, got)
	}
	if got, want := querySequences(t, cfg.TargetDB), sourceSequences; got != want {
		t.Fatalf("sequences differ:\nwant:\n%s\ngot:\n%s", want, got)
	}

	// Recovery from the artifact alone must also work when the metadata
	// database is unavailable.
	storageRows, err := restoredRepo.ListExecutionStorage(ctx, execution.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(storageRows) == 0 {
		t.Fatal("execution has no storage copy")
	}
	artifact := filepath.Join(cfg.StoragePath, storageRows[0].StoragePath)
	fileTargetName := createTestDatabase(t, adminDSN, "bm_it_file_target")
	fileTarget := databaseConfigFromDSN(t, adminDSN, fileTargetName)
	if _, err := Data(ctx, cfg, DataOptions{
		File:      artifact,
		TargetDSN: fileTarget.DSN(),
		Confirm:   true,
	}); err != nil {
		t.Fatalf("recover from file: %v", err)
	}
	if got, want := queryChecksums(t, fileTarget), sourceChecksums; got != want {
		t.Fatalf("file recovery checksums differ:\nwant:\n%s\ngot:\n%s", want, got)
	}

	// The metadata database must never be a restore target.
	if _, err := Data(ctx, cfg, DataOptions{
		ProfileID: pgutil.UUIDToString(profile.ID),
		TargetDSN: cfg.MetadataDB.DSN(),
		Confirm:   true,
	}); err == nil {
		t.Fatal("expected metadata database guard to reject the restore")
	}
}

func createTestDatabase(t *testing.T, adminDSN, prefix string) string {
	t.Helper()
	name := fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	conn, err := pgx.Connect(context.Background(), adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	if _, err := conn.Exec(context.Background(), "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dropTestDatabase(t, adminDSN, name) })
	return name
}

func dropTestDatabase(t *testing.T, adminDSN, name string) {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), adminDSN)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close(context.Background()) }()
	_, _ = conn.Exec(context.Background(), "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
}

func databaseConfigFromDSN(t *testing.T, dsn, name string) config.DatabaseConfig {
	t.Helper()
	parsed, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	sslMode := "disable"
	if u, parseErr := url.Parse(dsn); parseErr == nil {
		if mode := u.Query().Get("sslmode"); mode != "" {
			sslMode = mode
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
		DBName:   name,
		SSLMode:  sslMode,
	}
}

func seedTarget(t *testing.T, cfg config.DatabaseConfig) {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), cfg.DSN())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	if _, err := conn.Exec(context.Background(), integrationSchema); err != nil {
		t.Fatal(err)
	}
}

func queryChecksums(t *testing.T, cfg config.DatabaseConfig) string {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), cfg.DSN())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	rows, err := conn.Query(context.Background(), checksumQuery)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var lines []string
	for rows.Next() {
		var table string
		var count int64
		var checksum string
		if err := rows.Scan(&table, &count, &checksum); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, fmt.Sprintf("%s|%d|%s", table, count, checksum))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return strings.Join(lines, "\n")
}

func querySchema(t *testing.T, cfg config.DatabaseConfig) string {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), cfg.DSN())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	var indexes, foreignKeys int
	if err := conn.QueryRow(context.Background(),
		"SELECT count(*) FROM pg_indexes WHERE schemaname='public'").Scan(&indexes); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(context.Background(),
		"SELECT count(*) FROM pg_constraint WHERE contype='f'").Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("indexes=%d foreign_keys=%d", indexes, foreignKeys)
}

func querySequences(t *testing.T, cfg config.DatabaseConfig) string {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), cfg.DSN())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	var users, orders, events int64
	if err := conn.QueryRow(context.Background(), "SELECT last_value FROM users_id_seq").Scan(&users); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(context.Background(), "SELECT last_value FROM orders_id_seq").Scan(&orders); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(context.Background(), "SELECT last_value FROM events_id_seq").Scan(&events); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("users=%d orders=%d events=%d", users, orders, events)
}

func tableCount(t *testing.T, cfg config.DatabaseConfig) int {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), cfg.DSN())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	var count int
	if err := conn.QueryRow(context.Background(),
		"SELECT count(*) FROM pg_tables WHERE schemaname='public'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func assertMetadataSnapshotExists(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "metadata-*.dump.age"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatalf("no metadata snapshot in %s", dir)
	}
}
