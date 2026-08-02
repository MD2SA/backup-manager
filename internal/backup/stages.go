package backup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/MD2SA/backup-manager/internal/retention"
	"github.com/MD2SA/backup-manager/internal/storage"
	"github.com/MD2SA/backup-manager/internal/verification"
)

// VerificationStage verifies backup integrity
type VerificationStage struct {
	Strategies []verification.VerificationStrategy
}

func (s *VerificationStage) Name() string { return "Verification" }

func (s *VerificationStage) Execute(ctx *ExecutionContext) error {
	return Retry(ctx.Context, 3, 1*time.Second, func() error {
		for _, strategy := range s.Strategies {
			ctx.Log(fmt.Sprintf("Running verification strategy: %s", strategy.Name()))
			ok, result, err := strategy.Verify(ctx.Context, ctx.BackupPath)
			if err != nil {
				return fmt.Errorf("Verification strategy %s failed: %w", strategy.Name(), err)
			}
			if !ok {
				return fmt.Errorf("Verification strategy %s failed: %s", strategy.Name(), result)
			}

			if strategy.Name() == "Checksum" {
				ctx.Checksum = result
			}

			ctx.Log(fmt.Sprintf("Verification strategy %s passed: %s", strategy.Name(), result))
		}
		return nil
	})
}

// RetentionStage applies the retention policy
type RetentionStage struct {
	Engine   *retention.Engine
	Repo     repository.Repository
	Provider storage.StorageProvider
}

func (s *RetentionStage) Name() string { return "Retention" }

func (s *RetentionStage) Execute(ctx *ExecutionContext) error {
	profile, err := s.Repo.GetProfile(ctx.Context, ctx.ProfileID)
	if err != nil {
		return err
	}

	if !profile.RetentionPolicyID.Valid {
		ctx.Log("No retention policy configured for this profile")
		return nil
	}

	rp, err := s.Repo.GetRetentionPolicy(ctx.Context, profile.RetentionPolicyID)
	if err != nil {
		return err
	}

	policy := retention.Policy{
		KeepHourly:  int(rp.KeepHourly),
		KeepDaily:   int(rp.KeepDaily),
		KeepWeekly:  int(rp.KeepWeekly),
		KeepMonthly: int(rp.KeepMonthly),
		KeepYearly:  int(rp.KeepYearly),
		YearlyMonth: int(rp.YearlyMonth),
	}

	return s.Engine.Apply(ctx.Context, ctx.ProfileID, policy, s.Provider)
}

// StorageStage uploads the backup to a storage provider
type StorageStage struct {
	Provider storage.StorageProvider
}

func (s *StorageStage) Name() string { return "StorageUpload" }

func (s *StorageStage) Execute(ctx *ExecutionContext) error {
	return Retry(ctx.Context, 5, 2*time.Second, func() error {
		f, err := os.Open(ctx.BackupPath)
		if err != nil {
			return err
		}
		defer f.Close()

		key := fmt.Sprintf("%s/%d-%s.sql", ctx.ProfileID, time.Now().Unix(), ctx.ExecutionID)
		err = s.Provider.Upload(ctx.Context, key, f)
		if err == nil {
			ctx.BackupPath = key
		}
		return err
	})
}

// PostgresDumpStage executes pg_dump
type PostgresDumpStage struct {
	DBName           string
	User             string
	Password         string
	Host             string
	Port             string
	CompressionType  string
	CompressionLevel int
}

func (s *PostgresDumpStage) Name() string { return "PostgresDump" }

func (s *PostgresDumpStage) Execute(ctx *ExecutionContext) error {
	ctx.BackupPath = filepath.Join(ctx.TempDir, fmt.Sprintf("backup-%s-%d.sql", ctx.ProfileID, time.Now().Unix()))

	args := []string{
		"-h", s.Host,
		"-p", s.Port,
		"-U", s.User,
		"-d", s.DBName,
		"-f", ctx.BackupPath,
	}

	if s.CompressionType == "gzip" {
		args = append(args, "-Z", fmt.Sprintf("%d", s.CompressionLevel))
	}

	return Retry(ctx.Context, 3, 5*time.Second, func() error {
		cmd := exec.CommandContext(ctx.Context, "pg_dump", args...)

		cmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", s.Password))

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("pg_dump failed: %w", err)
		}

		info, err := os.Stat(ctx.BackupPath)
		if err != nil {
			return err
		}
		ctx.Size = info.Size()
		return nil
	})
}

// CleanupStage removes temporary backup file
type CleanupStage struct{}

func (s *CleanupStage) Name() string { return "Cleanup" }

func (s *CleanupStage) Execute(ctx *ExecutionContext) error {
	if ctx.BackupPath != "" {
		return os.Remove(ctx.BackupPath)
	}
	return nil
}
