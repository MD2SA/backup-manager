package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/MD2SA/backup-manager/internal/backup"
	"github.com/MD2SA/backup-manager/internal/config"
	"github.com/MD2SA/backup-manager/internal/database"
	"github.com/MD2SA/backup-manager/internal/logger"
	"github.com/MD2SA/backup-manager/internal/metarestore"
	"github.com/MD2SA/backup-manager/internal/pkg/crypto"
	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/MD2SA/backup-manager/internal/service"
	"github.com/MD2SA/backup-manager/internal/storage"
	"github.com/MD2SA/backup-manager/internal/storage/local"
	"github.com/MD2SA/backup-manager/internal/storage/s3"
	"github.com/jackc/pgx/v5/pgtype"
)

func runRecover(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: backup-manager recover <metadata|data> [flags]")
		os.Exit(2)
	}
	switch args[0] {
	case "metadata":
		recoverMetadata(args[1:])
	case "data":
		recoverData(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown recover command %q\n", args[0])
		os.Exit(2)
	}
}

type recoverS3Flags struct {
	bucket    string
	region    string
	key       string
	prefix    string
	profileID string
	endpoint  string
	accessKey string
	secretKey string
	pathStyle bool
	useSSL    bool
}

func registerRecoverS3Flags(fs *flag.FlagSet, opts *recoverS3Flags) {
	fs.StringVar(&opts.bucket, "s3-bucket", "", "S3 bucket containing metadata snapshots")
	fs.StringVar(&opts.region, "s3-region", envOr("AWS_REGION", "us-east-1"), "S3 region")
	fs.StringVar(&opts.key, "s3-key", "", "explicit metadata snapshot object key")
	fs.StringVar(&opts.prefix, "s3-prefix", "meta/", "S3 prefix used when --s3-key is empty")
	fs.StringVar(&opts.profileID, "profile-id", "", "restrict the metadata snapshot to one profile UUID")
	fs.StringVar(&opts.endpoint, "s3-endpoint", envOr("AWS_ENDPOINT_URL", ""), "S3-compatible endpoint")
	fs.StringVar(&opts.accessKey, "s3-access-key", envOr("AWS_ACCESS_KEY_ID", ""), "S3 access key (or AWS_ACCESS_KEY_ID)")
	fs.StringVar(&opts.secretKey, "s3-secret-key", envOr("AWS_SECRET_ACCESS_KEY", ""), "S3 secret key (or AWS_SECRET_ACCESS_KEY)")
	fs.BoolVar(&opts.pathStyle, "s3-path-style", false, "use path-style S3 addressing")
	fs.BoolVar(&opts.useSSL, "s3-use-ssl", true, "use https when the endpoint has no scheme")
}

