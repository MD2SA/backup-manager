package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/MD2SA/backup-manager/internal/metrics"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

// Metrics records HTTP request counts and durations. The /metrics endpoint
// itself is excluded to avoid polluting the series.
func Metrics(m *metrics.Metrics) func(http.Handler) http.Handler {
	if m == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			next.ServeHTTP(ww, r)

			route := r.URL.Path
			if pattern := chi.RouteContext(r.Context()).RoutePattern(); pattern != "" {
				route = pattern
			}
			if route == "/api/v1/metrics" {
				return
			}

			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}
			m.HTTPRequests.WithLabelValues(r.Method, route, strconv.Itoa(status)).Inc()
			m.HTTPDuration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
		})
	}
}
