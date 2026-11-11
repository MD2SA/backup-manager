package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/MD2SA/backup-manager/internal/config"
	"github.com/MD2SA/backup-manager/internal/storage/local"
)

func TestLatestSnapshotKey(t *testing.T) {
	keys := []string{
		"meta/p1/metadata-20260101-000000.dump.age",
		"meta/p1/metadata-20260102-000000.dump.age",
		"meta/p1/metadata-20260103-000000.dump.age",
		"meta/p1/notes.txt",
		"meta/p1/metadata-20260104-000000.dump.sha256",
	}
	if got, want := latestSnapshotKey(keys), "meta/p1/metadata-20260103-000000.dump.age"; got != want {
		t.Fatalf("latestSnapshotKey = %q, want %q", got, want)
	}
	if got := latestSnapshotKey([]string{"meta/p1/notes.txt"}); got != "" {
		t.Fatalf("expected no snapshot, got %q", got)
	}
}

func TestListMetadataSnapshotsLocal(t *testing.T) {
	root := t.TempDir()
	mustWrite := func(rel, content string) {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("metadata/metadata-20260101-000000.dump.age", "x")
	mustWrite("meta/profile-a/metadata-20260102-000000.dump.age", "x")
	mustWrite("meta/profile-b/metadata-20260103-000000.dump.age", "x")
	mustWrite("meta/profile-b/other.txt", "x")

	provider, err := local.New(root)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := listMetadataSnapshots(context.Background(), provider, []string{"meta/", "metadata/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 3 {
		t.Fatalf("expected 3 snapshots, got %v", keys)
	}
	if got, want := latestSnapshotKey(keys), filepath.Join("meta", "profile-b", "metadata-20260103-000000.dump.age"); got != want {
		t.Fatalf("latest = %q, want %q", got, want)
	}

	filtered, err := listMetadataSnapshots(context.Background(), provider, []string{metadataPrefix("profile-a")})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 {
		t.Fatalf("expected one profile snapshot, got %v", filtered)
	}
}

func TestRestoreTargetForRecovery(t *testing.T) {
	cfg := config.Config{
		MetadataDB: config.DatabaseConfig{
			Host: "db", Port: "5432", User: "meta", DBName: "metadata",
		},
		TargetDB: config.DatabaseConfig{
			Host:            "db",
			Port:            "5432",
			User:            "backup_reader",
			Password:        "read",
			DBName:          "app",
			SSLMode:         "disable",
			RestoreUser:     "restore_admin",
			RestorePassword: "write",
		},
	}

	target, err := restoreTargetForRecovery("", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if target.User != "restore_admin" || target.Password != "write" || target.DBName != "app" {
		t.Fatalf("unexpected config target: %+v", target)
	}

	target, err = restoreTargetForRecovery("postgres://u:p@scratch:5432/drill?sslmode=require", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if target.Host != "scratch" || target.DBName != "drill" || target.SSLMode != "require" {
		t.Fatalf("unexpected DSN target: %+v", target)
	}

	if _, err := restoreTargetForRecovery("postgres://u:p@db:5432/metadata", cfg); err == nil {
		t.Fatal("expected metadata database target to be refused")
	}
}
