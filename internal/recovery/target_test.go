package recovery

import (
	"testing"

	"github.com/MD2SA/backup-manager/internal/config"
)

func TestParseTargetDSN(t *testing.T) {
	target, err := ParseTargetDSN("postgres://user:pass@localhost:55432/scratch?sslmode=require")
	if err != nil {
		t.Fatal(err)
	}
	if target.Host != "localhost" || target.Port != "55432" || target.User != "user" ||
		target.Password != "pass" || target.DBName != "scratch" || target.SSLMode != "require" {
		t.Fatalf("unexpected target: %+v", target)
	}
}

func TestResolveTarget(t *testing.T) {
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

	target, err := ResolveTarget("", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if target.User != "restore_admin" || target.Password != "write" || target.DBName != "app" {
		t.Fatalf("unexpected config target: %+v", target)
	}

	target, err = ResolveTarget("postgres://u:p@scratch:5432/drill?sslmode=require", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if target.Host != "scratch" || target.DBName != "drill" || target.SSLMode != "require" {
		t.Fatalf("unexpected DSN target: %+v", target)
	}

	if _, err := ResolveTarget("postgres://u:p@db:5432/metadata", cfg); err == nil {
		t.Fatal("expected metadata database target to be refused")
	}
}
