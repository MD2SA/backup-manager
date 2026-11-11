package scheduler

import (
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testID(n byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{n}, Valid: true}
}

func TestSyncAddsMultipleJobs(t *testing.T) {
	s := New(testLogger(), nil)
	defer s.Stop()

	job1 := Job{ProfileID: testID(1), Schedule: "0 * * * *"}
	job2 := Job{ProfileID: testID(2), Schedule: "0 0 * * *"}
	if err := s.Sync([]Job{job1, job2}); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if got := s.ActiveJobs(); got != 2 {
		t.Fatalf("active jobs = %d, want 2", got)
	}
	if _, ok := s.NextRun(job1.ProfileID); !ok {
		t.Fatal("job1 has no next run")
	}
	if _, ok := s.NextRun(job2.ProfileID); !ok {
		t.Fatal("job2 has no next run")
	}
}

func TestSyncUpdatesScheduleAndRemovesDisabled(t *testing.T) {
	s := New(testLogger(), nil)
	defer s.Stop()

	if err := s.Sync([]Job{
		{ProfileID: testID(1), Schedule: "0 * * * *"},
		{ProfileID: testID(2), Schedule: "0 0 * * *"},
	}); err != nil {
		t.Fatal(err)
	}

	if err := s.Sync([]Job{{ProfileID: testID(1), Schedule: "0 0 * * *"}}); err != nil {
		t.Fatalf("Sync update: %v", err)
	}
	if got := s.ActiveJobs(); got != 1 {
		t.Fatalf("active jobs = %d, want 1", got)
	}
	if _, ok := s.NextRun(testID(2)); ok {
		t.Fatal("disabled profile should have no next run")
	}
	if _, ok := s.NextRun(testID(1)); !ok {
		t.Fatal("updated profile should be scheduled")
	}
}

func TestSyncInvalidScheduleKeepsExisting(t *testing.T) {
	s := New(testLogger(), nil)
	defer s.Stop()

	if err := s.Sync([]Job{{ProfileID: testID(1), Schedule: "0 * * * *"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Sync([]Job{{ProfileID: testID(1), Schedule: "not a cron"}}); err == nil {
		t.Fatal("expected invalid schedule error")
	}
	if got := s.ActiveJobs(); got != 1 {
		t.Fatalf("active jobs = %d, want 1", got)
	}
	if _, ok := s.NextRun(testID(1)); !ok {
		t.Fatal("existing job should remain scheduled")
	}
}

func TestRemoveAndClear(t *testing.T) {
	s := New(testLogger(), nil)
	defer s.Stop()

	if err := s.Sync([]Job{
		{ProfileID: testID(1), Schedule: "0 * * * *"},
		{ProfileID: testID(2), Schedule: "0 0 * * *"},
	}); err != nil {
		t.Fatal(err)
	}

	s.RemoveJob(testID(1))
	if got := s.ActiveJobs(); got != 1 {
		t.Fatalf("active jobs after remove = %d, want 1", got)
	}
	s.Clear()
	if got := s.ActiveJobs(); got != 0 {
		t.Fatalf("active jobs after clear = %d, want 0", got)
	}
}
