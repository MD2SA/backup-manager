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

type ProviderHandler struct {
	Repo repository.Repository
}

// List storage providers
// @Summary List all storage providers
// @Description Get a list of all configured storage providers.
// @Description The 'config' field varies by type:
// @Description - local: {"path": "/tmp/backups"}
// @Description - s3: {"region": "us-east-1", "bucket": "...", "access_key": "...", "secret_key": "..."}
// @Tags providers
// @Produce json
// @Success 200 {array} db.StorageProvider
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /storage-providers [get]
func (h *ProviderHandler) ListStorage(w http.ResponseWriter, r *http.Request) {
	providers, err := h.Repo.ListStorageProviders(r.Context())
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}
	apiutil.Success(w, http.StatusOK, providers)
}

// Create storage provider
// @Summary Create a new storage provider
// @Description Configure a new storage destination for backups.
// @Description Supported types and configurations:
// @Description - local: {"path": "/tmp/backups"}
// @Description - s3: {"region": "us-east-1", "bucket": "my-backups", "access_key": "...", "secret_key": "..."}
// @Tags providers
// @Accept json
// @Produce json
// @Param provider body db.CreateStorageProviderParams true "Storage provider configuration"
// @Success 201 {object} db.StorageProvider
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /storage-providers [post]
func (h *ProviderHandler) CreateStorage(w http.ResponseWriter, r *http.Request) {
	var params db.CreateStorageProviderParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	params.ID = pgutil.NewUUID()
	provider, err := h.Repo.CreateStorageProvider(r.Context(), params)
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}
	apiutil.Success(w, http.StatusCreated, provider)
}

// Update storage provider
// @Summary Update an existing storage provider
// @Description Update a storage provider's configuration by ID.
// @Description Supported types and configurations:
// @Description - local: {"path": "/tmp/backups"}
// @Description - s3: {"region": "us-east-1", "bucket": "my-backups", "access_key": "...", "secret_key": "..."}
// @Tags providers
// @Accept json
// @Produce json
// @Param id path string true "Provider ID"
// @Param provider body db.UpdateStorageProviderParams true "Updated configuration"
// @Success 200 {object} db.StorageProvider
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /storage-providers/{id} [put]
func (h *ProviderHandler) UpdateStorage(w http.ResponseWriter, r *http.Request) {
	id, _ := pgutil.ParseUUID(chi.URLParam(r, "id"))
	var params db.UpdateStorageProviderParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	params.ID = id
	provider, err := h.Repo.UpdateStorageProvider(r.Context(), params)
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}
	apiutil.Success(w, http.StatusOK, provider)
}

// Delete storage provider
// @Summary Delete a storage provider
// @Description Remove a storage provider configuration
// @Tags providers
// @Param id path string true "Provider ID"
// @Success 204 "No Content"
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /storage-providers/{id} [delete]
func (h *ProviderHandler) DeleteStorage(w http.ResponseWriter, r *http.Request) {
	id, _ := pgutil.ParseUUID(chi.URLParam(r, "id"))
	if err := h.Repo.DeleteStorageProvider(r.Context(), id); err != nil {
		apiutil.InternalError(w, err)
		return
	}
	apiutil.Success(w, http.StatusNoContent, nil)
}

// List notification providers
// @Summary List all notification providers
// @Description Get a list of all configured notification providers.
// @Description The 'config' field varies by type:
// @Description - discord: {"webhook_url": "https://discord.com/api/webhooks/..."}
// @Tags providers
// @Produce json
// @Success 200 {array} db.NotificationProvider
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /notification-providers [get]
func (h *ProviderHandler) ListNotification(w http.ResponseWriter, r *http.Request) {
	providers, err := h.Repo.ListNotificationProviders(r.Context())
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}
	apiutil.Success(w, http.StatusOK, providers)
}

// Create notification provider
// @Summary Create a new notification provider
// @Description Configure a new destination for backup alerts.
// @Description Supported types and configurations:
// @Description - discord: {"webhook_url": "https://discord.com/api/webhooks/..."}
// @Tags providers
// @Accept json
// @Produce json
// @Param provider body db.CreateNotificationProviderParams true "Notification provider configuration"
// @Success 201 {object} db.NotificationProvider
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /notification-providers [post]
func (h *ProviderHandler) CreateNotification(w http.ResponseWriter, r *http.Request) {
	var params db.CreateNotificationProviderParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	params.ID = pgutil.NewUUID()
	provider, err := h.Repo.CreateNotificationProvider(r.Context(), params)
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}
	apiutil.Success(w, http.StatusCreated, provider)
}

// Update notification provider
// @Summary Update an existing notification provider
// @Description Update a notification provider's configuration by ID.
// @Description Supported types and configurations:
// @Description - discord: {"webhook_url": "https://discord.com/api/webhooks/..."}
// @Tags providers
// @Accept json
// @Produce json
// @Param id path string true "Provider ID"
// @Param provider body db.UpdateNotificationProviderParams true "Updated configuration"
// @Success 200 {object} db.NotificationProvider
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /notification-providers/{id} [put]
func (h *ProviderHandler) UpdateNotification(w http.ResponseWriter, r *http.Request) {
	id, _ := pgutil.ParseUUID(chi.URLParam(r, "id"))
	var params db.UpdateNotificationProviderParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	params.ID = id
	provider, err := h.Repo.UpdateNotificationProvider(r.Context(), params)
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}
	apiutil.Success(w, http.StatusOK, provider)
}

// Delete notification provider
// @Summary Delete a notification provider
// @Description Remove a notification provider configuration
// @Tags providers
// @Param id path string true "Provider ID"
// @Success 204 "No Content"
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /notification-providers/{id} [delete]
func (h *ProviderHandler) DeleteNotification(w http.ResponseWriter, r *http.Request) {
	id, _ := pgutil.ParseUUID(chi.URLParam(r, "id"))
	if err := h.Repo.DeleteNotificationProvider(r.Context(), id); err != nil {
		apiutil.InternalError(w, err)
		return
	}
	apiutil.Success(w, http.StatusNoContent, nil)
}
