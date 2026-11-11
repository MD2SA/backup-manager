package backup

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"strings"
	"testing"

	appcrypto "github.com/MD2SA/backup-manager/internal/pkg/crypto"
)

type memoryStorage struct {
	data map[string][]byte
}

func newMemoryStorage() *memoryStorage {
	return &memoryStorage{data: make(map[string][]byte)}
}

func (m *memoryStorage) Name() string { return "memory" }

func (m *memoryStorage) Upload(_ context.Context, key string, reader io.Reader) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	m.data[key] = data
	return nil
}

func (m *memoryStorage) Download(_ context.Context, key string) (io.ReadCloser, error) {
	data, ok := m.data[key]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (m *memoryStorage) Delete(_ context.Context, key string) error {
	delete(m.data, key)
	return nil
}

func (m *memoryStorage) Exists(_ context.Context, key string) (bool, error) {
	_, ok := m.data[key]
	return ok, nil
}

func (m *memoryStorage) List(_ context.Context, prefix string) ([]string, error) {
	var keys []string
	for key := range m.data {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func gzipData(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestPrepareArtifact_Plain(t *testing.T) {
	content := []byte("CREATE TABLE t (id int);\n")
	store := newMemoryStorage()
	store.data["backup.sql"] = content

	artifact, err := PrepareArtifact(context.Background(), store, "backup.sql", t.TempDir(), sha256Hex(content), false, "", "", nil)
	if err != nil {
		t.Fatalf("PrepareArtifact: %v", err)
	}
	defer artifact.Cleanup()

	if artifact.Gzip || artifact.CustomFormat {
		t.Fatalf("unexpected format flags: gzip=%t custom=%t", artifact.Gzip, artifact.CustomFormat)
	}
	if !artifact.ChecksumVerified {
		t.Fatal("checksum should be verified")
	}
	reader, err := artifact.Open()
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(reader)
	_ = reader.Close()
	if !bytes.Equal(got, content) {
		t.Fatalf("artifact content mismatch: %q", got)
	}

	path := artifact.Path
	artifact.Cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("cleanup should remove artifact, stat err=%v", err)
	}
}

func TestPrepareArtifact_Gzip(t *testing.T) {
	plain := []byte("CREATE TABLE t (id int);\n")
	compressed := gzipData(t, plain)
	store := newMemoryStorage()
	store.data["backup.sql"] = compressed

	artifact, err := PrepareArtifact(context.Background(), store, "backup.sql", t.TempDir(), sha256Hex(compressed), false, "", "", nil)
	if err != nil {
		t.Fatalf("PrepareArtifact: %v", err)
	}
	defer artifact.Cleanup()

	if !artifact.Gzip {
		t.Fatal("expected gzip detection")
	}
	if !artifact.ChecksumVerified {
		t.Fatal("checksum should be verified")
	}
	reader, err := artifact.Open()
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(reader)
	_ = reader.Close()
	if !bytes.Equal(got, plain) {
		t.Fatalf("decompressed content mismatch: %q", got)
	}
}

func TestPrepareArtifact_EncryptedPassphrase(t *testing.T) {
	plain := []byte("CREATE TABLE secret (id int);\n")
	compressed := gzipData(t, plain)
	checksum := sha256Hex(compressed)

	dir := t.TempDir()
	plainFile := dir + "/plain.sql"
	encFile := dir + "/plain.sql.age"
	if err := os.WriteFile(plainFile, compressed, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := appcrypto.EncryptWithPassphrase(plainFile, encFile, "correct horse"); err != nil {
		t.Fatal(err)
	}
	encrypted, err := os.ReadFile(encFile)
	if err != nil {
		t.Fatal(err)
	}

	store := newMemoryStorage()
	store.data["backup.sql.age"] = encrypted

	artifact, err := PrepareArtifact(context.Background(), store, "backup.sql.age", t.TempDir(), checksum, true, "correct horse", "", nil)
	if err != nil {
		t.Fatalf("PrepareArtifact: %v", err)
	}
	defer artifact.Cleanup()

	if !artifact.Gzip || !artifact.ChecksumVerified {
		t.Fatalf("unexpected artifact: %+v", artifact)
	}
	reader, err := artifact.Open()
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(reader)
	_ = reader.Close()
	if !bytes.Equal(got, plain) {
		t.Fatalf("decrypted content mismatch: %q", got)
	}
}

func TestPrepareArtifact_AgeHeaderDetectedWhenFlagIsFalse(t *testing.T) {
	plain := []byte("SELECT 1;\n")
	pub, priv, err := appcrypto.GenerateX25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	plainFile := dir + "/plain.sql"
	encFile := dir + "/plain.sql.age"
	if err := os.WriteFile(plainFile, plain, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := appcrypto.EncryptWithAge(plainFile, encFile, pub); err != nil {
		t.Fatal(err)
	}
	encrypted, err := os.ReadFile(encFile)
	if err != nil {
		t.Fatal(err)
	}

	store := newMemoryStorage()
	store.data["backup.sql"] = encrypted

	// isEncrypted is deliberately false: the Age header must win.
	artifact, err := PrepareArtifact(context.Background(), store, "backup.sql", t.TempDir(), sha256Hex(plain), false, "", priv, nil)
	if err != nil {
		t.Fatalf("PrepareArtifact: %v", err)
	}
	defer artifact.Cleanup()

	reader, err := artifact.Open()
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(reader)
	_ = reader.Close()
	if !bytes.Equal(got, plain) {
		t.Fatalf("decrypted content mismatch: %q", got)
	}
}

func TestPrepareArtifact_ChecksumMismatch(t *testing.T) {
	content := []byte("CREATE TABLE t (id int);\n")
	store := newMemoryStorage()
	store.data["backup.sql"] = content

	_, err := PrepareArtifact(context.Background(), store, "backup.sql", t.TempDir(), sha256Hex([]byte("other")), false, "", "", nil)
	if err == nil {
		t.Fatal("expected checksum mismatch")
	}
}

func TestPrepareArtifact_MarkedEncryptedButNotAge(t *testing.T) {
	content := []byte("CREATE TABLE t (id int);\n")
	store := newMemoryStorage()
	store.data["backup.sql"] = content

	_, err := PrepareArtifact(context.Background(), store, "backup.sql", t.TempDir(), sha256Hex(content), true, "pass", "", nil)
	if err == nil {
		t.Fatal("expected error for an encrypted flag without an Age artifact")
	}
}

func TestPrepareArtifact_LegacyWithoutChecksum(t *testing.T) {
	content := []byte("SELECT 1;\n")
	store := newMemoryStorage()
	store.data["backup.sql"] = content

	artifact, err := PrepareArtifact(context.Background(), store, "backup.sql", t.TempDir(), "", false, "", "", nil)
	if err != nil {
		t.Fatalf("PrepareArtifact: %v", err)
	}
	defer artifact.Cleanup()
	if artifact.ChecksumVerified {
		t.Fatal("checksum cannot be verified when no checksum was recorded")
	}
}

func TestPrepareArtifact_CustomFormat(t *testing.T) {
	content := append([]byte("PGDMP"), make([]byte, 20)...)
	store := newMemoryStorage()
	store.data["backup.dump"] = content

	artifact, err := PrepareArtifact(context.Background(), store, "backup.dump", t.TempDir(), sha256Hex(content), false, "", "", nil)
	if err != nil {
		t.Fatalf("PrepareArtifact: %v", err)
	}
	defer artifact.Cleanup()
	if !artifact.CustomFormat {
		t.Fatal("expected custom-format detection")
	}
}
