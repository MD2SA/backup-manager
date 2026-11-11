package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/MD2SA/backup-manager/internal/config"
	"github.com/MD2SA/backup-manager/internal/recovery"
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

func (f recoverS3Flags) options() recovery.S3Options {
	return recovery.S3Options{
		Bucket:    f.bucket,
		Region:    f.region,
		Key:       f.key,
		Prefix:    f.prefix,
		Endpoint:  f.endpoint,
		AccessKey: f.accessKey,
		SecretKey: f.secretKey,
		PathStyle: f.pathStyle,
		UseSSL:    f.useSSL,
	}
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

	result, err := recovery.Metadata(context.Background(), cfg, recovery.MetadataOptions{
		File:        *file,
		StoragePath: *storagePath,
		ProfileID:   s3Opts.profileID,
		List:        *listOnly,
		Passphrase:  *passphrase,
		Identity:    *identity,
		Replace:     *replace,
		DryRun:      *dryRun,
		Timeout:     *timeout,
		S3:          s3Opts.options(),
		Progressf: func(format string, args ...any) {
			fmt.Printf(format+"\n", args...)
		},
	})
	if err != nil {
		fatal("Metadata recovery failed: %v", err)
	}
	if *listOnly {
		printMetadataSnapshots(result)
		return
	}

	if result.DryRun {
		fmt.Println("Metadata snapshot is valid; nothing was written")
		return
	}
	fmt.Println("Metadata recovery completed")
}

func printMetadataSnapshots(result recovery.MetadataResult) {
	if len(result.Snapshots) == 0 {
		fmt.Println("No metadata snapshots found")
		return
	}
	for _, key := range result.Snapshots {
		display := key
		if result.Root != "" {
			display = filepath.Join(result.Root, key)
		}
		marker := " "
		if key == result.Latest {
			marker = "*"
		}
		fmt.Printf("%s %s\n", marker, display)
	}
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

	cfg, err := config.LoadLenient(*configPath)
	if err != nil {
		fatal("Configuration error:\n%v", err)
	}

	result, err := recovery.Data(context.Background(), cfg, recovery.DataOptions{
		ExecutionID:          *executionID,
		ProfileID:            *profileID,
		File:                 *file,
		ProviderID:           *providerID,
		TargetDSN:            *targetDSN,
		Confirm:              *confirm,
		VerifyOnly:           *verifyOnly,
		TerminateConnections: *terminate,
		Passphrase:           *passphrase,
		Identity:             *identity,
		KeepArtifact:         *keepArtifact,
		Timeout:              *timeout,
		Logf: func(format string, args ...any) {
			fmt.Printf("  %s\n", fmt.Sprintf(format, args...))
		},
		Progressf: func(format string, args ...any) {
			fmt.Printf(format+"\n", args...)
		},
	})
	if err != nil {
		fatal("%v", err)
	}

	if result.ArtifactSource != "" {
		fmt.Printf("Artifact: %s\n", result.ArtifactSource)
	}
	if *keepArtifact {
		fmt.Printf("Artifact kept at %s\n", result.ArtifactPath)
	}
	if result.ChecksumVerified {
		fmt.Println("Checksum: verified")
	} else {
		fmt.Println("Checksum: not recorded/verified")
	}
	if *verifyOnly {
		fmt.Println("Verification completed; target database untouched")
		return
	}
	if result.Applied {
		fmt.Printf("Restore completed into %s\n", result.TargetDatabase)
	}
}
