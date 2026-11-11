package config

import "testing"

func TestParsePostgresClientVersion(t *testing.T) {
	tests := []struct {
		output string
		want   int
		ok     bool
	}{
		{"pg_dump (PostgreSQL) 17.11", 17, true},
		{"psql (PostgreSQL) 18.6", 18, true},
		{"pg_restore (PostgreSQL) 13.21", 13, true},
		{"pg_dump (PostgreSQL) 18.6 (Ubuntu 18.6-1.pgdg24.04+1)", 18, true},
		{"not a version", 0, false},
	}
	for _, tt := range tests {
		got, err := parsePostgresClientVersion(tt.output)
		if tt.ok && err != nil {
			t.Errorf("%q: %v", tt.output, err)
		}
		if !tt.ok && err == nil {
			t.Errorf("%q: expected an error", tt.output)
		}
		if got != tt.want {
			t.Errorf("%q: major = %d, want %d", tt.output, got, tt.want)
		}
	}
}
