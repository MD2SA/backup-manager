package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/MD2SA/backup-manager/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// AuditRecorder persists security-relevant mutations.
type AuditRecorder interface {
	RecordAudit(ctx context.Context, event auth.AuditEvent) error
}

// Audit records every successful or failed mutating API request with the
// authenticated user/key. Reads are intentionally not audited to keep the log
// focused.
func Audit(recorder AuditRecorder, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			principal, ok := auth.PrincipalFromContext(r.Context())
			if !ok {
				return
			}

			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}

			route := r.URL.Path
			if pattern := chi.RouteContext(r.Context()).RoutePattern(); pattern != "" {
				route = pattern
			}

			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 2*time.Second)
			defer cancel()

			if err := recorder.RecordAudit(ctx, auth.AuditEvent{
				Principal:    principal,
				Action:       r.Method + " " + route,
				ResourceType: resourceType(r.URL.Path),
				ResourceID:   chi.URLParam(r, "id"),
				RequestID:    GetRequestID(r.Context()),
				IP:           GetClientIP(r.Context()),
				Metadata: map[string]any{
					"status": status,
					"path":   r.URL.Path,
					"method": r.Method,
				},
			}); err != nil {
				logger.Error("Failed to write audit entry", "error", err)
			}
		})
	}
}

func resourceType(path string) string {
	trimmed := strings.TrimPrefix(path, "/api/v1/")
	if trimmed == path {
		return ""
	}
	if index := strings.IndexByte(trimmed, '/'); index >= 0 {
		return trimmed[:index]
	}
	return trimmed
}
