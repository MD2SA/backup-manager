package crypto

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"filippo.io/age"
)

// EncryptWithAge encrypts a source file to a destination file using an X25519 public key.
func EncryptWithAge(srcPath, dstPath, publicKey string) error {
	recipient, err := age.ParseX25519Recipient(publicKey)
	if err != nil {
		return fmt.Errorf("failed to parse public key: %w", err)
	}

	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer func() { _ = dst.Close() }()

	w, err := age.Encrypt(dst, recipient)
	if err != nil {
		return fmt.Errorf("failed to create age writer: %w", err)
	}

	if _, err := io.Copy(w, src); err != nil {
		_ = w.Close()
		return fmt.Errorf("failed to encrypt data: %w", err)
	}

	return w.Close()
}

// DecryptWithAge decrypts a source file to a destination file using an X25519 identity (private key).
func DecryptWithAge(srcPath, dstPath, identityStr string) error {
	identity, err := age.ParseX25519Identity(identityStr)
	if err != nil {
		return fmt.Errorf("failed to parse identity: %w", err)
	}

	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer func() { _ = dst.Close() }()

	r, err := age.Decrypt(src, identity)
	if err != nil {
		return fmt.Errorf("failed to create age reader: %w", err)
	}

	if _, err := io.Copy(dst, r); err != nil {
		return fmt.Errorf("failed to decrypt data: %w", err)
	}

	return nil
}

// EncryptWithPassphrase encrypts a source file to a destination file using a passphrase.
func EncryptWithPassphrase(srcPath, dstPath, passphrase string) error {
	r, err := age.NewScryptRecipient(passphrase)
	if err != nil {
		return fmt.Errorf("failed to create scrypt recipient: %w", err)
	}

	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer func() { _ = dst.Close() }()

	w, err := age.Encrypt(dst, r)
	if err != nil {
		return fmt.Errorf("failed to create age writer: %w", err)
	}

	if _, err := io.Copy(w, src); err != nil {
		_ = w.Close()
		return fmt.Errorf("failed to encrypt data: %w", err)
	}

	return w.Close()
}

// DecryptWithPassphrase decrypts a source file to a destination file using a passphrase.
func DecryptWithPassphrase(srcPath, dstPath, passphrase string) error {
	i, err := age.NewScryptIdentity(passphrase)
	if err != nil {
		return fmt.Errorf("failed to create scrypt identity: %w", err)
	}

	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer func() { _ = dst.Close() }()

	r, err := age.Decrypt(src, i)
	if err != nil {
		return fmt.Errorf("failed to create age reader: %w", err)
	}

	if _, err := io.Copy(dst, r); err != nil {
		return fmt.Errorf("failed to decrypt data: %w", err)
	}

	return nil
}

// DecryptKeyed decrypts a source file to a destination file using an optional
// passphrase (scrypt identity) and/or an optional X25519 identity (private key).
// At least one must be non-empty. Each provided identity is tried in turn.
func DecryptKeyed(srcPath, dstPath, passphrase, identityStr string) error {
	if passphrase == "" && identityStr == "" {
		return errors.New("no decryption key provided: passphrase or age identity is required")
	}

	var identities []age.Identity
	if passphrase != "" {
		id, err := age.NewScryptIdentity(passphrase)
		if err != nil {
			return fmt.Errorf("failed to create scrypt identity: %w", err)
		}
		identities = append(identities, id)
	}
	if identityStr != "" {
		id, err := age.ParseX25519Identity(identityStr)
		if err != nil {
			return fmt.Errorf("failed to parse identity: %w", err)
		}
		identities = append(identities, id)
	}

	srcData, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}

	var lastErr error
	for _, identity := range identities {
		src := bytes.NewReader(srcData)
		plain, err := age.Decrypt(src, identity)
		if err != nil {
			lastErr = err
			continue
		}

		dstData, err := io.ReadAll(plain)
		if err != nil {
			return fmt.Errorf("failed to decrypt data: %w", err)
		}
		return os.WriteFile(dstPath, dstData, 0644)
	}

	return fmt.Errorf("decryption failed (wrong passphrase or identity?): %w", lastErr)
}

// GenerateX25519KeyPair generates a new X25519 key pair for use with Age.
func GenerateX25519KeyPair() (publicKey string, privateKey string, err error) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return "", "", err
	}

	return identity.Recipient().String(), identity.String(), nil
}
