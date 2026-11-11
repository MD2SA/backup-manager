package monitor

import (
	"context"
	"time"

	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/robfig/cron/v3"
)

type HealthStatus string

const (
	StatusHealthy  HealthStatus = "healthy"
	StatusWarning  HealthStatus = "warning"
	StatusCritical HealthStatus = "critical"
)

type ScheduledProfileHealth struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Schedule      string     `json:"schedule"`
	LastSuccess   *time.Time `json:"last_success"`
	NextScheduled *time.Time `json:"next_scheduled"`
	LastStatus    string     `json:"last_status"`
	Overdue       bool       `json:"overdue"`
}

type HealthSummary struct {
	Status              HealthStatus             `json:"status"`
	LastSuccess         *time.Time               `json:"last_success"`
	NextScheduled       *time.Time               `json:"next_scheduled"`
	TotalStorageUsed    int64                    `json:"total_storage_used"`
	ActiveProfiles      int                      `json:"active_profiles"`
	OverdueProfiles     int                      `json:"overdue_profiles"`
	LastExecutionStatus string                   `json:"last_execution_status"`
	ScheduledProfiles   []ScheduledProfileHealth `json:"scheduled_profiles"`
}

type Service struct {
	repo          repository.Repository
	overdueFactor float64
	overdueGrace  time.Duration
}

func New(repo repository.Repository, overdueFactor float64, overdueGrace time.Duration) *Service {
	if overdueFactor < 1 {
		overdueFactor = 2
	}
	if overdueGrace < 0 {
		overdueGrace = 0
	}
	return &Service{repo: repo, overdueFactor: overdueFactor, overdueGrace: overdueGrace}
}

func (s *Service) GetHealthSummary(ctx context.Context) (*HealthSummary, error) {
	profiles, err := s.repo.ListProfiles(ctx)
	if err != nil {
		return nil, err
	}

	summary := &HealthSummary{
		Status:            StatusHealthy,
		ScheduledProfiles: make([]ScheduledProfileHealth, 0),
	}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	now := time.Now()

	var latestExecutionAt time.Time

	for _, profile := range profiles {
		executions, err := s.repo.ListExecutionsByProfile(ctx, profile.ID)
		if err != nil {
			continue
		}

		var latest *db.Execution
		var lastSuccess *time.Time
		for i := range executions {
			execution := executions[i]
			if latest == nil {
				latest = &execution
			}
			if lastSuccess == nil && execution.Status == "success" {
				at := execution.CreatedAt.Time
				if execution.StartTime.Valid {
					at = execution.StartTime.Time
				}
				lastSuccess = &at
			}
		}

		if latest != nil && latest.Size.Valid {
			summary.TotalStorageUsed += latest.Size.Int64
		}

		if !profile.Enabled {
			continue
		}

		summary.ActiveProfiles++
		scheduled := ScheduledProfileHealth{
			ID:       pgutil.UUIDToString(profile.ID),
			Name:     profile.Name,
			Schedule: profile.Schedule,
		}
		if lastSuccess != nil {
			scheduled.LastSuccess = lastSuccess
			if summary.LastSuccess == nil || lastSuccess.After(*summary.LastSuccess) {
				summary.LastSuccess = lastSuccess
			}
		}
		if latest != nil {
			scheduled.LastStatus = latest.Status
			executionAt := latest.CreatedAt.Time
			if latest.StartTime.Valid {
				executionAt = latest.StartTime.Time
			}
			if executionAt.After(latestExecutionAt) {
				latestExecutionAt = executionAt
				summary.LastExecutionStatus = latest.Status
			}
			if latest.Status == "failed" {
				summary.Status = StatusWarning
			}
		}

		schedule, parseErr := parser.Parse(profile.Schedule)
		if parseErr == nil {
			if next := schedule.Next(now); !next.IsZero() {
				scheduled.NextScheduled = &next
				if summary.NextScheduled == nil || next.Before(*summary.NextScheduled) {
					summary.NextScheduled = &next
				}
			}

			if gap, ok := maxGap(schedule, now); ok {
				threshold := time.Duration(float64(gap)*s.overdueFactor) + s.overdueGrace
				since := profile.CreatedAt.Time
				if profile.ScheduledSince.Valid {
					since = profile.ScheduledSince.Time
				}
				if now.Sub(since) > threshold {
					if lastSuccess == nil || now.Sub(*lastSuccess) > threshold {
						scheduled.Overdue = true
						summary.OverdueProfiles++
						summary.Status = StatusWarning
					}
				}
			}
		}

		summary.ScheduledProfiles = append(summary.ScheduledProfiles, scheduled)
	}

	if summary.ActiveProfiles > 0 && summary.OverdueProfiles == summary.ActiveProfiles {
		summary.Status = StatusCritical
	}

	return summary, nil
}

// maxGap samples the next runs of a schedule and returns the largest interval.
// It approximates the expected cadence without needing a "previous run" API.
func maxGap(schedule cron.Schedule, now time.Time) (time.Duration, bool) {
	prev := now
	var largest time.Duration
	for i := 0; i < 20; i++ {
		next := schedule.Next(prev)
		if next.IsZero() {
			return 0, false
		}
		if gap := next.Sub(prev); gap > largest {
			largest = gap
		}
		prev = next
	}
	return largest, largest > 0
}
