package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/MD2SA/backup-manager/internal/monitor"
	"github.com/MD2SA/backup-manager/internal/notification"
	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/jackc/pgx/v5/pgtype"
)

type fakeHealth struct {
	summary *monitor.HealthSummary
	err     error
}

func (f *fakeHealth) GetHealthSummary(context.Context) (*monitor.HealthSummary, error) {
	return f.summary, f.err
}

type fakeProfiles struct {
	profiles map[string]repository.ProfileFull
}

func (f *fakeProfiles) GetProfile(_ context.Context, id pgtype.UUID) (repository.ProfileFull, error) {
	profile, ok := f.profiles[pgutil.UUIDToString(id)]
	if !ok {
		return repository.ProfileFull{}, errors.New("not found")
	}
	return profile, nil
}

type sentNotification struct {
	providerIDs []pgtype.UUID
	event       notification.Event
	message     string
	metadata    map[string]interface{}
}

type fakeSender struct {
	sent []sentNotification
}

func (f *fakeSender) All(_ context.Context, providerIDs []pgtype.UUID, event notification.Event, message string, metadata map[string]interface{}) {
	f.sent = append(f.sent, sentNotification{providerIDs: providerIDs, event: event, message: message, metadata: metadata})
}

func TestOverdueNotifier_SendsOncePerEpisode(t *testing.T) {
	profileID := pgutil.NewUUID()
	providerID := pgutil.NewUUID()
	id := pgutil.UUIDToString(profileID)
	lastSuccess := time.Now().Add(-2 * time.Hour)

	health := &fakeHealth{summary: &monitor.HealthSummary{
		ScheduledProfiles: []monitor.ScheduledProfileHealth{{
			ID:          id,
			Name:        "nightly",
			Schedule:    "0 0 * * *",
			LastSuccess: &lastSuccess,
			Overdue:     true,
		}},
	}}
	profiles := &fakeProfiles{profiles: map[string]repository.ProfileFull{
		id: {
			Profile:                 db.Profile{ID: profileID, Name: "nightly"},
			NotificationProviderIDs: []pgtype.UUID{providerID},
		},
	}}
	sender := &fakeSender{}
	notifier := NewOverdueNotifier(slog.New(slog.NewTextHandler(io.Discard, nil)), health, profiles, sender, time.Minute, 24*time.Hour)

	notifier.check(context.Background())
	notifier.check(context.Background())
	if len(sender.sent) != 1 {
		t.Fatalf("sent %d notifications within the repeat window, want 1", len(sender.sent))
	}
	if sender.sent[0].event != notification.EventOverdue {
		t.Fatalf("unexpected event: %s", sender.sent[0].event)
	}
	if sender.sent[0].message == "" || sender.sent[0].metadata["profile_name"] != "nightly" {
		t.Fatalf("unexpected notification: %+v", sender.sent[0])
	}

	// Recovery clears the episode; a later overdue state notifies again.
	health.summary.ScheduledProfiles[0].Overdue = false
	notifier.check(context.Background())
	health.summary.ScheduledProfiles[0].Overdue = true
	notifier.check(context.Background())
	if len(sender.sent) != 2 {
		t.Fatalf("sent %d notifications after a new episode, want 2", len(sender.sent))
	}
}

func TestOverdueNotifier_SkipsProfilesWithoutProviders(t *testing.T) {
	profileID := pgutil.NewUUID()
	id := pgutil.UUIDToString(profileID)
	health := &fakeHealth{summary: &monitor.HealthSummary{
		ScheduledProfiles: []monitor.ScheduledProfileHealth{{
			ID: id, Name: "no-provider", Schedule: "0 0 * * *", Overdue: true,
		}},
	}}
	profiles := &fakeProfiles{profiles: map[string]repository.ProfileFull{
		id: {Profile: db.Profile{ID: profileID, Name: "no-provider"}},
	}}
	sender := &fakeSender{}
	notifier := NewOverdueNotifier(nil, health, profiles, sender, time.Minute, time.Hour)
	notifier.check(context.Background())
	if len(sender.sent) != 0 {
		t.Fatalf("expected no notifications, got %d", len(sender.sent))
	}
}

func TestOverdueNotifier_HealthFailure(t *testing.T) {
	health := &fakeHealth{err: errors.New("database down")}
	sender := &fakeSender{}
	notifier := NewOverdueNotifier(nil, health, &fakeProfiles{}, sender, time.Minute, time.Hour)
	notifier.check(context.Background())
	if len(sender.sent) != 0 {
		t.Fatalf("expected no notifications, got %d", len(sender.sent))
	}
}

func TestOverdueNotifier_InvalidProfileID(t *testing.T) {
	health := &fakeHealth{summary: &monitor.HealthSummary{
		ScheduledProfiles: []monitor.ScheduledProfileHealth{{
			ID: "not-a-uuid", Name: "bad", Schedule: "0 0 * * *", Overdue: true,
		}},
	}}
	sender := &fakeSender{}
	notifier := NewOverdueNotifier(nil, health, &fakeProfiles{}, sender, time.Minute, time.Hour)
	notifier.check(context.Background())
	if len(sender.sent) != 0 {
		t.Fatalf("expected no notifications, got %d", len(sender.sent))
	}
}
