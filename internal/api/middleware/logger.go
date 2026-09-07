package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

func Logger(logger *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			t1 := time.Now()

			defer func() {
				attrs := []any{
					"method", r.Method,
					"path", r.URL.Path,
					"status", ww.Status(),
					"duration", time.Since(t1),
					"remote", GetClientIP(r.Context()),
				}
				if reqID := GetRequestID(r.Context()); reqID != "" {
					attrs = append(attrs, "request_id", reqID)
				}
				logger.Info("request completed", attrs...)
			}()

			next.ServeHTTP(ww, r)
		})
	}
}
