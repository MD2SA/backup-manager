package auth

import (
	"context"
	"encoding/json"

	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/MD2SA/backup-manager/internal/repository/db"
)

// AuditEvent describes a security-relevant action.
type AuditEvent struct {
	Principal    Principal
	Action       string
	ResourceType string
	ResourceID   string
	RequestID    string
	IP           string
	Metadata     map[string]any
}

func (s *Service) RecordAudit(ctx context.Context, event AuditEvent) error {
	metadata := event.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return err
	}

	_, err = s.repo.CreateAuditEntry(ctx, db.CreateAuditEntryParams{
		UserID:       event.Principal.UserID,
		ApiKeyID:     event.Principal.KeyID,
		Action:       event.Action,
		ResourceType: pgutil.ToText(event.ResourceType),
		ResourceID:   pgutil.ToText(event.ResourceID),
		RequestID:    pgutil.ToText(event.RequestID),
		Ip:           pgutil.ToText(event.IP),
		Metadata:     encoded,
	})
	return err
}

func (s *Service) ListAuditEntries(ctx context.Context, limit, offset int32) ([]db.AuditLog, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.ListAuditEntries(ctx, db.ListAuditEntriesParams{Limit: limit, Offset: offset})
}
