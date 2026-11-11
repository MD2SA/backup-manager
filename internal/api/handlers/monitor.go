package handlers

import (
	"net/http"

	"github.com/MD2SA/backup-manager/internal/metrics"
	"github.com/MD2SA/backup-manager/internal/monitor"
	"github.com/MD2SA/backup-manager/internal/pkg/apiutil"
)

type MonitorHandler struct {
	Service *monitor.Service
	Metrics *metrics.Metrics
}

// Health summary
// @Summary Get system health summary
// @Description Get an aggregated view of system health, storage usage, and active profiles
// @Tags health
// @Produce json
// @Success 200 {object} monitor.HealthSummary
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /health/summary [get]
func (h *MonitorHandler) HealthSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := h.Service.GetHealthSummary(r.Context())
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}

	if h.Metrics != nil {
		h.Metrics.ActiveProfiles.Set(float64(summary.ActiveProfiles))
		h.Metrics.OverdueProfiles.Set(float64(summary.OverdueProfiles))
		for _, profile := range summary.ScheduledProfiles {
			if profile.LastSuccess != nil {
				h.Metrics.LastSuccess.WithLabelValues(profile.ID, profile.Name).Set(float64(profile.LastSuccess.Unix()))
			}
		}
	}

	apiutil.Success(w, http.StatusOK, summary)
}
