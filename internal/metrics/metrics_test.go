package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsExposeApplicationSeries(t *testing.T) {
	m := New()
	m.BackupsTotal.WithLabelValues("profile-1", "daily", "success").Inc()
	m.RestoresTotal.WithLabelValues("success").Inc()
	m.ActiveProfiles.Set(2)
	m.OverdueProfiles.Set(1)

	families, err := m.Registry.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	names := make(map[string]bool)
	for _, family := range families {
		names[family.GetName()] = true
	}
	for _, want := range []string{
		"backup_manager_backups_total",
		"backup_manager_restores_total",
		"backup_manager_active_profiles",
		"backup_manager_overdue_profiles",
		"go_goroutines",
	} {
		if !names[want] {
			t.Errorf("missing metric %s", want)
		}
	}
}

func TestMetricsHandler(t *testing.T) {
	m := New()
	m.QueueDepth.Set(3)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/metrics", nil)
	m.Handler().ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "backup_manager_queue_depth 3") {
		t.Fatalf("unexpected metrics body: %s", body)
	}
}
