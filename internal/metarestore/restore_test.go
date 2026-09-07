package metarestore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MD2SA/backup-manager/internal/config"
	"github.com/MD2SA/backup-manager/internal/pkg/crypto"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestSniffFormat(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    snapshotFormat
	}{
		{"age header", "age-encryption.org/v1\n-> scrypt 123\n---", formatAge},
		{"legacy enc_v1 json", `{"enc_v1":"abc=="}`, formatEncV1},
		{"plain pg_dump magic", "PGDMP\x00\x00\x00\x00rest", formatPlain},
		{"unknown treated as plain", "whatever bytes here", formatPlain},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "snapshot")
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatal(err)
			}
			got, err := sniffFormat(path)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("sniffFormat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveSnapshot(t *testing.T) {
	dir := t.TempDir()
	meta := filepath.Join(dir, "metadata")
	writeFile(t, filepath.Join(meta, "metadata-20260907-100000.dump"), "x")
	writeFile(t, filepath.Join(meta, "metadata-20260907-120000.dump.age"), "y")
	writeFile(t, filepath.Join(meta, "unrelated.txt"), "z")

	got, err := ResolveSnapshot(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got) != "metadata-20260907-120000.dump.age" {
		t.Errorf("ResolveSnapshot() = %s, want newest .age snapshot", filepath.Base(got))
	}

	explicit := filepath.Join(meta, "metadata-20260907-100000.dump")
	got, err = ResolveSnapshot(dir, explicit)
	if err != nil {
		t.Fatal(err)
	}
	if got != explicit {
		t.Errorf("ResolveSnapshot(explicit) = %s, want %s", got, explicit)
	}
}

func TestResolveSnapshot_Empty(t *testing.T) {
	dir := t.TempDir()
	if _, err := ResolveSnapshot(dir, ""); err == nil {
		t.Fatal("expected error when no snapshots exist")
	}
}

func TestDecryptTo_Formats(t *testing.T) {
	dir := t.TempDir()
	plainContent := "PGDMP\x00\x00\x00\x00fake-dump"

	t.Run("plaintext copy", func(t *testing.T) {
		src := filepath.Join(dir, "plain.dump")
		dst := filepath.Join(dir, "out.dump")
		writeFile(t, src, plainContent)
		if err := DecryptTo(src, dst, "", ""); err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(dst); string(got) != plainContent {
			t.Errorf("plaintext copy mismatch: %q", got)
		}
	})

	t.Run("age with passphrase", func(t *testing.T) {
		plain := filepath.Join(dir, "age-src.dump")
		src := filepath.Join(dir, "age.dump.age")
		dst := filepath.Join(dir, "age-out.dump")
		writeFile(t, plain, plainContent)
		if err := crypto.EncryptWithPassphrase(plain, src, "secret"); err != nil {
			t.Fatal(err)
		}
		if err := DecryptTo(src, dst, "secret", ""); err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(dst); string(got) != plainContent {
			t.Errorf("age roundtrip mismatch: %q", got)
		}
	})

	t.Run("legacy enc_v1", func(t *testing.T) {
		src := filepath.Join(dir, "legacy.dump.age")
		dst := filepath.Join(dir, "legacy-out.dump")
		enc, err := crypto.EncodeProviderConfig(crypto.DeriveConfigKey("secret"), []byte(plainContent))
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, src, string(enc))
		if err := DecryptTo(src, dst, "secret", ""); err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(dst); string(got) != plainContent {
			t.Errorf("enc_v1 roundtrip mismatch: %q", got)
		}
	})

	t.Run("age without keys errors", func(t *testing.T) {
		plain := filepath.Join(dir, "nokey-plain.dump")
		src := filepath.Join(dir, "nokey.dump.age")
		writeFile(t, plain, plainContent)
		if err := crypto.EncryptWithPassphrase(plain, src, "secret"); err != nil {
			t.Fatal(err)
		}
		if err := DecryptTo(src, filepath.Join(dir, "nokey-out"), "", ""); err == nil {
			t.Fatal("expected error when decrypting age without keys")
		}
	})

	t.Run("legacy enc_v1 without passphrase errors", func(t *testing.T) {
		src := filepath.Join(dir, "legacy-nokey.dump.age")
		enc, err := crypto.EncodeProviderConfig(crypto.DeriveConfigKey("secret"), []byte(plainContent))
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, src, string(enc))
		if err := DecryptTo(src, filepath.Join(dir, "out"), "", ""); err == nil {
			t.Fatal("expected error when decrypting enc_v1 without passphrase")
		}
	})
}

func TestPsqlCommand_EnvAndArgs(t *testing.T) {
	cfg := config.DatabaseConfig{Host: "h", Port: "5432", User: "u", Password: "p", DBName: "d", SSLMode: "require"}
	cmd := psqlCommand(cfg, "-tAc", userObjectsQuery)

	argStr := strings.Join(cmd.Args, " ")
	for _, want := range []string{"-h h", "-p 5432", "-U u", "-d d", "-tAc", userObjectsQuery} {
		if !strings.Contains(argStr, want) {
			t.Errorf("psql args missing %q: %s", want, argStr)
		}
	}
	if !strings.Contains(strings.Join(cmd.Env, " "), "PGPASSWORD=p") {
		t.Error("PGPASSWORD not set in psql env")
	}
	if !strings.Contains(strings.Join(cmd.Env, " "), "PGSSLMODE=require") {
		t.Error("PGSSLMODE not set in psql env")
	}
}

func TestRestoreCommandArgs(t *testing.T) {
	cfg := config.DatabaseConfig{Host: "h", Port: "5432", User: "u", Password: "p", DBName: "d", SSLMode: "disable"}

	got := strings.Join(restoreCommand(cfg, "/tmp/file.dump", false).Args, " ")
	for _, want := range []string{"pg_restore", "-Fc", "--no-owner", "--no-acl", "--exit-on-error", "-h h", "-p 5432", "-U u", "-d d", "/tmp/file.dump"} {
		if !strings.Contains(got, want) {
			t.Errorf("restore args missing %q: %s", want, got)
		}
	}
	if strings.Contains(got, "--clean") {
		t.Errorf("unexpected --clean without replace: %s", got)
	}

	replaced := strings.Join(restoreCommand(cfg, "/tmp/file.dump", true).Args, " ")
	for _, flag := range []string{"--clean", "--if-exists"} {
		if !strings.Contains(replaced, flag) {
			t.Errorf("missing %s in replace args: %s", flag, replaced)
		}
	}
}
