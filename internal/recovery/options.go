package recovery

import "time"

// S3Options describes an S3 or S3-compatible snapshot source.
type S3Options struct {
	Bucket    string
	Region    string
	Key       string
	Prefix    string
	Endpoint  string
	AccessKey string
	SecretKey string
	PathStyle bool
	UseSSL    bool
}

// MetadataOptions controls metadata recovery.
type MetadataOptions struct {
	// File is an explicit local snapshot. When empty, StoragePath or S3 is
	// searched for the newest snapshot.
	File string
	// StoragePath overrides the local storage root (default: config storage.path).
	StoragePath string
	// ProfileID restricts discovery to meta/<profile-id>/.
	ProfileID string
	// List returns the available snapshots without restoring anything.
	List bool
	// Passphrase overrides the metadata backup passphrase.
	Passphrase string
	// Identity overrides the Age private key.
	Identity string
	// Replace allows restoring over a non-empty metadata database.
	Replace bool
	// DryRun validates the snapshot without writing to the database.
	DryRun bool
	// Timeout bounds the whole operation. Defaults to 10 minutes.
	Timeout time.Duration
	// S3 selects S3/S3-compatible discovery when Bucket or Key is set.
	S3 S3Options
	// Progressf receives high-level progress messages before restoring.
	Progressf func(format string, args ...any)
}

// MetadataResult describes the snapshot that was discovered or restored.
type MetadataResult struct {
	// Source is a local path or s3://bucket/key.
	Source string
	// Snapshots contains every matching key, relative to the storage root.
	Snapshots []string
	// Latest is the newest matching key.
	Latest string
	// Root is the local storage root used for display, when applicable.
	Root string
	// DryRun reports whether the snapshot was only validated.
	DryRun bool
}

// DataOptions controls data recovery.
type DataOptions struct {
	// ExecutionID selects a specific execution.
	ExecutionID string
	// ProfileID selects the latest successful execution of a profile.
	ProfileID string
	// File selects a local artifact and skips the metadata database.
	File string
	// ProviderID restricts recovery to one storage provider.
	ProviderID string
	// TargetDSN overrides target_database from the config.
	TargetDSN string
	// Confirm is required for a real restore.
	Confirm bool
	// VerifyOnly downloads, decrypts and verifies without applying.
	VerifyOnly bool
	// TerminateConnections disconnects active target clients during preflight.
	TerminateConnections bool
	// Passphrase overrides the encryption passphrase.
	Passphrase string
	// Identity overrides the Age private key.
	Identity string
	// KeepArtifact leaves the downloaded/decrypted artifact on disk.
	KeepArtifact bool
	// Timeout bounds the whole operation. Defaults to 30 minutes.
	Timeout time.Duration
	// Logf receives artifact preparation messages (already indented by callers).
	Logf func(format string, args ...any)
	// Progressf receives high-level progress messages.
	Progressf func(format string, args ...any)
}

// DataResult describes the recovered artifact and target.
type DataResult struct {
	ExecutionID        string
	ExecutionStatus    string
	ExecutionCreatedAt time.Time
	ProviderID         string
	StoragePath        string
	ArtifactSource     string
	ArtifactPath       string
	ChecksumVerified   bool
	Applied            bool
	TargetDatabase     string
}
