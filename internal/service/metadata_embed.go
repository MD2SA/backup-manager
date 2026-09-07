package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/MD2SA/backup-manager/internal/pkg/crypto"
	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/MD2SA/backup-manager/internal/selfbackup"
	"github.com/MD2SA/backup-manager/internal/storage"
)

// metadataKeyPrefix is the root key namespace for embedded metadata snapshots.
// Per-profile objects live under meta/<profile-id>/ so retention is isolated
// per profile even when profiles share the same provider.
const metadataKeyPrefix = "meta"

// metadataEncryption describes how the metadata dump must be encrypted before upload.
type metadataEncryption struct {
	mode string // "", "passphrase", or "age"
	key  string
}

// chooseMetadataEncryption applies the encryption chain for the embedded metadata:
// first the profile's own backup keys, then APP_METADATA_BACKUP_PASSPHRASE.
// An empty mode means the metadata must NOT be uploaded (never plaintext).
func chooseMetadataEncryption(profileEncrypted bool, encPassphrase, agePublicKey, metaPassphrase string) metadataEncryption {
	if profileEncrypted && encPassphrase != "" {
		return metadataEncryption{mode: "passphrase", key: encPassphrase}
	}
	if profileEncrypted && agePublicKey != "" {
		return metadataEncryption{mode: "age", key: agePublicKey}
	}
	if metaPassphrase != "" {
		return metadataEncryption{mode: "passphrase", key: metaPassphrase}
	}
	return metadataEncryption{}
}

// metadataKeysToDelete returns the keys that exceed the retention limit.
// Keys are expected to share the fixed meta/<profile-id>/ prefix, so the lexical
// order already reflects chronological order (YYYYMMDD-HHMMSS).
func metadataKeysToDelete(keys []string, retention int) []string {
	if retention < 0 {
		retention = 0
	}
	if len(keys) <= retention {
		return nil
	}
	sorted := append([]string(nil), keys...)
	sort.Strings(sorted)
	// Keep the N newest (lexically greatest); delete the oldest ones.
	return sorted[:len(sorted)-retention]
}

// embedMetadata snapshots the metadata database and uploads it, encrypted, to
// every storage provider of the profile. Its purpose is disaster recovery: the
// management state rides along with the normal backups.
//
// Failures here are logged and NEVER propagate: they must not fail the main
// backup execution.
func (s *BackupService) embedMetadata(ctx context.Context, profile repository.ProfileFull) {
	profileKey := pgutil.UUIDToString(profile.ID)
	ts := time.Now().Format("20060102-150405")
	base := filepath.Join(s.config.TempDir, fmt.Sprintf("metadata-%s-%s.dump", ts, pgutil.UUIDToString(profile.ID)))
	defer func() {
		_ = os.Remove(base)
		_ = os.Remove(base + ".age")
	}()

	if err := selfbackup.DumpFile(s.config.MetadataDB, base); err != nil {
		s.logger.Warn("Metadata embed skipped: dump failed", "profile_id", profileKey, "error", err)
		return
	}

	enc := chooseMetadataEncryption(
		profile.EncryptionEnabled,
		s.config.EncryptionPassphrase,
		s.config.AgePublicKey,
		s.config.MetadataBackupPassphrase,
	)

	key := fmt.Sprintf("%s/%s/metadata-%s.dump", metadataKeyPrefix, profileKey, ts)

	if enc.mode == "" {
		s.logger.Warn("Metadata embed skipped: no encryption available (enable profile encryption or set APP_METADATA_BACKUP_PASSPHRASE)", "profile_id", profileKey)
		return
	}

	key += ".age"
	switch enc.mode {
	case "age":
		if err := crypto.EncryptWithAge(base, base+".age", enc.key); err != nil {
			s.logger.Warn("Metadata embed skipped: encryption failed", "profile_id", profileKey, "mode", "age", "error", err)
			return
		}
	default:
		if err := crypto.EncryptWithPassphrase(base, base+".age", enc.key); err != nil {
			s.logger.Warn("Metadata embed skipped: encryption failed", "profile_id", profileKey, "mode", "passphrase", "error", err)
			return
		}
	}

	uploaded := false
	for _, sID := range profile.StorageProviderIDs {
		provider, err := s.providerService.ResolveStorageProvider(ctx, sID)
		if err != nil {
			s.logger.Warn("Metadata embed upload failed: failed to resolve provider", "profile_id", profileKey, "provider_id", pgutil.UUIDToString(sID), "error", err)
			continue
		}
		if err := uploadLocalFile(ctx, provider, key, base+".age"); err != nil {
			s.logger.Warn("Metadata embed upload failed", "profile_id", profileKey, "provider_id", pgutil.UUIDToString(sID), "key", key, "error", err)
			continue
		}
		uploaded = true
		if err := s.pruneMetadataKeys(ctx, provider, profileKey); err != nil {
			s.logger.Warn("Metadata embed prune failed", "profile_id", profileKey, "provider_id", pgutil.UUIDToString(sID), "error", err)
		}
	}

	if !uploaded {
		s.logger.Warn("Metadata embed: no provider accepted the metadata snapshot", "profile_id", profileKey)
		return
	}

	s.logger.Info("Metadata embedded", "profile_id", profileKey, "key", key, "encryption", enc.mode, "providers", len(profile.StorageProviderIDs))
}

// pruneMetadataKeys removes embedded metadata objects for a single
// profile/provider pair that exceed MetadataBackupRetention.
func (s *BackupService) pruneMetadataKeys(ctx context.Context, provider storage.StorageProvider, profileKey string) error {
	prefix := fmt.Sprintf("%s/%s/", metadataKeyPrefix, profileKey)
	keys, err := provider.List(ctx, prefix)
	if err != nil {
		return fmt.Errorf("list metadata keys: %w", err)
	}

	for _, key := range metadataKeysToDelete(keys, s.config.MetadataBackupRetention) {
		if err := provider.Delete(ctx, key); err != nil {
			return fmt.Errorf("delete %s: %w", key, err)
		}
	}

	return nil
}

func uploadLocalFile(ctx context.Context, provider storage.StorageProvider, key, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	if err := provider.Upload(ctx, key, f); err != nil {
		return err
	}

	return nil
}