func recoverMetadata(args []string) {
	fs := flag.NewFlagSet("recover metadata", flag.ExitOnError)
	configPath := fs.String("config", "", "path to config.yaml pointing at the metadata database to restore into")
	file := fs.String("file", "", "local metadata snapshot path")
	storagePath := fs.String("storage-path", "", "local storage root to search (default: storage.path from config)")
	listOnly := fs.Bool("list", false, "list available snapshots and exit")
	passphrase := fs.String("passphrase", "", "override the metadata backup passphrase")
	identity := fs.String("identity", "", "override the Age private key")
	replace := fs.Bool("replace", false, "restore over a non-empty metadata database")
	dryRun := fs.Bool("dry-run", false, "validate the snapshot without writing")
	timeout := fs.Duration("timeout", 10*time.Minute, "overall recovery timeout")
	var s3Opts recoverS3Flags
	registerRecoverS3Flags(fs, &s3Opts)
	_ = fs.Parse(args)

	cfg, err := config.LoadLenient(*configPath)
	if err != nil {
		fatal("Configuration error:\n%v", err)
	}
	log := logger.New(cfg.LogLevel)

	profileID := ""
	if s3Opts.profileID != "" {
		parsed, parseErr := pgutil.ParseUUID(s3Opts.profileID)
		if parseErr != nil || !parsed.Valid {
			fatal("Invalid --profile-id: %v", parseErr)
		}
		profileID = pgutil.UUIDToString(parsed)
	}

	snapshot := *file
	source := snapshot
	cleanup := func() {}
	if snapshot == "" {
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		defer cancel()

		switch {
		case s3Opts.bucket != "" || s3Opts.key != "":
			provider, providerErr := newRecoveryS3Provider(ctx, s3Opts)
			if providerErr != nil {
				fatal("S3 source error: %v", providerErr)
			}
			key := s3Opts.key
			if key == "" {
				prefix := s3Opts.prefix
				if profileID != "" {
					prefix = metadataPrefix(profileID)
				}
				keys, listErr := listMetadataSnapshots(ctx, provider, []string{prefix})
				if listErr != nil {
					fatal("List metadata snapshots: %v", listErr)
				}
				if *listOnly {
					printSnapshotCandidates(keys, "")
					return
				}
				key = latestSnapshotKey(keys)
				if key == "" {
					fatal("No metadata snapshots found under s3://%s/%s", s3Opts.bucket, prefix)
				}
			}
			if *listOnly {
				printSnapshotCandidates([]string{key}, key)
				return
			}
			snapshot, cleanup, err = downloadSnapshot(ctx, provider, key, cfg.TempDir)
			if err != nil {
				fatal("Download metadata snapshot: %v", err)
			}
			source = fmt.Sprintf("s3://%s/%s", s3Opts.bucket, key)

		default:
			root := *storagePath
			if root == "" {
				root = cfg.StoragePath
			}
			if root == "" {
				fatal("No local storage path available; pass --storage-path, --file or S3 flags")
			}
			provider, providerErr := local.New(root)
			if providerErr != nil {
				fatal("Local storage error: %v", providerErr)
			}
			prefixes := []string{"meta/", "metadata/"}
			if profileID != "" {
				prefixes = []string{metadataPrefix(profileID), "metadata/"}
			}
			keys, listErr := listMetadataSnapshots(ctx, provider, prefixes)
			if listErr != nil {
				fatal("List metadata snapshots: %v", listErr)
			}
			if *listOnly {
				printSnapshotCandidates(keys, root)
				return
			}
			key := latestSnapshotKey(keys)
			if key == "" {
				fatal("No metadata snapshots found under %s", root)
			}
			snapshot = filepath.Join(root, key)
			source = snapshot
		}
	}
	defer cleanup()

	if *listOnly {
		printSnapshotCandidates([]string{snapshot}, "")
		return
	}

	fmt.Printf("Metadata snapshot: %s\n", source)
	if err := metarestore.Run(log, cfg, metarestore.Options{
		File:       snapshot,
		Passphrase: *passphrase,
		Identity:   *identity,
		Replace:    *replace,
		DryRun:     *dryRun,
	}); err != nil {
		fatal("Metadata recovery failed: %v", err)
	}
	if *dryRun {
		fmt.Println("Metadata snapshot is valid; nothing was written")
		return
	}
	fmt.Println("Metadata recovery completed")
}

