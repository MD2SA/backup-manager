package dto

import (
	"encoding/json"
	"time"

	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/MD2SA/backup-manager/internal/pkg/validator"
	"github.com/MD2SA/backup-manager/internal/repository/db"
)

type UserResponse struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func ToUserResponse(user db.User) UserResponse {
	return UserResponse{
		ID:        pgutil.UUIDToString(user.ID),
		Email:     user.Email,
		Name:      user.Name,
		Active:    user.Active,
		CreatedAt: user.CreatedAt.Time,
		UpdatedAt: user.UpdatedAt.Time,
	}
}

type CreateUserRequest struct {
	Email string `json:"email" validate:"required,email"`
	Name  string `json:"name" validate:"required"`
}

func (r *CreateUserRequest) Validate() error {
	return validator.Get().Struct(r)
}

type UpdateUserRequest struct {
	Email  string `json:"email" validate:"required,email"`
	Name   string `json:"name" validate:"required"`
	Active *bool  `json:"active"`
}

func (r *UpdateUserRequest) Validate() error {
	return validator.Get().Struct(r)
}

type APIKeyResponse struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	ExpiresAt  *time.Time `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

func ToAPIKeyResponse(key db.ApiKey) APIKeyResponse {
	return APIKeyResponse{
		ID:         pgutil.UUIDToString(key.ID),
		UserID:     pgutil.UUIDToString(key.UserID),
		Name:       key.Name,
		Prefix:     key.KeyPrefix,
		ExpiresAt:  timestamptzPtr(key.ExpiresAt),
		RevokedAt:  timestamptzPtr(key.RevokedAt),
		LastUsedAt: timestamptzPtr(key.LastUsedAt),
		CreatedAt:  key.CreatedAt.Time,
	}
}

type CreateAPIKeyRequest struct {
	Name          string `json:"name" validate:"required"`
	ExpiresInDays *int   `json:"expires_in_days" validate:"omitempty,min=1,max=3650"`
}

func (r *CreateAPIKeyRequest) Validate() error {
	return validator.Get().Struct(r)
}

type CreateAPIKeyResponse struct {
	APIKeyResponse
	Key string `json:"key"`
}

type MeResponse struct {
	User      UserResponse `json:"user"`
	KeyID     string       `json:"key_id,omitempty"`
	KeyPrefix string       `json:"key_prefix,omitempty"`
	Insecure  bool         `json:"insecure"`
}

type AuditEntryResponse struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id,omitempty"`
	APIKeyID     string    `json:"api_key_id,omitempty"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type,omitempty"`
	ResourceID   string    `json:"resource_id,omitempty"`
	RequestID    string    `json:"request_id,omitempty"`
	IP           string    `json:"ip,omitempty"`
	Metadata     any       `json:"metadata" swaggertype:"object"`
	CreatedAt    time.Time `json:"created_at"`
}

func ToAuditEntryResponse(entry db.AuditLog) AuditEntryResponse {
	var metadata any
	if len(entry.Metadata) > 0 {
		_ = json.Unmarshal(entry.Metadata, &metadata)
	}
	return AuditEntryResponse{
		ID:           pgutil.UUIDToString(entry.ID),
		UserID:       pgutil.UUIDToString(entry.UserID),
		APIKeyID:     pgutil.UUIDToString(entry.ApiKeyID),
		Action:       entry.Action,
		ResourceType: entry.ResourceType.String,
		ResourceID:   entry.ResourceID.String,
		RequestID:    entry.RequestID.String,
		IP:           entry.Ip.String,
		Metadata:     metadata,
		CreatedAt:    entry.CreatedAt.Time,
	}
}
