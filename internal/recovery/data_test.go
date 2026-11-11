package recovery

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MD2SA/backup-manager/internal/backup"
	"github.com/MD2SA/backup-manager/internal/config"
	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/MD2SA/backup-manager/internal/storage"
	"github.com/MD2SA/backup-manager/internal/storage/local"
	"github.com/jackc/pgx/v5/pgtype"
)

type fakeStore struct {
	executions map[string]db.Execution
	byProfile  map[string][]db.Execution
	storage    map[string][]db.ExecutionStorage
	profiles   map[string]repository.ProfileFull
}

func (f *fakeStore) GetExecution(_ context.Context, id pgtype.UUID) (db.Execution, error) {
	execution, ok := f.executions[pgutil.UUIDToString(id)]
	if !ok {
		return db.Execution{}, fmt.Errorf("not found")
	}
	return execution, nil
}

func (f *fakeStore) ListExecutionsByProfile(_ context.Context, profileID pgtype.UUID) ([]db.Execution, error) {
	return f.byProfile[pgutil.UUIDToString(profileID)], nil
}

func (f *fakeStore) ListExecutionStorage(_ context.Context, executionID pgtype.UUID) ([]db.ExecutionStorage, error) {
	return f.storage[pgutil.UUIDToString(executionID)], nil
}

func (f *fakeStore) GetProfile(_ context.Context, id pgtype.UUID) (repository.ProfileFull, error) {
	profile, ok := f.profiles[pgutil.UUIDToString(id)]
	if !ok {
		return repository.ProfileFull{}, fmt.Errorf("not found")
	}
	return profile, nil
}

type fakeResolver struct {
	provider storage.StorageProvider
	err      error
}

func (f fakeResolver) ResolveStorageProvider(context.Context, pgtype.UUID) (storage.StorageProvider, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.provider != nil {
		return f.provider, nil
	}
	return nil, errors.New("resolver must not be called by these tests")
}

func TestResolveExecution(t *testing.T) {
	execID := testUUID(t, "11111111-1111-1111-1111-111111111111")
	profileID := testUUID(t, "22222222-2222-2222-2222-222222222222")
	success := db.Execution{ID: execID, ProfileID: profileID, Status: "success"}
	failed := db.Execution{ID: testUUID(t, "33333333-3333-3333-3333-333333333333"), ProfileID: profileID, Status: "failed"}
	store := &fakeStore{
		executions: map[string]db.Execution{pgutil.UUIDToString(execID): success},
		byProfile: map[string][]db.Execution{
			pgutil.UUIDToString(profileID): {failed, success},
		},
	}

	got, err := ResolveExecution(context.Background(), store, pgutil.UUIDToString(execID), "")
	if err != nil || got.ID != execID {
		t.Fatalf("by ID: %+v, %v", got, err)
	}

	got, err = ResolveExecution(context.Background(), store, "", pgutil.UUIDToString(profileID))
	if err != nil || got.ID != execID {
		t.Fatalf("latest success: %+v, %v", got, err)
	}

	if _, err := ResolveExecution(context.Background(), store, "bad", ""); err == nil {
		t.Fatal("expected invalid execution ID error")
	}
	if _, err := ResolveExecution(context.Background(), store, "", "bad"); err == nil {
		t.Fatal("expected invalid profile ID error")
	}

	store.byProfile[pgutil.UUIDToString(profileID)] = []db.Execution{failed}
	if _, err := ResolveExecution(context.Background(), store, "", pgutil.UUIDToString(profileID)); err == nil {
		t.Fatal("expected no successful execution error")
	}
}

