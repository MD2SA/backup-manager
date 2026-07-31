package handlers

import (
	"context"
	"net/http"

	"github.com/MD2SA/backup-manager/internal/pkg/apiutil"
	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type RestoreHandler struct {
	Repo     repository.Repository
	Executor func(ctx context.Context, executionID pgtype.UUID) error
}

// Restore execution
// @Summary Restore a backup
// @Description Trigger a restoration process from a specific backup execution
// @Tags restore
// @Param id path string true "Execution ID"
// @Success 202 "Accepted"
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /executions/{id}/restore [post]
func (h *RestoreHandler) Trigger(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := pgutil.ParseUUID(idStr)
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid execution ID")
		return
	}

	if err := h.Executor(r.Context(), id); err != nil {
		apiutil.InternalError(w, err)
		return
	}

	apiutil.Success(w, http.StatusAccepted, nil)
}
