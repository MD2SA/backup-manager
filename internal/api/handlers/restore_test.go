package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/MD2SA/backup-manager/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func testUUID() pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
}

func TestRestoreHandler_RequiresConfirm(t *testing.T) {
	called := false
	handler := &RestoreHandler{
		Enqueue: func(context.Context, pgtype.UUID, service.RestoreOptions) (db.Restore, error) {
			called = true
			return db.Restore{}, nil
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/executions/11111111-1111-4111-8111-111111111111/restore", strings.NewReader(`{}`))
	router := chi.NewRouter()
	router.Post("/executions/{id}/restore", handler.Trigger)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if called {
		t.Fatal("Enqueue must not be called without confirmation")
	}
}

func TestRestoreHandler_AcceptsConfirmedRestore(t *testing.T) {
	var captured service.RestoreOptions
	handler := &RestoreHandler{
		Enqueue: func(_ context.Context, _ pgtype.UUID, opts service.RestoreOptions) (db.Restore, error) {
			captured = opts
			return db.Restore{ID: testUUID(), Status: "pending"}, nil
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/executions/11111111-1111-4111-8111-111111111111/restore", strings.NewReader(`{"confirm":true,"terminate_connections":true}`))
	router := chi.NewRouter()
	router.Post("/executions/{id}/restore", handler.Trigger)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	if !captured.TerminateConnections {
		t.Error("terminate_connections was not forwarded")
	}
	if !captured.SingleTransaction {
		t.Error("single transaction should be enabled by default")
	}
	if rec.Header().Get("Location") == "" {
		t.Error("Location header is missing")
	}
}

func TestRestoreHandler_VerifyOnlyWithoutConfirm(t *testing.T) {
	var captured service.RestoreOptions
	handler := &RestoreHandler{
		Enqueue: func(_ context.Context, _ pgtype.UUID, opts service.RestoreOptions) (db.Restore, error) {
			captured = opts
			return db.Restore{ID: testUUID(), Status: "pending"}, nil
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/executions/11111111-1111-4111-8111-111111111111/restore", strings.NewReader(`{"verify_only":true}`))
	router := chi.NewRouter()
	router.Post("/executions/{id}/restore", handler.Trigger)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	if !captured.VerifyOnly {
		t.Error("verify_only was not forwarded")
	}
}

func TestRestoreHandler_InvalidProvider(t *testing.T) {
	handler := &RestoreHandler{
		Enqueue: func(context.Context, pgtype.UUID, service.RestoreOptions) (db.Restore, error) {
			t.Fatal("Enqueue should not be called with an invalid provider")
			return db.Restore{}, nil
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/executions/11111111-1111-4111-8111-111111111111/restore", strings.NewReader(`{"confirm":true,"provider_id":"nope"}`))
	router := chi.NewRouter()
	router.Post("/executions/{id}/restore", handler.Trigger)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestRestoreHandler_Conflict(t *testing.T) {
	handler := &RestoreHandler{
		Enqueue: func(context.Context, pgtype.UUID, service.RestoreOptions) (db.Restore, error) {
			return db.Restore{}, service.ErrRestoreConflict
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/executions/11111111-1111-4111-8111-111111111111/restore", strings.NewReader(`{"confirm":true}`))
	router := chi.NewRouter()
	router.Post("/executions/{id}/restore", handler.Trigger)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

type fakeRestoreRepo struct {
	repository.Repository
	restore db.Restore
	err     error
}

func (f *fakeRestoreRepo) GetRestore(context.Context, pgtype.UUID) (db.Restore, error) {
	return f.restore, f.err
}

func TestRestoreHandler_GetNotFound(t *testing.T) {
	handler := &RestoreHandler{Repo: &fakeRestoreRepo{err: pgx.ErrNoRows}}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/restores/11111111-1111-4111-8111-111111111111", nil)
	router := chi.NewRouter()
	router.Get("/restores/{id}", handler.Get)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestRestoreHandler_Get(t *testing.T) {
	handler := &RestoreHandler{Repo: &fakeRestoreRepo{restore: db.Restore{
		ID:          testUUID(),
		ExecutionID: testUUID(),
		Status:      "success",
	}}}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/restores/11111111-1111-4111-8111-111111111111", nil)
	router := chi.NewRouter()
	router.Get("/restores/{id}", handler.Get)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if response["status"] != "success" {
		t.Fatalf("unexpected status: %v", response["status"])
	}
}
