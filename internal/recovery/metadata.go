package recovery

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/MD2SA/backup-manager/internal/config"
	"github.com/MD2SA/backup-manager/internal/logger"
	"github.com/MD2SA/backup-manager/internal/metarestore"
	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/MD2SA/backup-manager/internal/storage"
	"github.com/MD2SA/backup-manager/internal/storage/local"
	"github.com/MD2SA/backup-manager/internal/storage/s3"
)

type metadataDeps struct {
	newS3   func(context.Context, S3Options) (storage.StorageProvider, error)
	restore func(*slog.Logger, config.Config, metarestore.Options) error
}

func defaultMetadataDeps() metadataDeps {
	return metadataDeps{
		newS3:   newS3Provider,
		restore: metarestore.Run,
	}
}

// Metadata discovers or restores a metadata snapshot. It never writes output;
// callers decide how to present the result.
func Metadata(ctx context.Context, cfg config.Config, opts MetadataOptions) (MetadataResult, error) {
	return runMetadata(ctx, cfg, opts, defaultMetadataDeps())
}

func runMetadata(ctx context.Context, cfg config.Config, opts MetadataOptions, deps metadataDeps) (MetadataResult, error) {
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	profileID, err := normalizeProfileID(opts.ProfileID)
	if err != nil {
		return MetadataResult{}, err
	}
	progress := opts.Progressf
	if progress == nil {
		progress = func(string, ...any) {}
	}

	if opts.File != "" {
		result := MetadataResult{
			Source:    opts.File,
			Snapshots: []string{opts.File},
			Latest:    opts.File,
		}
		if opts.List {
			return result, nil
		}
		progress("Metadata snapshot: %s", opts.File)
		if err := deps.restore(logger.New(cfg.LogLevel), cfg, metarestore.Options{
			File:       opts.File,
			Passphrase: opts.Passphrase,
			Identity:   opts.Identity,
			Replace:    opts.Replace,
			DryRun:     opts.DryRun,
		}); err != nil {
			return result, err
		}
		result.DryRun = opts.DryRun
		return result, nil
	}

	provider, prefixes, root, s3Source, err := metadataSource(ctx, cfg, opts, profileID, deps)
	if err != nil {
		return MetadataResult{}, err
	}

	// An explicit S3 key is used as-is; no listing is needed.
	if s3Source && opts.S3.Key != "" {
		result := MetadataResult{
			Source:    fmt.Sprintf("s3://%s/%s", opts.S3.Bucket, opts.S3.Key),
			Snapshots: []string{opts.S3.Key},
			Latest:    opts.S3.Key,
		}
		if opts.List {
			return result, nil
		}
		progress("Metadata snapshot: %s", result.Source)
		snapshot, cleanup, err := downloadSnapshot(ctx, provider, opts.S3.Key, cfg.TempDir)
		if err != nil {
			return result, err
		}
		defer cleanup()
		if err := deps.restore(logger.New(cfg.LogLevel), cfg, metarestore.Options{
			File:       snapshot,
			Passphrase: opts.Passphrase,
			Identity:   opts.Identity,
			Replace:    opts.Replace,
			DryRun:     opts.DryRun,
		}); err != nil {
			return result, err
		}
		result.DryRun = opts.DryRun
		return result, nil
	}

	keys, err := FindMetadataSnapshots(ctx, provider, prefixes)
	if err != nil {
		return MetadataResult{}, err
	}
	result := MetadataResult{
		Snapshots: keys,
		Latest:    LatestSnapshotKey(keys),
		Root:      root,
	}
	if opts.List {
		return result, nil
	}
	if result.Latest == "" {
		if s3Source {
			return result, fmt.Errorf("no metadata snapshots found under s3://%s/%s", opts.S3.Bucket, prefixes[0])
		}
		return result, fmt.Errorf("no metadata snapshots found under %s", root)
	}

	var snapshot string
	if s3Source {
		downloaded, cleanup, err := downloadSnapshot(ctx, provider, result.Latest, cfg.TempDir)
		if err != nil {
			return result, err
		}
		defer cleanup()
		snapshot = downloaded
		result.Source = fmt.Sprintf("s3://%s/%s", opts.S3.Bucket, result.Latest)
	} else {
		snapshot = filepath.Join(root, result.Latest)
		result.Source = snapshot
	}

	progress("Metadata snapshot: %s", result.Source)
	if err := deps.restore(logger.New(cfg.LogLevel), cfg, metarestore.Options{
		File:       snapshot,
		Passphrase: opts.Passphrase,
		Identity:   opts.Identity,
		Replace:    opts.Replace,
		DryRun:     opts.DryRun,
	}); err != nil {
		return result, err
	}
	result.DryRun = opts.DryRun
	return result, nil
}

