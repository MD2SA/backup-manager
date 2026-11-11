package recovery

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/MD2SA/backup-manager/internal/backup"
	"github.com/MD2SA/backup-manager/internal/config"
	"github.com/MD2SA/backup-manager/internal/database"
	"github.com/MD2SA/backup-manager/internal/pkg/crypto"
	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/MD2SA/backup-manager/internal/service"
	"github.com/MD2SA/backup-manager/internal/storage/local"
	"github.com/jackc/pgx/v5/pgtype"
)

type prepareArtifactFunc func(
	ctx context.Context,
	providers StorageProviderResolver,
	cfg config.Config,
	execution db.Execution,
	candidate ArtifactCandidate,
	passphrase, identity string,
	logf func(format string, args ...any),
) (*backup.Artifact, error)

type applyArtifactFunc func(
	ctx context.Context,
	artifact *backup.Artifact,
	target backup.RestoreTarget,
	singleTransaction bool,
) (string, error)

type preflightFunc func(
	ctx context.Context,
	target backup.RestoreTarget,
	terminateConnections bool,
) error

type dataDeps struct {
	open      func(context.Context, config.Config) (ExecutionStore, StorageProviderResolver, func(), error)
	prepare   prepareArtifactFunc
	preflight preflightFunc
	apply     applyArtifactFunc
}

func defaultDataDeps() dataDeps {
	return dataDeps{
		open: func(ctx context.Context, cfg config.Config) (ExecutionStore, StorageProviderResolver, func(), error) {
			pool, err := database.New(ctx, cfg.MetadataDB)
			if err != nil {
				return nil, nil, nil, err
			}
			var encKey []byte
			if cfg.ConfigEncryptionKey != "" {
				encKey = crypto.DeriveConfigKey(cfg.ConfigEncryptionKey)
			}
			repo := repository.NewPostgres(pool, encKey)
			return repo, service.NewProviderService(repo, cfg), pool.Close, nil
		},
		prepare: PrepareCandidateArtifact,
		preflight: func(ctx context.Context, target backup.RestoreTarget, terminate bool) error {
			return target.Preflight(ctx, terminate)
		},
		apply: backup.ApplyArtifact,
	}
}

// Data downloads, verifies and optionally applies a data backup. It validates
// destructive options before touching the metadata database or target.
func Data(ctx context.Context, cfg config.Config, opts DataOptions) (DataResult, error) {
	return runData(ctx, cfg, opts, defaultDataDeps())
}

