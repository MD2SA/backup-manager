// Package pgcompat handles compatibility gaps between PostgreSQL client and
// server versions.
package pgcompat

import (
	"bufio"
	"io"
	"strings"
)

// NewSQLFilter removes version-specific SET statements that newer pg_dump
// clients emit but older servers reject. PostgreSQL 17+ dumps contain
// `SET transaction_timeout = 0;`; restoring that dump into PostgreSQL 13-16
// fails with "unrecognized configuration parameter".
//
// Filtering is limited to the dump preamble. Once COPY/INSERT data starts, the
// stream is passed through unchanged so a data row that happens to contain the
// same text is never modified.
func NewSQLFilter(reader io.Reader) io.Reader {
	return &sqlFilter{reader: bufio.NewReader(reader)}
}

// IsTransactionTimeoutError reports whether command output is the known
// transaction_timeout incompatibility.
func IsTransactionTimeoutError(text string) bool {
	return strings.Contains(text, "transaction_timeout") &&
		strings.Contains(text, "unrecognized configuration parameter")
}

type sqlFilter struct {
	reader      *bufio.Reader
	pending     []byte
	inCopy      bool
	dataStarted bool
	done        bool
}

func (r *sqlFilter) Read(p []byte) (int, error) {
	for len(r.pending) == 0 {
		if r.done {
			return 0, io.EOF
		}
		line, err := r.reader.ReadBytes('\n')
		if len(line) > 0 {
			r.pending = r.processLine(line)
		}
		if err != nil {
			r.done = true
			if len(line) == 0 && len(r.pending) == 0 {
				return 0, err
			}
		}
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

func (r *sqlFilter) processLine(line []byte) []byte {
	trimmed := strings.TrimSpace(string(line))

	if r.inCopy {
		if trimmed == `\.` {
			r.inCopy = false
		}
		return line
	}

	if strings.HasPrefix(trimmed, "COPY ") {
		r.inCopy = true
		r.dataStarted = true
		return line
	}
	if strings.HasPrefix(trimmed, "INSERT INTO ") {
		r.dataStarted = true
		return line
	}
	if !r.dataStarted && isUnsupportedDumpSetting(trimmed) {
		return nil
	}
	return line
}

func isUnsupportedDumpSetting(line string) bool {
	normalized := strings.ToLower(line)
	return strings.HasPrefix(normalized, "set transaction_timeout")
}
