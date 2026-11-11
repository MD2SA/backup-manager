package middleware

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MD2SA/backup-manager/internal/auth"
	"github.com/jackc/pgx/v5/pgtype"
)

type fakeAuthenticator struct {
	principal auth.Principal
	err       error
	key       string
}

func (f *fakeAuthenticator) Authenticate(_ context.Context, rawKey string) (auth.Principal, error) {
	f.key = rawKey
	return f.principal, f.err
}

func TestAuthenticate_ValidKey(t *testing.T) {
	principal := auth.Principal{UserID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, Email: "a@example.com"}
	authenticator := &fakeAuthenticator{principal: principal}

	var got auth.Principal
	handler := Authenticate(authenticator)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = auth.PrincipalFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "bm_test")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got.UserID != principal.UserID || got.Email != principal.Email {
		t.Fatalf("unexpected principal: %+v", got)
	}
	if authenticator.key != "bm_test" {
		t.Fatalf("authenticator received %q", authenticator.key)
	}
}

func TestAuthenticate_InvalidKey(t *testing.T) {
	authenticator := &fakeAuthenticator{err: auth.ErrUnauthorized}
	handler := Authenticate(authenticator)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not be called")
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}

type fakeAuditRecorder struct {
	events []auth.AuditEvent
}

func (f *fakeAuditRecorder) RecordAudit(_ context.Context, event auth.AuditEvent) error {
	f.events = append(f.events, event)
	return nil
}

func TestAudit_MutatingRequest(t *testing.T) {
	recorder := &fakeAuditRecorder{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	principal := auth.Principal{UserID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, KeyID: pgtype.UUID{Bytes: [16]byte{2}, Valid: true}}
	handler := Audit(recorder, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", nil)
	req = req.WithContext(auth.WithPrincipal(req.Context(), principal))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if len(recorder.events) != 1 {
		t.Fatalf("audit events = %d", len(recorder.events))
	}
	event := recorder.events[0]
	if event.Action != "POST /api/v1/users" {
		t.Fatalf("action = %q", event.Action)
	}
	if event.Metadata["status"] != http.StatusCreated {
		t.Fatalf("status metadata = %v", event.Metadata["status"])
	}
}

func TestAudit_ReadRequestIsSkipped(t *testing.T) {
	recorder := &fakeAuditRecorder{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	principal := auth.Principal{UserID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}}
	handler := Audit(recorder, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req = req.WithContext(auth.WithPrincipal(req.Context(), principal))
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if len(recorder.events) != 0 {
		t.Fatalf("GET should not be audited, got %d events", len(recorder.events))
	}
}