func recoverData(args []string) {
	fs := flag.NewFlagSet("recover data", flag.ExitOnError)
	configPath := fs.String("config", "", "path to config.yaml")
	executionID := fs.String("execution", "", "execution UUID to recover")
	profileID := fs.String("profile", "", "use the latest successful execution of this profile UUID")
	file := fs.String("file", "", "local backup artifact to recover")
	providerID := fs.String("provider-id", "", "optional storage provider UUID")
	targetDSN := fs.String("target-dsn", "", "target PostgreSQL DSN (default: target_database from config)")
	confirm := fs.Bool("confirm", false, "confirm applying the restore")
	verifyOnly := fs.Bool("verify-only", false, "download/decrypt/verify only")
	terminate := fs.Bool("terminate-connections", false, "terminate other target database connections")
	passphrase := fs.String("passphrase", "", "override encryption passphrase")
	identity := fs.String("identity", "", "override Age private key")
	keepArtifact := fs.Bool("keep-artifact", false, "keep the downloaded/decrypted artifact and print its path")
	timeout := fs.Duration("timeout", 30*time.Minute, "overall timeout")
	_ = fs.Parse(args)

	if *file != "" && (*executionID != "" || *profileID != "") {
		fatal("--file cannot be combined with --execution/--profile")
	}
	if *file == "" && *executionID == "" && *profileID == "" {
		fatal("Provide --execution, --profile or --file")
	}
	if *executionID != "" && *profileID != "" {
		fatal("Provide either --execution or --profile, not both")
	}
	if *file != "" && *providerID != "" {
		fatal("--provider-id cannot be used with --file")
	}
	if !*verifyOnly && !*confirm {
		fatal("Refusing to restore without --confirm (or use --verify-only)")
	}

	cfg, err := config.LoadLenient(*configPath)
	if err != nil {
		fatal("Configuration error:\n%v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	encPassphrase := *passphrase
	if encPassphrase == "" {
		encPassphrase = cfg.EncryptionPassphrase
	}
	encIdentity := *identity
	if encIdentity == "" {
		encIdentity = cfg.AgePrivateKey
	}

	var artifact *backup.Artifact
	if *file != "" {
		absolute, absErr := filepath.Abs(*file)
		if absErr != nil {
			fatal("Resolve file: %v", absErr)
		}
		provider, localErr := local.New(filepath.Dir(absolute))
		if localErr != nil {
			fatal("Local artifact source: %v", localErr)
		}
		artifact, err = backup.PrepareArtifact(ctx, provider, filepath.Base(absolute), cfg.TempDir, "", false, encPassphrase, encIdentity, recoveryLogf)
		if err != nil {
			fatal("Verify artifact: %v", err)
		}
		fmt.Printf("Artifact: %s\n", absolute)
	} else {
		pool, poolErr := database.New(ctx, cfg.MetadataDB)
		if poolErr != nil {
			fatal("Metadata database connection failed: %v", poolErr)
		}
		defer pool.Close()

		var encKey []byte
		if cfg.ConfigEncryptionKey != "" {
			encKey = crypto.DeriveConfigKey(cfg.ConfigEncryptionKey)
		}
		repo := repository.NewPostgres(pool, encKey)
		providers := service.NewProviderService(repo, cfg)

		execution, resolveErr := resolveRecoveryExecution(ctx, repo, *executionID, *profileID)
		if resolveErr != nil {
			fatal("Resolve execution: %v", resolveErr)
		}

		var requested pgtype.UUID
		if *providerID != "" {
			requested, err = pgutil.ParseUUID(*providerID)
			if err != nil || !requested.Valid {
				fatal("Invalid provider ID: %v", err)
			}
		}

		fmt.Printf("Execution: %s\n", executionLabel(execution))
		candidates, candidateErr := executionArtifactCandidates(ctx, repo, execution, requested)
		if candidateErr != nil {
			fatal("Resolve storage copies: %v", candidateErr)
		}
		if len(candidates) == 0 {
			fatal("Execution has no storage copy")
		}

		var failures []string
		for _, candidate := range candidates {
			fmt.Printf("Trying provider %s\n", pgutil.UUIDToString(candidate.providerID))
			candidateArtifact, prepErr := prepareCandidateArtifact(
				ctx, providers, cfg, execution, candidate, encPassphrase, encIdentity, recoveryLogf,
			)
			if prepErr != nil {
				failures = append(failures, fmt.Sprintf("%s: %v", pgutil.UUIDToString(candidate.providerID), prepErr))
				fmt.Fprintf(os.Stderr, "  provider %s failed: %v\n", pgutil.UUIDToString(candidate.providerID), prepErr)
				continue
			}
			artifact = candidateArtifact
			fmt.Printf("Using provider %s path=%s\n", pgutil.UUIDToString(candidate.providerID), candidate.storagePath)
			break
		}
		if artifact == nil {
			fatal("No usable backup artifact:\n  %s", strings.Join(failures, "\n  "))
		}
	}
	if *keepArtifact {
		fmt.Printf("Artifact kept at %s\n", artifact.Path)
	} else {
		defer artifact.Cleanup()
	}

	if artifact.ChecksumVerified {
		fmt.Println("Checksum: verified")
	} else {
		fmt.Println("Checksum: not recorded/verified")
	}

	if *verifyOnly {
		fmt.Println("Verification completed; target database untouched")
		return
	}

	target, err := restoreTargetForRecovery(*targetDSN, cfg)
	if err != nil {
		fatal("Target database: %v", err)
	}
	if err := target.Preflight(ctx, *terminate); err != nil {
		fatal("Target preflight failed: %v", err)
	}
	output, err := backup.ApplyArtifact(ctx, artifact, target, true)
	if err != nil {
		fatal("Restore failed: %v\n%s", err, output)
	}
	fmt.Printf("Restore completed into %s\n", target.DBName)
}

func resolveRecoveryExecution(ctx context.Context, repo repository.Repository, executionID, profileID string) (db.Execution, error) {
	if executionID != "" {
		id, err := pgutil.ParseUUID(executionID)
		if err != nil || !id.Valid {
			return db.Execution{}, fmt.Errorf("invalid execution ID: %w", err)
		}
		execution, err := repo.GetExecution(ctx, id)
		if err != nil {
			return db.Execution{}, fmt.Errorf("execution not found: %w", err)
		}
		return execution, nil
	}

	id, err := pgutil.ParseUUID(profileID)
	if err != nil || !id.Valid {
		return db.Execution{}, fmt.Errorf("invalid profile ID: %w", err)
	}
	executions, err := repo.ListExecutionsByProfile(ctx, id)
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

func newRecoveryS3Provider(ctx context.Context, opts recoverS3Flags) (*s3.S3Provider, error) {
	if opts.bucket == "" {
		return nil, fmt.Errorf("--s3-bucket is required when --s3-key is not provided")
	}
	if opts.accessKey == "" || opts.secretKey == "" {
		return nil, fmt.Errorf("S3 credentials are required (--s3-access-key/--s3-secret-key or AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY)")
	}
	return s3.New(ctx, s3.Options{
		Region:         opts.region,
		Bucket:         opts.bucket,
		AccessKey:      opts.accessKey,
		SecretKey:      opts.secretKey,
		Endpoint:       opts.endpoint,
		ForcePathStyle: opts.pathStyle,
		UseSSL:         opts.useSSL,
	})
}

func metadataPrefix(profileID string) string {
	return "meta/" + profileID + "/"
}

// listMetadataSnapshots lists every metadata dump under the given prefixes and
// keeps only snapshot objects. Local keys use os separators; callers normalize
// them when joining the storage root.
func listMetadataSnapshots(ctx context.Context, provider storage.StorageProvider, prefixes []string) ([]string, error) {
	seen := make(map[string]bool)
	var snapshots []string
	for _, prefix := range prefixes {
		keys, err := listSnapshotKeys(ctx, provider, prefix)
		if err != nil {
			return nil, fmt.Errorf("list %s: %w", prefix, err)
		}
		for _, key := range keys {
			if !isMetadataSnapshotKey(key) || seen[key] {
				continue
			}
			seen[key] = true
			snapshots = append(snapshots, key)
		}
	}
	sort.Strings(snapshots)
	return snapshots, nil
}

type recursiveLister interface {
	ListRecursive(ctx context.Context, prefix string) ([]string, error)
}

func listSnapshotKeys(ctx context.Context, provider storage.StorageProvider, prefix string) ([]string, error) {
	if lister, ok := provider.(recursiveLister); ok {
		return lister.ListRecursive(ctx, prefix)
	}
	return provider.List(ctx, prefix)
}

func isMetadataSnapshotKey(key string) bool {
	base := path.Base(key)
	if !strings.HasPrefix(base, "metadata-") {
		return false
	}
	return strings.HasSuffix(base, ".dump.age") || strings.HasSuffix(base, ".dump")
}

func latestSnapshotKey(keys []string) string {
	var candidates []string
	for _, key := range keys {
		if isMetadataSnapshotKey(key) {
			candidates = append(candidates, key)
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	sort.Slice(candidates, func(i, j int) bool {
		left, right := path.Base(candidates[i]), path.Base(candidates[j])
		if left == right {
			return candidates[i] > candidates[j]
		}
		return left < right
	})
	return candidates[len(candidates)-1]
}

func printSnapshotCandidates(keys []string, root string) {
	if len(keys) == 0 {
		fmt.Println("No metadata snapshots found")
		return
	}
	latest := latestSnapshotKey(keys)
	for _, key := range keys {
		display := key
		if root != "" {
			display = filepath.Join(root, key)
		}
		marker := " "
		if key == latest {
			marker = "*"
		}
		fmt.Printf("%s %s\n", marker, display)
	}
}

func downloadSnapshot(ctx context.Context, provider storage.StorageProvider, key, tempDir string) (string, func(), error) {
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	f, err := os.CreateTemp(tempDir, "metadata-recover-*")
	if err != nil {
		return "", nil, err
	}
	name := f.Name()
	cleanup := func() { _ = os.Remove(name) }

	reader, err := provider.Download(ctx, key)
	if err != nil {
		_ = f.Close()
		cleanup()
		return "", nil, err
	}
	defer func() { _ = reader.Close() }()

	if _, err := io.Copy(f, reader); err != nil {
		_ = f.Close()
		cleanup()
		return "", nil, err
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", nil, err
	}
	if err := os.Chmod(name, 0o600); err != nil {
		cleanup()
		return "", nil, err
	}
	return name, cleanup, nil
}

func restoreTargetForRecovery(dsn string, cfg config.Config) (backup.RestoreTarget, error) {
	var target backup.RestoreTarget
	if dsn != "" {
		parsed, err := parseScratchTarget(dsn)
		if err != nil {
			return backup.RestoreTarget{}, err
		}
		target = parsed
	} else {
		target = backup.RestoreTarget{
			Host:     cfg.TargetDB.Host,
			Port:     cfg.TargetDB.Port,
			User:     cfg.TargetDB.User,
			Password: cfg.TargetDB.Password,
			DBName:   cfg.TargetDB.DBName,
			SSLMode:  cfg.TargetDB.SSLMode,
		}
		if cfg.TargetDB.RestoreUser != "" {
			target.User = cfg.TargetDB.RestoreUser
			target.Password = cfg.TargetDB.RestorePassword
		}
		if target.Host == "" || target.DBName == "" {
			return backup.RestoreTarget{}, fmt.Errorf("target database is not configured; use --target-dsn")
		}
	}

	if sameDatabaseTarget(target, cfg.MetadataDB) {
		return backup.RestoreTarget{}, fmt.Errorf("refusing to restore into the metadata database %q", target.DBName)
	}
	return target, nil
}

func sameDatabaseTarget(target backup.RestoreTarget, metadata config.DatabaseConfig) bool {
	return target.DBName != "" &&
		strings.EqualFold(target.Host, metadata.Host) &&
		target.Port == metadata.Port &&
		strings.EqualFold(target.DBName, metadata.DBName)
}

func recoveryLogf(format string, args ...any) {
	fmt.Printf("  %s\n", fmt.Sprintf(format, args...))
}
