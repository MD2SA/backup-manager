package recovery

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/MD2SA/backup-manager/internal/config"
	"github.com/MD2SA/backup-manager/internal/metarestore"
	"github.com/MD2SA/backup-manager/internal/storage"
	"github.com/MD2SA/backup-manager/internal/storage/local"
)

var errTest = errors.New("test error")

type restoreCall struct {
	called bool
	opts   metarestore.Options
}

func fakeRestore(call *restoreCall) func(*slog.Logger, config.Config, metarestore.Options) error {
	return func(_ *slog.Logger, _ config.Config, opts metarestore.Options) error {
		call.called = true
		call.opts = opts
		return nil
	}
}

func TestLatestSnapshotKey(t *testing.T) {
	keys := []string{
		"meta/p1/metadata-20260101-000000.dump.age",
		"meta/p1/metadata-20260102-000000.dump.age",
		"meta/p1/metadata-20260103-000000.dump.age",
		"meta/p1/notes.txt",
		"meta/p1/metadata-20260104-000000.dump.sha256",
	}
	if got, want := LatestSnapshotKey(keys), "meta/p1/metadata-20260103-000000.dump.age"; got != want {
		t.Fatalf("LatestSnapshotKey = %q, want %q", got, want)
	}
	if got := LatestSnapshotKey([]string{"meta/p1/notes.txt"}); got != "" {
		t.Fatalf("expected no snapshot, got %q", got)
	}
}

func TestFindMetadataSnapshotsLocal(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "metadata", "metadata-20260101-000000.dump.age"), "x")
	mustWrite(t, filepath.Join(root, "meta", "profile-a", "metadata-20260102-000000.dump.age"), "x")
	mustWrite(t, filepath.Join(root, "meta", "profile-b", "metadata-20260103-000000.dump.age"), "x")
	mustWrite(t, filepath.Join(root, "meta", "profile-b", "other.txt"), "x")

	provider, err := local.New(root)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := FindMetadataSnapshots(context.Background(), provider, []string{"meta/", "metadata/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 3 {
		t.Fatalf("expected 3 snapshots, got %v", keys)
	}
	if got, want := LatestSnapshotKey(keys), filepath.Join("meta", "profile-b", "metadata-20260103-000000.dump.age"); got != want {
		t.Fatalf("latest = %q, want %q", got, want)
	}

	filtered, err := FindMetadataSnapshots(context.Background(), provider, []string{metadataPrefix("profile-a")})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 {
		t.Fatalf("expected one profile snapshot, got %v", filtered)
	}
}

