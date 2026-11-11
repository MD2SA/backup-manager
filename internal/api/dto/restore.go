package dto

import (
	"encoding/json"
	"time"

	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/MD2SA/backup-manager/internal/pkg/validator"
	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/jackc/pgx/v5/pgtype"
)

type RestoreRequest struct {
	Confirm              bool   `json:"confirm"`
	VerifyOnly           bool   `json:"verify_only"`
	ProviderID           string `json:"provider_id,omitempty" validate:"omitempty,uuid4"`
	TerminateConnections bool   `json:"terminate_connections"`
}

func (r *RestoreRequest) Validate() error {
	return validator.Get().Struct(r)
}

type RestoreResponse struct {
	ID                string     `json:"id"`
	ExecutionID       string     `json:"execution_id"`
	Status            string     `json:"status"`
	RequestID         string     `json:"request_id,omitempty"`
	ProviderID        string     `json:"provider_id,omitempty"`
	StoragePath       string     `json:"storage_path,omitempty"`
	TargetDB          string     `json:"target_db,omitempty"`
	RequestedByUserID string     `json:"requested_by_user_id,omitempty"`
	RequestedByKeyID  string     `json:"requested_by_key_id,omitempty"`
	ChecksumVerified  bool       `json:"checksum_verified"`
	StartedAt         *time.Time `json:"started_at"`
	FinishedAt        *time.Time `json:"finished_at"`
	Duration          string     `json:"duration"`
	ErrorMessage      string     `json:"error_message,omitempty"`
	Logs              any        `json:"logs" swaggertype:"array,string"`
	CreatedAt         time.Time  `json:"created_at"`
}

func ToRestoreResponse(r db.Restore) RestoreResponse {
	var logs any
	if len(r.Logs) > 0 {
		_ = json.Unmarshal(r.Logs, &logs)
	}

	return RestoreResponse{
		ID:                pgutil.UUIDToString(r.ID),
		ExecutionID:       pgutil.UUIDToString(r.ExecutionID),
		Status:            r.Status,
		RequestID:         r.RequestID.String,
		ProviderID:        pgutil.UUIDToString(r.ProviderID),
		StoragePath:       r.StoragePath.String,
		TargetDB:          r.TargetDb.String,
		RequestedByUserID: pgutil.UUIDToString(r.RequestedByUserID),
		RequestedByKeyID:  pgutil.UUIDToString(r.RequestedByKeyID),
		ChecksumVerified:  r.ChecksumVerified,
		StartedAt:         timestamptzPtr(r.StartedAt),
		FinishedAt:        timestamptzPtr(r.FinishedAt),
		Duration:          pgutil.IntervalToString(r.Duration),
		ErrorMessage:      r.ErrorMessage.String,
		Logs:              logs,
		CreatedAt:         r.CreatedAt.Time,
	}
}

func timestamptzPtr(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	value := ts.Time
	return &value
}
