package crypto

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestDecryptKeyed_Passphrase(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain.dump")
	enc := filepath.Join(dir, "plain.dump.age")
	out := filepath.Join(dir, "out.dump")
	writeFile(t, plain, "hello metadata")
	if err := EncryptWithPassphrase(plain, enc, "segredo-123"); err != nil {
		t.Fatal(err)
	}
	if err := DecryptKeyed(enc, out, "segredo-123", ""); err != nil {
		t.Fatalf("decrypt with correct passphrase: %v", err)
	}
	if got := readFile(t, out); got != "hello metadata" {
		t.Fatalf("unexpected plaintext: %q", got)
	}
}

func TestDecryptKeyed_IdentityAndBoth(t *testing.T) {
	pub, priv, err := GenerateX25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	plain := filepath.Join(dir, "plain.dump")
	enc := filepath.Join(dir, "plain.dump.age")
	out := filepath.Join(dir, "out.dump")
	writeFile(t, plain, "hello identity")
	if err := EncryptWithAge(plain, enc, pub); err != nil {
		t.Fatal(err)
	}

	if err := DecryptKeyed(enc, out, "", priv); err != nil {
		t.Fatalf("decrypt with identity: %v", err)
	}
	if got := readFile(t, out); got != "hello identity" {
		t.Fatalf("unexpected plaintext: %q", got)
	}

	// Both keys provided: a wrong passphrase is tried first, then the identity.
	out2 := filepath.Join(dir, "out2.dump")
	if err := DecryptKeyed(enc, out2, "errada", priv); err != nil {
		t.Fatalf("decrypt with wrong passphrase + correct identity: %v", err)
	}
	if got := readFile(t, out2); got != "hello identity" {
		t.Fatalf("unexpected plaintext: %q", got)
	}
}

func TestDecryptKeyed_NoKeys(t *testing.T) {
	dir := t.TempDir()
	err := DecryptKeyed(filepath.Join(dir, "f"), filepath.Join(dir, "o"), "", "")
	if err == nil {
		t.Fatal("expected error when no keys are provided")
	}
}

func TestDecryptKeyed_WrongKey(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain.dump")
	enc := filepath.Join(dir, "plain.dump.age")
	writeFile(t, plain, "hello")
	if err := EncryptWithPassphrase(plain, enc, "certa"); err != nil {
		t.Fatal(err)
	}
	if err := DecryptKeyed(enc, filepath.Join(dir, "out"), "errada", ""); err == nil {
		t.Fatal("expected error with wrong passphrase")
	}
}
