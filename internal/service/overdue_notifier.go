package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/MD2SA/backup-manager/internal/monitor"
	"github.com/MD2SA/backup-manager/internal/notification"
	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/jackc/pgx/v5/pgtype"
)

type healthSummaryProvider interface {
	GetHealthSummary(ctx context.Context) (*monitor.HealthSummary, error)
}

type profileReader interface {
	GetProfile(ctx context.Context, id pgtype.UUID) (repository.ProfileFull, error)
}

type overdueSender interface {
	All(ctx context.Context, providerIDs []pgtype.UUID, event notification.Event, message string, metadata map[string]interface{})
}

// OverdueNotifier periodically checks for overdue profiles and sends one
// notification per overdue episode, repeating at most once per Repeat interval.
// It covers the case where a schedule stops firing entirely, which is invisible
// to the normal per-execution failure notifications.
type OverdueNotifier struct {
	logger   *slog.Logger
	health   healthSummaryProvider
	profiles profileReader
	sender   overdueSender
	interval time.Duration
	repeat   time.Duration

	mu       sync.Mutex
	lastSent map[string]time.Time
	cancel   context.CancelFunc
	done     chan struct{}
}

func NewOverdueNotifier(
	logger *slog.Logger,
	health healthSummaryProvider,
	profiles profileReader,
	sender overdueSender,
	interval, repeat time.Duration,
) *OverdueNotifier {
	if logger == nil {
		logger = slog.Default()
	}
	if interval < 0 {
		interval = 0
	}
	if repeat <= 0 {
		repeat = 24 * time.Hour
	}
	return &OverdueNotifier{
		logger:   logger,
		health:   health,
		profiles: profiles,
		sender:   sender,
		interval: interval,
		repeat:   repeat,
		lastSent: make(map[string]time.Time),
	}
}

func (n *OverdueNotifier) Start(ctx context.Context) {
	if n.interval <= 0 {
		n.logger.Debug("Overdue notifications disabled")
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	n.cancel = cancel
	n.done = make(chan struct{})

	go func() {
		defer close(n.done)
		ticker := time.NewTicker(n.interval)
		defer ticker.Stop()
		n.check(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n.check(ctx)
			}
		}
	}()
	n.logger.Info("Overdue notifications enabled", "interval", n.interval, "repeat", n.repeat)
}

func (n *OverdueNotifier) Stop() {
	if n.cancel == nil {
		return
	}
	n.cancel()
	<-n.done
}

func (n *OverdueNotifier) check(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()

	summary, err := n.health.GetHealthSummary(ctx)
	if err != nil {
		n.logger.Error("Overdue notification check failed", "error", err)
		return
	}

	overdue := make(map[string]monitor.ScheduledProfileHealth)
	for _, profile := range summary.ScheduledProfiles {
		if profile.Overdue {
			overdue[profile.ID] = profile
		}
	}

	// A recovered profile starts a new overdue episode and may notify again.
	n.mu.Lock()
	for id := range n.lastSent {
		if _, stillOverdue := overdue[id]; !stillOverdue {
			delete(n.lastSent, id)
		}
	}
	n.mu.Unlock()

	for id, health := range overdue {
		n.mu.Lock()
		lastSent, alreadySent := n.lastSent[id]
		n.mu.Unlock()
		if alreadySent && time.Since(lastSent) < n.repeat {
			continue
		}

		profileID, parseErr := pgutil.ParseUUID(id)
		if parseErr != nil || !profileID.Valid {
			n.logger.Warn("Overdue profile has an invalid ID", "profile_id", id)
			continue
		}
		profile, getErr := n.profiles.GetProfile(ctx, profileID)
		if getErr != nil {
			n.logger.Error("Failed to load overdue profile", "profile_id", id, "error", getErr)
			continue
		}
		if len(profile.NotificationProviderIDs) == 0 {
			continue
		}

		lastSuccess := "never"
		if health.LastSuccess != nil {
			lastSuccess = health.LastSuccess.UTC().Format(time.RFC3339)
		}
		message := fmt.Sprintf(
			"Backup overdue for profile: %s (schedule %s). Last success: %s",
			health.Name, health.Schedule, lastSuccess,
		)
		n.sender.All(ctx, profile.NotificationProviderIDs, notification.EventOverdue, message, map[string]interface{}{
			"profile_id":   health.ID,
			"profile_name": health.Name,
			"schedule":     health.Schedule,
			"last_success": lastSuccess,
		})
		n.mu.Lock()
		n.lastSent[id] = time.Now()
		n.mu.Unlock()
		n.logger.Warn("Overdue backup notification sent", "profile_id", health.ID, "profile_name", health.Name)
	}
}
