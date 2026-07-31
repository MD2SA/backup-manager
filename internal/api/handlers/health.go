package handlers

import "net/http"

// Health check
// @Summary Simple health check
// @Description Check if the API server is up and running
// @Tags health
// @Produce plain
// @Success 200 {string} string "OK"
// @Router /health [get]
func Health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}
