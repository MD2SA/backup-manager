package handlers

import (
	"context"
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
	"github.com/jackc/pgx/v5/pgtype"
)

type ProfileHandler struct {
	Repo            repository.Repository
	OnTrigger       func(pgtype.UUID) error
	OnSchedulerSync func(context.Context) error
}

// List profiles
// @Summary List all backup profiles
// @Description Get a list of all configured backup profiles.
// @Description Profiles link databases to storage destinations via schedules.
// @Tags profiles
// @Produce json
// @Param limit query int false "Page size (default 50, max 200)"
// @Param cursor query string false "Opaque next cursor"
// @Success 200 {object} dto.ProfilePage
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /profiles [get]
func (h *ProfileHandler) List(w http.ResponseWriter, r *http.Request) {
	pageReq, err := pagination.ParseRequest(r)
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	cursorName, cursorID, hasCursor, err := pagination.ParseNameCursor(pageReq.Cursor, "profiles")
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	params := db.ListProfilesPageParams{PageLimit: pageReq.Limit + 1}
	if hasCursor {
		params.CursorName = pgutil.ToText(cursorName)
		params.CursorID = cursorID
	}

	profiles, err := h.Repo.ListProfilesPage(r.Context(), params)
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}
	profiles, hasMore := pagination.TrimPage(profiles, pageReq.Limit)

	res := make([]dto.ProfileResponse, len(profiles))
	for i, p := range profiles {
		storageIDs := make([]string, len(p.StorageProviderIDs))
		for j, id := range p.StorageProviderIDs {
			storageIDs[j] = pgutil.UUIDToString(id)
		}
		notificationIDs := make([]string, len(p.NotificationProviderIDs))
		for j, id := range p.NotificationProviderIDs {
			notificationIDs[j] = pgutil.UUIDToString(id)
		}
		res[i] = dto.ToProfileResponse(p.Profile, storageIDs, notificationIDs)
	}

	next := ""
	if hasMore && len(profiles) > 0 {
		last := profiles[len(profiles)-1]
		next = pagination.NameCursor("profiles", last.Name, last.ID)
	}
	pagination.SetNextLink(w, r, next)
	apiutil.Success(w, http.StatusOK, dto.ProfilePage{Items: res, NextCursor: next, HasMore: hasMore})
}

// Get profile
// @Summary Get a backup profile
// @Description Get a profile by ID, including its linked storage and notification providers.
// @Tags profiles
// @Produce json
// @Param id path string true "Profile ID"
// @Success 200 {object} dto.ProfileResponse
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 404 {object} apiutil.ErrorResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /profiles/{id} [get]
func (h *ProfileHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := pgutil.ParseUUID(chi.URLParam(r, "id"))
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid profile ID")
		return
	}

	p, err := h.Repo.GetProfile(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			apiutil.Error(w, http.StatusNotFound, "Profile not found")
			return
		}
		apiutil.InternalError(w, err)
		return
	}

	storageIDs := make([]string, len(p.StorageProviderIDs))
	for i, storageID := range p.StorageProviderIDs {
		storageIDs[i] = pgutil.UUIDToString(storageID)
	}
	notificationIDs := make([]string, len(p.NotificationProviderIDs))
	for i, notificationID := range p.NotificationProviderIDs {
		notificationIDs[i] = pgutil.UUIDToString(notificationID)
	}

	apiutil.Success(w, http.StatusOK, dto.ToProfileResponse(p.Profile, storageIDs, notificationIDs))
}

