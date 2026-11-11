package config

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"filippo.io/age"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// Check performs production preflight checks that go beyond schema validation:
// file permissions, required binaries, writable directories, Age key
// consistency, cron syntax and, optionally, database connectivity.
//
// Warnings are returned separately so the CLI can print them without failing.
func Check(cfg Config, connect bool) (warnings []string, err error) {
	var errs []error
	warn := func(format string, args ...any) {
		warnings = append(warnings, fmt.Sprintf(format, args...))
	}
	add := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf(format, args...))
	}

	if info, statErr := os.Stat(cfg.SecretsPath); statErr != nil {
		add("secrets file %s: %v", cfg.SecretsPath, statErr)
	} else if info.Mode().Perm()&0o077 != 0 {
		warn("secrets file %s is readable by group/others (mode %04o); use chmod 600", cfg.SecretsPath, info.Mode().Perm())
	}

	for _, bin := range []string{"pg_dump", "psql"} {
		if _, lookErr := exec.LookPath(bin); lookErr != nil {
			add("%s not found in PATH (backup/restore will fail)", bin)
		}
	}
	if _, lookErr := exec.LookPath("pg_restore"); lookErr != nil {
		warn("pg_restore not found in PATH (metadata-restore will fail)")
	}

	checkDir(cfg.StoragePath, "storage.path", add)
	checkDir(cfg.TempDir, "storage.temp_dir", add)

	if cfg.IsProduction() && !filepath.IsAbs(cfg.StoragePath) {
		warn("storage.path is relative (%q); use an absolute path in production", cfg.StoragePath)
	}
	if cfg.IsProduction() && !filepath.IsAbs(cfg.TempDir) {
		warn("storage.temp_dir is relative (%q); use an absolute path in production", cfg.TempDir)
	}

	if cfg.AgePublicKey != "" {
		if _, ageErr := age.ParseX25519Recipient(cfg.AgePublicKey); ageErr != nil {
			add("encryption.age_public_key: %v", ageErr)
		}
		if cfg.AgePrivateKey == "" && cfg.EncryptionPassphrase == "" {
			warn("Age public key is configured but no private key or passphrase is available to restore those backups")
		}
	}
	if cfg.AgePrivateKey != "" {
		identity, ageErr := age.ParseX25519Identity(cfg.AgePrivateKey)
		if ageErr != nil {
			add("Age private key: %v", ageErr)
		} else if cfg.AgePublicKey != "" && identity.Recipient().String() != cfg.AgePublicKey {
			add("Age public key and private key do not match")
		}
	}

	if !cfg.IsTargetDBConfigured() {
		warn("target database is not configured; backups and restores will fail until it is")
	}

	if connect {
		if pingErr := pingDatabase(cfg.MetadataDB); pingErr != nil {
			add("metadata database: %v", pingErr)
		}
		if cfg.IsTargetDBConfigured() {
			if pingErr := pingDatabase(cfg.TargetDB); pingErr != nil {
				add("target database: %v", pingErr)
			}
			if cfg.TargetDB.RestoreUser != "" {
				restoreDB := cfg.TargetDB
				restoreDB.User = cfg.TargetDB.RestoreUser
				restoreDB.Password = cfg.TargetDB.RestorePassword
				if pingErr := pingDatabase(restoreDB); pingErr != nil {
					add("target restore database: %v", pingErr)
				}
			}
			checkPostgresClientCompatibility(cfg.TargetDB, add, warn)
		}
	}

	return warnings, errors.Join(errs...)
}

var postgresVersionPattern = regexp.MustCompile(`\b(\d+)\.\d+\b`)

func parsePostgresClientVersion(output string) (int, error) {
	match := postgresVersionPattern.FindStringSubmatch(output)
	if len(match) < 2 {
		return 0, fmt.Errorf("cannot parse PostgreSQL version from %q", strings.TrimSpace(output))
	}
	major, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, fmt.Errorf("parse PostgreSQL major from %q: %w", output, err)
	}
	return major, nil
}

func postgresClientVersion(binary string) (int, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, binary, "--version").Output()
	if err != nil {
		return 0, "", err
	}
	raw := strings.TrimSpace(string(out))
	major, err := parsePostgresClientVersion(raw)
	return major, raw, err
}

