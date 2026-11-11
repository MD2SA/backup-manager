package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/MD2SA/backup-manager/internal/api/dto"
	"github.com/MD2SA/backup-manager/internal/pagination"
	"github.com/MD2SA/backup-manager/internal/pkg/apiutil"
	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type ExecutionHandler struct {
	Repo repository.Repository
}

// List executions by profile
// @Summary List executions for a specific profile
// @Description Get a history of all backup executions for a profile.
// @Description Results are sorted by creation date (newest first).
// @Description Status can be: pending, running, success, failed.
// @Description The 'logs' field contains an array of strings formatted as 'TIMESTAMP: MESSAGE'.
// @Tags executions
// @Produce json
// @Param id path string true "Profile ID"
// @Param limit query int false "Page size (default 50, max 200)"
// @Param cursor query string false "Opaque next cursor"
// @Success 200 {object} dto.ExecutionPage
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /profiles/{id}/executions [get]
func (h *ExecutionHandler) ListByProfile(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := pgutil.ParseUUID(idStr)
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid profile ID")
		return
	}

	pageReq, err := pagination.ParseRequest(r)
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	cursorTime, cursorID, hasCursor, err := pagination.ParseTimeCursor(pageReq.Cursor, "profile_executions")
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	params := db.ListExecutionsByProfilePageParams{
		ProfileID: id,
		PageLimit: pageReq.Limit + 1,
	}
	if hasCursor {
		params.CursorTime = pgutil.ToTimestamptz(cursorTime)
		params.CursorID = cursorID
	}

	executions, err := h.Repo.ListExecutionsByProfilePage(r.Context(), params)
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}
	executions, hasMore := pagination.TrimPage(executions, pageReq.Limit)

	res := make([]dto.ExecutionResponse, len(executions))
	for i, e := range executions {
		res[i] = dto.ToExecutionResponse(e)
	}

	next := ""
	if hasMore && len(executions) > 0 {
		last := executions[len(executions)-1]
		next = pagination.TimeCursor("profile_executions", last.CreatedAt.Time, last.ID)
	}
	pagination.SetNextLink(w, r, next)
	apiutil.Success(w, http.StatusOK, dto.ExecutionPage{Items: res, NextCursor: next, HasMore: hasMore})
}

// List all executions
// @Summary List all backup executions
// @Description Get a history of all backup executions in the system across all profiles.
// @Description Results are sorted by creation date (newest first).
// @Tags executions
// @Produce json
// @Param limit query int false "Page size (default 50, max 200)"
// @Param cursor query string false "Opaque next cursor"
// @Success 200 {object} dto.ExecutionPage
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /executions [get]
func (h *ExecutionHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	pageReq, err := pagination.ParseRequest(r)
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	cursorTime, cursorID, hasCursor, err := pagination.ParseTimeCursor(pageReq.Cursor, "executions")
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	params := db.ListExecutionsPageParams{PageLimit: pageReq.Limit + 1}
	if hasCursor {
		params.CursorTime = pgutil.ToTimestamptz(cursorTime)
		params.CursorID = cursorID
	}

	executions, err := h.Repo.ListExecutionsPage(r.Context(), params)
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}
	executions, hasMore := pagination.TrimPage(executions, pageReq.Limit)

	res := make([]dto.ExecutionResponse, len(executions))
	for i, e := range executions {
		res[i] = dto.ToExecutionResponse(e)
	}

	next := ""
	if hasMore && len(executions) > 0 {
		last := executions[len(executions)-1]
		next = pagination.TimeCursor("executions", last.CreatedAt.Time, last.ID)
	}
	pagination.SetNextLink(w, r, next)
	apiutil.Success(w, http.StatusOK, dto.ExecutionPage{Items: res, NextCursor: next, HasMore: hasMore})
}

// Get execution
// @Summary Get execution details
// @Description Get full details of a specific backup execution.
// @Description The 'logs' field contains an array of strings formatted as 'TIMESTAMP: MESSAGE'.
// @Description These logs provide a step-by-step trace of the backup pipeline stages.
// @Tags executions
// @Produce json
// @Param id path string true "Execution ID"
// @Success 200 {object} dto.ExecutionResponse
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 404 {object} apiutil.ErrorResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /executions/{id} [get]
func (h *ExecutionHandler) Get(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := pgutil.ParseUUID(idStr)
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid execution ID")
		return
	}

	execution, err := h.Repo.GetExecution(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			apiutil.Error(w, http.StatusNotFound, "Execution not found")
			return
		}
		apiutil.InternalError(w, err)
		return
	}

	apiutil.Success(w, http.StatusOK, dto.ToExecutionResponse(execution))
}

// Pin execution
// @Summary Pin or unpin a backup execution
// @Description Prevent a backup execution from being automatically cleaned up by pinning it.
// @Description Pinned executions are ignored by the retention engine's deletion logic.
// @Tags executions
// @Accept json
// @Param id path string true "Execution ID"
// @Param request body dto.PinRequest true "Pin status"
// @Success 204 "No Content"
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /executions/{id}/pin [post]
func (h *ExecutionHandler) Pin(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := pgutil.ParseUUID(idStr)
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid execution ID")
		return
	}

	var req dto.PinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	params := db.SetExecutionPinnedParams{
		ID:       id,
		IsPinned: req.Pinned,
	}

	if err := h.Repo.SetExecutionPinned(r.Context(), params); err != nil {
		apiutil.InternalError(w, err)
		return
	}

	apiutil.Success(w, http.StatusNoContent, nil)
}
