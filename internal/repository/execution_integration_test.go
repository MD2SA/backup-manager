package repository

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MD2SA/backup-manager/internal/database"
	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestReconcileInterruptedBackupsIntegration_Postgres(t *testing.T) {
	adminDSN := os.Getenv("BACKUP_MANAGER_TEST_DSN")
	if adminDSN == "" {
		t.Skip("BACKUP_MANAGER_TEST_DSN is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("connect admin: %v", err)
	}
	defer func() { _ = admin.Close(context.Background()) }()

	name := "bm_reconcile_" + randomSuffix(t)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create database: %v", err)
	}
	defer dropDatabase(t, adminDSN, name)

	cfg := databaseConfig(t, replaceDatabase(t, adminDSN, name))
	if err := database.Migrate(ctx, cfg, filepath.Join("..", "..", "sql", "migrations"), 30*time.Second); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := database.New(ctx, cfg)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()
	repo := NewPostgres(pool, nil)

	profileID := pgutil.NewUUID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO profiles (id, name, schedule, compression_type, compression_level) VALUES ($1,$2,'0 0 * * *','gzip',6)`,
		profileID, "reconcile",
	); err != nil {
		t.Fatalf("insert profile: %v", err)
	}

	ids := map[string]string{}
	for _, status := range []string{"pending", "running", "success", "failed"} {
		id := pgutil.NewUUID()
		ids[status] = pgutil.UUIDToString(id)
		if _, err := pool.Exec(ctx,
			`INSERT INTO executions (id, profile_id, status, start_time) VALUES ($1,$2,$3,now())`,
			id, profileID, status,
		); err != nil {
			t.Fatalf("insert %s execution: %v", status, err)
		}
	}

	reconciled, err := repo.ReconcileInterruptedBackups(ctx)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if reconciled != 2 {
		t.Fatalf("reconciled %d executions, want 2", reconciled)
	}

	for _, status := range []string{"pending", "running"} {
		var gotStatus, message string
		var endTime pgtype.Timestamptz
		if err := pool.QueryRow(ctx,
			`SELECT status, error_message, end_time FROM executions WHERE id = $1`,
			ids[status],
		).Scan(&gotStatus, &message, &endTime); err != nil {
			t.Fatalf("read %s execution: %v", status, err)
		}
		if gotStatus != "failed" || message != "interrupted by restart" || !endTime.Valid {
			t.Fatalf("%s execution not reconciled: status=%s message=%q end_time=%v", status, gotStatus, message, endTime)
		}
	}

	for _, status := range []string{"success", "failed"} {
		var gotStatus string
		if err := pool.QueryRow(ctx, `SELECT status FROM executions WHERE id = $1`, ids[status]).Scan(&gotStatus); err != nil {
			t.Fatalf("read %s execution: %v", status, err)
		}
		if gotStatus != status {
			t.Fatalf("%s execution changed to %s", status, gotStatus)
		}
	}
}