func postgresServerVersion(cfg DatabaseConfig) (int, string, error) {
	conn, err := sql.Open("pgx", cfg.DSN())
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var raw string
	if err := conn.QueryRowContext(ctx, "SHOW server_version_num").Scan(&raw); err != nil {
		return 0, "", err
	}
	num, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, raw, fmt.Errorf("parse server_version_num %q: %w", raw, err)
	}
	major := num / 10000
	if major < 10 {
		// PostgreSQL 9.x used a different encoding; treat it as unsupported.
		return major, raw, fmt.Errorf("unsupported PostgreSQL version %s", raw)
	}
	return major, raw, nil
}

func checkPostgresClientCompatibility(cfg DatabaseConfig, add func(string, ...any), warn func(string, ...any)) {
	serverMajor, serverRaw, err := postgresServerVersion(cfg)
	if err != nil {
		warn("could not determine target PostgreSQL version: %v", err)
		return
	}
	for _, binary := range []string{"pg_dump", "pg_restore", "psql"} {
		if _, lookErr := exec.LookPath(binary); lookErr != nil {
			continue
		}
		major, raw, versionErr := postgresClientVersion(binary)
		if versionErr != nil {
			warn("could not determine %s version: %v", binary, versionErr)
			continue
		}
		if major < serverMajor {
			add("%s cannot operate on PostgreSQL %d (server %s); installed: %s; use a PostgreSQL %d client or newer", binary, serverMajor, serverRaw, raw, serverMajor)
		}
	}
}

func checkDir(path, field string, add func(string, ...any)) {
	if path == "" {
		return
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		add("%s: cannot create %s: %v", field, path, err)
		return
	}
	f, err := os.CreateTemp(path, ".backup-manager-check-*")
	if err != nil {
		add("%s: %s is not writable: %v", field, path, err)
		return
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
}

func pingDatabase(db DatabaseConfig) error {
	conn, err := sql.Open("pgx", db.DSN())
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := conn.PingContext(ctx); err != nil {
		return fmt.Errorf("%s@%s:%s/%s: %w", db.User, db.Host, db.Port, db.DBName, err)
	}
	return nil
}

// RedactedSummary returns a human-readable, secret-free view of the effective
// configuration for `config check`.
func (c Config) RedactedSummary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "config file:    %s\n", c.ConfigPath)
	fmt.Fprintf(&b, "secrets file:   %s\n", c.SecretsPath)
	fmt.Fprintf(&b, "server:         :%s env=%s log_level=%s\n", c.Port, c.Env, c.LogLevel)
	fmt.Fprintf(&b, "metadata db:    %s@%s:%s/%s sslmode=%s\n", c.MetadataDB.User, c.MetadataDB.Host, c.MetadataDB.Port, c.MetadataDB.DBName, c.MetadataDB.SSLMode)
	if c.IsTargetDBConfigured() {
		fmt.Fprintf(&b, "target db:      %s@%s:%s/%s sslmode=%s\n", c.TargetDB.User, c.TargetDB.Host, c.TargetDB.Port, c.TargetDB.DBName, c.TargetDB.SSLMode)
	} else {
		fmt.Fprintf(&b, "target db:      (not configured)\n")
	}
	fmt.Fprintf(&b, "storage:        %s (temp %s)\n", c.StoragePath, c.TempDir)
	fmt.Fprintf(&b, "encryption:     %s\n", c.encryptionMode())
	fmt.Fprintf(&b, "admin key:      %s\n", setStatus(c.AdminKey))
	fmt.Fprintf(&b, "config key:     %s\n", setStatus(c.ConfigEncryptionKey))
	fmt.Fprintf(&b, "metadata backup: schedule=%q retention=%d embed=%t\n", c.MetadataBackupSchedule, c.MetadataBackupRetention, c.MetadataBackupEmbed)
	fmt.Fprintf(&b, "timeouts:       backup=%s restore=%s\n", c.BackupTimeout, c.RestoreTimeout)
	fmt.Fprintf(&b, "overdue notify: interval=%s repeat=%s\n", c.OverdueNotifyInterval, c.OverdueNotifyRepeat)
	return b.String()
}

func (c Config) encryptionMode() string {
	var modes []string
	if c.EncryptionPassphrase != "" {
		modes = append(modes, "passphrase")
	}
	if c.AgePublicKey != "" {
		modes = append(modes, "age")
	}
	if len(modes) == 0 {
		return "none"
	}
	if len(modes) == 2 {
		return "age + passphrase"
	}
	return modes[0]
}

func setStatus(value string) string {
	if value == "" {
		return "not set"
	}
	return "set"
}
