package verification

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

type ChecksumStrategy struct{}

func (s *ChecksumStrategy) Name() string { return "Checksum" }

func (s *ChecksumStrategy) Verify(ctx context.Context, path string) (bool, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, "", err
	}

	checksum := hex.EncodeToString(h.Sum(nil))
	return true, checksum, nil
}
