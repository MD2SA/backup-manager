package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/MD2SA/backup-manager/internal/pkg/apiutil"
	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/go-chi/chi/v5"
)

type ExecutionHandler struct {
	Repo repository.Repository
}

// List executions by profile
// @Summary List executions for a specific profile
// @Description Get a history of all backup executions for a profile
// @Tags executions
// @Produce json
// @Param id path string true "Profile ID"
// @Success 200 {array} db.Execution
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

	executions, err := h.Repo.ListExecutionsByProfile(r.Context(), id)
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}

	apiutil.Success(w, http.StatusOK, executions)
}

// Get execution
// @Summary Get execution details
// @Description Get full details of a specific backup execution, including logs
// @Tags executions
// @Produce json
// @Param id path string true "Execution ID"
// @Success 200 {object} db.Execution
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
		apiutil.InternalError(w, err)
		return
	}

	apiutil.Success(w, http.StatusOK, execution)
}

// Pin execution
// @Summary Pin or unpin a backup execution
// @Description Prevent a backup execution from being automatically cleaned up by pinning it
// @Tags executions
// @Accept json
// @Param id path string true "Execution ID"
// @Param request body object true "Pin status (e.g. {'pinned': true})"
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

	var req struct {
		Pinned bool `json:"pinned"`
	}
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
