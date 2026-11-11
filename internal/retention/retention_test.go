package retention

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/MD2SA/backup-manager/internal/storage"
	"github.com/jackc/pgx/v5/pgtype"
)

type fakeRepo struct {
	executions []db.Execution
	storage    map[pgtype.UUID][]db.ExecutionStorage
}

func newFakeRepo(executions []db.Execution) *fakeRepo {
	return &fakeRepo{executions: executions, storage: make(map[pgtype.UUID][]db.ExecutionStorage)}
}

func (f *fakeRepo) ListExecutionsByProfileForRetention(context.Context, pgtype.UUID) ([]db.Execution, error) {
	return f.executions, nil
}

func (f *fakeRepo) ListExecutionStorage(_ context.Context, executionID pgtype.UUID) ([]db.ExecutionStorage, error) {
	return f.storage[executionID], nil
}

func (f *fakeRepo) DeleteExecutionStorage(_ context.Context, arg db.DeleteExecutionStorageParams) error {
	records := f.storage[arg.ExecutionID]
	out := records[:0]
	for _, record := range records {
		if record.ProviderID != arg.ProviderID {
			out = append(out, record)
		}
	}
	f.storage[arg.ExecutionID] = out
	return nil
}

func (f *fakeRepo) DeleteExecutionIfNoStorage(_ context.Context, id pgtype.UUID) (int64, error) {
	if len(f.storage[id]) > 0 {
		return 0, nil
	}
	out := f.executions[:0]
	var deleted int64
	for _, execution := range f.executions {
		if execution.ID == id {
			deleted++
			continue
		}
		out = append(out, execution)
	}
	f.executions = out
	return deleted, nil
}

type fakeProvider struct {
	deleted []string
	fail    bool
}

func (p *fakeProvider) Name() string { return "fake" }

func (p *fakeProvider) Upload(context.Context, string, io.Reader) error { return nil }

func (p *fakeProvider) Download(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}

func (p *fakeProvider) Delete(_ context.Context, key string) error {
	if p.fail {
		return errors.New("delete failed")
	}
	p.deleted = append(p.deleted, key)
	return nil
}

func (p *fakeProvider) Exists(context.Context, string) (bool, error) { return true, nil }

func (p *fakeProvider) List(context.Context, string) ([]string, error) { return nil, nil }

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func execution(id byte, day int, pinned bool, legacyPath string) db.Execution {
	return db.Execution{
		ID:          pgtype.UUID{Bytes: [16]byte{id}, Valid: true},
		StartTime:   pgtype.Timestamptz{Time: time.Date(2026, time.January, day, 2, 0, 0, 0, time.UTC), Valid: true},
		CreatedAt:   pgtype.Timestamptz{Time: time.Date(2026, time.January, day, 2, 0, 0, 0, time.UTC), Valid: true},
		StoragePath: pgtype.Text{String: legacyPath, Valid: legacyPath != ""},
		IsPinned:    pinned,
		Status:      "success",
	}
}

func dailyKeepOne() Policy {
	return Policy{KeepDaily: 1, YearlyMonth: 1}
}

func providerIDs() (pgtype.UUID, pgtype.UUID) {
	return pgtype.UUID{Bytes: [16]byte{201}, Valid: true}, pgtype.UUID{Bytes: [16]byte{202}, Valid: true}
}

