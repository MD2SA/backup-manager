package scheduler

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/robfig/cron/v3"
)

// Job is one enabled profile schedule.
type Job struct {
	ProfileID pgtype.UUID
	Schedule  string
}

type scheduledJob struct {
	entryID  cron.EntryID
	schedule string
	parsed   cron.Schedule
}

// Scheduler owns one cron entry per enabled profile.
type Scheduler struct {
	cron    *cron.Cron
	logger  *slog.Logger
	mu      sync.Mutex
	jobs    map[pgtype.UUID]scheduledJob
	started bool

	onTrigger func(profileID pgtype.UUID)
}

func New(logger *slog.Logger, onTrigger func(profileID pgtype.UUID)) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scheduler{
		cron:      cron.New(),
		logger:    logger,
		jobs:      make(map[pgtype.UUID]scheduledJob),
		onTrigger: onTrigger,
	}
}

func (s *Scheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return
	}
	s.cron.Start()
	s.started = true
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cron.Stop()
	s.started = false
}

// Sync reconciles the scheduler with the desired set of enabled profiles.
// Schedules are validated before any existing job is touched.
func (s *Scheduler) Sync(jobs []Job) error {
	desired := make(map[pgtype.UUID]Job, len(jobs))
	parsedSchedules := make(map[pgtype.UUID]cron.Schedule, len(jobs))
	for _, job := range jobs {
		parsed, err := cron.ParseStandard(job.Schedule)
		if err != nil {
			return fmt.Errorf("invalid schedule for profile %s: %w", job.ProfileID, err)
		}
		desired[job.ProfileID] = job
		parsedSchedules[job.ProfileID] = parsed
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove profiles that are no longer enabled.
	for profileID, existing := range s.jobs {
		if _, keep := desired[profileID]; !keep {
			s.cron.Remove(existing.entryID)
			delete(s.jobs, profileID)
			s.logger.Info("Unscheduled profile", "profile_id", profileID)
		}
	}

	// Add or update the remaining profiles. The new entry is added before the
	// old one is removed so an AddFunc failure cannot leave us without a job.
	for profileID, job := range desired {
		profileID := profileID
		if existing, ok := s.jobs[profileID]; ok && existing.schedule == job.Schedule {
			continue
		}

		entryID, err := s.cron.AddFunc(job.Schedule, func() {
			s.logger.Info("Triggering scheduled backup", "profile_id", profileID)
			if s.onTrigger != nil {
				s.onTrigger(profileID)
			}
		})
		if err != nil {
			return fmt.Errorf("schedule profile %s: %w", profileID, err)
		}

		if existing, ok := s.jobs[profileID]; ok {
			s.cron.Remove(existing.entryID)
		}
		s.jobs[profileID] = scheduledJob{
			entryID:  entryID,
			schedule: job.Schedule,
			parsed:   parsedSchedules[profileID],
		}
		s.logger.Info("Scheduled profile", "profile_id", profileID, "schedule", job.Schedule)
	}

	return nil
}

// RemoveJob unschedules one profile.
func (s *Scheduler) RemoveJob(profileID pgtype.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.jobs[profileID]; ok {
		s.cron.Remove(existing.entryID)
		delete(s.jobs, profileID)
	}
}

// Clear removes every scheduled job.
func (s *Scheduler) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for profileID, existing := range s.jobs {
		s.cron.Remove(existing.entryID)
		delete(s.jobs, profileID)
	}
}

// ActiveJobs returns the number of scheduled profiles.
func (s *Scheduler) ActiveJobs() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.jobs)
}

// NextRun returns the next execution time for a profile, if it is scheduled.
func (s *Scheduler) NextRun(profileID pgtype.UUID) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.jobs[profileID]
	if !ok || existing.parsed == nil {
		return time.Time{}, false
	}
	next := existing.parsed.Next(time.Now())
	if next.IsZero() {
		return time.Time{}, false
	}
	return next, true
}
