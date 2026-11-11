package repository

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/MD2SA/backup-manager/internal/config"
	"github.com/MD2SA/backup-manager/internal/database"
	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestPaginationIntegration_Postgres(t *testing.T) {
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

	name := "bm_page_" + randomSuffix(t)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create database: %v", err)
	}
	defer dropDatabase(t, adminDSN, name)

	dsn := replaceDatabase(t, adminDSN, name)
	cfg := databaseConfig(t, dsn)
	if err := database.Migrate(ctx, cfg, filepath.Join("..", "..", "sql", "migrations"), 30*time.Second); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := database.New(ctx, cfg)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()
	repo := NewPostgres(pool, nil)

	base := time.Now().UTC().Truncate(time.Microsecond)
	for i := 0; i < 5; i++ {
		id := pgutil.NewUUID()
		if _, err := pool.Exec(ctx,
			`INSERT INTO users (id, email, name, created_at, updated_at) VALUES ($1,$2,$3,$4,$4)`,
			id, fmt.Sprintf("u%d@example.com", i), fmt.Sprintf("User %d", i), base.Add(time.Duration(i)*time.Minute),
		); err != nil {
			t.Fatalf("insert user: %v", err)
		}
	}

	// Users are ordered by created_at ASC; page through them.
	var users []db.User
	cursorTime := pgtype.Timestamptz{}
	cursorID := pgtype.UUID{}
	for {
		page, err := repo.ListUsersPage(ctx, db.ListUsersPageParams{
			CursorTime: cursorTime,
			CursorID:   cursorID,
			PageLimit:  2,
		})
		if err != nil {
			t.Fatalf("users page: %v", err)
		}
		users = append(users, page...)
		if len(page) < 2 {
			break
		}
		last := page[len(page)-1]
		cursorTime = last.CreatedAt
		cursorID = last.ID
	}
	if len(users) != 5 {
		t.Fatalf("users = %d, want 5 (duplicates or gaps)", len(users))
	}

	// Profiles are ordered by name ASC.
	profileTimes := base
	profileIDs := make([]pgtype.UUID, 0, 3)
	for i, profileName := range []string{"alpha", "beta", "gamma"} {
		id := pgutil.NewUUID()
		profileIDs = append(profileIDs, id)
		if _, err := pool.Exec(ctx,
			`INSERT INTO profiles (id, name, enabled, schedule, compression_type, compression_level, created_at, updated_at)
			 VALUES ($1,$2,false,'0 0 * * *','none',0,$3,$3)`,
			id, profileName, profileTimes.Add(time.Duration(i)*time.Minute),
		); err != nil {
			t.Fatalf("insert profile: %v", err)
		}
	}
	var profiles []db.Profile
	cursorName := pgtype.Text{}
	cursorID = pgtype.UUID{}
	for {
		page, err := repo.ListProfilesPage(ctx, db.ListProfilesPageParams{
			CursorName: cursorName,
			CursorID:   cursorID,
			PageLimit:  2,
		})
		if err != nil {
			t.Fatalf("profiles page: %v", err)
		}
		for _, full := range page {
			profiles = append(profiles, full.Profile)
		}
		if len(page) < 2 {
			break
		}
		last := page[len(page)-1].Profile
		cursorName = pgutil.ToText(last.Name)
		cursorID = last.ID
	}
	if len(profiles) != 3 {
		t.Fatalf("profiles = %d, want 3", len(profiles))
	}

	// Executions are ordered by created_at DESC.
	execProfileID := profileIDs[0]
	for i := 0; i < 5; i++ {
		id := pgutil.NewUUID()
		if _, err := pool.Exec(ctx,
			`INSERT INTO executions (id, profile_id, status, created_at) VALUES ($1,$2,'success',$3)`,
			id, execProfileID, base.Add(time.Duration(i)*time.Minute),
		); err != nil {
			t.Fatalf("insert execution: %v", err)
		}
	}
	var executions []db.Execution
	cursorTime = pgtype.Timestamptz{}
	cursorID = pgtype.UUID{}
	for {
		page, err := repo.ListExecutionsPage(ctx, db.ListExecutionsPageParams{
			CursorTime: cursorTime,
			CursorID:   cursorID,
			PageLimit:  2,
		})
		if err != nil {
			t.Fatalf("executions page: %v", err)
		}
		executions = append(executions, page...)
		if len(page) < 2 {
			break
		}
		last := page[len(page)-1]
		cursorTime = last.CreatedAt
		cursorID = last.ID
	}
	if len(executions) != 5 {
		t.Fatalf("executions = %d, want 5", len(executions))
	}
	for i := 1; i < len(executions); i++ {
		if executions[i].CreatedAt.Time.After(executions[i-1].CreatedAt.Time) {
			t.Fatalf("executions are not descending: %v > %v", executions[i].CreatedAt.Time, executions[i-1].CreatedAt.Time)
		}
	}

	// The same keyset works with a profile filter.
	byProfile, err := repo.ListExecutionsByProfilePage(ctx, db.ListExecutionsByProfilePageParams{
		ProfileID: execProfileID,
		PageLimit: 3,
	})
	if err != nil {
		t.Fatalf("executions by profile: %v", err)
	}
	if len(byProfile) != 3 {
		t.Fatalf("by-profile page = %d, want 3", len(byProfile))
	}

	// Audit log uses the same descending keyset.
	for i := 0; i < 3; i++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO audit_log (action, created_at) VALUES ($1,$2)`,
			fmt.Sprintf("action-%d", i), base.Add(time.Duration(i)*time.Minute),
		); err != nil {
			t.Fatalf("insert audit: %v", err)
		}
	}
	audit, err := repo.ListAuditEntriesPage(ctx, db.ListAuditEntriesPageParams{PageLimit: 2})
	if err != nil {
		t.Fatalf("audit page: %v", err)
	}
	if len(audit) != 2 {
		t.Fatalf("audit page = %d, want 2", len(audit))
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
		SSLMode:  "disable",
	}
}

func dropDatabase(t *testing.T, adminDSN, name string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Logf("drop database: %v", err)
		return
	}
	defer func() { _ = admin.Close(context.Background()) }()
	_, _ = admin.Exec(ctx, `SELECT pg_terminate_backend(pid)
		FROM pg_stat_activity
		WHERE datname = $1 AND pid <> pg_backend_pid()`, name)
	if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Logf("drop database: %v", err)
	}
}
