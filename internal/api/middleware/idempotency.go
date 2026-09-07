package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

const idempotencyTTL = 24 * time.Hour

type idempotencyStore struct {
	mu    sync.Mutex
	items map[string]idempotencyEntry
}

type idempotencyEntry struct {
	status    int
	body      []byte
	expiresAt time.Time
}

var globalIdempotencyStore = &idempotencyStore{
	items: make(map[string]idempotencyEntry),
}

func (s *idempotencyStore) get(key string) (int, []byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.items[key]
	if !ok || time.Now().After(entry.expiresAt) {
		delete(s.items, key)
		return 0, nil, false
	}
	return entry.status, entry.body, true
}

func (s *idempotencyStore) set(key string, status int, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[key] = idempotencyEntry{
		status:    status,
		body:      body,
		expiresAt: time.Now().Add(idempotencyTTL),
	}
}

func idempotencyKeyHash(idempotencyKey, method, uri string) string {
	h := sha256.New()
	h.Write([]byte(method + "|" + uri + "|" + idempotencyKey))
	return hex.EncodeToString(h.Sum(nil))
}

const idempotencyHeader = "X-Idempotency-Key"

// Idempotency wraps an http.HandlerFunc so that a POST with an
// X-Idempotency-Key header is executed at most once within a 24-hour window.
// Only successful responses (2xx) are memoised.
func Idempotency(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get(idempotencyHeader)
		if key == "" {
			next.ServeHTTP(w, r)
			return
		}

		hash := idempotencyKeyHash(key, r.Method, r.RequestURI)
		if status, body, ok := globalIdempotencyStore.get(hash); ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write(body)
			return
		}

		rw := &idempotencyResponseWriter{ResponseWriter: w}
		next.ServeHTTP(rw, r)

		if rw.status == 0 {
			rw.status = http.StatusOK
		}

		// The handler has finished; flush the buffered response to the client
		// and cache successful responses.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(rw.status)
		_, _ = w.Write(rw.body.Bytes())

		if rw.status >= 200 && rw.status < 300 {
			globalIdempotencyStore.set(hash, rw.status, rw.body.Bytes())
		}
	}
}

type idempotencyResponseWriter struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (rw *idempotencyResponseWriter) WriteHeader(code int) {
	rw.status = code
}

func (rw *idempotencyResponseWriter) Write(b []byte) (int, error) {
	if rw.status == 0 {
		rw.status = http.StatusOK
	}
	return rw.body.Write(b)
}
