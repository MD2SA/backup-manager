package selfbackup

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/MD2SA/backup-manager/internal/config"
	"github.com/MD2SA/backup-manager/internal/pkg/crypto"
	"github.com/robfig/cron/v3"
)

type Service struct {
	logger *slog.Logger
	cfg    config.Config
	cron   *cron.Cron
}

func New(logger *slog.Logger, cfg config.Config) *Service {
	return &Service{
		logger: logger,
		cfg:    cfg,
		cron:   cron.New(),
	}
}

func (s *Service) Start() {
	if s.cfg.MetadataBackupSchedule == "" {
		s.logger.Debug("Metadata backup disabled (APP_METADATA_BACKUP_SCHEDULE not set)")
		return
	}

	if _, err := s.cron.AddFunc(s.cfg.MetadataBackupSchedule, s.run); err != nil {
		s.logger.Error("Invalid metadata backup schedule", "schedule", s.cfg.MetadataBackupSchedule, "error", err)
		return
	}

	s.logger.Info("Metadata backup enabled", "schedule", s.cfg.MetadataBackupSchedule, "retention", s.cfg.MetadataBackupRetention)
	s.cron.Start()
}

func (s *Service) Stop() {
	s.cron.Stop()
}

func (s *Service) run() {
	s.logger.Info("Metadata backup started")

	if err := s.dump(); err != nil {
		s.logger.Error("Metadata backup failed", "error", err)
		return
	}

	if err := s.prune(); err != nil {
		s.logger.Warn("Metadata backup prune failed", "error", err)
	}

	s.logger.Info("Metadata backup completed")
}

func (s *Service) dump() error {
	dir := filepath.Join(s.cfg.StoragePath, "metadata")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create metadata backup dir: %w", err)
	}

	ts := time.Now().Format("20060102-150405")
	dumpFile := filepath.Join(dir, fmt.Sprintf("metadata-%s.dump", ts))

	if err := DumpFile(s.cfg.MetadataDB, dumpFile); err != nil {
		return err
	}

	if s.cfg.MetadataBackupPassphrase != "" {
		encrypted := dumpFile + ".age"
		if err := crypto.EncryptWithPassphrase(dumpFile, encrypted, s.cfg.MetadataBackupPassphrase); err != nil {
			return fmt.Errorf("encrypt metadata backup: %w", err)
		}
		_ = os.Remove(dumpFile)
		s.logger.Info("Metadata backup encrypted", "path", encrypted)
	}

	return nil
}

// DumpFile dumps the metadata database to destPath using pg_dump (custom format).
// It is shared by the scheduled local self-backup and the per-execution embedding
// into the profile's storage providers so both use the exact same implementation.
func DumpFile(mdb config.DatabaseConfig, destPath string) error {
	if err := exec.Command("pg_dump", "--version").Run(); err != nil {
		return fmt.Errorf("pg_dump not found in PATH: %w", err)
	}

	args := []string{
		"--no-owner",
		"--no-acl",
		"-h", mdb.Host,
		"-p", mdb.Port,
		"-U", mdb.User,
		"-d", mdb.DBName,
		"-Fc",
		"-f", destPath,
	}

	cmd := exec.Command("pg_dump", args...)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("PGPASSWORD=%s", mdb.Password),
		"PGSSLMODE="+mdb.SSLMode,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pg_dump failed: %w", err)
	}

	return nil
}

func (s *Service) prune() error {
	dir := filepath.Join(s.cfg.StoragePath, "metadata")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	type named struct {
		name    string
		modTime time.Time
	}

	var dumps []named
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "metadata-") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		dumps = append(dumps, named{name: name, modTime: info.ModTime()})
	}

	if len(dumps) <= s.cfg.MetadataBackupRetention {
		return nil
	}

	sort.Slice(dumps, func(i, j int) bool {
		return dumps[i].modTime.After(dumps[j].modTime)
	})

	for _, d := range dumps[s.cfg.MetadataBackupRetention:] {
		if err := os.Remove(filepath.Join(dir, d.name)); err != nil {
			s.logger.Warn("Failed to remove old metadata backup", "file", d.name, "error", err)
		}
	}

	return nil
}
