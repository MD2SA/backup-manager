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

type RetentionHandler struct {
	Repo repository.Repository
}

// List retention policies
// @Summary List all retention policies
// @Description Get a list of all configured retention policies (how many backups to keep)
// @Tags retention
// @Produce json
// @Success 200 {array} db.RetentionPolicy
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /retention-policies [get]
func (h *RetentionHandler) List(w http.ResponseWriter, r *http.Request) {
	policies, err := h.Repo.ListRetentionPolicies(r.Context())
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}
	apiutil.Success(w, http.StatusOK, policies)
}

// Create retention policy
// @Summary Create a new retention policy
// @Description Define a new policy for automatic backup cleanup
// @Tags retention
// @Accept json
// @Produce json
// @Param policy body db.CreateRetentionPolicyParams true "Retention policy configuration"
// @Success 201 {object} db.RetentionPolicy
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /retention-policies [post]
func (h *RetentionHandler) Create(w http.ResponseWriter, r *http.Request) {
	var params db.CreateRetentionPolicyParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	params.ID = pgutil.NewUUID()
	policy, err := h.Repo.CreateRetentionPolicy(r.Context(), params)
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}
	apiutil.Success(w, http.StatusCreated, policy)
}

// Update retention policy
// @Summary Update an existing retention policy
// @Description Update a retention policy's configuration by ID
// @Tags retention
// @Accept json
// @Produce json
// @Param id path string true "Policy ID"
// @Param policy body db.UpdateRetentionPolicyParams true "Updated configuration"
// @Success 200 {object} db.RetentionPolicy
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /retention-policies/{id} [put]
func (h *RetentionHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, _ := pgutil.ParseUUID(chi.URLParam(r, "id"))
	var params db.UpdateRetentionPolicyParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	params.ID = id
	policy, err := h.Repo.UpdateRetentionPolicy(r.Context(), params)
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}
	apiutil.Success(w, http.StatusOK, policy)
}

// Delete retention policy
// @Summary Delete a retention policy
// @Description Remove a retention policy configuration
// @Tags retention
// @Param id path string true "Policy ID"
// @Success 204 "No Content"
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /retention-policies/{id} [delete]
func (h *RetentionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, _ := pgutil.ParseUUID(chi.URLParam(r, "id"))
	if err := h.Repo.DeleteRetentionPolicy(r.Context(), id); err != nil {
		apiutil.InternalError(w, err)
		return
	}
	apiutil.Success(w, http.StatusNoContent, nil)
}
