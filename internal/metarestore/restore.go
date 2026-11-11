package metarestore

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/MD2SA/backup-manager/internal/config"
	"github.com/MD2SA/backup-manager/internal/pgcompat"
	"github.com/MD2SA/backup-manager/internal/pkg/crypto"
)

// Options controls the metadata restore run.
type Options struct {
	// File is an explicit snapshot path. When empty, the newest snapshot in
	// APP_STORAGE_PATH/metadata is used.
	File string
	// Passphrase overrides APP_METADATA_BACKUP_PASSPHRASE.
	Passphrase string
	// Passphrases tries several passphrases in order. It is used when the
	// metadata snapshot may have been encrypted with either the profile
	// encryption passphrase or the metadata backup passphrase.
	Passphrases []string
	// Identity overrides APP_AGE_PRIVATE_KEY.
	Identity string
	// Replace allows restoring over a database that already contains data
	// (pg_restore --clean --if-exists).
	Replace bool
	// DryRun validates the snapshot without writing anything to the database.
	DryRun bool
}

// snapshotFormat identifies how a snapshot file is protected on disk.
type snapshotFormat int

const (
	formatUnknown snapshotFormat = iota
	formatPlain
	formatAge
	formatEncV1
)

func (f snapshotFormat) String() string {
	switch f {
	case formatPlain:
		return "plaintext"
	case formatAge:
		return "age"
	case formatEncV1:
		return "enc_v1"
	default:
		return "unknown"
	}
}

// sniffFormat inspects the file header. pg_dump custom-format archives start
// with the PGDMP magic, age stanzas with the age header and the legacy in-app
// envelope is a JSON object. Anything else is treated as an unknown plaintext
// snapshot and left for pg_restore to validate.
func sniffFormat(path string) (snapshotFormat, error) {
	f, err := os.Open(path)
	if err != nil {
		return formatUnknown, err
	}
	defer func() { _ = f.Close() }()

	head := make([]byte, 32)
	n, err := io.ReadFull(f, head)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return formatUnknown, err
	}
	head = head[:n]

	switch {
	case bytes.HasPrefix(head, []byte("age-encryption.org/v1")):
		return formatAge, nil
	case len(head) > 0 && head[0] == '{':
		return formatEncV1, nil
	case bytes.HasPrefix(head, []byte("PGDMP")):
		return formatPlain, nil
	default:
		return formatPlain, nil
	}
}

// DecryptTo renders the snapshot at src to a plaintext pg_dump at dst,
// supporting age (passphrase/identity), the legacy enc_v1 envelope and
// plaintext copies. passphrase and identity may be empty depending on format.
func DecryptTo(src, dst, passphrase, identity string) error {
	var passphrases []string
	if passphrase != "" {
		passphrases = append(passphrases, passphrase)
	}
	return DecryptToMany(src, dst, passphrases, identity)
}

// DecryptToMany is DecryptTo with several passphrase candidates. The first
// successful decryption wins; identity-based Age decryption is always tried
// alongside each passphrase.
func DecryptToMany(src, dst string, passphrases []string, identity string) error {
	format, err := sniffFormat(src)
	if err != nil {
		return err
	}

	switch format {
	case formatAge:
		if len(passphrases) == 0 && identity == "" {
			return errors.New("snapshot is encrypted (age) but no passphrase or age identity was provided")
		}
		if len(passphrases) == 0 {
			return crypto.DecryptKeyed(src, dst, "", identity)
		}
		var lastErr error
		for _, passphrase := range passphrases {
			if err := crypto.DecryptKeyed(src, dst, passphrase, identity); err != nil {
				lastErr = err
				continue
			}
			return nil
		}
		return lastErr

	case formatEncV1:
		if len(passphrases) == 0 {
			return errors.New("snapshot uses the legacy enc_v1 format but no passphrase was provided")
		}
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		var lastErr error
		for _, passphrase := range passphrases {
			plain, err := crypto.DecodeProviderConfig(crypto.DeriveConfigKey(passphrase), data)
			if err != nil {
				lastErr = err
				continue
			}
			return os.WriteFile(dst, plain, 0644)
		}
		return fmt.Errorf("decrypt legacy snapshot: %w", lastErr)

	default:
		srcFile, err := os.Open(src)
		if err != nil {
			return err
		}
		defer func() { _ = srcFile.Close() }()
		dstFile, err := os.Create(dst)
		if err != nil {
			return err
		}
		defer func() { _ = dstFile.Close() }()
		if _, err := io.Copy(dstFile, srcFile); err != nil {
			return err
		}
		return nil
	}
}