func runData(ctx context.Context, cfg config.Config, opts DataOptions, deps dataDeps) (DataResult, error) {
	if err := validateDataOptions(opts); err != nil {
		return DataResult{}, err
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	passphrase := opts.Passphrase
	if passphrase == "" {
		passphrase = cfg.EncryptionPassphrase
	}
	identity := opts.Identity
	if identity == "" {
		identity = cfg.AgePrivateKey
	}
	logf := opts.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	progress := opts.Progressf
	if progress == nil {
		progress = func(string, ...any) {}
	}

	var (
		artifact *backup.Artifact
		result   DataResult
		err      error
	)
	if opts.File != "" {
		artifact, result, err = prepareFileArtifact(ctx, cfg, opts, passphrase, identity, logf)
		if err != nil {
			return DataResult{}, err
		}
	} else {
		artifact, result, err = prepareExecutionArtifact(ctx, cfg, opts, deps, passphrase, identity, logf, progress)
		if err != nil {
			return result, err
		}
	}
	if !opts.KeepArtifact {
		defer artifact.Cleanup()
	}

	result.ArtifactPath = artifact.Path
	result.ChecksumVerified = artifact.ChecksumVerified
	if opts.VerifyOnly {
		return result, nil
	}

	target, err := ResolveTarget(opts.TargetDSN, cfg)
	if err != nil {
		return result, fmt.Errorf("target database: %w", err)
	}
	if err := deps.preflight(ctx, target, opts.TerminateConnections); err != nil {
		return result, fmt.Errorf("target preflight failed: %w", err)
	}
	output, err := deps.apply(ctx, artifact, target, true)
	if err != nil {
		return result, fmt.Errorf("restore failed: %w\n%s", err, output)
	}
	result.Applied = true
	result.TargetDatabase = target.DBName
	return result, nil
}

func validateDataOptions(opts DataOptions) error {
	if opts.File != "" && (opts.ExecutionID != "" || opts.ProfileID != "") {
		return fmt.Errorf("--file cannot be combined with --execution/--profile")
	}
	if opts.File == "" && opts.ExecutionID == "" && opts.ProfileID == "" {
		return fmt.Errorf("provide --execution, --profile or --file")
	}
	if opts.ExecutionID != "" && opts.ProfileID != "" {
		return fmt.Errorf("provide either --execution or --profile, not both")
	}
	if opts.File != "" && opts.ProviderID != "" {
		return fmt.Errorf("--provider-id cannot be used with --file")
	}
	if !opts.VerifyOnly && !opts.Confirm {
		return fmt.Errorf("refusing to restore without --confirm (or use --verify-only)")
	}
	return nil
}

func prepareFileArtifact(
	ctx context.Context,
	cfg config.Config,
	opts DataOptions,
	passphrase, identity string,
	logf func(format string, args ...any),
) (*backup.Artifact, DataResult, error) {
	absolute, err := filepath.Abs(opts.File)
	if err != nil {
		return nil, DataResult{}, fmt.Errorf("resolve file: %w", err)
	}
	provider, err := local.New(filepath.Dir(absolute))
	if err != nil {
		return nil, DataResult{}, fmt.Errorf("local artifact source: %w", err)
	}
	artifact, err := backup.PrepareArtifact(
		ctx,
		provider,
		filepath.Base(absolute),
		cfg.TempDir,
		"",
		false,
		passphrase,
		identity,
		logf,
	)
	if err != nil {
		return nil, DataResult{}, fmt.Errorf("verify artifact: %w", err)
	}
	return artifact, DataResult{ArtifactSource: absolute}, nil
}

func prepareExecutionArtifact(
	ctx context.Context,
	cfg config.Config,
	opts DataOptions,
	deps dataDeps,
	passphrase, identity string,
	logf func(format string, args ...any),
	progress func(format string, args ...any),
) (*backup.Artifact, DataResult, error) {
	store, providers, closeStore, err := deps.open(ctx, cfg)
	if err != nil {
		return nil, DataResult{}, fmt.Errorf("metadata database connection failed: %w", err)
	}
	defer closeStore()

	execution, err := ResolveExecution(ctx, store, opts.ExecutionID, opts.ProfileID)
	if err != nil {
		return nil, DataResult{}, fmt.Errorf("resolve execution: %w", err)
	}

	var requested pgtype.UUID
	if opts.ProviderID != "" {
		requested, err = parseUUID(opts.ProviderID)
		if err != nil {
			return nil, DataResult{}, fmt.Errorf("invalid provider ID: %w", err)
		}
	}

	result := DataResult{
		ExecutionID:        pgutil.UUIDToString(execution.ID),
		ExecutionStatus:    execution.Status,
		ExecutionCreatedAt: executionCreatedAt(execution),
	}
	progress("Execution: %s", ExecutionLabel(execution))

	candidates, err := ExecutionArtifactCandidates(ctx, store, execution, requested)
	if err != nil {
		return nil, result, fmt.Errorf("resolve storage copies: %w", err)
	}
	if len(candidates) == 0 {
		return nil, result, fmt.Errorf("execution has no storage copy")
	}

	var failures []string
	for _, candidate := range candidates {
		progress("Trying provider %s", pgutil.UUIDToString(candidate.ProviderID))
		candidateArtifact, prepErr := deps.prepare(ctx, providers, cfg, execution, candidate, passphrase, identity, logf)
		if prepErr != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", pgutil.UUIDToString(candidate.ProviderID), prepErr))
			continue
		}
		progress("Using provider %s path=%s", pgutil.UUIDToString(candidate.ProviderID), candidate.StoragePath)
		result.ProviderID = pgutil.UUIDToString(candidate.ProviderID)
		result.StoragePath = candidate.StoragePath
		return candidateArtifact, result, nil
	}
	return nil, result, fmt.Errorf("no usable backup artifact:\n  %s", strings.Join(failures, "\n  "))
}
