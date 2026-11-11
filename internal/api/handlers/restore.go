package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/MD2SA/backup-manager/internal/api/dto"
	apimiddleware "github.com/MD2SA/backup-manager/internal/api/middleware"
	"github.com/MD2SA/backup-manager/internal/auth"
	"github.com/MD2SA/backup-manager/internal/pagination"
	"github.com/MD2SA/backup-manager/internal/pkg/apiutil"
	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/MD2SA/backup-manager/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type RestoreHandler struct {
	Repo    repository.Repository
	Enqueue func(ctx context.Context, executionID pgtype.UUID, opts service.RestoreOptions) (db.Restore, error)
}

// Trigger restore
// @Summary Restore a backup
// @Description Enqueue an asynchronous restore for a backup execution.
// @Description A real restore requires "confirm": true. Use "verify_only": true
// @Description to download, decrypt and verify the artifact without touching the target database.
// @Description The target database must be empty; see the configuration guide.
// @Tags restores
// @Accept json
// @Produce json
// @Param id path string true "Execution ID"
// @Param request body dto.RestoreRequest false "Restore options"
// @Success 202 {object} dto.RestoreResponse
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 404 {object} apiutil.ErrorResponse
// @Failure 409 {object} apiutil.ErrorResponse
// @Failure 503 {object} apiutil.ErrorResponse
// @Router /executions/{id}/restore [post]
func (h *RestoreHandler) Trigger(w http.ResponseWriter, r *http.Request) {
	executionID, err := pgutil.ParseUUID(chi.URLParam(r, "id"))
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid execution ID")
		return
	}

	var req dto.RestoreRequest
	if r.Body != nil {
		if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil && !errors.Is(decodeErr, io.EOF) {
			apiutil.Error(w, http.StatusBadRequest, "Invalid request body")
			return
		}
	}
	if err := req.Validate(); err != nil {
		apiutil.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if !req.Confirm && !req.VerifyOnly {
		apiutil.Error(w, http.StatusBadRequest, "Restore requires confirm=true (or verify_only=true)")
		return
	}

	providerID, err := pgutil.ParseUUID(req.ProviderID)
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid provider ID")
		return
	}

	principal, _ := auth.PrincipalFromContext(r.Context())
	restore, err := h.Enqueue(r.Context(), executionID, service.RestoreOptions{
		ProviderID:           providerID,
		TerminateConnections: req.TerminateConnections,
		VerifyOnly:           req.VerifyOnly,
		SingleTransaction:    true,
		RequestID:            apimiddleware.GetRequestID(r.Context()),
		UserID:               principal.UserID,
		KeyID:                principal.KeyID,
	})
	if err != nil {
		writeRestoreError(w, err)
		return
	}

	w.Header().Set("Location", "/api/v1/restores/"+pgutil.UUIDToString(restore.ID))
	apiutil.Success(w, http.StatusAccepted, dto.ToRestoreResponse(restore))
}

// Get restore
// @Summary Get a restore job
// @Description Get the status, logs and result of a restore job.
// @Tags restores
// @Produce json
// @Param id path string true "Restore ID"
// @Success 200 {object} dto.RestoreResponse
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 404 {object} apiutil.ErrorResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /restores/{id} [get]
func (h *RestoreHandler) Get(w http.ResponseWriter, r *http.Request) {
	restoreID, err := pgutil.ParseUUID(chi.URLParam(r, "id"))
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid restore ID")
		return
	}

	restore, err := h.Repo.GetRestore(r.Context(), restoreID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			apiutil.Error(w, http.StatusNotFound, "Restore not found")
			return
		}
		apiutil.InternalError(w, err)
		return
	}

	apiutil.Success(w, http.StatusOK, dto.ToRestoreResponse(restore))
}

