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
	AgePassphrase string
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
	defer func() { _ = os.Remove(tmpFile) }()

	reader, err := p.Storage.Download(ctx, storagePath)
	if err != nil {
		return fmt.Errorf("failed to download backup: %w", err)
	}
	defer func() { _ = reader.Close() }()

	if isEncrypted {
		// Download to a temporary encrypted file first
		encFile := tmpFile + ".age"
		defer func() { _ = os.Remove(encFile) }()

		f, err := os.Create(encFile)
		if err != nil {
			return err
		}
		if _, err := f.ReadFrom(reader); err != nil {
			_ = f.Close()
			return err
		}
		_ = f.Close()

		// Attempt decryption
		if p.AgePassphrase != "" {
			if err := crypto.DecryptWithPassphrase(encFile, tmpFile, p.AgePassphrase); err == nil {
				goto restored
			}
		}

		if p.AgePrivateKey != "" {
			if err := crypto.DecryptWithAge(encFile, tmpFile, p.AgePrivateKey); err == nil {
				goto restored
			}
		}

		return fmt.Errorf("decryption failed: no valid passphrase or private key provided for encrypted backup")
	} else {
		f, err := os.Create(tmpFile)
		if err != nil {
			return err
		}
		if _, err := f.ReadFrom(reader); err != nil {
			_ = f.Close()
			return err
		}
		_ = f.Close()
	}

restored:
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
