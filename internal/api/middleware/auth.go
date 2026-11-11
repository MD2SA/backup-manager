package middleware

import (
	"net/http"

	"github.com/MD2SA/backup-manager/internal/auth"
	"github.com/MD2SA/backup-manager/internal/pkg/apiutil"
)

// Authenticate resolves X-API-Key into an auth.Principal. In insecure
// development mode the authenticator returns a synthetic principal even when
// no key is supplied.
func Authenticate(authenticator auth.Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, err := authenticator.Authenticate(r.Context(), r.Header.Get("X-API-Key"))
			if err != nil {
				apiutil.Error(w, http.StatusUnauthorized, "Invalid or missing API key")
				return
			}
			next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), principal)))
		})
	}
}
