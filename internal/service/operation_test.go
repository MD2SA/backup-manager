package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestOperationContextDisabled(t *testing.T) {
	ctx := context.Background()
	got, cancel := operationContext(ctx, 0)
	defer cancel()
	if got != ctx {
		t.Fatal("zero timeout should preserve the parent context")
	}
}

func TestOperationContextTimeout(t *testing.T) {
	ctx := context.Background()
	opCtx, cancel := operationContext(ctx, time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)
	if !errors.Is(opCtx.Err(), context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", opCtx.Err())
	}
}

func TestOperationError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)

	got := operationError("backup", time.Minute, ctx, errors.New("pg_dump failed"))
	if !strings.Contains(got.Error(), "backup timed out after 1m0s") {
		t.Fatalf("unexpected error: %v", got)
	}

	got = operationError("backup", time.Minute, ctx, nil)
	if !strings.Contains(got.Error(), "backup timed out after 1m0s") {
		t.Fatalf("unexpected nil-error timeout: %v", got)
	}

	original := errors.New("other")
	if got := operationError("backup", time.Minute, context.Background(), original); got != original {
		t.Fatalf("non-timeout error was wrapped: %v", got)
	}
}
