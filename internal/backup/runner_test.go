package backup

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func runnerID(n byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{n}, Valid: true}
}

func TestBackupRunnerQueuesDifferentProfilesSerially(t *testing.T) {
	started := make(chan pgtype.UUID, 2)
	release := make(chan struct{}, 2)
	runner := NewBackupRunner(discardLogger(), func(_ context.Context, id pgtype.UUID) error {
		started <- id
		<-release
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	runner.Start(ctx)
	defer func() {
		cancel()
		runner.Wait()
	}()

	id1, id2 := runnerID(1), runnerID(2)
	if err := runner.Enqueue(id1); err != nil {
		t.Fatal(err)
	}
	if err := runner.Enqueue(id2); err != nil {
		t.Fatal(err)
	}

	if got := <-started; got != id1 {
		t.Fatalf("first = %v, want %v", got, id1)
	}
	select {
	case got := <-started:
		t.Fatalf("second started before the first finished: %v", got)
	case <-time.After(150 * time.Millisecond):
	}

	release <- struct{}{}
	if got := <-started; got != id2 {
		t.Fatalf("second = %v, want %v", got, id2)
	}
	release <- struct{}{}
}

func TestBackupRunnerRejectsDuplicateProfile(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	runner := NewBackupRunner(discardLogger(), func(context.Context, pgtype.UUID) error {
		started <- struct{}{}
		<-release
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	runner.Start(ctx)
	defer func() {
		cancel()
		runner.Wait()
	}()

	id := runnerID(1)
	if err := runner.Enqueue(id); err != nil {
		t.Fatal(err)
	}
	<-started
	if err := runner.Enqueue(id); !errors.Is(err, ErrBackupAlreadyQueued) {
		t.Fatalf("expected ErrBackupAlreadyQueued, got %v", err)
	}
	close(release)
}

func TestBackupRunnerQueueFull(t *testing.T) {
	runner := NewBackupRunner(discardLogger(), func(context.Context, pgtype.UUID) error { return nil })
	for i := 0; i < 100; i++ {
		if err := runner.Enqueue(runnerID(byte(i + 1))); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}
	if err := runner.Enqueue(runnerID(101)); !errors.Is(err, ErrBackupQueueFull) {
		t.Fatalf("expected ErrBackupQueueFull, got %v", err)
	}
}
