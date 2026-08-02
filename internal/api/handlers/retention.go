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

type RetentionHandler struct {
	Repo repository.Repository
}

// List retention policies
// @Summary List all retention policies
// @Description Get a list of all configured retention policies.
// @Description Policies define how many backups are kept for different time intervals:
// @Description - keep_hourly: Last N hours.
// @Description - keep_daily: Last N days.
// @Description - keep_weekly: Last N weeks.
// @Description - yearly_month: The month (1-12) chosen for yearly preservation.
// @Tags retention
// @Produce json
// @Success 200 {array} dto.RetentionPolicyResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /retention-policies [get]
func (h *RetentionHandler) List(w http.ResponseWriter, r *http.Request) {
	policies, err := h.Repo.ListRetentionPolicies(r.Context())
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}

	res := make([]dto.RetentionPolicyResponse, len(policies))
	for i, p := range policies {
		res[i] = dto.ToRetentionPolicyResponse(p)
	}

	apiutil.Success(w, http.StatusOK, res)
}

// Create retention policy
// @Summary Create a new retention policy
// @Description Define a new policy for automatic backup cleanup.
// @Description Fields define the 'Grandfather-Father-Son' retention strategy:
// @Description - keep_daily: 7 means keep one backup per day for 7 days.
// @Description - yearly_month: 1 means the January backup is kept as the yearly one.
// @Tags retention
// @Accept json
// @Produce json
// @Param policy body dto.RetentionPolicyRequest true "Retention policy configuration"
// @Success 201 {object} dto.RetentionPolicyResponse
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /retention-policies [post]
func (h *RetentionHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req dto.RetentionPolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := req.Validate(); err != nil {
		apiutil.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	params := db.CreateRetentionPolicyParams{
		Name:        req.Name,
		KeepHourly:  req.KeepHourly,
		KeepDaily:   req.KeepDaily,
		KeepWeekly:  req.KeepWeekly,
		KeepMonthly: req.KeepMonthly,
		KeepYearly:  req.KeepYearly,
		YearlyMonth: req.YearlyMonth,
	}

	policy, err := h.Repo.CreateRetentionPolicy(r.Context(), params)
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}
	apiutil.Success(w, http.StatusCreated, dto.ToRetentionPolicyResponse(policy))
}

// Update retention policy
// @Summary Update an existing retention policy
// @Description Update a retention policy's configuration by ID.
// @Description Allows changing how many backups are kept at each level.
// @Tags retention
// @Accept json
// @Produce json
// @Param id path string true "Policy ID"
// @Param policy body dto.RetentionPolicyRequest true "Updated configuration"
// @Success 200 {object} dto.RetentionPolicyResponse
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /retention-policies/{id} [put]
func (h *RetentionHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := pgutil.ParseUUID(chi.URLParam(r, "id"))
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid policy ID")
		return
	}

	var req dto.RetentionPolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := req.Validate(); err != nil {
		apiutil.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	params := db.UpdateRetentionPolicyParams{
		ID:          id,
		Name:        req.Name,
		KeepHourly:  req.KeepHourly,
		KeepDaily:   req.KeepDaily,
		KeepWeekly:  req.KeepWeekly,
		KeepMonthly: req.KeepMonthly,
		KeepYearly:  req.KeepYearly,
		YearlyMonth: req.YearlyMonth,
	}

	policy, err := h.Repo.UpdateRetentionPolicy(r.Context(), params)
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}
	apiutil.Success(w, http.StatusOK, dto.ToRetentionPolicyResponse(policy))
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
	id, err := pgutil.ParseUUID(chi.URLParam(r, "id"))
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid policy ID")
		return
	}

	if err := h.Repo.DeleteRetentionPolicy(r.Context(), id); err != nil {
		apiutil.InternalError(w, err)
		return
	}
	apiutil.Success(w, http.StatusNoContent, nil)
}
