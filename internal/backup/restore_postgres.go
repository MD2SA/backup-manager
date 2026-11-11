package backup

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/MD2SA/backup-manager/internal/pgcompat"
	"github.com/jackc/pgx/v5"
)

// RestoreTarget describes the PostgreSQL database that will receive a restore.
type RestoreTarget struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// DSN renders the connection string used for preflight checks.
func (t RestoreTarget) DSN() string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(t.User, t.Password),
		Host:   net.JoinHostPort(t.Host, t.Port),
		Path:   "/" + t.DBName,
	}
	q := u.Query()
	q.Set("sslmode", t.SSLMode)
	u.RawQuery = q.Encode()
	return u.String()
}

// Preflight validates that the target database is safe to restore into:
// reachable, empty, and free of competing client connections.
func (t RestoreTarget) Preflight(ctx context.Context, terminateConnections bool) error {
	conn, err := t.connect(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(context.Background()) }()

	if err := checkEmpty(ctx, conn, t.DBName); err != nil {
		return err
	}

	active, err := activeConnectionCount(ctx, conn)
	if err != nil {
		return err
	}
	if active == 0 {
		return nil
	}
	if !terminateConnections {
		return fmt.Errorf(
			"target database %q has %d active connection(s); close them or retry with terminate_connections=true",
			t.DBName, active,
		)
	}

	if _, err := conn.Exec(ctx, terminateConnectionsQuery); err != nil {
		return fmt.Errorf("terminate target database connections: %w", err)
	}

	// Give PostgreSQL a moment to reap the terminated backends.
	for attempt := 0; attempt < 10; attempt++ {
		active, err = activeConnectionCount(ctx, conn)
		if err != nil {
			return err
		}
		if active == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return fmt.Errorf("target database %q still has %d active connection(s) after termination", t.DBName, active)
}

// CheckEmpty verifies that the target database contains no user objects. It is
// used at enqueue time so an obvious mistake returns 409 immediately instead of
// failing asynchronously.
func (t RestoreTarget) CheckEmpty(ctx context.Context) error {
	conn, err := t.connect(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(context.Background()) }()
	return checkEmpty(ctx, conn, t.DBName)
}

func (t RestoreTarget) connect(ctx context.Context) (*pgx.Conn, error) {
	conn, err := pgx.Connect(ctx, t.DSN())
	if err != nil {
		return nil, fmt.Errorf("connect to target database: %w", err)
	}
	return conn, nil
}

func checkEmpty(ctx context.Context, conn *pgx.Conn, dbName string) error {
	var objects int64
	if err := conn.QueryRow(ctx, userObjectCountQuery).Scan(&objects); err != nil {
		return fmt.Errorf("inspect target database: %w", err)
	}
	if objects > 0 {
		return fmt.Errorf(
			"target database %q is not empty (%d user objects found); restore requires an empty database",
			dbName, objects,
		)
	}
	return nil
}

const userObjectCountQuery = `SELECT count(*)
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname NOT IN ('pg_catalog', 'information_schema')
  AND c.relkind IN ('r', 'p', 'v', 'm', 'S')`

const activeConnectionsQuery = `SELECT count(*)
FROM pg_stat_activity
WHERE datname = current_database()
  AND pid <> pg_backend_pid()
  AND backend_type = 'client backend'`

const terminateConnectionsQuery = `SELECT pg_terminate_backend(pid)
FROM pg_stat_activity
WHERE datname = current_database()
  AND pid <> pg_backend_pid()
  AND backend_type = 'client backend'`

func activeConnectionCount(ctx context.Context, conn *pgx.Conn) (int64, error) {
	var count int64
	if err := conn.QueryRow(ctx, activeConnectionsQuery).Scan(&count); err != nil {
		return 0, fmt.Errorf("inspect target database connections: %w", err)
	}
	return count, nil
}

// ApplyArtifact applies a prepared artifact to the target database.
//
// Plain SQL artifacts are streamed to psql. Custom-format archives are applied
// with pg_restore; if a newer archive contains settings an older server does
// not understand, a filtered SQL fallback is used. Restores run with
// ON_ERROR_STOP/--exit-on-error and, by default, a single transaction.
func ApplyArtifact(ctx context.Context, artifact *Artifact, target RestoreTarget, singleTransaction bool) (string, error) {
	if artifact.CustomFormat {
		return applyCustomArtifact(ctx, artifact, target, singleTransaction)
	}

	reader, err := artifact.Open()
	if err != nil {
		return "", fmt.Errorf("open artifact: %w", err)
	}
	defer func() { _ = reader.Close() }()

	output, err := runPSQL(ctx, pgcompat.NewSQLFilter(reader), target, singleTransaction)
	if err != nil {
		return output, fmt.Errorf("psql restore failed: %w\n%s", err, output)
	}
	return output, nil
}

func applyCustomArtifact(ctx context.Context, artifact *Artifact, target RestoreTarget, singleTransaction bool) (string, error) {
	args := []string{
		"--exit-on-error",
		"--no-owner",
		"--no-acl",
		"--clean",
		"--if-exists",
		"-w",
	}
	if singleTransaction {
		args = append(args, "--single-transaction")
	}
	args = append(args,
		"-h", target.Host,
		"-p", target.Port,
		"-U", target.User,
		"-d", target.DBName,
		artifact.Path,
	)

	output, err := runRestoreCommand(ctx, target, "pg_restore", args, nil)
	if err == nil {
		return output, nil
	}
	if !pgcompat.IsTransactionTimeoutError(output) {
		return output, fmt.Errorf("pg_restore failed: %w\n%s", err, output)
	}

	// A newer pg_restore archive can contain SET transaction_timeout, which
	// older servers reject. Convert the archive to SQL, filter the preamble and
	// apply it with psql instead.
	sqlFile, createErr := os.CreateTemp("", "backup-manager-restore-*.sql")
	if createErr != nil {
		return output, fmt.Errorf("pg_restore compatibility fallback: %w", createErr)
	}
	sqlPath := sqlFile.Name()
	_ = sqlFile.Close()
	defer func() { _ = os.Remove(sqlPath) }()

	convertArgs := []string{
		"--exit-on-error",
		"--no-owner",
		"--no-acl",
		"--clean",
		"--if-exists",
		"-w",
		"-f", sqlPath,
		artifact.Path,
	}
	convertOutput, convertErr := runRestoreCommand(ctx, target, "pg_restore", convertArgs, nil)
	if convertErr != nil {
		return convertOutput, fmt.Errorf("pg_restore SQL conversion failed: %w\n%s", convertErr, convertOutput)
	}

	filtered, openErr := os.Open(sqlPath)
	if openErr != nil {
		return output, fmt.Errorf("open converted SQL: %w", openErr)
	}
	defer func() { _ = filtered.Close() }()

	fallbackOutput, fallbackErr := runPSQL(ctx, pgcompat.NewSQLFilter(filtered), target, singleTransaction)
	if fallbackErr != nil {
		return fallbackOutput, fmt.Errorf("psql compatibility fallback failed: %w\n%s", fallbackErr, fallbackOutput)
	}
	return fallbackOutput, nil
}

func runPSQL(ctx context.Context, reader io.Reader, target RestoreTarget, singleTransaction bool) (string, error) {
	args := []string{
		"-X",
		"-w",
		"-v", "ON_ERROR_STOP=1",
	}
	if singleTransaction {
		args = append(args, "--single-transaction")
	}
	args = append(args,
		"-h", target.Host,
		"-p", target.Port,
		"-U", target.User,
		"-d", target.DBName,
	)
	return runRestoreCommand(ctx, target, "psql", args, reader)
}

func runRestoreCommand(ctx context.Context, target RestoreTarget, binary string, args []string, stdin io.Reader) (string, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = append(os.Environ(),
		"PGPASSWORD="+target.Password,
		"PGSSLMODE="+target.SSLMode,
		"PGAPPNAME=backup-manager-restore",
		"PGCONNECT_TIMEOUT=10",
	)
	cmd.Stdin = stdin

	output := &limitedBuffer{max: 128 * 1024}
	cmd.Stdout = output
	cmd.Stderr = output

	err := cmd.Run()
	return output.String(), err
}

// limitedBuffer captures a bounded amount of command output so a noisy psql
// session cannot exhaust memory or the metadata database.
type limitedBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	original := len(p)
	if b.max <= 0 {
		b.max = 128 * 1024
	}
	remaining := b.max - b.buf.Len()
	if remaining <= 0 {
		b.truncated = true
		return original, nil
	}
	if len(p) > remaining {
		p = p[:remaining]
		b.truncated = true
	}
	_, _ = b.buf.Write(p)
	return original, nil
}

func (b *limitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	out := b.buf.String()
	if b.truncated {
		out += "\n[output truncated]"
	}
	return out
}
