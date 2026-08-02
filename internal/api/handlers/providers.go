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

type ProviderHandler struct {
	Repo repository.Repository
}

// List storage providers
// @Summary List all storage providers
// @Description Get a list of all configured storage providers.
// @Description The 'config' field varies by 'type':
// @Description - 'local': Use {"path": "/tmp/backups"} to define the base directory on the server.
// @Description - 's3': Use {"region": "us-east-1", "bucket": "my-backups", "access_key": "...", "secret_key": "..."} for AWS S3 compatible storage.
// @Tags storage-providers
// @Produce json
// @Success 200 {array} dto.StorageProviderResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /storage-providers [get]
func (h *ProviderHandler) ListStorage(w http.ResponseWriter, r *http.Request) {
	providers, err := h.Repo.ListStorageProviders(r.Context())
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}

	res := make([]dto.StorageProviderResponse, len(providers))
	for i, p := range providers {
		res[i] = dto.ToStorageProviderResponse(p)
	}

	apiutil.Success(w, http.StatusOK, res)
}

// Create storage provider
// @Summary Create a new storage provider
// @Description Configure a new storage destination for backups.
// @Description You must specify a 'type' and its corresponding 'config' object:
// @Description - 'local': Base directory for storing backups locally. Requires 'path'.
// @Description - 's3': AWS S3 or compatible storage. Requires 'region', 'bucket', 'access_key', and 'secret_key'.
// @Tags storage-providers
// @Accept json
// @Produce json
// @Param provider body dto.StorageProviderRequest true "Storage provider configuration"
// @Success 201 {object} dto.StorageProviderResponse
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /storage-providers [post]
func (h *ProviderHandler) CreateStorage(w http.ResponseWriter, r *http.Request) {
	var req dto.StorageProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := req.Validate(); err != nil {
		apiutil.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	params := db.CreateStorageProviderParams{
		Name:   req.Name,
		Type:   req.Type,
		Config: req.Config,
	}

	provider, err := h.Repo.CreateStorageProvider(r.Context(), params)
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}
	apiutil.Success(w, http.StatusCreated, dto.ToStorageProviderResponse(provider))
}

// Update storage provider
// @Summary Update an existing storage provider
// @Description Update a storage provider's configuration by ID.
// @Description Ensure the 'config' object matches the 'type':
// @Description - 'local': {"path": "/tmp/backups"}
// @Description - 's3': {"region": "us-east-1", "bucket": "my-backups", "access_key": "...", "secret_key": "..."}
// @Tags storage-providers
// @Accept json
// @Produce json
// @Param id path string true "Provider ID"
// @Param provider body dto.StorageProviderRequest true "Updated configuration"
// @Success 200 {object} dto.StorageProviderResponse
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /storage-providers/{id} [put]
func (h *ProviderHandler) UpdateStorage(w http.ResponseWriter, r *http.Request) {
	id, err := pgutil.ParseUUID(chi.URLParam(r, "id"))
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid provider ID")
		return
	}

	var req dto.StorageProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := req.Validate(); err != nil {
		apiutil.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	params := db.UpdateStorageProviderParams{
		ID:     id,
		Name:   req.Name,
		Type:   req.Type,
		Config: req.Config,
	}

	provider, err := h.Repo.UpdateStorageProvider(r.Context(), params)
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}
	apiutil.Success(w, http.StatusOK, dto.ToStorageProviderResponse(provider))
}

// Delete storage provider
// @Summary Delete a storage provider
// @Description Remove a storage provider configuration
// @Tags storage-providers
// @Param id path string true "Provider ID"
// @Success 204 "No Content"
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /storage-providers/{id} [delete]
func (h *ProviderHandler) DeleteStorage(w http.ResponseWriter, r *http.Request) {
	id, err := pgutil.ParseUUID(chi.URLParam(r, "id"))
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid provider ID")
		return
	}

	if err := h.Repo.DeleteStorageProvider(r.Context(), id); err != nil {
		apiutil.InternalError(w, err)
		return
	}
	apiutil.Success(w, http.StatusNoContent, nil)
}

