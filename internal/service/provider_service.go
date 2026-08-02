package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/MD2SA/backup-manager/internal/config"
	"github.com/MD2SA/backup-manager/internal/notification"
	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/MD2SA/backup-manager/internal/storage"
	"github.com/MD2SA/backup-manager/internal/storage/local"
	"github.com/jackc/pgx/v5/pgtype"
)

type ProviderService struct {
	repo repository.Repository
	cfg  config.Config
}

func NewProviderService(repo repository.Repository, cfg config.Config) *ProviderService {
	return &ProviderService{
		repo: repo,
		cfg:  cfg,
	}
}

func (s *ProviderService) ResolveStorageProvider(ctx context.Context, id pgtype.UUID) (storage.StorageProvider, error) {
	if !id.Valid {
		return local.New(s.cfg.StoragePath)
	}

	sp, err := s.repo.GetStorageProvider(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get storage provider from repo: %w", err)
	}

	var configMap map[string]interface{}
	if len(sp.Config) > 0 {
		if err := json.Unmarshal(sp.Config, &configMap); err != nil {
			return nil, fmt.Errorf("failed to unmarshal storage provider config: %w", err)
		}
	}

	provider, err := storage.NewProvider(ctx, sp.Type, configMap)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage provider: %w", err)
	}

	return provider, nil
}

func (s *ProviderService) ResolveNotificationProvider(ctx context.Context, id pgtype.UUID) (notification.NotificationProvider, error) {
	if !id.Valid {
		return nil, nil
	}

	np, err := s.repo.GetNotificationProvider(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get notification provider from repo: %w", err)
	}

	var configMap map[string]interface{}
	if len(np.Config) > 0 {
		if err := json.Unmarshal(np.Config, &configMap); err != nil {
			return nil, fmt.Errorf("failed to unmarshal notification provider config: %w", err)
		}
	}

	provider, err := createNotificationProvider(np.Type, configMap)
	if err != nil {
		return nil, fmt.Errorf("failed to create notification provider: %w", err)
	}

	return provider, nil
}
