package monitor

import (
	"context"
	"time"

	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/robfig/cron/v3"
)

type HealthStatus string

const (
	StatusHealthy  HealthStatus = "healthy"
	StatusWarning  HealthStatus = "warning"
	StatusCritical HealthStatus = "critical"
)

type HealthSummary struct {
	Status              HealthStatus `json:"status"`
	LastSuccess         *time.Time   `json:"last_success"`
	NextScheduled       *time.Time   `json:"next_scheduled"`
	TotalStorageUsed    int64        `json:"total_storage_used"`
	ActiveProfiles      int          `json:"active_profiles"`
	OverdueProfiles     int          `json:"overdue_profiles"`
	LastExecutionStatus string       `json:"last_execution_status"`
}

type Service struct {
	repo repository.Repository
}

func New(repo repository.Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) GetHealthSummary(ctx context.Context) (*HealthSummary, error) {
	profiles, err := s.repo.ListProfiles(ctx)
	if err != nil {
		return nil, err
	}

	summary := &HealthSummary{Status: StatusHealthy}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	now := time.Now()

	var latestExecutionAt time.Time

	for _, p := range profiles {
		if p.Enabled {
			summary.ActiveProfiles++

			// Next scheduled is computed even when the profile has no execution
			// yet. With several enabled profiles, keep the earliest run.
			if schedule, parseErr := parser.Parse(p.Schedule); parseErr == nil {
				next := schedule.Next(now)
				if summary.NextScheduled == nil || next.Before(*summary.NextScheduled) {
					summary.NextScheduled = &next
				}
			}
		}

		executions, err := s.repo.ListExecutionsByProfile(ctx, p.ID)
		if err != nil {
			continue
		}
		if len(executions) == 0 {
			continue
		}

		latest := executions[0]
		if latest.Size.Valid {
			summary.TotalStorageUsed += latest.Size.Int64
		}

		executionAt := latest.CreatedAt.Time
		if latest.StartTime.Valid {
			executionAt = latest.StartTime.Time
		}

		if p.Enabled {
			if executionAt.After(latestExecutionAt) {
				latestExecutionAt = executionAt
				summary.LastExecutionStatus = latest.Status
			}
			if latest.Status == "failed" {
				summary.Status = StatusWarning
			}
			if latest.Status == "success" && latest.StartTime.Valid {
				t := latest.StartTime.Time
				if summary.LastSuccess == nil || t.After(*summary.LastSuccess) {
					summary.LastSuccess = &t
				}
			}
		}
	}

	return summary, nil
}