// List notification providers
// @Summary List all notification providers
// @Description Get a list of all configured notification providers.
// @Description The 'config' field varies by 'type':
// @Description - 'discord': Requires {"webhook_url": "https://discord.com/api/webhooks/..."} to send alerts to a Discord channel.
// @Tags notification-providers
// @Produce json
// @Success 200 {array} dto.NotificationProviderResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /notification-providers [get]
func (h *ProviderHandler) ListNotification(w http.ResponseWriter, r *http.Request) {
	providers, err := h.Repo.ListNotificationProviders(r.Context())
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}

	res := make([]dto.NotificationProviderResponse, len(providers))
	for i, p := range providers {
		res[i] = dto.ToNotificationProviderResponse(p)
	}

	apiutil.Success(w, http.StatusOK, res)
}

// Create notification provider
// @Summary Create a new notification provider
// @Description Configure a new destination for backup alerts.
// @Description You must specify a 'type' and its corresponding 'config' object:
// @Description - 'discord': Requires 'webhook_url' in the config object.
// @Tags notification-providers
// @Accept json
// @Produce json
// @Param provider body dto.NotificationProviderRequest true "Notification provider configuration"
// @Success 201 {object} dto.NotificationProviderResponse
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /notification-providers [post]
func (h *ProviderHandler) CreateNotification(w http.ResponseWriter, r *http.Request) {
	var req dto.NotificationProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := req.Validate(); err != nil {
		apiutil.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	params := db.CreateNotificationProviderParams{
		Name:   req.Name,
		Type:   req.Type,
		Config: req.Config,
	}

	provider, err := h.Repo.CreateNotificationProvider(r.Context(), params)
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}
	apiutil.Success(w, http.StatusCreated, dto.ToNotificationProviderResponse(provider))
}

// Update notification provider
// @Summary Update an existing notification provider
// @Description Update a notification provider's configuration by ID.
// @Description Ensure the 'config' object matches the 'type':
// @Description - 'discord': {"webhook_url": "https://discord.com/api/webhooks/..."}
// @Tags notification-providers
// @Accept json
// @Produce json
// @Param id path string true "Provider ID"
// @Param provider body dto.NotificationProviderRequest true "Updated configuration"
// @Success 200 {object} dto.NotificationProviderResponse
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /notification-providers/{id} [put]
func (h *ProviderHandler) UpdateNotification(w http.ResponseWriter, r *http.Request) {
	id, err := pgutil.ParseUUID(chi.URLParam(r, "id"))
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid provider ID")
		return
	}

	var req dto.NotificationProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := req.Validate(); err != nil {
		apiutil.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	params := db.UpdateNotificationProviderParams{
		ID:     id,
		Name:   req.Name,
		Type:   req.Type,
		Config: req.Config,
	}

	provider, err := h.Repo.UpdateNotificationProvider(r.Context(), params)
	if err != nil {
		apiutil.InternalError(w, err)
		return
	}
	apiutil.Success(w, http.StatusOK, dto.ToNotificationProviderResponse(provider))
}

// Delete notification provider
// @Summary Delete a notification provider
// @Description Remove a notification provider configuration
// @Tags notification-providers
// @Param id path string true "Provider ID"
// @Success 204 "No Content"
// @Failure 500 {object} apiutil.ErrorResponse
// @Router /notification-providers/{id} [delete]
func (h *ProviderHandler) DeleteNotification(w http.ResponseWriter, r *http.Request) {
	id, err := pgutil.ParseUUID(chi.URLParam(r, "id"))
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid provider ID")
		return
	}

	if err := h.Repo.DeleteNotificationProvider(r.Context(), id); err != nil {
		apiutil.InternalError(w, err)
		return
	}
	apiutil.Success(w, http.StatusNoContent, nil)
}
