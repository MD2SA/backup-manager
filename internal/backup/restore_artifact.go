package backup

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/MD2SA/backup-manager/internal/pkg/crypto"
	"github.com/MD2SA/backup-manager/internal/storage"
)

// Artifact is a downloaded, decrypted and verified backup file ready to be
// applied to the target database.
type Artifact struct {
	Path             string
	Gzip             bool
	CustomFormat     bool
	ChecksumVerified bool

	tempPaths []string
}

func (a *Artifact) Cleanup() {
	for _, path := range a.tempPaths {
		_ = os.Remove(path)
	}
}

// Open returns the artifact stream, transparently decompressing gzip.
func (a *Artifact) Open() (io.ReadCloser, error) {
	f, err := os.Open(a.Path)
	if err != nil {
		return nil, err
	}
	if !a.Gzip {
		return f, nil
	}

	gz, err := gzip.NewReader(f)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("open gzip artifact: %w", err)
	}
	return &readerCloser{
		Reader: gz,
		close: func() error {
			_ = gz.Close()
			return f.Close()
		},
	}, nil
}

type readerCloser struct {
	io.Reader
	close func() error
}

func (r *readerCloser) Close() error {
	return r.close()
}

// PrepareArtifact downloads an execution artifact, decrypts it when needed,
// verifies its checksum and detects its on-disk format.
//
// expectedChecksum is the SHA-256 of the pre-encryption artifact (the value
// stored by the backup verification stage). When empty (legacy executions),
// verification is skipped and logged.
func PrepareArtifact(
	ctx context.Context,
	provider storage.StorageProvider,
	storagePath string,
	tempDir string,
	expectedChecksum string,
	isEncrypted bool,
	passphrase string,
	identity string,
	logf func(format string, args ...any),
) (_ *Artifact, err error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	if err := os.MkdirAll(tempDir, 0o700); err != nil {
		return nil, fmt.Errorf("create restore temp directory: %w", err)
	}

	base, err := os.CreateTemp(tempDir, "backup-manager-restore-*")
	if err != nil {
		return nil, fmt.Errorf("create restore temp file: %w", err)
	}
	downloaded := base.Name()
	_ = base.Close()

	cleanup := []string{downloaded}
	defer func() {
		if err != nil {
			for _, path := range cleanup {
				_ = os.Remove(path)
			}
		}
	}()

	if err := downloadToFile(ctx, provider, storagePath, downloaded); err != nil {
		return nil, err
	}

	ageHeader, err := hasAgeHeader(downloaded)
	if err != nil {
		return nil, err
	}
	if isEncrypted && !ageHeader {
		return nil, fmt.Errorf("execution is marked encrypted but the artifact is not an Age file")
	}
	if !isEncrypted && ageHeader {
		logf("artifact is Age-encrypted but the execution flag is false; decrypting anyway")
		isEncrypted = true
	}

	finalPath := downloaded
	if isEncrypted {
		decrypted := downloaded + ".dec"
		cleanup = append(cleanup, decrypted)

		method, decErr := decryptArtifact(downloaded, decrypted, passphrase, identity)
		if decErr != nil {
			return nil, decErr
		}
		logf("decrypted artifact using %s", method)
		finalPath = decrypted

		// The encrypted copy is no longer needed.
		_ = os.Remove(downloaded)
		cleanup = removePath(cleanup, downloaded)
	}

	checksumVerified := false
	if expectedChecksum != "" {
		actual, hashErr := sha256File(finalPath)
		if hashErr != nil {
			return nil, fmt.Errorf("checksum artifact: %w", hashErr)
		}
		if !strings.EqualFold(actual, normalizeChecksum(expectedChecksum)) {
			return nil, fmt.Errorf("checksum mismatch: expected %s, got %s", normalizeChecksum(expectedChecksum), actual)
		}
		checksumVerified = true
		logf("checksum verified (sha256)")
	} else {
		logf("execution has no recorded checksum; skipping integrity verification")
	}

	gzipFormat, customFormat, err := detectArtifactFormat(finalPath)
	if err != nil {
		return nil, err
	}
	if gzipFormat {
		logf("artifact is gzip-compressed")
	}
	if customFormat {
		logf("artifact is a PostgreSQL custom-format archive")
	}

	return &Artifact{
		Path:             finalPath,
		Gzip:             gzipFormat,
		CustomFormat:     customFormat,
		ChecksumVerified: checksumVerified,
		tempPaths:        cleanup,
	}, nil
}

