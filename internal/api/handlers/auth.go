package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/MD2SA/backup-manager/internal/api/dto"
	"github.com/MD2SA/backup-manager/internal/auth"
	"github.com/MD2SA/backup-manager/internal/pagination"
	"github.com/MD2SA/backup-manager/internal/pkg/apiutil"
	"github.com/MD2SA/backup-manager/internal/pkg/pgutil"
	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type AuthHandler struct {
	Service *auth.Service
}

// Me
// @Summary Get the authenticated user
// @Description Get the user, key ID and key prefix associated with the current API key.
// @Tags auth
// @Produce json
// @Success 200 {object} dto.MeResponse
// @Failure 401 {object} apiutil.ErrorResponse
// @Router /auth/me [get]
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		apiutil.Error(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	if principal.Insecure() {
		apiutil.Success(w, http.StatusOK, dto.MeResponse{
			User:     dto.UserResponse{Email: principal.Email, Name: principal.Name, Active: true},
			Insecure: true,
		})
		return
	}

	user, err := h.Service.GetUser(r.Context(), principal.UserID)
	if err != nil {
		writeAuthError(w, err)
		return
	}

	apiutil.Success(w, http.StatusOK, dto.MeResponse{
		User:      dto.ToUserResponse(user),
		KeyID:     pgutil.UUIDToString(principal.KeyID),
		KeyPrefix: principal.KeyPrefix,
	})
}

// List own API keys
// @Summary List the current user's API keys
// @Description List key metadata (never the key itself).
// @Tags auth
// @Produce json
// @Param limit query int false "Page size (default 50, max 200)"
// @Param cursor query string false "Opaque next cursor"
// @Success 200 {object} dto.APIKeyPage
// @Failure 401 {object} apiutil.ErrorResponse
// @Router /auth/keys [get]
func (h *AuthHandler) ListKeys(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok || principal.Insecure() {
		apiutil.Error(w, http.StatusBadRequest, "API key management requires a database user")
		return
	}

	listAPIKeysPage(w, r, h.Service, principal.UserID)
}

// Create own API key
// @Summary Create an API key for the current user
// @Description Create a new API key. The plaintext key is returned exactly once.
// @Tags auth
// @Accept json
// @Produce json
// @Param request body dto.CreateAPIKeyRequest true "Key metadata"
// @Success 201 {object} dto.CreateAPIKeyResponse
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 401 {object} apiutil.ErrorResponse
// @Router /auth/keys [post]
func (h *AuthHandler) CreateKey(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok || principal.Insecure() {
		apiutil.Error(w, http.StatusBadRequest, "API key management requires a database user")
		return
	}

	var req dto.CreateAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if err := req.Validate(); err != nil {
		apiutil.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	key, raw, err := h.Service.CreateAPIKey(r.Context(), principal.UserID, req.Name, expiresAt(req.ExpiresInDays))
	if err != nil {
		writeAuthError(w, err)
		return
	}
	apiutil.Success(w, http.StatusCreated, dto.CreateAPIKeyResponse{
		APIKeyResponse: dto.ToAPIKeyResponse(key),
		Key:            raw,
	})
}

// Revoke own API key
// @Summary Revoke one of the current user's API keys
// @Tags auth
// @Param id path string true "API key ID"
// @Success 200 {object} dto.APIKeyResponse
// @Failure 400 {object} apiutil.ErrorResponse
// @Failure 403 {object} apiutil.ErrorResponse
// @Failure 404 {object} apiutil.ErrorResponse
// @Router /auth/keys/{id}/revoke [post]
func (h *AuthHandler) RevokeKey(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok || principal.Insecure() {
		apiutil.Error(w, http.StatusBadRequest, "API key management requires a database user")
		return
	}
	keyID, err := pgutil.ParseUUID(chi.URLParam(r, "id"))
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid API key ID")
		return
	}

	key, err := h.Service.RevokeAPIKeyForUser(r.Context(), keyID, principal.UserID)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	apiutil.Success(w, http.StatusOK, dto.ToAPIKeyResponse(key))
}

type UserHandler struct {
	Service *auth.Service
}

