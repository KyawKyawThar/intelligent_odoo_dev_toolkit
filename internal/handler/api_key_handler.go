package handler

import (
	"net/http"

	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"
	"Intelligent_Dev_ToolKit_Odoo/internal/service"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// APIKeyHandler handles CRUD operations for agent API keys.
type APIKeyHandler struct {
	*BaseHandler
	svc *service.APIKeyService
}

func NewAPIKeyHandler(svc *service.APIKeyService, base *BaseHandler) *APIKeyHandler {
	return &APIKeyHandler{BaseHandler: base, svc: svc}
}

func (h *APIKeyHandler) mustUUIDParam(w http.ResponseWriter, r *http.Request, param string) (uuid.UUID, bool) {
	raw := chi.URLParam(r, param)
	if raw == "" {
		h.WriteErr(w, r, api.ErrBadRequest("missing "+param+" path parameter"))
		return uuid.Nil, false
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		h.WriteErr(w, r, api.ErrBadRequest(param+" must be a valid UUID"))
		return uuid.Nil, false
	}
	return id, true
}

// HandleCreate generates a new API key for an environment.
//
//	@Summary      Create API key
//	@Description  Generate a new agent API key for the given environment. The raw key is returned once and never shown again. If expires_at is omitted the key expires in 90 days. If scopes is omitted it defaults to ["agent:write"].
//	@Tags         api-keys
//	@Accept       json
//	@Produce      json
//	@Param        env_id  path      string                    true  "Environment ID (UUID)"
//	@Param        body    body      dto.CreateAPIKeyRequest   true  "Key options"
//	@Success      201     {object}  dto.APIKeyCreatedResponse
//	@Failure      400     {object}  api.Error
//	@Failure      401     {object}  api.Error
//	@Failure      404     {object}  api.Error
//	@Router       /environments/{env_id}/api-keys [post]
//	@Security     BearerAuth
func (h *APIKeyHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.MustTenantID(w, r)
	if !ok {
		return
	}
	userID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}
	envID, ok := h.mustUUIDParam(w, r, "env_id")
	if !ok {
		return
	}

	var req dto.CreateAPIKeyRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	resp, err := h.svc.Create(r.Context(), tenantID, envID, userID, &req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	dto.WriteCreated(w, r, resp)
}

// HandleList returns all API keys for an environment (raw key omitted).
//
//	@Summary      List API keys
//	@Description  Return all API keys for the given environment. Raw key values are never included.
//	@Tags         api-keys
//	@Produce      json
//	@Param        env_id  path      string  true  "Environment ID (UUID)"
//	@Success      200     {object}  dto.APIKeyListResponse
//	@Failure      401     {object}  api.Error
//	@Failure      404     {object}  api.Error
//	@Router       /environments/{env_id}/api-keys [get]
//	@Security     BearerAuth
func (h *APIKeyHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.MustTenantID(w, r)
	if !ok {
		return
	}
	envID, ok := h.mustUUIDParam(w, r, "env_id")
	if !ok {
		return
	}

	resp, err := h.svc.List(r.Context(), tenantID, envID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	dto.WriteSuccess(w, r, resp)
}

// HandleRevoke deactivates an API key immediately.
//
//	@Summary      Revoke API key
//	@Description  Mark an API key as inactive. The agent using it will receive 401 on the next request.
//	@Tags         api-keys
//	@Produce      json
//	@Param        env_id  path  string  true  "Environment ID (UUID)"
//	@Param        key_id  path  string  true  "API Key ID (UUID)"
//	@Success      204
//	@Failure      401  {object}  api.Error
//	@Failure      404  {object}  api.Error
//	@Router       /environments/{env_id}/api-keys/{key_id} [delete]
//	@Security     BearerAuth
func (h *APIKeyHandler) HandleRevoke(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.MustTenantID(w, r)
	if !ok {
		return
	}
	keyID, ok := h.mustUUIDParam(w, r, "key_id")
	if !ok {
		return
	}

	if err := h.svc.Revoke(r.Context(), tenantID, keyID); err != nil {
		h.HandleErr(w, r, err)
		return
	}

	dto.WriteNoContent(w)
}
