package monitor

import (
	"context"
	"testing"
	"time"

	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/jackc/pgx/v5/pgtype"
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
	return repository.ProfileFull{Profile: db.Profile{
		ID:       pgtype.UUID{Bytes: [16]byte{id}, Valid: true},
		Enabled:  enabled,
		Schedule: schedule,
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

func TestGetHealthSummary_MultipleEnabledProfilesWithoutExecutions(t *testing.T) {
	repo := &fakeRepo{
		profiles: []repository.ProfileFull{
			profile(1, true, "0 * * * *"),
			profile(2, true, "0 0 * * *"),
			profile(3, false, "0 0 * * *"),
		},
		executions: make(map[pgtype.UUID][]db.Execution),
	}
	summary, err := New(repo).GetHealthSummary(context.Background())
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
	summary, err := New(repo).GetHealthSummary(context.Background())
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
