package maintenance

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/MD2SA/backup-manager/internal/auth"
	"github.com/MD2SA/backup-manager/internal/metrics"
	"github.com/robfig/cron/v3"
)

// Service periodically removes audit entries older than the configured
// retention window.
type Service struct {
	logger        *slog.Logger
	auth          *auth.Service
	retentionDays int
	schedule      string
	metrics       *metrics.Metrics
	cron          *cron.Cron
	mu            sync.Mutex
}

func New(logger *slog.Logger, authService *auth.Service, retentionDays int, schedule string, metricsService *metrics.Metrics) *Service {
	return &Service{
		logger:        logger,
		auth:          authService,
		retentionDays: retentionDays,
		schedule:      schedule,
		metrics:       metricsService,
		cron:          cron.New(),
	}
}

func (s *Service) Start() {
	if s.retentionDays <= 0 {
		s.logger.Debug("Audit retention disabled")
		return
	}
	s.run()
	if s.schedule == "" {
		return
	}
	if _, err := s.cron.AddFunc(s.schedule, s.run); err != nil {
		s.logger.Error("Invalid audit cleanup schedule", "schedule", s.schedule, "error", err)
		return
	}
	s.cron.Start()
	s.logger.Info("Audit cleanup enabled", "schedule", s.schedule, "retention_days", s.retentionDays)
}

func (s *Service) Stop() {
	s.cron.Stop()
}

func (s *Service) run() {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cutoff := time.Now().AddDate(0, 0, -s.retentionDays)
	deleted, err := s.auth.CleanupAudit(ctx, cutoff)
	if err != nil {
		s.logger.Error("Audit cleanup failed", "error", err)
		return
	}
	if deleted > 0 {
		s.logger.Info("Audit cleanup completed", "deleted", deleted, "cutoff", cutoff)
		if s.metrics != nil {
			s.metrics.AuditDeleted.Add(float64(deleted))
		}
	}
}
