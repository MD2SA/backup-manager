package crypto

import (
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

// GenerateX25519KeyPair generates a new X25519 key pair for use with Age.
func GenerateX25519KeyPair() (publicKey string, privateKey string, err error) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return "", "", err
	}

	return identity.Recipient().String(), identity.String(), nil
}
