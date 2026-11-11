package backup

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/jackc/pgx/v5/pgtype"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRestoreRunner_EnqueueAndRejectDuplicate(t *testing.T) {
	started := make(chan pgtype.UUID, 1)
	release := make(chan struct{})
	runner := NewRestoreRunner(discardLogger(), func(_ context.Context, id pgtype.UUID) error {
		started <- id
		<-release
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	runner.Start(ctx)

	id := pgutil.NewUUID()
	if err := runner.Enqueue(id); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	select {
	case got := <-started:
		if got != id {
			t.Fatalf("executor got %v, want %v", got, id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("executor did not start")
	}

	if err := runner.Enqueue(id); err == nil {
		t.Fatal("expected duplicate enqueue to fail")
	}

	close(release)
	cancel()
	runner.Wait()
}

func TestRestoreRunner_QueueFull(t *testing.T) {
	runner := NewRestoreRunner(discardLogger(), func(context.Context, pgtype.UUID) error { return nil })

	for i := 0; i < 10; i++ {
		id := pgtype.UUID{Bytes: [16]byte{byte(i + 1)}, Valid: true}
		if err := runner.Enqueue(id); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}

	id := pgtype.UUID{Bytes: [16]byte{99}, Valid: true}
	if err := runner.Enqueue(id); err != ErrRestoreQueueFull {
		t.Fatalf("expected ErrRestoreQueueFull, got %v", err)
	}
}
