package service

import (
	"context"
	"log/slog"

	"github.com/MD2SA/backup-manager/internal/notification"
	"github.com/jackc/pgx/v5/pgtype"
)

// Notifier resolves notification providers and sends events. It is shared by
// backup and restore flows so both report through the profile's channels.
type Notifier struct {
	providerService *ProviderService
	logger          *slog.Logger
}

func NewNotifier(providerService *ProviderService, logger *slog.Logger) *Notifier {
	return &Notifier{providerService: providerService, logger: logger}
}

func (n *Notifier) All(ctx context.Context, providerIDs []pgtype.UUID, event notification.Event, message string, metadata map[string]interface{}) {
	for _, id := range providerIDs {
		n.One(ctx, id, event, message, metadata)
	}
}

func (n *Notifier) One(ctx context.Context, providerID pgtype.UUID, event notification.Event, message string, metadata map[string]interface{}) {
	if !providerID.Valid {
		return
	}

	provider, err := n.providerService.ResolveNotificationProvider(ctx, providerID)
	if err != nil || provider == nil {
		n.logger.Error("Failed to resolve notification provider", "error", err)
		return
	}

	if err := provider.Send(ctx, event, message, metadata); err != nil {
		n.logger.Error("Failed to send notification", "error", err)
	}
}
