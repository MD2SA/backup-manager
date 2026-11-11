package main

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/MD2SA/backup-manager/internal/backup"
	"github.com/MD2SA/backup-manager/internal/config"
	"github.com/MD2SA/backup-manager/internal/database"
	"github.com/MD2SA/backup-manager/internal/pkg/crypto"
	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/MD2SA/backup-manager/internal/service"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func runVerify(args []string) {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	configPath := fs.String("config", "", "path to config.yaml")
	executionID := fs.String("execution", "", "execution UUID to verify")
	profileID := fs.String("profile", "", "profile UUID whose latest executions should be verified")
	latest := fs.Int("latest", 1, "number of latest executions to verify when --profile is used")
	scratchDSN := fs.String("scratch-dsn", "", "optional empty PostgreSQL DSN to apply the verified artifact to")
	timeout := fs.Duration("timeout", 10*time.Minute, "overall verification timeout")
	_ = fs.Parse(args)

	cfg, err := config.LoadLenient(*configPath)
	if err != nil {
		fatal("Configuration error:\n%v", err)
	}
	if *executionID == "" && *profileID == "" {
		fatal("Provide --execution or --profile")
	}
	if *latest < 1 {
		fatal("--latest must be at least 1")
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	pool, err := database.New(ctx, cfg.MetadataDB)
	if err != nil {
		fatal("Metadata database connection failed: %v", err)
	}
	defer pool.Close()

	var encKey []byte
	if cfg.ConfigEncryptionKey != "" {
		encKey = crypto.DeriveConfigKey(cfg.ConfigEncryptionKey)
	}
	repo := repository.NewPostgres(pool, encKey)
	providers := service.NewProviderService(repo, cfg)

	var executions []db.Execution
	switch {
	case *executionID != "":
		id, parseErr := pgutil.ParseUUID(*executionID)
		if parseErr != nil || !id.Valid {
			fatal("Invalid execution ID: %v", parseErr)
		}
		execution, getErr := repo.GetExecution(ctx, id)
		if getErr != nil {
			fatal("Execution not found: %v", getErr)
		}
		executions = append(executions, execution)
	default:
		id, parseErr := pgutil.ParseUUID(*profileID)
		if parseErr != nil || !id.Valid {
			fatal("Invalid profile ID: %v", parseErr)
		}
		all, listErr := repo.ListExecutionsByProfile(ctx, id)
		if listErr != nil {
			fatal("List executions failed: %v", listErr)
		}
		if len(all) < *latest {
			*latest = len(all)
		}
		executions = all[:*latest]
	}

	if len(executions) == 0 {
		fatal("No executions to verify")
	}

	var scratch *backup.RestoreTarget
	if *scratchDSN != "" {
		if len(executions) != 1 {
			fatal("--scratch-dsn requires exactly one execution (use --execution)")
		}
		target, parseErr := parseScratchTarget(*scratchDSN)
		if parseErr != nil {
			fatal("Invalid scratch DSN: %v", parseErr)
		}
		scratch = &target
	}

	failed := false
	for _, execution := range executions {
		if err := verifyExecution(ctx, repo, providers, cfg, execution, scratch); err != nil {
			failed = true
			fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", pgutil.UUIDToString(execution.ID), err)
		}
	}
	if failed {
		os.Exit(1)
	}
}

func verifyExecution(
	ctx context.Context,
	repo repository.Repository,
	providers *service.ProviderService,
	cfg config.Config,
	execution db.Execution,
	scratch *backup.RestoreTarget,
) error {
	candidates, err := executionArtifactCandidates(ctx, repo, execution, pgtype.UUID{})
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return fmt.Errorf("execution has no storage copy")
	}

	var lastErr error
	verified := false
	applied := false
	for _, candidate := range candidates {
		artifact, prepErr := prepareCandidateArtifact(
			ctx,
			providers,
			cfg,
			execution,
			candidate,
			cfg.EncryptionPassphrase,
			cfg.AgePrivateKey,
			func(format string, args ...any) {
				fmt.Printf("  %s\n", fmt.Sprintf(format, args...))
			},
		)
		if prepErr != nil {
			lastErr = prepErr
			fmt.Fprintf(os.Stderr, "  provider %s: %v\n", pgutil.UUIDToString(candidate.providerID), prepErr)
			continue
		}

		verified = true
		fmt.Printf("OK %s provider=%s path=%s checksum_verified=%t gzip=%t custom=%t\n",
			pgutil.UUIDToString(execution.ID),
			pgutil.UUIDToString(candidate.providerID),
			candidate.storagePath,
			artifact.ChecksumVerified,
			artifact.Gzip,
			artifact.CustomFormat,
		)

		if scratch != nil && !applied {
			if preflightErr := scratch.Preflight(ctx, false); preflightErr != nil {
				artifact.Cleanup()
				return preflightErr
			}
			output, applyErr := backup.ApplyArtifact(ctx, artifact, *scratch, true)
			artifact.Cleanup()
			if applyErr != nil {
				return fmt.Errorf("apply to scratch: %w\n%s", applyErr, output)
			}
			applied = true
			fmt.Printf("OK %s applied to scratch database %s\n", pgutil.UUIDToString(execution.ID), scratch.DBName)
			continue
		}
		artifact.Cleanup()
	}

	if !verified {
		return lastErr
	}
	return nil
}

func parseScratchTarget(dsn string) (backup.RestoreTarget, error) {
	parsed, err := pgx.ParseConfig(dsn)
	if err != nil {
		return backup.RestoreTarget{}, err
	}
	sslMode := "disable"
	if u, parseErr := url.Parse(dsn); parseErr == nil {
		if mode := u.Query().Get("sslmode"); mode != "" {
			sslMode = mode
		}
	}
	port := int(parsed.Port)
	if port == 0 {
		port = 5432
	}
	return backup.RestoreTarget{
		Host:     parsed.Host,
		Port:     strconv.Itoa(port),
		User:     parsed.User,
		Password: parsed.Password,
		DBName:   parsed.Database,
		SSLMode:  sslMode,
	}, nil
}
