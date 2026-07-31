package backup

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgtype"
)

type BackupRunner struct {
	logger   *slog.Logger
	queue    chan pgtype.UUID
	executor func(ctx context.Context, profileID pgtype.UUID) error
	mu       sync.Mutex
	active   map[pgtype.UUID]struct{}
}

func NewBackupRunner(logger *slog.Logger, executor func(ctx context.Context, profileID pgtype.UUID) error) *BackupRunner {
	return &BackupRunner{
		logger:   logger,
		queue:    make(chan pgtype.UUID, 100),
		executor: executor,
		active:   make(map[pgtype.UUID]struct{}),
	}
}

func (r *BackupRunner) Start(ctx context.Context) {
	go func() {
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
				delete(r.active, profileID)
				r.mu.Unlock()
			}
		}
	}()
}

func (r *BackupRunner) Enqueue(profileID pgtype.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.active) > 0 {
		return fmt.Errorf("a backup is already in progress or queued")
	}

	select {
	case r.queue <- profileID:
		r.active[profileID] = struct{}{}
		return nil
	default:
		return fmt.Errorf("backup queue is full")
	}
}