// List users
// @Summary List users
// @Tags users
// @Produce json
// @Param limit query int false "Page size (default 50, max 200)"
// @Param cursor query string false "Opaque next cursor"
// @Success 200 {object} dto.UserPage
// @Router /users [get]
func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	pageReq, err := pagination.ParseRequest(r)
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	cursorTime, cursorID, hasCursor, err := pagination.ParseTimeCursor(pageReq.Cursor, "users")
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	params := db.ListUsersPageParams{PageLimit: pageReq.Limit + 1}
	if hasCursor {
		params.CursorTime = pgutil.ToTimestamptz(cursorTime)
		params.CursorID = cursorID
	}

	users, err := h.Service.ListUsersPage(r.Context(), params)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	users, hasMore := pagination.TrimPage(users, pageReq.Limit)
	next := ""
	if hasMore && len(users) > 0 {
		last := users[len(users)-1]
		next = pagination.TimeCursor("users", last.CreatedAt.Time, last.ID)
	}
	pagination.SetNextLink(w, r, next)

	res := make([]dto.UserResponse, len(users))
	for i, user := range users {
		res[i] = dto.ToUserResponse(user)
	}
	apiutil.Success(w, http.StatusOK, dto.UserPage{Items: res, NextCursor: next, HasMore: hasMore})
}

// Create user
// @Summary Create a user
// @Tags users
// @Accept json
// @Produce json
// @Param request body dto.CreateUserRequest true "User"
// @Success 201 {object} dto.UserResponse
// @Failure 409 {object} apiutil.ErrorResponse
// @Router /users [post]
func (h *UserHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if err := req.Validate(); err != nil {
		apiutil.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	user, err := h.Service.CreateUser(r.Context(), req.Email, req.Name)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	apiutil.Success(w, http.StatusCreated, dto.ToUserResponse(user))
}

// Get user
// @Summary Get a user
// @Tags users
// @Param id path string true "User ID"
// @Success 200 {object} dto.UserResponse
// @Router /users/{id} [get]
func (h *UserHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := pgutil.ParseUUID(chi.URLParam(r, "id"))
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid user ID")
		return
	}
	user, err := h.Service.GetUser(r.Context(), id)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	apiutil.Success(w, http.StatusOK, dto.ToUserResponse(user))
}

// Update user
// @Summary Update a user
// @Tags users
// @Param id path string true "User ID"
// @Param request body dto.UpdateUserRequest true "User"
// @Success 200 {object} dto.UserResponse
// @Router /users/{id} [put]
func (h *UserHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := pgutil.ParseUUID(chi.URLParam(r, "id"))
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid user ID")
		return
	}
	var req dto.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if err := req.Validate(); err != nil {
		apiutil.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	existing, err := h.Service.GetUser(r.Context(), id)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	active := existing.Active
	if req.Active != nil {
		active = *req.Active
	}

	user, err := h.Service.UpdateUser(r.Context(), id, req.Email, req.Name, active)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	apiutil.Success(w, http.StatusOK, dto.ToUserResponse(user))
}

// Deactivate user
// @Summary Deactivate a user and revoke its keys
// @Tags users
// @Param id path string true "User ID"
// @Success 200 {object} dto.UserResponse
// @Router /users/{id} [delete]
func (h *UserHandler) Deactivate(w http.ResponseWriter, r *http.Request) {
	id, err := pgutil.ParseUUID(chi.URLParam(r, "id"))
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid user ID")
		return
	}
	user, err := h.Service.DeactivateUser(r.Context(), id)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	apiutil.Success(w, http.StatusOK, dto.ToUserResponse(user))
}

// List user keys
// @Summary List a user's API keys
// @Tags users
// @Param id path string true "User ID"
// @Param limit query int false "Page size (default 50, max 200)"
// @Param cursor query string false "Opaque next cursor"
// @Success 200 {object} dto.APIKeyPage
// @Router /users/{id}/keys [get]
func (h *UserHandler) ListKeys(w http.ResponseWriter, r *http.Request) {
	userID, err := pgutil.ParseUUID(chi.URLParam(r, "id"))
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid user ID")
		return
	}
	if _, err := h.Service.GetUser(r.Context(), userID); err != nil {
		writeAuthError(w, err)
		return
	}
	listAPIKeysPage(w, r, h.Service, userID)
}

// Create user key
// @Summary Create an API key for a user
// @Description The plaintext key is returned exactly once.
// @Tags users
// @Param id path string true "User ID"
// @Param request body dto.CreateAPIKeyRequest true "Key metadata"
// @Success 201 {object} dto.CreateAPIKeyResponse
// @Router /users/{id}/keys [post]
func (h *UserHandler) CreateKey(w http.ResponseWriter, r *http.Request) {
	userID, err := pgutil.ParseUUID(chi.URLParam(r, "id"))
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid user ID")
		return
	}
	var req dto.CreateAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if err := req.Validate(); err != nil {
		apiutil.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	key, raw, err := h.Service.CreateAPIKey(r.Context(), userID, req.Name, expiresAt(req.ExpiresInDays))
	if err != nil {
		writeAuthError(w, err)
		return
	}
	apiutil.Success(w, http.StatusCreated, dto.CreateAPIKeyResponse{
		APIKeyResponse: dto.ToAPIKeyResponse(key),
		Key:            raw,
	})
}

