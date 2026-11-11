package main

import (
	"context"
	"fmt"

	"github.com/MD2SA/backup-manager/internal/backup"
	"github.com/MD2SA/backup-manager/internal/config"
	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/MD2SA/backup-manager/internal/service"
	"github.com/jackc/pgx/v5/pgtype"
)

// artifactCandidate is one storage copy of an execution artifact.
type artifactCandidate struct {
	providerID  pgtype.UUID
	storagePath string
}

// executionArtifactCandidates resolves every storage copy recorded for an
// execution. When requestedProvider is valid, only that provider is considered.
// Legacy executions without execution_storage rows fall back to the profile's
// storage providers and the execution's single storage path.
func executionArtifactCandidates(
	ctx context.Context,
	repo repository.Repository,
	execution db.Execution,
	requestedProvider pgtype.UUID,
) ([]artifactCandidate, error) {
	records, err := repo.ListExecutionStorage(ctx, execution.ID)
	if err != nil {
		return nil, err
	}

	var candidates []artifactCandidate
	if requestedProvider.Valid {
		for _, record := range records {
			if record.ProviderID == requestedProvider {
				candidates = append(candidates, artifactCandidate{
					providerID:  record.ProviderID,
					storagePath: record.StoragePath,
				})
			}
		}
		if len(candidates) == 0 && execution.StoragePath.Valid {
			candidates = append(candidates, artifactCandidate{
				providerID:  requestedProvider,
				storagePath: execution.StoragePath.String,
			})
		}
		return candidates, nil
	}

	for _, record := range records {
		candidates = append(candidates, artifactCandidate{
			providerID:  record.ProviderID,
			storagePath: record.StoragePath,
		})
	}
	if len(candidates) == 0 && execution.StoragePath.Valid {
		profile, profileErr := repo.GetProfile(ctx, execution.ProfileID)
		if profileErr != nil {
			return nil, profileErr
		}
		for _, providerID := range profile.StorageProviderIDs {
			candidates = append(candidates, artifactCandidate{
				providerID:  providerID,
				storagePath: execution.StoragePath.String,
			})
		}
	}
	return candidates, nil
}

// prepareCandidateArtifact downloads, decrypts, checksums and inspects one
// storage copy.
func prepareCandidateArtifact(
	ctx context.Context,
	providers *service.ProviderService,
	cfg config.Config,
	execution db.Execution,
	candidate artifactCandidate,
	passphrase, identity string,
	logf func(format string, args ...any),
) (*backup.Artifact, error) {
	provider, err := providers.ResolveStorageProvider(ctx, candidate.providerID)
	if err != nil {
		return nil, err
	}

	return backup.PrepareArtifact(
		ctx,
		provider,
		candidate.storagePath,
		cfg.TempDir,
		execution.Checksum.String,
		execution.IsEncrypted,
		passphrase,
		identity,
		logf,
	)
}

func executionLabel(execution db.Execution) string {
	created := ""
	if execution.CreatedAt.Valid {
		created = execution.CreatedAt.Time.Format("2006-01-02 15:04:05 MST")
	}
	return fmt.Sprintf("%s (status=%s, created=%s)", pgutil.UUIDToString(execution.ID), execution.Status, created)
}
