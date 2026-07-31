package backup

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/MD2SA/backup-manager/internal/storage"
)

type RestorePipeline struct {
	Storage  storage.StorageProvider
	DBConfig struct {
		Host     string
		Port     string
		User     string
		Password string
		DBName   string
	}
}

func (p *RestorePipeline) Run(ctx context.Context, storagePath string) error {
	tmpFile := fmt.Sprintf("/tmp/restore-%d.sql", os.Getpid())
	defer os.Remove(tmpFile)

	reader, err := p.Storage.Download(ctx, storagePath)
	if err != nil {
		return fmt.Errorf("failed to download backup: %w", err)
	}
	defer reader.Close()

	f, err := os.Create(tmpFile)
	if err != nil {
		return err
	}

	if _, err := f.ReadFrom(reader); err != nil {
		f.Close()
		return err
	}
	f.Close()

	cmd := exec.CommandContext(ctx, "psql",
		"-h", p.DBConfig.Host,
		"-p", p.DBConfig.Port,
		"-U", p.DBConfig.User,
		"-d", p.DBConfig.DBName,
		"-f", tmpFile,
	)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("psql restore failed: %w", err)
	}

	return nil
}