func metadataSource(
	ctx context.Context,
	cfg config.Config,
	opts MetadataOptions,
	profileID string,
	deps metadataDeps,
) (storage.StorageProvider, []string, string, bool, error) {
	if opts.S3.Bucket != "" || opts.S3.Key != "" {
		provider, err := deps.newS3(ctx, opts.S3)
		if err != nil {
			return nil, nil, "", true, err
		}
		prefix := opts.S3.Prefix
		if prefix == "" {
			prefix = "meta/"
		}
		if profileID != "" {
			prefix = metadataPrefix(profileID)
		}
		return provider, []string{prefix}, "", true, nil
	}

	root := opts.StoragePath
	if root == "" {
		root = cfg.StoragePath
	}
	if root == "" {
		return nil, nil, "", false, fmt.Errorf("no local storage path available; pass --storage-path, --file or S3 flags")
	}
	provider, err := local.New(root)
	if err != nil {
		return nil, nil, "", false, err
	}

	prefixes := []string{"meta/", "metadata/"}
	if profileID != "" {
		prefixes = []string{metadataPrefix(profileID), "metadata/"}
	}
	return provider, prefixes, root, false, nil
}

// FindMetadataSnapshots lists and deduplicates metadata dumps under prefixes.
func FindMetadataSnapshots(ctx context.Context, provider storage.StorageProvider, prefixes []string) ([]string, error) {
	seen := make(map[string]bool)
	var snapshots []string
	for _, prefix := range prefixes {
		keys, err := listSnapshotKeys(ctx, provider, prefix)
		if err != nil {
			return nil, fmt.Errorf("list %s: %w", prefix, err)
		}
		for _, key := range keys {
			if !IsMetadataSnapshotKey(key) || seen[key] {
				continue
			}
			seen[key] = true
			snapshots = append(snapshots, key)
		}
	}
	sort.Strings(snapshots)
	return snapshots, nil
}

// IsMetadataSnapshotKey reports whether a key is a metadata dump snapshot.
func IsMetadataSnapshotKey(key string) bool {
	base := path.Base(key)
	if !strings.HasPrefix(base, "metadata-") {
		return false
	}
	return strings.HasSuffix(base, ".dump.age") || strings.HasSuffix(base, ".dump")
}

// LatestSnapshotKey returns the newest snapshot. Timestamps live in the base
// name, so ordering by base name (and then by full key) avoids comparing
// unrelated directory prefixes.
func LatestSnapshotKey(keys []string) string {
	var candidates []string
	for _, key := range keys {
		if IsMetadataSnapshotKey(key) {
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

type recursiveLister interface {
	ListRecursive(ctx context.Context, prefix string) ([]string, error)
}

func listSnapshotKeys(ctx context.Context, provider storage.StorageProvider, prefix string) ([]string, error) {
	if lister, ok := provider.(recursiveLister); ok {
		return lister.ListRecursive(ctx, prefix)
	}
	return provider.List(ctx, prefix)
}

func metadataPrefix(profileID string) string {
	return "meta/" + profileID + "/"
}

func normalizeProfileID(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	parsed, err := pgutil.ParseUUID(raw)
	if err != nil {
		return "", fmt.Errorf("invalid profile ID: %w", err)
	}
	if !parsed.Valid {
		return "", fmt.Errorf("invalid profile ID %q", raw)
	}
	return pgutil.UUIDToString(parsed), nil
}

func newS3Provider(ctx context.Context, opts S3Options) (storage.StorageProvider, error) {
	if opts.Bucket == "" {
		return nil, fmt.Errorf("--s3-bucket is required when --s3-key is not provided")
	}
	if opts.AccessKey == "" || opts.SecretKey == "" {
		return nil, fmt.Errorf("S3 credentials are required (--s3-access-key/--s3-secret-key or AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY)")
	}
	return s3.New(ctx, s3.Options{
		Region:         opts.Region,
		Bucket:         opts.Bucket,
		AccessKey:      opts.AccessKey,
		SecretKey:      opts.SecretKey,
		Endpoint:       opts.Endpoint,
		ForcePathStyle: opts.PathStyle,
		UseSSL:         opts.UseSSL,
	})
}

func downloadSnapshot(ctx context.Context, provider storage.StorageProvider, key, tempDir string) (string, func(), error) {
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	if err := os.MkdirAll(tempDir, 0o700); err != nil {
		return "", nil, fmt.Errorf("create recovery temp directory: %w", err)
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
