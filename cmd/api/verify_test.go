package main

import "testing"

func TestParseScratchTarget(t *testing.T) {
	target, err := parseScratchTarget("postgres://user:pass@localhost:55432/scratch?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	if target.Host != "localhost" || target.Port != "55432" || target.User != "user" ||
		target.Password != "pass" || target.DBName != "scratch" || target.SSLMode != "disable" {
		t.Fatalf("unexpected target: %+v", target)
	}
}
