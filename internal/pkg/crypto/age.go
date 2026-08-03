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
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	w, err := age.Encrypt(dst, recipient)
	if err != nil {
		return fmt.Errorf("failed to create age writer: %w", err)
	}

	if _, err := io.Copy(w, src); err != nil {
		w.Close()
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
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	r, err := age.Decrypt(src, identity)
	if err != nil {
		return fmt.Errorf("failed to create age reader: %w", err)
	}

	if _, err := io.Copy(dst, r); err != nil {
		return fmt.Errorf("failed to decrypt data: %w", err)
	}

	return nil
}
