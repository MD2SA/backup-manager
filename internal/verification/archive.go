package verification

import (
	"bufio"
	"context"
	"os"
	"strings"
)

type ArchiveStrategy struct{}

func (s *ArchiveStrategy) Name() string { return "Archive" }

func (s *ArchiveStrategy) Verify(ctx context.Context, path string) (bool, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, "file unreadable", err
	}
	defer f.Close()

	// Check if file is empty
	info, err := f.Stat()
	if err != nil {
		return false, "stat failed", err
	}
	if info.Size() < 10 {
		return false, "file too small", nil
	}

	// Read first few lines for pg_dump header
	scanner := bufio.NewScanner(f)
	foundHeader := false
	for i := 0; i < 20 && scanner.Scan(); i++ {
		line := scanner.Text()
		if strings.Contains(line, "PostgreSQL database dump") || strings.Contains(line, "--") {
			foundHeader = true
			break
		}
	}

	if !foundHeader {
		return false, "missing pg_dump header", nil
	}

	return true, "valid archive", nil
}
