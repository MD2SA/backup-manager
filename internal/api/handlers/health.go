package handlers

import (
	"context"
	"net/http"
	"time"
)

// Health check
// @Summary Simple liveness check
// @Description Check if the API server process is up. This endpoint is public
// @Description (no API key) and is used by container healthchecks.
// @Tags health
// @Produce plain
// @Success 200 {string} string "OK"
// @Router /healthz [get]
func Health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

// Ready reports whether the service can reach its metadata database. It is
// public so orchestrators can probe it without credentials.
func Ready(ping func(context.Context) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ping == nil {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("READY"))
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if err := ping(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("NOT READY"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("READY"))
	}
}
