package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

type requestIDContextKey struct{}

// requestIDKey is the context key storing the request ID.
var requestIDKey = &requestIDContextKey{}

// headerRequestID is the canonical form of the request ID header.
const headerRequestID = "X-Request-ID"

// newRequestID generates a random hex request identifier.
func newRequestID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// RequestID generates (or echoes an incoming) request ID, injects it into the
// request context, and sets the X-Request-ID response header for correlation.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(headerRequestID)
		if id == "" {
			generated, err := newRequestID()
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			id = generated
		}

		r = r.WithContext(context.WithValue(r.Context(), requestIDKey, id))
		w.Header().Set(headerRequestID, id)
		next.ServeHTTP(w, r)
	})
}

// GetRequestID returns the request ID for the given context, or "" if unset.
func GetRequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}