// Create profile
// @Summary Create a new backup profile
// @Description Create a new backup profile with the specified configuration.
// @Description Requires at least one storage_provider_ids and a retention_policy_id.
// @Description The schedule must be a valid cron expression (e.g., "0 0 * * *").
// @Tags profiles
// @Accept json
// @Produce json
// @Param profile body dto.ProfileRequest true "Profile configuration"
// @Success 201 {object} dto.ProfileResponse
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /profiles [post]
func (h *ProfileHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req dto.ProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := req.Validate(); err != nil {
		apiutil.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	storageIDs := make([]pgtype.UUID, len(req.StorageProviderIDs))
	for i, idStr := range req.StorageProviderIDs {
		storageIDs[i], _ = pgutil.ParseUUID(idStr)
	}

	notificationIDs := make([]pgtype.UUID, len(req.NotificationProviderIDs))
	for i, idStr := range req.NotificationProviderIDs {
		notificationIDs[i], _ = pgutil.ParseUUID(idStr)
	}

	retentionID, _ := pgutil.ParseUUID(req.RetentionPolicyID)

	params := db.CreateProfileParams{
		Name:              req.Name,
		Description:       pgutil.ToText(req.Description),
		Enabled:           req.Enabled,
		Schedule:          req.Schedule,
		RetentionPolicyID: retentionID,
		CompressionType:   req.CompressionType,
		CompressionLevel:  req.CompressionLevel,
		EncryptionEnabled: req.EncryptionEnabled,
	}

	profile, err := h.Repo.CreateProfile(r.Context(), params, storageIDs, notificationIDs)
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}
	if !h.syncScheduler(w, r) {
		return
	}

	apiutil.Success(w, http.StatusCreated, dto.ToProfileResponse(profile.Profile, req.StorageProviderIDs, req.NotificationProviderIDs))
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
// @Param profile body dto.ProfileRequest true "Updated profile configuration"
// @Success 200 {object} dto.ProfileResponse
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /profiles/{id} [put]
func (h *ProfileHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := pgutil.ParseUUID(chi.URLParam(r, "id"))
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid profile ID")
		return
	}

	var req dto.ProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := req.Validate(); err != nil {
		apiutil.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	storageIDs := make([]pgtype.UUID, len(req.StorageProviderIDs))
	for i, idStr := range req.StorageProviderIDs {
		storageIDs[i], _ = pgutil.ParseUUID(idStr)
	}

	notificationIDs := make([]pgtype.UUID, len(req.NotificationProviderIDs))
	for i, idStr := range req.NotificationProviderIDs {
		notificationIDs[i], _ = pgutil.ParseUUID(idStr)
	}

	retentionID, _ := pgutil.ParseUUID(req.RetentionPolicyID)

	params := db.UpdateProfileParams{
		ID:                id,
		Name:              req.Name,
		Description:       pgutil.ToText(req.Description),
		Enabled:           req.Enabled,
		Schedule:          req.Schedule,
		RetentionPolicyID: retentionID,
		CompressionType:   req.CompressionType,
		CompressionLevel:  req.CompressionLevel,
		EncryptionEnabled: req.EncryptionEnabled,
	}

	profile, err := h.Repo.UpdateProfile(r.Context(), params, storageIDs, notificationIDs)
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}
	if !h.syncScheduler(w, r) {
		return
	}

	apiutil.Success(w, http.StatusOK, dto.ToProfileResponse(profile.Profile, req.StorageProviderIDs, req.NotificationProviderIDs))
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
	id, err := pgutil.ParseUUID(chi.URLParam(r, "id"))
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid profile ID")
		return
	}

	if err := h.Repo.DeleteProfile(r.Context(), id); err != nil {
		apiutil.InternalError(w, err)
		return
	}
	if !h.syncScheduler(w, r) {
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
	id, err := pgutil.ParseUUID(chi.URLParam(r, "id"))
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
// @Summary Enable scheduling for a profile
// @Description Enable scheduled backups for this profile. Other profiles remain enabled.
// @Tags profiles
// @Param id path string true "Profile ID"
// @Success 200 {object} dto.ProfileResponse
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /profiles/{id}/activate [post]
func (h *ProfileHandler) Activate(w http.ResponseWriter, r *http.Request) {
	h.setEnabled(w, r, true)
}

// Deactivate profile
// @Summary Disable scheduling for a profile
// @Description Disable scheduled backups for this profile without affecting other profiles.
// @Tags profiles
// @Param id path string true "Profile ID"
// @Success 200 {object} dto.ProfileResponse
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /profiles/{id}/deactivate [post]
func (h *ProfileHandler) Deactivate(w http.ResponseWriter, r *http.Request) {
	h.setEnabled(w, r, false)
}

func (h *ProfileHandler) setEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
	id, err := pgutil.ParseUUID(chi.URLParam(r, "id"))
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid profile ID")
		return
	}

	if _, err := h.Repo.SetProfileEnabled(r.Context(), id, enabled); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			apiutil.Error(w, http.StatusNotFound, "Profile not found")
			return
		}
		apiutil.InternalError(w, err)
		return
	}

	if !h.syncScheduler(w, r) {
		return
	}

	p, err := h.Repo.GetProfile(r.Context(), id)
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}

	storageIDs := make([]string, len(p.StorageProviderIDs))
	for i, storageID := range p.StorageProviderIDs {
		storageIDs[i] = pgutil.UUIDToString(storageID)
	}
	notificationIDs := make([]string, len(p.NotificationProviderIDs))
	for i, notificationID := range p.NotificationProviderIDs {
		notificationIDs[i] = pgutil.UUIDToString(notificationID)
	}

	apiutil.Success(w, http.StatusOK, dto.ToProfileResponse(p.Profile, storageIDs, notificationIDs))
}

func (h *ProfileHandler) syncScheduler(w http.ResponseWriter, r *http.Request) bool {
	if h.OnSchedulerSync == nil {
		return true
	}
	if err := h.OnSchedulerSync(r.Context()); err != nil {
		apiutil.InternalError(w, err)
		return false
	}
	return true
}