// List restores for an execution
// @Summary List restores for a backup execution
// @Description Get the restore history of a specific backup execution, newest first.
// @Tags restores
// @Produce json
// @Param id path string true "Execution ID"
// @Param limit query int false "Page size (default 50, max 200)"
// @Param cursor query string false "Opaque next cursor"
// @Success 200 {object} dto.RestorePage
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 404 {object} apiutil.ErrorResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /executions/{id}/restores [get]
func (h *RestoreHandler) ListByExecution(w http.ResponseWriter, r *http.Request) {
	executionID, err := pgutil.ParseUUID(chi.URLParam(r, "id"))
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid execution ID")
		return
	}
	if _, err := h.Repo.GetExecution(r.Context(), executionID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			apiutil.Error(w, http.StatusNotFound, "Execution not found")
			return
		}
		apiutil.InternalError(w, err)
		return
	}

	pageReq, err := pagination.ParseRequest(r)
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	cursorTime, cursorID, hasCursor, err := pagination.ParseTimeCursor(pageReq.Cursor, "execution_restores")
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	params := db.ListRestoresByExecutionPageParams{
		ExecutionID: executionID,
		PageLimit:   pageReq.Limit + 1,
	}
	if hasCursor {
		params.CursorTime = pgutil.ToTimestamptz(cursorTime)
		params.CursorID = cursorID
	}

	restores, err := h.Repo.ListRestoresByExecutionPage(r.Context(), params)
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}
	restores, hasMore := pagination.TrimPage(restores, pageReq.Limit)
	next := ""
	if hasMore && len(restores) > 0 {
		last := restores[len(restores)-1]
		next = pagination.TimeCursor("execution_restores", last.CreatedAt.Time, last.ID)
	}
	pagination.SetNextLink(w, r, next)
	apiutil.Success(w, http.StatusOK, dto.RestorePage{
		Items:      toRestoreResponses(restores),
		NextCursor: next,
		HasMore:    hasMore,
	})
}

// List restores
// @Summary List all restore jobs
// @Description Get the global restore history, newest first.
// @Tags restores
// @Produce json
// @Param limit query int false "Page size (default 50, max 200)"
// @Param cursor query string false "Opaque next cursor"
// @Success 200 {object} dto.RestorePage
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /restores [get]
func (h *RestoreHandler) List(w http.ResponseWriter, r *http.Request) {
	pageReq, err := pagination.ParseRequest(r)
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	cursorTime, cursorID, hasCursor, err := pagination.ParseTimeCursor(pageReq.Cursor, "restores")
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	params := db.ListRestoresPageParams{PageLimit: pageReq.Limit + 1}
	if hasCursor {
		params.CursorTime = pgutil.ToTimestamptz(cursorTime)
		params.CursorID = cursorID
	}

	restores, err := h.Repo.ListRestoresPage(r.Context(), params)
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}
	restores, hasMore := pagination.TrimPage(restores, pageReq.Limit)
	next := ""
	if hasMore && len(restores) > 0 {
		last := restores[len(restores)-1]
		next = pagination.TimeCursor("restores", last.CreatedAt.Time, last.ID)
	}
	pagination.SetNextLink(w, r, next)
	apiutil.Success(w, http.StatusOK, dto.RestorePage{
		Items:      toRestoreResponses(restores),
		NextCursor: next,
		HasMore:    hasMore,
	})
}

func toRestoreResponses(restores []db.Restore) []dto.RestoreResponse {
	out := make([]dto.RestoreResponse, len(restores))
	for i, restore := range restores {
		out[i] = dto.ToRestoreResponse(restore)
	}
	return out
}

func writeRestoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		apiutil.Error(w, http.StatusNotFound, "Execution not found")
	case errors.Is(err, service.ErrRestoreConflict):
		apiutil.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrRestoreNoArtifact):
		apiutil.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrTargetNotReady):
		apiutil.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrRestoreProvider):
		apiutil.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrTargetNotConfigured):
		apiutil.Error(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, service.ErrRestoreQueueFull):
		apiutil.Error(w, http.StatusServiceUnavailable, err.Error())
	default:
		apiutil.InternalError(w, err)
	}
}
