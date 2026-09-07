package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIdempotency_StoreAndReplay(t *testing.T) {
	callCount := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusAccepted)
	})

	wrapped := Idempotency(handler)

	req := httptest.NewRequest("POST", "/profiles/123/run", nil)
	req.Header.Set("X-Idempotency-Key", "test-key-1")
	w1 := httptest.NewRecorder()

	wrapped.ServeHTTP(w1, req)

	if callCount != 1 {
		t.Fatalf("expected handler to be called once, got %d", callCount)
	}
	if w1.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w1.Code)
	}

	// Second request with same key should be replayed without calling handler.
	req2 := httptest.NewRequest("POST", "/profiles/123/run", nil)
	req2.Header.Set("X-Idempotency-Key", "test-key-1")
	w2 := httptest.NewRecorder()

	wrapped.ServeHTTP(w2, req2)

	if callCount != 1 {
		t.Fatalf("handler should not be called again, got %d calls", callCount)
	}
	if w2.Code != http.StatusAccepted {
		t.Fatalf("replayed response should be 202, got %d", w2.Code)
	}
}

func TestIdempotency_NoKeyPassesThrough(t *testing.T) {
	callCount := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	})

	wrapped := Idempotency(handler)

	req := httptest.NewRequest("POST", "/test", nil)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if callCount != 1 {
		t.Fatalf("expected handler to be called once, got %d", callCount)
	}
}

func TestIdempotency_DifferentKeys(t *testing.T) {
	callCount := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusAccepted)
	})

	wrapped := Idempotency(handler)

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/test", nil)
		req.Header.Set("X-Idempotency-Key", "key-1")
		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, req)
	}

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Idempotency-Key", "key-2")
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if callCount != 2 {
		t.Fatalf("expected 2 handler calls (different keys), got %d", callCount)
	}
}

func TestIdempotency_FailureNotCached(t *testing.T) {
	callCount := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusInternalServerError)
	})

	wrapped := Idempotency(handler)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/test", nil)
		req.Header.Set("X-Idempotency-Key", "fail-key")
		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, req)
	}

	if callCount != 3 {
		t.Fatalf("expected 3 handler calls (failures not cached), got %d", callCount)
	}
}

func TestIdempotency_DifferentMethodsDontCollide(t *testing.T) {
	runCalls := 0
	restoreCalls := 0

	runHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		runCalls++
		w.WriteHeader(http.StatusAccepted)
	})
	restoreHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		restoreCalls++
		w.WriteHeader(http.StatusAccepted)
	})

	wrappedRun := Idempotency(runHandler)
	wrappedRestore := Idempotency(restoreHandler)

	req1 := httptest.NewRequest("POST", "/profiles/1/run", nil)
	req1.Header.Set("X-Idempotency-Key", "shared-key")
	w1 := httptest.NewRecorder()
	wrappedRun.ServeHTTP(w1, req1)

	req2 := httptest.NewRequest("POST", "/executions/1/restore", nil)
	req2.Header.Set("X-Idempotency-Key", "shared-key")
	w2 := httptest.NewRecorder()
	wrappedRestore.ServeHTTP(w2, req2)

	if runCalls != 1 {
		t.Fatalf("expected 1 run call, got %d", runCalls)
	}
	if restoreCalls != 1 {
		t.Fatalf("expected 1 restore call, got %d", restoreCalls)
	}
}
