package backup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgtype"
)

var ErrRestoreQueueFull = errors.New("restore queue is full")

// RestoreRunner executes restores sequentially. Downloads and decryption could
// run in parallel, but applying to the target database must not overlap with a
// backup dump or another restore.
type RestoreRunner struct {
	logger   *slog.Logger
	queue    chan pgtype.UUID
	executor func(ctx context.Context, restoreID pgtype.UUID) error

	mu     sync.Mutex
	active map[pgtype.UUID]struct{}
	wg     sync.WaitGroup
}

func NewRestoreRunner(logger *slog.Logger, executor func(ctx context.Context, restoreID pgtype.UUID) error) *RestoreRunner {
	return &RestoreRunner{
		logger:   logger,
		queue:    make(chan pgtype.UUID, 10),
		executor: executor,
		active:   make(map[pgtype.UUID]struct{}),
	}
}

func (r *RestoreRunner) Start(ctx context.Context) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case restoreID := <-r.queue:
				r.logger.Info("Processing restore task", "restore_id", restoreID)
				if err := r.executor(ctx, restoreID); err != nil {
					r.logger.Error("Restore task failed", "restore_id", restoreID, "error", err)
				}
				r.mu.Lock()
				delete(r.active, restoreID)
				r.mu.Unlock()
			}
		}
	}()
}

// Wait blocks until the runner goroutine stops. It is used during graceful
// shutdown so restore state updates finish before the database pool closes.
func (r *RestoreRunner) Wait() {
	r.wg.Wait()
}

func (r *RestoreRunner) Enqueue(restoreID pgtype.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.active[restoreID]; exists {
		return fmt.Errorf("restore %s is already queued", restoreID)
	}

	select {
	case r.queue <- restoreID:
		r.active[restoreID] = struct{}{}
		return nil
	default:
		return ErrRestoreQueueFull
	}
}