func TestApply_DeletesAllProviderCopies(t *testing.T) {
	execs := []db.Execution{
		execution(3, 3, false, ""),
		execution(2, 2, false, ""),
		execution(1, 1, false, ""),
	}
	repo := newFakeRepo(execs)
	p1ID, p2ID := providerIDs()
	repo.storage[execs[1].ID] = []db.ExecutionStorage{
		{ExecutionID: execs[1].ID, ProviderID: p1ID, StoragePath: "p1/e2"},
		{ExecutionID: execs[1].ID, ProviderID: p2ID, StoragePath: "p2/e2"},
	}
	repo.storage[execs[2].ID] = []db.ExecutionStorage{
		{ExecutionID: execs[2].ID, ProviderID: p1ID, StoragePath: "p1/e1"},
		{ExecutionID: execs[2].ID, ProviderID: p2ID, StoragePath: "p2/e1"},
	}

	p1, p2 := &fakeProvider{}, &fakeProvider{}
	engine := New(repo, testLogger())
	err := engine.Apply(context.Background(), pgtype.UUID{}, dailyKeepOne(), map[pgtype.UUID]storage.StorageProvider{
		p1ID: p1,
		p2ID: p2,
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if len(repo.executions) != 1 || repo.executions[0].ID != execs[0].ID {
		t.Fatalf("expected only the newest execution to remain, got %+v", repo.executions)
	}
	for _, key := range []string{"p1/e2", "p2/e2", "p1/e1", "p2/e1"} {
		if !contains(p1.deleted, key) && !contains(p2.deleted, key) {
			t.Errorf("expected %s to be deleted", key)
		}
	}
}

func TestApply_PinnedIsKept(t *testing.T) {
	execs := []db.Execution{
		execution(1, 1, false, ""),
		execution(2, 2, true, ""),
	}
	repo := newFakeRepo(execs)
	p1ID, _ := providerIDs()
	repo.storage[execs[1].ID] = []db.ExecutionStorage{
		{ExecutionID: execs[1].ID, ProviderID: p1ID, StoragePath: "p1/pinned"},
	}

	p1 := &fakeProvider{}
	engine := New(repo, testLogger())
	if err := engine.Apply(context.Background(), pgtype.UUID{}, dailyKeepOne(), map[pgtype.UUID]storage.StorageProvider{p1ID: p1}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(repo.executions) != 2 {
		t.Fatalf("pinned execution should be kept, got %d executions", len(repo.executions))
	}
	if contains(p1.deleted, "p1/pinned") {
		t.Fatal("pinned artifact should not be deleted")
	}
}

func TestApply_DeleteFailureKeepsExecution(t *testing.T) {
	execs := []db.Execution{
		execution(1, 1, false, ""),
		execution(2, 2, false, ""),
	}
	repo := newFakeRepo(execs)
	p1ID, _ := providerIDs()
	repo.storage[execs[1].ID] = []db.ExecutionStorage{
		{ExecutionID: execs[1].ID, ProviderID: p1ID, StoragePath: "p1/e2"},
	}

	p1 := &fakeProvider{fail: true}
	engine := New(repo, testLogger())
	if err := engine.Apply(context.Background(), pgtype.UUID{}, dailyKeepOne(), map[pgtype.UUID]storage.StorageProvider{p1ID: p1}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(repo.executions) != 2 {
		t.Fatalf("execution with a failed object delete should remain, got %d", len(repo.executions))
	}
	if len(repo.storage[execs[1].ID]) != 1 {
		t.Fatal("storage record should remain after a failed delete")
	}
}

func TestApply_LegacyExecutionUsesStoragePath(t *testing.T) {
	execs := []db.Execution{
		execution(1, 1, false, ""),
		execution(2, 2, false, "legacy/e2.sql"),
	}
	repo := newFakeRepo(execs)
	p1ID, p2ID := providerIDs()
	p1, p2 := &fakeProvider{}, &fakeProvider{}

	engine := New(repo, testLogger())
	if err := engine.Apply(context.Background(), pgtype.UUID{}, dailyKeepOne(), map[pgtype.UUID]storage.StorageProvider{
		p1ID: p1,
		p2ID: p2,
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !contains(p1.deleted, "legacy/e2.sql") || !contains(p2.deleted, "legacy/e2.sql") {
		t.Fatalf("legacy artifact should be deleted from all providers: p1=%v p2=%v", p1.deleted, p2.deleted)
	}
	if len(repo.executions) != 1 {
		t.Fatalf("legacy execution should be removed, got %d", len(repo.executions))
	}
}

func TestApply_IgnoresFailedExecutionForLatest(t *testing.T) {
	failed := execution(3, 3, false, "")
	failed.Status = "failed"
	execs := []db.Execution{
		failed,
		execution(2, 2, false, ""),
		execution(1, 1, false, ""),
	}
	repo := newFakeRepo(execs)
	p1ID, _ := providerIDs()
	engine := New(repo, testLogger())

	if err := engine.Apply(context.Background(), pgtype.UUID{}, dailyKeepOne(), map[pgtype.UUID]storage.StorageProvider{p1ID: &fakeProvider{}}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(repo.executions) != 2 {
		t.Fatalf("failed execution should remain and the newest successful should be kept, got %d executions: %+v", len(repo.executions), repo.executions)
	}
	foundFailed := false
	foundKept := false
	for _, execution := range repo.executions {
		if execution.Status == "failed" {
			foundFailed = true
		}
		if execution.ID == execs[1].ID {
			foundKept = true
		}
	}
	if !foundFailed || !foundKept {
		t.Fatalf("unexpected retention result: %+v", repo.executions)
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
