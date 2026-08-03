package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/MD2SA/backup-manager/internal/pkg/apiutil"
)

// ApiKeyAuth provides simple API key authentication.
// If adminKey is empty, it skips validation (Insecure Mode).
func ApiKeyAuth(adminKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if adminKey == "" {
				next.ServeHTTP(w, r)
				return
			}

			key := r.Header.Get("X-API-Key")
			if subtle.ConstantTimeCompare([]byte(key), []byte(adminKey)) != 1 {
				apiutil.Error(w, http.StatusUnauthorized, "Invalid or missing API Key")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
