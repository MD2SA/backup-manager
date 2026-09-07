package verification

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
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
	defer func() { _ = f.Close() }()

	// Check if file is empty
	info, err := f.Stat()
	if err != nil {
		return false, "stat failed", err
	}
	if info.Size() < 10 {
		return false, "file too small", nil
	}

	// Read magic bytes to detect compression and format
	header := make([]byte, 262)
	n, err := f.Read(header)
	if err != nil && err != io.EOF {
		return false, "failed to read header", err
	}
	_, _ = f.Seek(0, 0)

	var reader io.Reader = f
	isCompressed := false

	// Detect Gzip
	if n >= 2 && header[0] == 0x1f && header[1] == 0x8b {
		isCompressed = true
		gz, err := gzip.NewReader(f)
		if err != nil {
			return false, "invalid gzip stream", err
		}
		defer func() { _ = gz.Close() }()
		reader = gz

		header = make([]byte, 262)
		n, _ = io.ReadFull(reader, header)
		reader = io.MultiReader(bytes.NewReader(header[:n]), reader)
	}

	format := "plain_sql"
	if n >= 5 && string(header[:5]) == "PGDMP" {
		format = "postgres_custom"
	} else if n >= 262 && string(header[257:262]) == "ustar" {
		format = "tar"
	}

	switch format {
	case "postgres_custom":
		return true, "valid postgres custom archive", nil
	case "tar":
		return true, "valid tar archive", nil
	case "plain_sql":
		scanner := bufio.NewScanner(reader)
		foundHeader := false
		for i := 0; i < 50 && scanner.Scan(); i++ {
			line := scanner.Text()
			if strings.Contains(line, "PostgreSQL database dump") ||
				strings.Contains(line, "--") ||
				strings.Contains(line, "SELECT") ||
				strings.Contains(line, "CREATE TABLE") {
				foundHeader = true
				break
			}
		}
		if !foundHeader {
			msg := "missing pg_dump markers"
			if isCompressed {
				msg += " (compressed stream)"
			}
			return false, msg, nil
		}
		return true, "valid plain sql archive", nil
	default:
		return false, fmt.Sprintf("unsupported or unrecognized backup format: %s", format), nil
	}
}
