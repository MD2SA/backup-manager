package backup

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/MD2SA/backup-manager/internal/pkg/crypto"
	"github.com/MD2SA/backup-manager/internal/storage"
)

type RestorePipeline struct {
	Storage       storage.StorageProvider
	TempDir       string
	AgePrivateKey string
	DBConfig      struct {
		Host     string
		Port     string
		User     string
		Password string
		DBName   string
	}
}

func (p *RestorePipeline) Run(ctx context.Context, storagePath string, isEncrypted bool) error {
	tmpFile := filepath.Join(p.TempDir, fmt.Sprintf("restore-%d.sql", os.Getpid()))
	defer os.Remove(tmpFile)

	reader, err := p.Storage.Download(ctx, storagePath)
	if err != nil {
		return fmt.Errorf("failed to download backup: %w", err)
	}
	defer reader.Close()

	if isEncrypted {
		if p.AgePrivateKey == "" {
			return fmt.Errorf("backup is encrypted but no Age private key is configured")
		}

		// Download to a temporary encrypted file first
		encFile := tmpFile + ".age"
		defer os.Remove(encFile)

		f, err := os.Create(encFile)
		if err != nil {
			return err
		}
		if _, err := f.ReadFrom(reader); err != nil {
			f.Close()
			return err
		}
		f.Close()

		// Decrypt to tmpFile
		if err := crypto.DecryptWithAge(encFile, tmpFile, p.AgePrivateKey); err != nil {
			return fmt.Errorf("decryption failed: %w", err)
		}
	} else {
		f, err := os.Create(tmpFile)
		if err != nil {
			return err
		}
		if _, err := f.ReadFrom(reader); err != nil {
			f.Close()
			return err
		}
		f.Close()
	}

	cmd := exec.CommandContext(ctx, "psql",
		"-h", p.DBConfig.Host,
		"-p", p.DBConfig.Port,
		"-U", p.DBConfig.User,
		"-d", p.DBConfig.DBName,
		"-f", tmpFile,
	)

	cmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", p.DBConfig.Password))

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("psql restore failed: %w", err)
	}

	return nil
}

