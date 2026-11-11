package monitor

import (
	"context"
	"testing"
	"time"

	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/robfig/cron/v3"
)

type fakeRepo struct {
	repository.Repository
	profiles   []repository.ProfileFull
	executions map[pgtype.UUID][]db.Execution
}

func (f *fakeRepo) ListProfiles(context.Context) ([]repository.ProfileFull, error) {
	return f.profiles, nil
}

func (f *fakeRepo) ListExecutionsByProfile(_ context.Context, id pgtype.UUID) ([]db.Execution, error) {
	return f.executions[id], nil
}

func profile(id byte, enabled bool, schedule string) repository.ProfileFull {
	return profileWithSince(id, enabled, schedule, time.Time{})
}

func profileWithSince(id byte, enabled bool, schedule string, since time.Time) repository.ProfileFull {
	return repository.ProfileFull{Profile: db.Profile{
		ID:             pgtype.UUID{Bytes: [16]byte{id}, Valid: true},
		Enabled:        enabled,
		Schedule:       schedule,
		CreatedAt:      pgtype.Timestamptz{Time: time.Now().Add(-365 * 24 * time.Hour), Valid: true},
		ScheduledSince: pgtype.Timestamptz{Time: since, Valid: !since.IsZero()},
	}}
}

func execution(id byte, status string, at time.Time, size int64) db.Execution {
	return db.Execution{
		ID:        pgtype.UUID{Bytes: [16]byte{id}, Valid: true},
		Status:    status,
		StartTime: pgtype.Timestamptz{Time: at, Valid: true},
		CreatedAt: pgtype.Timestamptz{Time: at, Valid: true},
		Size:      pgtype.Int8{Int64: size, Valid: true},
	}
}

func TestGetHealthSummary_OverdueRules(t *testing.T) {
	now := time.Now()
	old := now.Add(-5 * time.Hour)

	t.Run("old profile without success is overdue", func(t *testing.T) {
		repo := &fakeRepo{
			profiles:   []repository.ProfileFull{profileWithSince(1, true, "0 * * * *", old)},
			executions: make(map[pgtype.UUID][]db.Execution),
		}
		summary, err := New(repo, 2, 15*time.Minute).GetHealthSummary(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if summary.OverdueProfiles != 1 || summary.Status != StatusCritical {
			t.Fatalf("overdue=%d status=%s", summary.OverdueProfiles, summary.Status)
		}
	})

	t.Run("recently enabled profile is not overdue", func(t *testing.T) {
		repo := &fakeRepo{
			profiles:   []repository.ProfileFull{profileWithSince(1, true, "0 * * * *", now.Add(-10*time.Minute))},
			executions: make(map[pgtype.UUID][]db.Execution),
		}
		summary, err := New(repo, 2, 15*time.Minute).GetHealthSummary(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if summary.OverdueProfiles != 0 {
			t.Fatalf("overdue=%d, want 0", summary.OverdueProfiles)
		}
	})

	t.Run("recent success is not overdue", func(t *testing.T) {
		p := profileWithSince(1, true, "0 * * * *", old)
		repo := &fakeRepo{
			profiles: []repository.ProfileFull{p},
			executions: map[pgtype.UUID][]db.Execution{
				p.ID: {execution(2, "success", now.Add(-5*time.Minute), 10)},
			},
		}
		summary, err := New(repo, 2, 15*time.Minute).GetHealthSummary(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if summary.OverdueProfiles != 0 {
			t.Fatalf("overdue=%d, want 0", summary.OverdueProfiles)
		}
	})

	t.Run("disabled profile is ignored", func(t *testing.T) {
		repo := &fakeRepo{
			profiles:   []repository.ProfileFull{profileWithSince(1, false, "0 * * * *", old)},
			executions: make(map[pgtype.UUID][]db.Execution),
		}
		summary, err := New(repo, 2, 15*time.Minute).GetHealthSummary(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if summary.ActiveProfiles != 0 || summary.OverdueProfiles != 0 {
			t.Fatalf("active=%d overdue=%d", summary.ActiveProfiles, summary.OverdueProfiles)
		}
	})
}

func TestMaxGap(t *testing.T) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	now := time.Now()
	for _, tc := range []struct {
		schedule string
		min      time.Duration
		max      time.Duration
	}{
		{"*/15 * * * *", 14 * time.Minute, 16 * time.Minute},
		{"0 * * * *", 59 * time.Minute, 61 * time.Minute},
		{"0 0 * * *", 23 * time.Hour, 25 * time.Hour},
	} {
		schedule, err := parser.Parse(tc.schedule)
		if err != nil {
			t.Fatal(err)
		}
		gap, ok := maxGap(schedule, now)
		if !ok || gap < tc.min || gap > tc.max {
			t.Errorf("%s: gap=%v ok=%v, want between %v and %v", tc.schedule, gap, ok, tc.min, tc.max)
		}
	}
}

func TestGetHealthSummary_MultipleEnabledProfilesWithoutExecutions(t *testing.T) {
	repo := &fakeRepo{
		profiles: []repository.ProfileFull{
			profile(1, true, "0 * * * *"),
			profile(2, true, "0 0 * * *"),
			profile(3, false, "0 0 * * *"),
		},
		executions: make(map[pgtype.UUID][]db.Execution),
	}
	summary, err := New(repo, 2, 15*time.Minute).GetHealthSummary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if summary.ActiveProfiles != 2 {
		t.Fatalf("active profiles = %d, want 2", summary.ActiveProfiles)
	}
	if summary.NextScheduled == nil {
		t.Fatal("next scheduled must be computed even without executions")
	}
}

func TestGetHealthSummary_AggregatesLatestStatus(t *testing.T) {
	older := time.Now().Add(-2 * time.Hour)
	newer := time.Now().Add(-time.Hour)
	p1 := profile(1, true, "0 * * * *")
	p2 := profile(2, true, "0 0 * * *")
	repo := &fakeRepo{
		profiles: []repository.ProfileFull{p1, p2},
		executions: map[pgtype.UUID][]db.Execution{
			p1.ID: []db.Execution{execution(10, "success", older, 100)},
			p2.ID: []db.Execution{execution(11, "failed", newer, 200)},
		},
	}
	summary, err := New(repo, 2, 15*time.Minute).GetHealthSummary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if summary.Status != StatusWarning {
		t.Fatalf("status = %s, want warning", summary.Status)
	}
	if summary.LastExecutionStatus != "failed" {
		t.Fatalf("last status = %q, want failed", summary.LastExecutionStatus)
	}
	if summary.LastSuccess == nil {
		t.Fatal("last success should be set")
	}
	if summary.TotalStorageUsed != 300 {
		t.Fatalf("storage = %d, want 300", summary.TotalStorageUsed)
	}
}
