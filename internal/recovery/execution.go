package recovery

import (
	"context"
	"fmt"
	"time"

	"github.com/MD2SA/backup-manager/internal/backup"
	"github.com/MD2SA/backup-manager/internal/config"
	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/MD2SA/backup-manager/internal/storage"
	"github.com/jackc/pgx/v5/pgtype"
)

// ExecutionStore is the subset of the metadata repository used by recovery.
// Keeping it narrow makes the recovery workflow easy to test with fakes.
type ExecutionStore interface {
	GetExecution(ctx context.Context, id pgtype.UUID) (db.Execution, error)
	ListExecutionsByProfile(ctx context.Context, profileID pgtype.UUID) ([]db.Execution, error)
	ListExecutionStorage(ctx context.Context, executionID pgtype.UUID) ([]db.ExecutionStorage, error)
	GetProfile(ctx context.Context, id pgtype.UUID) (repository.ProfileFull, error)
}

// StorageProviderResolver resolves a provider ID to a storage backend.
type StorageProviderResolver interface {
	ResolveStorageProvider(ctx context.Context, id pgtype.UUID) (storage.StorageProvider, error)
}

// ArtifactCandidate is one storage copy of an execution artifact.
type ArtifactCandidate struct {
	ProviderID  pgtype.UUID
	StoragePath string
}

// ResolveExecution returns an execution by ID, or the latest successful
// execution of a profile.
func ResolveExecution(ctx context.Context, store ExecutionStore, executionID, profileID string) (db.Execution, error) {
	if executionID != "" {
		id, err := parseUUID(executionID)
		if err != nil {
			return db.Execution{}, fmt.Errorf("invalid execution ID: %w", err)
		}
		execution, err := store.GetExecution(ctx, id)
		if err != nil {
			return db.Execution{}, fmt.Errorf("execution not found: %w", err)
		}
		return execution, nil
	}

	id, err := parseUUID(profileID)
	if err != nil {
		return db.Execution{}, fmt.Errorf("invalid profile ID: %w", err)
	}
	executions, err := store.ListExecutionsByProfile(ctx, id)
	if err != nil {
		return db.Execution{}, fmt.Errorf("list executions: %w", err)
	}
	for _, execution := range executions {
		if execution.Status == string(backup.StatusSuccess) {
			return execution, nil
		}
	}
	return db.Execution{}, fmt.Errorf("profile has no successful execution")
}

// ExecutionArtifactCandidates resolves every storage copy recorded for an
// execution. When requestedProvider is valid, only that provider is considered.
// Legacy executions without execution_storage rows fall back to the profile's
// storage providers and the execution's single storage path.
func ExecutionArtifactCandidates(
	ctx context.Context,
	store ExecutionStore,
	execution db.Execution,
	requestedProvider pgtype.UUID,
) ([]ArtifactCandidate, error) {
	records, err := store.ListExecutionStorage(ctx, execution.ID)
	if err != nil {
		return nil, err
	}

	var candidates []ArtifactCandidate
	if requestedProvider.Valid {
		for _, record := range records {
			if record.ProviderID == requestedProvider {
				candidates = append(candidates, ArtifactCandidate{
					ProviderID:  record.ProviderID,
					StoragePath: record.StoragePath,
				})
			}
		}
		if len(candidates) == 0 && execution.StoragePath.Valid {
			candidates = append(candidates, ArtifactCandidate{
				ProviderID:  requestedProvider,
				StoragePath: execution.StoragePath.String,
			})
		}
		return candidates, nil
	}

	for _, record := range records {
		candidates = append(candidates, ArtifactCandidate{
			ProviderID:  record.ProviderID,
			StoragePath: record.StoragePath,
		})
	}
	if len(candidates) == 0 && execution.StoragePath.Valid {
		profile, profileErr := store.GetProfile(ctx, execution.ProfileID)
		if profileErr != nil {
			return nil, profileErr
		}
		for _, providerID := range profile.StorageProviderIDs {
			candidates = append(candidates, ArtifactCandidate{
				ProviderID:  providerID,
				StoragePath: execution.StoragePath.String,
			})
		}
	}
	return candidates, nil
}

// PrepareCandidateArtifact downloads, decrypts, checksums and inspects one
// storage copy.
func PrepareCandidateArtifact(
	ctx context.Context,
	providers StorageProviderResolver,
	cfg config.Config,
	execution db.Execution,
	candidate ArtifactCandidate,
	passphrase, identity string,
	logf func(format string, args ...any),
) (*backup.Artifact, error) {
	provider, err := providers.ResolveStorageProvider(ctx, candidate.ProviderID)
	if err != nil {
		return nil, err
	}

	return backup.PrepareArtifact(
		ctx,
		provider,
		candidate.StoragePath,
		cfg.TempDir,
		execution.Checksum.String,
		execution.IsEncrypted,
		passphrase,
		identity,
		logf,
	)
}

// ExecutionLabel formats an execution for human-readable CLI output.
func ExecutionLabel(execution db.Execution) string {
	created := ""
	if execution.CreatedAt.Valid {
		created = execution.CreatedAt.Time.Format("2006-01-02 15:04:05 MST")
	}
	return fmt.Sprintf("%s (status=%s, created=%s)", pgutil.UUIDToString(execution.ID), execution.Status, created)
}

func executionCreatedAt(execution db.Execution) time.Time {
	if execution.CreatedAt.Valid {
		return execution.CreatedAt.Time
	}
	return time.Time{}
}

func parseUUID(raw string) (pgtype.UUID, error) {
	id, err := pgutil.ParseUUID(raw)
	if err != nil {
		return pgtype.UUID{}, err
	}
	if !id.Valid {
		return pgtype.UUID{}, fmt.Errorf("invalid UUID %q", raw)
	}
	return id, nil
}
