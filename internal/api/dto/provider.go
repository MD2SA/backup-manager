package dto

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/MD2SA/backup-manager/internal/pkg/validator"
	"github.com/MD2SA/backup-manager/internal/repository/db"
)

type StorageProviderRequest struct {
	Name   string          `json:"name" validate:"required"`
	Type   string          `json:"type" validate:"required,oneof=local s3"`
	Config json.RawMessage `json:"config" validate:"required" swaggertype:"object"`
}

func (r *StorageProviderRequest) Validate() error {
	v := validator.Get()
	if err := v.Struct(r); err != nil {
		return err
	}

	switch r.Type {
	case "local":
		var cfg LocalConfig
		if err := json.Unmarshal(r.Config, &cfg); err != nil {
			return fmt.Errorf("invalid local storage config: %w", err)
		}
		return v.Struct(cfg)
	case "s3":
		var cfg S3Config
		if err := json.Unmarshal(r.Config, &cfg); err != nil {
			return fmt.Errorf("invalid s3 storage config: %w", err)
		}
		return v.Struct(cfg)
	default:
		return fmt.Errorf("unsupported storage type: %s", r.Type)
	}
}

type StorageProviderResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Config    any       `json:"config" swaggertype:"object"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func ToStorageProviderResponse(p db.StorageProvider) StorageProviderResponse {
	var config any
	_ = json.Unmarshal(p.Config, &config)
	return StorageProviderResponse{
		ID:        pgutil.UUIDToString(p.ID),
		Name:      p.Name,
		Type:      p.Type,
		Config:    config,
		CreatedAt: p.CreatedAt.Time,
		UpdatedAt: p.UpdatedAt.Time,
	}
}

type NotificationProviderRequest struct {
	Name   string          `json:"name" validate:"required"`
	Type   string          `json:"type" validate:"required,oneof=discord"`
	Config json.RawMessage `json:"config" validate:"required" swaggertype:"object"`
}

func (r *NotificationProviderRequest) Validate() error {
	v := validator.Get()
	if err := v.Struct(r); err != nil {
		return err
	}

	switch r.Type {
	case "discord":
		var cfg DiscordConfig
		if err := json.Unmarshal(r.Config, &cfg); err != nil {
			return fmt.Errorf("invalid discord notification config: %w", err)
		}
		return v.Struct(cfg)
	default:
		return fmt.Errorf("unsupported notification type: %s", r.Type)
	}
}

type NotificationProviderResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Config    any       `json:"config" swaggertype:"object"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func ToNotificationProviderResponse(p db.NotificationProvider) NotificationProviderResponse {
	var config any
	_ = json.Unmarshal(p.Config, &config)
	return NotificationProviderResponse{
		ID:        pgutil.UUIDToString(p.ID),
		Name:      p.Name,
		Type:      p.Type,
		Config:    config,
		CreatedAt: p.CreatedAt.Time,
		UpdatedAt: p.UpdatedAt.Time,
	}
}