func TestExecutionArtifactCandidates(t *testing.T) {
	execID := testUUID(t, "11111111-1111-1111-1111-111111111111")
	profileID := testUUID(t, "22222222-2222-2222-2222-222222222222")
	providerA := testUUID(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	providerB := testUUID(t, "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	execution := db.Execution{ID: execID, ProfileID: profileID, StoragePath: pgtype.Text{String: "legacy/path.sql", Valid: true}}
	store := &fakeStore{
		storage: map[string][]db.ExecutionStorage{
			pgutil.UUIDToString(execID): {
				{ExecutionID: execID, ProviderID: providerA, StoragePath: "a.sql"},
				{ExecutionID: execID, ProviderID: providerB, StoragePath: "b.sql"},
			},
		},
	}

	candidates, err := ExecutionArtifactCandidates(context.Background(), store, execution, pgtype.UUID{})
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 || candidates[0].ProviderID != providerA || candidates[1].StoragePath != "b.sql" {
		t.Fatalf("unexpected candidates: %+v", candidates)
	}

	candidates, err = ExecutionArtifactCandidates(context.Background(), store, execution, providerB)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].ProviderID != providerB {
		t.Fatalf("unexpected filtered candidates: %+v", candidates)
	}

	store.storage = nil
	store.profiles = map[string]repository.ProfileFull{
		pgutil.UUIDToString(profileID): {StorageProviderIDs: []pgtype.UUID{providerA, providerB}},
	}
	candidates, err = ExecutionArtifactCandidates(context.Background(), store, execution, pgtype.UUID{})
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 || candidates[0].StoragePath != "legacy/path.sql" {
		t.Fatalf("unexpected legacy candidates: %+v", candidates)
	}
}

func TestRunDataValidation(t *testing.T) {
	tests := []struct {
		name string
		opts DataOptions
	}{
		{"file and execution", DataOptions{File: "a.sql", ExecutionID: "x", Confirm: true}},
		{"no source", DataOptions{Confirm: true}},
		{"execution and profile", DataOptions{ExecutionID: "x", ProfileID: "y", Confirm: true}},
		{"file and provider", DataOptions{File: "a.sql", ProviderID: "p", Confirm: true}},
		{"missing confirm", DataOptions{ExecutionID: "x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := runData(context.Background(), config.Config{}, tt.opts, dataDeps{}); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestRunDataVerifyOnlyDoesNotApply(t *testing.T) {
	fixture := newDataFixture(t)
	preflighted, applied := false, false
	deps := fixture.deps(
		func(context.Context, StorageProviderResolver, config.Config, db.Execution, ArtifactCandidate, string, string, func(string, ...any)) (*backup.Artifact, error) {
			return &backup.Artifact{Path: fixture.artifactPath, ChecksumVerified: true}, nil
		},
		func(context.Context, backup.RestoreTarget, bool) error { preflighted = true; return nil },
		func(context.Context, *backup.Artifact, backup.RestoreTarget, bool) (string, error) {
			applied = true
			return "", nil
		},
	)

	result, err := runData(context.Background(), fixture.cfg, DataOptions{
		ExecutionID: fixture.executionID,
		VerifyOnly:  true,
	}, deps)
	if err != nil {
		t.Fatal(err)
	}
	if preflighted || applied {
		t.Fatalf("verify-only touched the target: preflighted=%t applied=%t", preflighted, applied)
	}
	if !result.ChecksumVerified || result.Applied {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.ProviderID != pgutil.UUIDToString(fixture.providerID) {
		t.Fatalf("unexpected provider: %+v", result)
	}
}

func TestRunDataApplies(t *testing.T) {
	fixture := newDataFixture(t)
	var targetDB string
	deps := fixture.deps(
		func(context.Context, StorageProviderResolver, config.Config, db.Execution, ArtifactCandidate, string, string, func(string, ...any)) (*backup.Artifact, error) {
			return &backup.Artifact{Path: fixture.artifactPath, ChecksumVerified: true}, nil
		},
		func(_ context.Context, target backup.RestoreTarget, terminate bool) error {
			if terminate {
				t.Fatal("terminate should be false")
			}
			return nil
		},
		func(_ context.Context, _ *backup.Artifact, target backup.RestoreTarget, single bool) (string, error) {
			targetDB = target.DBName
			if !single {
				t.Fatal("expected a single-transaction restore")
			}
			return "ok", nil
		},
	)

	result, err := runData(context.Background(), fixture.cfg, DataOptions{
		ExecutionID: fixture.executionID,
		Confirm:     true,
	}, deps)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Applied || result.TargetDatabase != "app" || targetDB != "app" {
		t.Fatalf("unexpected result: %+v (target=%q)", result, targetDB)
	}
}

func TestRunDataTriesNextProvider(t *testing.T) {
	fixture := newDataFixture(t)
	second := testUUID(t, "cccccccc-cccc-cccc-cccc-cccccccccccc")
	fixture.store.storage[fixture.executionID] = append(
		fixture.store.storage[fixture.executionID],
		db.ExecutionStorage{ProviderID: second, StoragePath: "second.sql"},
	)

	calls := 0
	deps := fixture.deps(
		func(_ context.Context, _ StorageProviderResolver, _ config.Config, _ db.Execution, candidate ArtifactCandidate, _, _ string, _ func(string, ...any)) (*backup.Artifact, error) {
			calls++
			if candidate.ProviderID == fixture.providerID {
				return nil, errors.New("provider unavailable")
			}
			return &backup.Artifact{Path: fixture.artifactPath}, nil
		},
		func(context.Context, backup.RestoreTarget, bool) error { return nil },
		func(context.Context, *backup.Artifact, backup.RestoreTarget, bool) (string, error) { return "", nil },
	)

	result, err := runData(context.Background(), fixture.cfg, DataOptions{
		ExecutionID: fixture.executionID,
		VerifyOnly:  true,
	}, deps)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 attempts, got %d", calls)
	}
	if result.ProviderID != pgutil.UUIDToString(second) {
		t.Fatalf("unexpected provider: %+v", result)
	}
}

func TestRunDataNoStorage(t *testing.T) {
	fixture := newDataFixture(t)
	fixture.store.storage = nil
	fixture.execution.StoragePath = pgtype.Text{}
	fixture.store.executions[fixture.executionID] = fixture.execution

	deps := fixture.deps(nil, nil, nil)
	_, err := runData(context.Background(), fixture.cfg, DataOptions{
		ExecutionID: fixture.executionID,
		VerifyOnly:  true,
	}, deps)
	if err == nil {
		t.Fatal("expected no storage copy error")
	}
}

func TestRunDataFileVerifyOnly(t *testing.T) {
	cfg := testConfig(t)
	path := filepath.Join(t.TempDir(), "backup.sql")
	if err := os.WriteFile(path, []byte("SELECT 1;"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := runData(context.Background(), cfg, DataOptions{
		File:       path,
		VerifyOnly: true,
	}, dataDeps{})
	if err != nil {
		t.Fatal(err)
	}
	absolute, _ := filepath.Abs(path)
	if result.ArtifactSource != absolute || result.ChecksumVerified {
		t.Fatalf("unexpected result: %+v", result)
	}
}

type dataFixture struct {
	cfg          config.Config
	store        *fakeStore
	execution    db.Execution
	executionID  string
	providerID   pgtype.UUID
	artifactPath string
}

func newDataFixture(t *testing.T) *dataFixture {
	t.Helper()
	execID := testUUID(t, "11111111-1111-1111-1111-111111111111")
	profileID := testUUID(t, "22222222-2222-2222-2222-222222222222")
	providerID := testUUID(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	execution := db.Execution{
		ID:        execID,
		ProfileID: profileID,
		Status:    "success",
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		Checksum:  pgtype.Text{String: "abc", Valid: true},
	}
	store := &fakeStore{
		executions: map[string]db.Execution{pgutil.UUIDToString(execID): execution},
		storage: map[string][]db.ExecutionStorage{
			pgutil.UUIDToString(execID): {
				{ExecutionID: execID, ProviderID: providerID, StoragePath: "artifact.sql"},
			},
		},
	}
	cfg := testConfig(t)
	cfg.TargetDB = config.DatabaseConfig{
		Host: "db", Port: "5432", User: "restore", Password: "pass", DBName: "app", SSLMode: "disable",
	}
	cfg.MetadataDB = config.DatabaseConfig{
		Host: "db", Port: "5432", User: "meta", Password: "pass", DBName: "metadata", SSLMode: "disable",
	}
	return &dataFixture{
		cfg:          cfg,
		store:        store,
		execution:    execution,
		executionID:  pgutil.UUIDToString(execID),
		providerID:   providerID,
		artifactPath: filepath.Join(t.TempDir(), "artifact.sql"),
	}
}

func (f *dataFixture) deps(prepare prepareArtifactFunc, preflight preflightFunc, apply applyArtifactFunc) dataDeps {
	if prepare == nil {
		prepare = func(context.Context, StorageProviderResolver, config.Config, db.Execution, ArtifactCandidate, string, string, func(string, ...any)) (*backup.Artifact, error) {
			return &backup.Artifact{Path: f.artifactPath}, nil
		}
	}
	if preflight == nil {
		preflight = func(context.Context, backup.RestoreTarget, bool) error { return nil }
	}
	if apply == nil {
		apply = func(context.Context, *backup.Artifact, backup.RestoreTarget, bool) (string, error) { return "", nil }
	}
	return dataDeps{
		open: func(context.Context, config.Config) (ExecutionStore, StorageProviderResolver, func(), error) {
			return f.store, fakeResolver{}, func() {}, nil
		},
		prepare:   prepare,
		preflight: preflight,
		apply:     apply,
	}
}

func testUUID(t *testing.T, raw string) pgtype.UUID {
	t.Helper()
	id, err := pgutil.ParseUUID(raw)
	if err != nil || !id.Valid {
		t.Fatalf("invalid test UUID %q: %v", raw, err)
	}
	return id
}

func TestDataPublicValidation(t *testing.T) {
	if _, err := Data(context.Background(), config.Config{}, DataOptions{}); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestPrepareCandidateArtifact(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "artifact.sql"), []byte("SELECT 1;"), 0o600); err != nil {
		t.Fatal(err)
	}
	provider, err := local.New(root)
	if err != nil {
		t.Fatal(err)
	}
	execID := testUUID(t, "11111111-1111-1111-1111-111111111111")
	providerID := testUUID(t, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	cfg := testConfig(t)
	artifact, err := PrepareCandidateArtifact(
		context.Background(),
		fakeResolver{provider: provider},
		cfg,
		db.Execution{ID: execID},
		ArtifactCandidate{ProviderID: providerID, StoragePath: "artifact.sql"},
		"", "",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer artifact.Cleanup()
	if artifact.ChecksumVerified {
		t.Fatal("unexpected checksum verification")
	}
	if _, err := os.Stat(artifact.Path); err != nil {
		t.Fatalf("artifact missing: %v", err)
	}
}

func TestRunDataExecutionOpenError(t *testing.T) {
	deps := dataDeps{
		open: func(context.Context, config.Config) (ExecutionStore, StorageProviderResolver, func(), error) {
			return nil, nil, nil, errors.New("metadata down")
		},
	}
	_, err := runData(context.Background(), config.Config{}, DataOptions{ExecutionID: "x", VerifyOnly: true}, deps)
	if err == nil {
		t.Fatal("expected open error")
	}
}

func TestRunDataAllProvidersFail(t *testing.T) {
	fixture := newDataFixture(t)
	deps := fixture.deps(
		func(context.Context, StorageProviderResolver, config.Config, db.Execution, ArtifactCandidate, string, string, func(string, ...any)) (*backup.Artifact, error) {
			return nil, errors.New("broken")
		},
		nil, nil,
	)
	_, err := runData(context.Background(), fixture.cfg, DataOptions{ExecutionID: fixture.executionID, VerifyOnly: true}, deps)
	if err == nil {
		t.Fatal("expected provider failure")
	}
}

func TestRunDataTargetErrors(t *testing.T) {
	fixture := newDataFixture(t)
	fixture.cfg.TargetDB = config.DatabaseConfig{}
	deps := fixture.deps(nil, nil, nil)
	if _, err := runData(context.Background(), fixture.cfg, DataOptions{ExecutionID: fixture.executionID, Confirm: true}, deps); err == nil {
		t.Fatal("expected target configuration error")
	}

	fixture = newDataFixture(t)
	deps = fixture.deps(
		nil,
		func(context.Context, backup.RestoreTarget, bool) error { return errors.New("not empty") },
		nil,
	)
	if _, err := runData(context.Background(), fixture.cfg, DataOptions{ExecutionID: fixture.executionID, Confirm: true}, deps); err == nil {
		t.Fatal("expected preflight error")
	}
}

func TestRunDataFileMissing(t *testing.T) {
	cfg := testConfig(t)
	if _, err := runData(context.Background(), cfg, DataOptions{
		File:       filepath.Join(t.TempDir(), "missing.sql"),
		VerifyOnly: true,
	}, dataDeps{}); err == nil {
		t.Fatal("expected missing file error")
	}
}
