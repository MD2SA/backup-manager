package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/MD2SA/backup-manager/internal/api/dto"
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
// @Description Get a history of all backup executions for a profile.
// @Description Results are sorted by creation date (newest first).
// @Description Status can be: pending, running, success, failed.
// @Description The 'logs' field contains an array of strings formatted as 'TIMESTAMP: MESSAGE'.
// @Tags executions
// @Produce json
// @Param id path string true "Profile ID"
// @Success 200 {array} dto.ExecutionResponse
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

	res := make([]dto.ExecutionResponse, len(executions))
	for i, e := range executions {
		res[i] = dto.ToExecutionResponse(e)
	}

	apiutil.Success(w, http.StatusOK, res)
}

// List all executions
// @Summary List all backup executions
// @Description Get a history of all backup executions in the system across all profiles.
// @Description Results are sorted by creation date (newest first).
// @Tags executions
// @Produce json
// @Success 200 {array} dto.ExecutionResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /executions [get]
func (h *ExecutionHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	executions, err := h.Repo.ListAllExecutions(r.Context())
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}

	res := make([]dto.ExecutionResponse, len(executions))
	for i, e := range executions {
		res[i] = dto.ToExecutionResponse(e)
	}

	apiutil.Success(w, http.StatusOK, res)
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