// Revoke user key
// @Summary Revoke an API key belonging to a user
// @Tags users
// @Param id path string true "User ID"
// @Param keyID path string true "API key ID"
// @Success 200 {object} dto.APIKeyResponse
// @Router /users/{id}/keys/{keyID}/revoke [post]
func (h *UserHandler) RevokeKey(w http.ResponseWriter, r *http.Request) {
	userID, err := pgutil.ParseUUID(chi.URLParam(r, "id"))
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid user ID")
		return
	}
	keyID, err := pgutil.ParseUUID(chi.URLParam(r, "keyID"))
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, "Invalid API key ID")
		return
	}
	key, err := h.Service.RevokeAPIKeyForUser(r.Context(), keyID, userID)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	apiutil.Success(w, http.StatusOK, dto.ToAPIKeyResponse(key))
}

type AuditHandler struct {
	Service *auth.Service
}

// List audit entries
// @Summary List audit entries
// @Tags audit
// @Produce json
// @Param limit query int false "Page size (default 50, max 200)"
// @Param cursor query string false "Opaque next cursor"
// @Success 200 {object} dto.AuditPage
// @Router /audit [get]
func (h *AuditHandler) List(w http.ResponseWriter, r *http.Request) {
	pageReq, err := pagination.ParseRequest(r)
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	cursorTime, cursorID, hasCursor, err := pagination.ParseTimeCursor(pageReq.Cursor, "audit")
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	params := db.ListAuditEntriesPageParams{PageLimit: pageReq.Limit + 1}
	if hasCursor {
		params.CursorTime = pgutil.ToTimestamptz(cursorTime)
		params.CursorID = cursorID
	}

	entries, err := h.Service.ListAuditEntriesPage(r.Context(), params)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	entries, hasMore := pagination.TrimPage(entries, pageReq.Limit)
	next := ""
	if hasMore && len(entries) > 0 {
		last := entries[len(entries)-1]
		next = pagination.TimeCursor("audit", last.CreatedAt.Time, last.ID)
	}
	pagination.SetNextLink(w, r, next)

	res := make([]dto.AuditEntryResponse, len(entries))
	for i, entry := range entries {
		res[i] = dto.ToAuditEntryResponse(entry)
	}
	apiutil.Success(w, http.StatusOK, dto.AuditPage{Items: res, NextCursor: next, HasMore: hasMore})
}

func listAPIKeysPage(w http.ResponseWriter, r *http.Request, service *auth.Service, userID pgtype.UUID) {
	pageReq, err := pagination.ParseRequest(r)
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	cursorTime, cursorID, hasCursor, err := pagination.ParseTimeCursor(pageReq.Cursor, "api_keys")
	if err != nil {
		apiutil.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	params := db.ListAPIKeysByUserPageParams{
		UserID:    userID,
		PageLimit: pageReq.Limit + 1,
	}
	if hasCursor {
		params.CursorTime = pgutil.ToTimestamptz(cursorTime)
		params.CursorID = cursorID
	}

	keys, err := service.ListAPIKeysPage(r.Context(), params)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	keys, hasMore := pagination.TrimPage(keys, pageReq.Limit)
	next := ""
	if hasMore && len(keys) > 0 {
		last := keys[len(keys)-1]
		next = pagination.TimeCursor("api_keys", last.CreatedAt.Time, last.ID)
	}
	pagination.SetNextLink(w, r, next)
	apiutil.Success(w, http.StatusOK, dto.APIKeyPage{
		Items:      toAPIKeyResponses(keys),
		NextCursor: next,
		HasMore:    hasMore,
	})
}

func toAPIKeyResponses(keys []db.ApiKey) []dto.APIKeyResponse {
	res := make([]dto.APIKeyResponse, len(keys))
	for i, key := range keys {
		res[i] = dto.ToAPIKeyResponse(key)
	}
	return res
}

func expiresAt(days *int) pgtype.Timestamptz {
	if days == nil {
		return pgtype.Timestamptz{}
	}
	return pgutil.ToTimestamptz(time.Now().AddDate(0, 0, *days))
}

func writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrUserExists):
		apiutil.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, auth.ErrUserNotFound), errors.Is(err, auth.ErrKeyNotFound):
		apiutil.Error(w, http.StatusNotFound, err.Error())
	case errors.Is(err, auth.ErrLastActiveUser), errors.Is(err, auth.ErrInactiveUser):
		apiutil.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, auth.ErrInvalidUser), errors.Is(err, auth.ErrInvalidKeyName):
		apiutil.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, auth.ErrKeyForbidden):
		apiutil.Error(w, http.StatusForbidden, err.Error())
	default:
		apiutil.InternalError(w, err)
	}
}
