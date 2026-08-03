package backup

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type ExecutionStatus string

const (
	StatusPending ExecutionStatus = "pending"
	StatusRunning ExecutionStatus = "running"
	StatusSuccess ExecutionStatus = "success"
	StatusFailed  ExecutionStatus = "failed"
)

type ExecutionContext struct {
	Context     context.Context
	ExecutionID pgtype.UUID
	ProfileID   pgtype.UUID
	ProfileName string // Slugified profile name for storage organization
	IsEncrypted bool   // Flag to indicate if the backup was encrypted

	// Pipeline state
	StartTime time.Time
	EndTime   time.Time

	BackupPath string // Resulting storage key
	LocalPath  string // Temporary local file path
	TempDir    string
	Checksum   string
	Size       int64

	Logs  []string
	Error error
}

func (c *ExecutionContext) Log(msg string) {
	c.Logs = append(c.Logs, time.Now().Format(time.RFC3339)+": "+msg)
}

type Stage interface {
	Name() string
	Execute(ctx *ExecutionContext) error
}
