package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/MD2SA/backup-manager/internal/pkg/apiutil"
	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/robfig/cron/v3"
)

type ProfileHandler struct {
	Repo       repository.Repository
	OnTrigger  func(pgtype.UUID) error
	OnActivate func(db.Profile)
}

// List profiles
// @Summary List all backup profiles
// @Description Get a list of all configured backup profiles.
// @Description Profiles link databases to storage destinations via schedules.
// @Tags profiles
// @Produce json
// @Success 200 {array} db.Profile
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /profiles [get]
func (h *ProfileHandler) List(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.Repo.ListProfiles(r.Context())
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}

	apiutil.Success(w, http.StatusOK, profiles)
}

// Create profile
// @Summary Create a new backup profile
// @Description Create a new backup profile with the specified configuration.
// @Description Requires a valid storage_provider_id and retention_policy_id.
// @Description The schedule must be a valid cron expression (e.g., "0 0 * * *").
// @Tags profiles
// @Accept json
// @Produce json
// @Param profile body db.CreateProfileParams true "Profile configuration"
// @Success 201 {object} db.Profile
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /profiles [post]
func (h *ProfileHandler) Create(w http.ResponseWriter, r *http.Request) {
	var params db.CreateProfileParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if params.Name == "" {
		apiutil.Error(w, http.StatusBadRequest, "Name is required")
		return
	}

	if _, err := cron.ParseStandard(params.Schedule); err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid cron schedule")
		return
	}

	params.ID = pgutil.NewUUID()
	profile, err := h.Repo.CreateProfile(r.Context(), params)
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}

	apiutil.Success(w, http.StatusCreated, profile)
}

// Update profile
// @Summary Update an existing backup profile
// @Description Update a backup profile by its ID.
// @Description The schedule must be a valid cron expression:
// @Description - "0 * * * *" (Hourly)
// @Description - "0 2 * * *" (Daily at 2:00 AM)
// @Description - "0 0 * * 0" (Weekly on Sunday)
// @Tags profiles
// @Accept json
// @Produce json
// @Param id path string true "Profile ID"
// @Param profile body db.UpdateProfileParams true "Updated profile configuration"
// @Success 200 {object} db.Profile
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /profiles/{id} [put]
func (h *ProfileHandler) Update(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := pgutil.ParseUUID(idStr)
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid profile ID")
		return
	}

	var params db.UpdateProfileParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if params.Name == "" {
		apiutil.Error(w, http.StatusBadRequest, "Name is required")
		return
	}

	if _, err := cron.ParseStandard(params.Schedule); err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid cron schedule")
		return
	}

	params.ID = id
	profile, err := h.Repo.UpdateProfile(r.Context(), params)
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}

	apiutil.Success(w, http.StatusOK, profile)
}

// Delete profile
// @Summary Delete a backup profile
// @Description Delete a backup profile by its ID
// @Tags profiles
// @Param id path string true "Profile ID"
// @Success 204 "No Content"
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /profiles/{id} [delete]
func (h *ProfileHandler) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := pgutil.ParseUUID(idStr)
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid profile ID")
		return
	}

	if err := h.Repo.DeleteProfile(r.Context(), id); err != nil {
		apiutil.InternalError(w, err)
		return
	}

	apiutil.Success(w, http.StatusNoContent, nil)
}

// Run profile now
// @Summary Run a backup profile immediately
// @Description Trigger an asynchronous backup execution for a specific profile now.
// @Description This bypasses the schedule and enqueues the job in the runner.
// @Tags profiles
// @Param id path string true "Profile ID"
// @Success 202 "Accepted"
// @Failure 400 {object} apiutil.ErrorResponse
// @Router /profiles/{id}/run [post]
func (h *ProfileHandler) RunNow(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := pgutil.ParseUUID(idStr)
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid profile ID")
		return
	}

	if h.OnTrigger != nil {
		if err := h.OnTrigger(id); err != nil {
			apiutil.Error(w, http.StatusConflict, err.Error())
			return
		}
	}

	apiutil.Success(w, http.StatusAccepted, nil)
}

// Activate profile
// @Summary Activate a backup profile (sets as scheduled)
// @Description Set a profile as the active one for scheduled backups.
// @Description This updates the system scheduler to use this profile's schedule.
// @Description Note: The current system logic may only support one active profile at a time.
// @Tags profiles
// @Param id path string true "Profile ID"
// @Success 200 {object} db.Profile
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /profiles/{id}/activate [post]
func (h *ProfileHandler) Activate(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := pgutil.ParseUUID(idStr)
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid profile ID")
		return
	}

	if err := h.Repo.ActivateProfile(r.Context(), id); err != nil {
		apiutil.InternalError(w, err)
		return
	}

	p, err := h.Repo.GetProfile(r.Context(), id)
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}

	if h.OnActivate != nil {
		h.OnActivate(p)
	}

	apiutil.Success(w, http.StatusOK, p)
}