// ResolveSnapshot returns the explicit snapshot when provided, otherwise the
// newest metadata snapshot in <storagePath>/metadata. Snapshots share the
// metadata-<YYYYMMDD-HHMMSS> name pattern, so lexical order is chronological.
func ResolveSnapshot(storagePath, explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}

	dir := filepath.Join(storagePath, "metadata")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("cannot read metadata snapshots: %w", err)
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), "metadata-") {
			names = append(names, e.Name())
		}
	}

	if len(names) == 0 {
		return "", fmt.Errorf("no metadata snapshot found in %s", dir)
	}

	sort.Strings(names)
	return filepath.Join(dir, names[len(names)-1]), nil
}

// ValidateDump confirms the file is a valid pg_dump archive without touching
// the database (pg_restore --list only reads the file).
func ValidateDump(path string) error {
	cmd := exec.Command("pg_restore", "--list", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("snapshot is not a valid pg_dump archive: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// dbIsEmpty reports whether the target metadata database has no user objects.
func dbIsEmpty(cfg config.DatabaseConfig) (bool, error) {
	cmd := psqlCommand(cfg, "-tAc", userObjectsQuery)
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("inspect target database %q (does it exist and accept connections?): %w", cfg.DBName, err)
	}

	count, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return false, fmt.Errorf("unexpected database inspection output: %q", string(out))
	}
	return count == 0, nil
}

// userObjectsQuery counts tables, views, matviews and sequences outside the
// system schemas. goose_version and the application tables live in user schemas.
const userObjectsQuery = "SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname NOT IN ('pg_catalog', 'information_schema') AND c.relkind IN ('r', 'p', 'v', 'm', 'S')"

// restore restores a plaintext pg_dump archive into the metadata database.
func restore(cfg config.DatabaseConfig, plainFile string, replace bool) error {
	cmd := restoreCommand(cfg, plainFile, replace)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	if !pgcompat.IsTransactionTimeoutError(string(out)) {
		return fmt.Errorf("pg_restore failed: %v: %s", err, strings.TrimSpace(string(out)))
	}
	if fallbackErr := restoreViaSQL(cfg, plainFile, replace); fallbackErr != nil {
		return fmt.Errorf("pg_restore failed: %v: %s; compatibility fallback: %w", err, strings.TrimSpace(string(out)), fallbackErr)
	}
	return nil
}

// restoreViaSQL converts a custom-format archive to SQL, filters client/server
// compatibility settings and applies it with psql. It is used when a newer
// pg_restore archive contains settings an older server does not understand.
func restoreViaSQL(cfg config.DatabaseConfig, plainFile string, replace bool) error {
	sqlFile, err := os.CreateTemp(filepath.Dir(plainFile), "metadata-restore-*.sql")
	if err != nil {
		return fmt.Errorf("create compatibility SQL file: %w", err)
	}
	sqlPath := sqlFile.Name()
	_ = sqlFile.Close()
	defer func() { _ = os.Remove(sqlPath) }()

	convertArgs := []string{"--no-owner", "--no-acl", "--exit-on-error", "-f", sqlPath}
	if replace {
		convertArgs = append(convertArgs, "--clean", "--if-exists")
	}
	convertArgs = append(convertArgs, plainFile)
	convert := exec.Command("pg_restore", convertArgs...)
	convert.Env = connEnv(cfg)
	if out, err := convert.CombinedOutput(); err != nil {
		return fmt.Errorf("convert archive to SQL: %v: %s", err, strings.TrimSpace(string(out)))
	}

	filtered, err := os.Open(sqlPath)
	if err != nil {
		return fmt.Errorf("open compatibility SQL: %w", err)
	}
	defer func() { _ = filtered.Close() }()

	apply := exec.Command("psql",
		"-X", "-w", "-v", "ON_ERROR_STOP=1", "--single-transaction",
		"-h", cfg.Host, "-p", cfg.Port, "-U", cfg.User, "-d", cfg.DBName,
	)
	apply.Env = connEnv(cfg)
	apply.Stdin = pgcompat.NewSQLFilter(filtered)
	if out, err := apply.CombinedOutput(); err != nil {
		return fmt.Errorf("psql compatibility fallback failed: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func restoreCommand(cfg config.DatabaseConfig, plainFile string, replace bool) *exec.Cmd {
	args := []string{
		"-Fc",
		"--no-owner",
		"--no-acl",
		"--exit-on-error",
		"-h", cfg.Host,
		"-p", cfg.Port,
		"-U", cfg.User,
		"-d", cfg.DBName,
	}
	if replace {
		args = append(args, "--clean", "--if-exists")
	}
	args = append(args, plainFile)

	cmd := exec.Command("pg_restore", args...)
	cmd.Env = connEnv(cfg)
	return cmd
}

func psqlCommand(cfg config.DatabaseConfig, args ...string) *exec.Cmd {
	cmd := exec.Command("psql",
		append([]string{"-h", cfg.Host, "-p", cfg.Port, "-U", cfg.User, "-d", cfg.DBName}, args...)...)
	cmd.Env = connEnv(cfg)
	return cmd
}

func connEnv(cfg config.DatabaseConfig) []string {
	return append(os.Environ(),
		fmt.Sprintf("PGPASSWORD=%s", cfg.Password),
		"PGSSLMODE="+cfg.SSLMode,
	)
}

// Run resolves, decrypts, validates and restores a metadata snapshot.
// With DryRun it stops after validation without writing to the database.
// With Replace it allows restoring over an existing database.
func Run(logger *slog.Logger, cfg config.Config, opts Options) error {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.TempDir == "" {
		return errors.New("temporary directory is required (APP_TEMP_DIR)")
	}

	src, err := ResolveSnapshot(cfg.StoragePath, opts.File)
	if err != nil {
		return err
	}
	logger.Info("Metadata snapshot selected", "source", src)

	passphrases := append([]string(nil), opts.Passphrases...)
	if opts.Passphrase != "" {
		passphrases = append([]string{opts.Passphrase}, passphrases...)
	}
	if len(passphrases) == 0 {
		if cfg.EncryptionPassphrase != "" {
			passphrases = append(passphrases, cfg.EncryptionPassphrase)
		}
		if cfg.MetadataBackupPassphrase != "" {
			passphrases = append(passphrases, cfg.MetadataBackupPassphrase)
		}
	}
	identity := opts.Identity
	if identity == "" {
		identity = cfg.AgePrivateKey
	}

	format, err := sniffFormat(src)
	if err != nil {
		return err
	}
	if format == formatAge && len(passphrases) == 0 && identity == "" {
		return errors.New("snapshot is encrypted (age); provide APP_METADATA_BACKUP_PASSPHRASE / APP_AGE_PRIVATE_KEY or --passphrase / --identity")
	}
	if format == formatEncV1 && len(passphrases) == 0 {
		return errors.New("snapshot uses the legacy encryption format; provide the metadata backup passphrase (--passphrase or APP_METADATA_BACKUP_PASSPHRASE)")
	}

	plainFile := filepath.Join(cfg.TempDir, fmt.Sprintf("metadata-restore-%d.dump", os.Getpid()))
	defer func() { _ = os.Remove(plainFile) }()

	if err := DecryptToMany(src, plainFile, passphrases, identity); err != nil {
		return fmt.Errorf("decrypt snapshot: %w", err)
	}

	if err := ValidateDump(plainFile); err != nil {
		return err
	}
	logger.Info("Snapshot validated", "source", src, "format", format.String())

	if opts.DryRun {
		logger.Info("Dry run completed: snapshot is valid, nothing was written")
		return nil
	}

	if !opts.Replace {
		empty, err := dbIsEmpty(cfg.MetadataDB)
		if err != nil {
			return err
		}
		if !empty {
			return errors.New("target database contains data; pass --replace to restore over it")
		}
	}

	if err := restore(cfg.MetadataDB, plainFile, opts.Replace); err != nil {
		return err
	}

	logger.Info("Metadata restored", "database", cfg.MetadataDB.DBName, "source", src)
	return nil
}
