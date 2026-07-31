package scheduler

import (
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/robfig/cron/v3"
)

type Scheduler struct {
	cron    *cron.Cron
	logger  *slog.Logger
	mu      sync.Mutex
	entryID cron.EntryID

	// Callback for when a job triggers
	onTrigger func(profileID pgtype.UUID)
}

func New(logger *slog.Logger, onTrigger func(profileID pgtype.UUID)) *Scheduler {
	return &Scheduler{
		cron:      cron.New(),
		logger:    logger,
		onTrigger: onTrigger,
	}
}

func (s *Scheduler) Start() {
	s.cron.Start()
}

func (s *Scheduler) Stop() {
	s.cron.Stop()
}

func (s *Scheduler) SetActiveJob(profileID pgtype.UUID, schedule string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove existing job
	if s.entryID != 0 {
		s.cron.Remove(s.entryID)
	}

	id, err := s.cron.AddFunc(schedule, func() {
		s.logger.Info("Triggering scheduled backup", "profile_id", profileID)
		s.onTrigger(profileID)
	})
	if err != nil {
		return err
	}

	s.entryID = id
	return nil
}

func (s *Scheduler) ClearActiveJob() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.entryID != 0 {
		s.cron.Remove(s.entryID)
		s.entryID = 0
	}
}