func TestRunMetadata_File(t *testing.T) {
	call := &restoreCall{}
	cfg := testConfig(t)
	snapshot := filepath.Join(t.TempDir(), "snapshot.dump.age")
	mustWrite(t, snapshot, "snapshot")

	result, err := runMetadata(context.Background(), cfg, MetadataOptions{
		File:       snapshot,
		Passphrase: "pass",
	}, metadataDeps{restore: fakeRestore(call)})
	if err != nil {
		t.Fatal(err)
	}
	if !call.called || call.opts.File != snapshot || call.opts.Passphrase != "pass" {
		t.Fatalf("unexpected restore call: %+v", call)
	}
	if result.Source != snapshot || result.Latest != snapshot {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRunMetadata_ListDoesNotRestore(t *testing.T) {
	call := &restoreCall{}
	cfg := testConfig(t)
	root := cfg.StoragePath
	mustWrite(t, filepath.Join(root, "meta", "p1", "metadata-20260101-000000.dump.age"), "x")

	result, err := runMetadata(context.Background(), cfg, MetadataOptions{
		List: true,
	}, metadataDeps{restore: fakeRestore(call)})
	if err != nil {
		t.Fatal(err)
	}
	if call.called {
		t.Fatal("restore must not be called in list mode")
	}
	if len(result.Snapshots) != 1 || result.Root != root {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRunMetadata_LocalDiscovery(t *testing.T) {
	call := &restoreCall{}
	cfg := testConfig(t)
	key := filepath.Join("meta", "p1", "metadata-20260102-000000.dump.age")
	mustWrite(t, filepath.Join(cfg.StoragePath, key), "x")

	result, err := runMetadata(context.Background(), cfg, MetadataOptions{}, metadataDeps{restore: fakeRestore(call)})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(cfg.StoragePath, key)
	if result.Source != want || call.opts.File != want {
		t.Fatalf("source=%q restore=%q, want %q", result.Source, call.opts.File, want)
	}
}

func TestRunMetadata_S3Download(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "meta", "p1", "metadata-20260103-000000.dump.age"), "s3-body")
	provider, err := local.New(root)
	if err != nil {
		t.Fatal(err)
	}

	var downloaded string
	call := &restoreCall{}
	deps := metadataDeps{
		newS3: func(context.Context, S3Options) (storage.StorageProvider, error) {
			return provider, nil
		},
		restore: func(_ *slog.Logger, _ config.Config, opts metarestore.Options) error {
			data, readErr := os.ReadFile(opts.File)
			if readErr != nil {
				return readErr
			}
			downloaded = string(data)
			call.called = true
			call.opts = opts
			return nil
		},
	}

	cfg := testConfig(t)
	result, err := runMetadata(context.Background(), cfg, MetadataOptions{
		S3: S3Options{Bucket: "backups", Prefix: "meta/", Region: "us-east-1"},
	}, deps)
	if err != nil {
		t.Fatal(err)
	}
	if downloaded != "s3-body" {
		t.Fatalf("downloaded %q, want s3-body", downloaded)
	}
	if result.Source != "s3://backups/meta/p1/metadata-20260103-000000.dump.age" {
		t.Fatalf("unexpected source: %q", result.Source)
	}
	if !call.called {
		t.Fatal("restore was not called")
	}
}

func TestRunMetadata_NoSnapshots(t *testing.T) {
	cfg := testConfig(t)
	_, err := runMetadata(context.Background(), cfg, MetadataOptions{}, metadataDeps{restore: fakeRestore(&restoreCall{})})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestRunMetadata_InvalidProfileID(t *testing.T) {
	cfg := testConfig(t)
	_, err := runMetadata(context.Background(), cfg, MetadataOptions{ProfileID: "not-a-uuid"}, metadataDeps{restore: fakeRestore(&restoreCall{})})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func testConfig(t *testing.T) config.Config {
	t.Helper()
	return config.Config{
		LogLevel:    "error",
		TempDir:     t.TempDir(),
		StoragePath: t.TempDir(),
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRunMetadata_S3ExplicitKey(t *testing.T) {
	root := t.TempDir()
	key := filepath.Join("meta", "p1", "metadata-20260104-000000.dump.age")
	mustWrite(t, filepath.Join(root, key), "explicit")
	provider, err := local.New(root)
	if err != nil {
		t.Fatal(err)
	}
	call := &restoreCall{}
	deps := metadataDeps{
		newS3: func(context.Context, S3Options) (storage.StorageProvider, error) {
			return provider, nil
		},
		restore: fakeRestore(call),
	}

	cfg := testConfig(t)
	result, err := runMetadata(context.Background(), cfg, MetadataOptions{
		S3: S3Options{Bucket: "backups", Key: "meta/p1/metadata-20260104-000000.dump.age"},
	}, deps)
	if err != nil {
		t.Fatal(err)
	}
	if !call.called || result.Source != "s3://backups/"+filepath.ToSlash(key) {
		t.Fatalf("unexpected result: %+v call=%+v", result, call)
	}
}

func TestRunMetadata_FileList(t *testing.T) {
	call := &restoreCall{}
	cfg := testConfig(t)
	snapshot := filepath.Join(t.TempDir(), "snapshot.dump")
	result, err := runMetadata(context.Background(), cfg, MetadataOptions{
		File: snapshot,
		List: true,
	}, metadataDeps{restore: fakeRestore(call)})
	if err != nil {
		t.Fatal(err)
	}
	if call.called || len(result.Snapshots) != 1 || result.Latest != snapshot {
		t.Fatalf("unexpected list result: %+v", result)
	}
}

func TestRunMetadata_MissingStoragePath(t *testing.T) {
	cfg := config.Config{LogLevel: "error", TempDir: t.TempDir()}
	_, err := runMetadata(context.Background(), cfg, MetadataOptions{}, metadataDeps{restore: fakeRestore(&restoreCall{})})
	if err == nil {
		t.Fatal("expected missing storage path error")
	}
}

func TestMetadataPublicValidation(t *testing.T) {
	if _, err := Metadata(context.Background(), config.Config{}, MetadataOptions{ProfileID: "bad"}); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestNewS3ProviderValidation(t *testing.T) {
	if _, err := newS3Provider(context.Background(), S3Options{}); err == nil {
		t.Fatal("expected missing bucket error")
	}
	if _, err := newS3Provider(context.Background(), S3Options{Bucket: "b"}); err == nil {
		t.Fatal("expected missing credentials error")
	}
}

func TestRunMetadata_S3ProviderError(t *testing.T) {
	cfg := testConfig(t)
	_, err := runMetadata(context.Background(), cfg, MetadataOptions{
		S3: S3Options{Bucket: "backups", Prefix: "meta/"},
	}, metadataDeps{
		newS3: func(context.Context, S3Options) (storage.StorageProvider, error) {
			return nil, errTest
		},
		restore: fakeRestore(&restoreCall{}),
	})
	if err == nil {
		t.Fatal("expected S3 provider error")
	}
}

func TestRunMetadata_S3DownloadError(t *testing.T) {
	provider, err := local.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := testConfig(t)
	_, err = runMetadata(context.Background(), cfg, MetadataOptions{
		S3: S3Options{Bucket: "backups", Key: "meta/p1/missing.dump.age"},
	}, metadataDeps{
		newS3: func(context.Context, S3Options) (storage.StorageProvider, error) {
			return provider, nil
		},
		restore: fakeRestore(&restoreCall{}),
	})
	if err == nil {
		t.Fatal("expected download error")
	}
}
