package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics owns the application registry and all exported metric vectors.
type Metrics struct {
	Registry *prometheus.Registry

	BackupsTotal   *prometheus.CounterVec
	BackupDuration *prometheus.HistogramVec
	BackupBytes    *prometheus.GaugeVec
	LastSuccess    *prometheus.GaugeVec

	RestoresTotal   *prometheus.CounterVec
	RestoreDuration prometheus.Histogram

	QueueDepth      prometheus.Gauge
	SchedulerJobs   prometheus.Gauge
	ActiveProfiles  prometheus.Gauge
	OverdueProfiles prometheus.Gauge

	AuthFailures *prometheus.CounterVec
	HTTPRequests *prometheus.CounterVec
	HTTPDuration *prometheus.HistogramVec

	AuditDeleted prometheus.Counter
}

var backupBuckets = []float64{5, 15, 30, 60, 120, 300, 600, 1800, 3600, 7200}
var restoreBuckets = []float64{1, 5, 15, 30, 60, 120, 300, 600, 1800}

func New() *Metrics {
	registry := prometheus.NewRegistry()
	m := &Metrics{Registry: registry}

	m.BackupsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "backup_manager",
		Name:      "backups_total",
		Help:      "Total backup executions by profile and status.",
	}, []string{"profile_id", "profile_name", "status"})

	m.BackupDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "backup_manager",
		Name:      "backup_duration_seconds",
		Help:      "Backup execution duration in seconds.",
		Buckets:   backupBuckets,
	}, []string{"profile_id", "profile_name"})

	m.BackupBytes = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "backup_manager",
		Name:      "backup_bytes",
		Help:      "Size of the most recent successful backup artifact.",
	}, []string{"profile_id", "profile_name"})

	m.LastSuccess = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "backup_manager",
		Name:      "last_success_timestamp_seconds",
		Help:      "Unix timestamp of the last successful backup per profile.",
	}, []string{"profile_id", "profile_name"})

	m.RestoresTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "backup_manager",
		Name:      "restores_total",
		Help:      "Total restore jobs by status.",
	}, []string{"status"})

	m.RestoreDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "backup_manager",
		Name:      "restore_duration_seconds",
		Help:      "Restore job duration in seconds.",
		Buckets:   restoreBuckets,
	})

	m.QueueDepth = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "backup_manager",
		Name:      "queue_depth",
		Help:      "Number of backup profiles queued or running.",
	})
	m.SchedulerJobs = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "backup_manager",
		Name:      "scheduler_jobs",
		Help:      "Number of profiles currently scheduled.",
	})
	m.ActiveProfiles = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "backup_manager",
		Name:      "active_profiles",
		Help:      "Number of enabled profiles.",
	})
	m.OverdueProfiles = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "backup_manager",
		Name:      "overdue_profiles",
		Help:      "Number of enabled profiles whose last success is overdue.",
	})

	m.AuthFailures = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "backup_manager",
		Name:      "auth_failures_total",
		Help:      "API authentication failures.",
	}, []string{"reason"})

	m.HTTPRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "backup_manager",
		Name:      "http_requests_total",
		Help:      "HTTP requests by method, route and status.",
	}, []string{"method", "route", "status"})

	m.HTTPDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "backup_manager",
		Name:      "http_request_duration_seconds",
		Help:      "HTTP request duration in seconds.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"method", "route"})

	m.AuditDeleted = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "backup_manager",
		Name:      "audit_deleted_total",
		Help:      "Audit log entries removed by retention.",
	})

	registry.MustRegister(
		m.BackupsTotal, m.BackupDuration, m.BackupBytes, m.LastSuccess,
		m.RestoresTotal, m.RestoreDuration,
		m.QueueDepth, m.SchedulerJobs, m.ActiveProfiles, m.OverdueProfiles,
		m.AuthFailures, m.HTTPRequests, m.HTTPDuration, m.AuditDeleted,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	return m
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{})
}
