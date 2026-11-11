package backup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrBackupAlreadyQueued = errors.New("profile is already queued or running")
	ErrBackupQueueFull     = errors.New("backup queue is full")
)

// BackupRunner executes backups serially but accepts different profiles in its
// queue. This lets several enabled profiles be scheduled close together without
// dropping triggers.
type BackupRunner struct {
	logger   *slog.Logger
	queue    chan pgtype.UUID
	executor func(ctx context.Context, profileID pgtype.UUID) error

	mu      sync.Mutex
	pending map[pgtype.UUID]struct{}
	wg      sync.WaitGroup
}

func NewBackupRunner(logger *slog.Logger, executor func(ctx context.Context, profileID pgtype.UUID) error) *BackupRunner {
	return &BackupRunner{
		logger:   logger,
		queue:    make(chan pgtype.UUID, 100),
		executor: executor,
		pending:  make(map[pgtype.UUID]struct{}),
	}
}

func (r *BackupRunner) Start(ctx context.Context) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case profileID := <-r.queue:
				r.logger.Info("Processing backup task", "profile_id", profileID)
				if err := r.executor(ctx, profileID); err != nil {
					r.logger.Error("Backup task failed", "profile_id", profileID, "error", err)
				}
				r.mu.Lock()
				delete(r.pending, profileID)
				r.mu.Unlock()
			}
		}
	}()
}

// Wait blocks until the runner goroutine stops.
func (r *BackupRunner) Wait() {
	r.wg.Wait()
}

// Enqueue schedules a profile. Different profiles may be queued; the same
// profile cannot be queued twice while pending or running.
func (r *BackupRunner) Enqueue(profileID pgtype.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.pending[profileID]; exists {
		return fmt.Errorf("%w: %s", ErrBackupAlreadyQueued, profileID)
	}

	select {
	case r.queue <- profileID:
		r.pending[profileID] = struct{}{}
		return nil
	default:
		return ErrBackupQueueFull
	}
}