func downloadToFile(ctx context.Context, provider storage.StorageProvider, key, path string) error {
	reader, err := provider.Download(ctx, key)
	if err != nil {
		return fmt.Errorf("download %s: %w", key, err)
	}
	defer func() { _ = reader.Close() }()

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open restore temp file: %w", err)
	}
	if _, err := io.Copy(f, reader); err != nil {
		_ = f.Close()
		return fmt.Errorf("download %s: %w", key, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close restore temp file: %w", err)
	}
	return nil
}

func decryptArtifact(src, dst, passphrase, identity string) (string, error) {
	type attempt struct {
		name string
		key  string
		fn   func(string, string, string) error
	}

	var attempts []attempt
	if passphrase != "" {
		attempts = append(attempts, attempt{name: "passphrase", key: passphrase, fn: crypto.DecryptWithPassphrase})
	}
	if identity != "" {
		attempts = append(attempts, attempt{name: "age identity", key: identity, fn: crypto.DecryptWithAge})
	}
	if len(attempts) == 0 {
		return "", errors.New("artifact is encrypted but no encryption passphrase or Age private key is configured")
	}

	var errs []error
	for _, a := range attempts {
		tmp := dst + "." + a.name
		_ = os.Remove(tmp)
		if err := a.fn(src, tmp, a.key); err != nil {
			_ = os.Remove(tmp)
			errs = append(errs, fmt.Errorf("%s: %w", a.name, err))
			continue
		}
		if err := os.Rename(tmp, dst); err != nil {
			_ = os.Remove(tmp)
			return "", fmt.Errorf("finalize decrypted artifact: %w", err)
		}
		return a.name, nil
	}

	return "", fmt.Errorf("decryption failed (wrong passphrase or identity?): %w", errors.Join(errs...))
}

func hasAgeHeader(path string) (bool, error) {
	header, err := readHeader(path, 32)
	if err != nil {
		return false, err
	}
	return bytes.HasPrefix(header, []byte("age-encryption.org/v1")), nil
}

func detectArtifactFormat(path string) (gzipFormat bool, customFormat bool, err error) {
	header, err := readHeader(path, 6)
	if err != nil {
		return false, false, err
	}
	if len(header) >= 2 && header[0] == 0x1f && header[1] == 0x8b {
		gzipFormat = true
		// A custom-format archive can itself be gzip-compressed; peek inside.
		f, openErr := os.Open(path)
		if openErr != nil {
			return false, false, openErr
		}
		defer func() { _ = f.Close() }()
		gz, gzErr := gzip.NewReader(f)
		if gzErr != nil {
			return false, false, fmt.Errorf("inspect gzip artifact: %w", gzErr)
		}
		defer func() { _ = gz.Close() }()
		inner := make([]byte, 5)
		if _, readErr := io.ReadFull(gz, inner); readErr == nil && string(inner) == "PGDMP" {
			customFormat = true
		}
		return gzipFormat, customFormat, nil
	}
	return false, len(header) >= 5 && string(header[:5]) == "PGDMP", nil
}

func readHeader(path string, n int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read artifact header: %w", err)
	}
	defer func() { _ = f.Close() }()

	header := make([]byte, n)
	read, err := io.ReadFull(f, header)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, fmt.Errorf("read artifact header: %w", err)
	}
	return header[:read], nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func normalizeChecksum(value string) string {
	out := make([]byte, 0, len(value))
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			continue
		}
		out = append(out, c)
	}
	return string(out)
}

func removePath(paths []string, target string) []string {
	out := paths[:0]
	for _, path := range paths {
		if path != target {
			out = append(out, path)
		}
	}
	return out
}
