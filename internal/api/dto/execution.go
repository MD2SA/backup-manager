package dto

import (
	"encoding/json"
	"time"

	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/MD2SA/backup-manager/internal/repository/db"
)

type ExecutionResponse struct {
	ID           string    `json:"id"`
	ProfileID    string    `json:"profile_id"`
	Status       string    `json:"status"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	Duration     string    `json:"duration"`
	Size         int64     `json:"size"`
	Checksum     string    `json:"checksum"`
	StoragePath  string    `json:"storage_path"`
	Logs         any       `json:"logs" swaggertype:"array,string"`
	ErrorMessage string    `json:"error_message"`
	IsPinned     bool      `json:"is_pinned"`
	CreatedAt    time.Time `json:"created_at"`
}

func ToExecutionResponse(e db.Execution) ExecutionResponse {
	var logs any
	if len(e.Logs) > 0 {
		_ = json.Unmarshal(e.Logs, &logs)
	}

	return ExecutionResponse{
		ID:           pgutil.UUIDToString(e.ID),
		ProfileID:    pgutil.UUIDToString(e.ProfileID),
		Status:       e.Status,
		StartTime:    e.StartTime.Time,
		EndTime:      e.EndTime.Time,
		Duration:     pgutil.IntervalToString(e.Duration),
		Size:         e.Size.Int64,
		Checksum:     e.Checksum.String,
		StoragePath:  e.StoragePath.String,
		Logs:         logs,
		ErrorMessage: e.ErrorMessage.String,
		IsPinned:     e.IsPinned,
		CreatedAt:    e.CreatedAt.Time,
	}
}

type PinRequest struct {
	Pinned bool `json:"pinned"`
}
