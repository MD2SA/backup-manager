package dto

import (
	"errors"
	"time"

	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/MD2SA/backup-manager/internal/pkg/validator"
	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/robfig/cron/v3"
)

type ProfileRequest struct {
	Name                    string   `json:"name" validate:"required"`
	Description             string   `json:"description"`
	Enabled                 bool     `json:"enabled"`
	Schedule                string   `json:"schedule" validate:"required"`
	StorageProviderIDs      []string `json:"storage_provider_ids" validate:"required,min=1,dive,uuid4"`
	NotificationProviderIDs []string `json:"notification_provider_ids" validate:"omitempty,dive,uuid4"`
	RetentionPolicyID       string   `json:"retention_policy_id" validate:"required,uuid4"`
	CompressionType         string   `json:"compression_type" validate:"required,oneof=none gzip"`
	CompressionLevel        int32    `json:"compression_level" validate:"min=0,max=9"`
}

func (r *ProfileRequest) Validate() error {
	v := validator.Get()
	if err := v.Struct(r); err != nil {
		return err
	}

	if _, err := cron.ParseStandard(r.Schedule); err != nil {
		return errors.New("invalid cron schedule format")
	}

	return nil
}

type ProfileResponse struct {
	ID                      string    `json:"id"`
	Name                    string    `json:"name"`
	Description             string    `json:"description"`
	Enabled                 bool      `json:"enabled"`
	Schedule                string    `json:"schedule"`
	StorageProviderIDs      []string  `json:"storage_provider_ids"`
	NotificationProviderIDs []string  `json:"notification_provider_ids"`
	RetentionPolicyID       string    `json:"retention_policy_id"`
	CompressionType         string    `json:"compression_type"`
	CompressionLevel        int32     `json:"compression_level"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

func ToProfileResponse(p db.Profile, storageIDs []string, notificationIDs []string) ProfileResponse {
	return ProfileResponse{
		ID:                      pgutil.UUIDToString(p.ID),
		Name:                    p.Name,
		Description:             p.Description.String,
		Enabled:                 p.Enabled,
		Schedule:                p.Schedule,
		StorageProviderIDs:      storageIDs,
		NotificationProviderIDs: notificationIDs,
		RetentionPolicyID:       pgutil.UUIDToString(p.RetentionPolicyID),
		CompressionType:         p.CompressionType,
		CompressionLevel:        p.CompressionLevel,
		CreatedAt:               p.CreatedAt.Time,
		UpdatedAt:               p.UpdatedAt.Time,
	}
}
