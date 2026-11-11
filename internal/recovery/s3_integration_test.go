package recovery

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MD2SA/backup-manager/internal/config"
	"github.com/MD2SA/backup-manager/internal/database"
	"github.com/MD2SA/backup-manager/internal/pkg/crypto"
	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/MD2SA/backup-manager/internal/service"
	"github.com/jackc/pgx/v5/pgtype"
)

const fakeBucket = "backups"

// fakeObjectStore is a minimal path-style S3 server used to exercise the real
// AWS SDK upload, list and download paths without MinIO.
type fakeObjectStore struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func newFakeObjectStore() *fakeObjectStore {
	return &fakeObjectStore{objects: make(map[string][]byte)}
}

func (f *fakeObjectStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/"+fakeBucket+"/")

	switch {
	case r.Method == http.MethodPut && key != "":
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.objects[key] = body
		f.mu.Unlock()
		w.Header().Set("ETag", `"etag"`)
		w.WriteHeader(http.StatusOK)

	case r.Method == http.MethodGet && r.URL.Path == "/"+fakeBucket:
		f.list(w, r.URL.Query().Get("prefix"))

	case r.Method == http.MethodGet && key != "":
		f.mu.Lock()
		body, ok := f.objects[key]
		f.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)

	case r.Method == http.MethodHead && key != "":
		f.mu.Lock()
		body, ok := f.objects[key]
		f.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		w.WriteHeader(http.StatusOK)

	case r.Method == http.MethodDelete && key != "":
		f.mu.Lock()
		delete(f.objects, key)
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "unsupported request", http.StatusBadRequest)
	}
}

func (f *fakeObjectStore) list(w http.ResponseWriter, prefix string) {
	f.mu.Lock()
	var keys []string
	for key := range f.objects {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	type item struct {
		key  string
		size int
	}
	items := make([]item, 0, len(keys))
	for _, key := range keys {
		items = append(items, item{key: key, size: len(f.objects[key])})
	}
	f.mu.Unlock()

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">`)
	b.WriteString("<Name>" + fakeBucket + "</Name>")
	b.WriteString("<Prefix>" + html.EscapeString(prefix) + "</Prefix>")
	fmt.Fprintf(&b, "<KeyCount>%d</KeyCount>", len(items))
	b.WriteString("<MaxKeys>1000</MaxKeys><IsTruncated>false</IsTruncated>")
	for _, item := range items {
		b.WriteString("<Contents><Key>" + html.EscapeString(item.key) + "</Key>")
		b.WriteString("<LastModified>2026-01-01T00:00:00.000Z</LastModified>")
		b.WriteString(`<ETag>"etag"</ETag>`)
		fmt.Fprintf(&b, "<Size>%d</Size><StorageClass>STANDARD</StorageClass></Contents>", item.size)
	}
	b.WriteString("</ListBucketResult>")
	w.Header().Set("Content-Type", "application/xml")
	_, _ = io.WriteString(w, b.String())
}

func (f *fakeObjectStore) hasPrefix(prefix string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for key := range f.objects {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func TestRecoveryIntegration_S3(t *testing.T) {
	adminDSN := os.Getenv("BACKUP_MANAGER_TEST_DSN")
	if adminDSN == "" {
		t.Skip("BACKUP_MANAGER_TEST_DSN is not set")
	}
	for _, binary := range []string{"pg_dump", "pg_restore", "psql"} {
		if _, err := exec.LookPath(binary); err != nil {
			t.Skipf("%s not found in PATH", binary)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store := newFakeObjectStore()
	server := httptest.NewServer(store)
	defer server.Close()

	metaName := createTestDatabase(t, adminDSN, "bm_it_s3_meta")
	targetName := createTestDatabase(t, adminDSN, "bm_it_s3_target")

	publicKey, privateKey, err := crypto.GenerateX25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		LogLevel:                "error",
		TempDir:                 t.TempDir(),
		ConfigEncryptionKey:     "test-config-encryption-key",
		MetadataBackupEmbed:     true,
		MetadataBackupRetention: 14,
		AgePublicKey:            publicKey,
		AgePrivateKey:           privateKey,
		MetadataDB:              databaseConfigFromDSN(t, adminDSN, metaName),
		TargetDB:                databaseConfigFromDSN(t, adminDSN, targetName),
	}
	if err := database.Migrate(ctx, cfg.MetadataDB, "../../sql/migrations", 30*time.Second); err != nil {
		t.Fatalf("migrate metadata: %v", err)
	}
	seedTarget(t, cfg.TargetDB)
	sourceChecksums := queryChecksums(t, cfg.TargetDB)

	pool, err := database.New(ctx, cfg.MetadataDB)
	if err != nil {
		t.Fatal(err)
	}
	encKey := crypto.DeriveConfigKey(cfg.ConfigEncryptionKey)
	repo := repository.NewPostgres(pool, encKey)
	providers := service.NewProviderService(repo, cfg)

	providerConfig, err := json.Marshal(map[string]any{
		"region":           "us-east-1",
		"bucket":           fakeBucket,
		"access_key":       "test",
		"secret_key":       "test",
		"endpoint":         server.URL,
		"force_path_style": true,
		"use_ssl":          false,
	})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := repo.CreateStorageProvider(ctx, db.CreateStorageProviderParams{
		Name:   "s3",
		Type:   "s3",
		Config: providerConfig,
	})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := repo.CreateProfile(ctx, db.CreateProfileParams{
		Name:              "s3-integration",
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
		discardLogger(),
		nil,
		nil,
		nil,
	)
	if err := backupService.ExecuteBackup(ctx, profile.ID); err != nil {
		t.Fatalf("backup: %v", err)
	}
	if !store.hasPrefix("meta/" + pgutil.UUIDToString(profile.ID) + "/") {
		t.Fatal("metadata snapshot was not uploaded to S3")
	}
	executions, err := repo.ListExecutionsByProfile(ctx, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(executions) != 1 || executions[0].Status != "success" {
		t.Fatalf("unexpected executions: %+v", executions)
	}
	pool.Close()

	dropTestDatabase(t, adminDSN, metaName)
	dropTestDatabase(t, adminDSN, targetName)
	targetName = createTestDatabase(t, adminDSN, "bm_it_s3_target_restored")
	newMetaName := createTestDatabase(t, adminDSN, "bm_it_s3_meta_restored")
	cfg.MetadataDB = databaseConfigFromDSN(t, adminDSN, newMetaName)
	cfg.TargetDB = databaseConfigFromDSN(t, adminDSN, targetName)

	if _, err := Metadata(ctx, cfg, MetadataOptions{
		S3: S3Options{
			Bucket:    fakeBucket,
			Region:    "us-east-1",
			Prefix:    "meta/",
			Endpoint:  server.URL,
			AccessKey: "test",
			SecretKey: "test",
			PathStyle: true,
			UseSSL:    false,
		},
	}); err != nil {
		t.Fatalf("recover metadata from S3: %v", err)
	}

	if _, err := Data(ctx, cfg, DataOptions{
		ProfileID: pgutil.UUIDToString(profile.ID),
		Confirm:   true,
	}); err != nil {
		t.Fatalf("recover data from S3: %v", err)
	}
	if got, want := queryChecksums(t, cfg.TargetDB), sourceChecksums; got != want {
		t.Fatalf("S3 recovery checksums differ:\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
