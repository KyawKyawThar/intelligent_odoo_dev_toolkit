package handler

import (
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"
	"Intelligent_Dev_ToolKit_Odoo/internal/middleware"
	"Intelligent_Dev_ToolKit_Odoo/internal/service"
	"net/http"

	"github.com/google/uuid"
)

type SchemaHandler struct {
	*BaseHandler
	svc *service.SchemaService
}

func NewSchemaHandler(svc *service.SchemaService, base *BaseHandler) *SchemaHandler {
	return &SchemaHandler{
		BaseHandler: base,
		svc:         svc,
	}
}

// mustTenantID reads tenant_id from context (works for both JWT and API-key auth paths).
func (h *SchemaHandler) mustTenantID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	raw := middleware.GetTenantID(r.Context())
	if raw == "" {
		h.WriteErr(w, r, api.ErrUnauthorized("Tenant ID missing"))
		return uuid.Nil, false
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		h.WriteErr(w, r, api.ErrUnauthorized("Malformed tenant ID"))
		return uuid.Nil, false
	}
	return id, true
}

// =============================================================================
// POST /agent/schema
// =============================================================================

// HandleStore accepts a schema snapshot pushed by an agent.
//
//	@Summary      Store schema snapshot
//	@Description  Agent endpoint: persist a collected Odoo schema snapshot
//	@Tags         agent
//	@Accept       json
//	@Produce      json
//	@Param        body  body      dto.StoreSchemaRequest  true  "Schema payload"
//	@Success      201   {object}  dto.SchemaSnapshotResponse
//	@Failure      400   {object}  api.Error
//	@Failure      401   {object}  api.Error
//	@Failure      404   {object}  api.Error
//	@Router       /agent/schema [post]
//	@Security     ApiKeyAuth
func (h *SchemaHandler) HandleStore(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.mustTenantID(w, r)
	if !ok {
		return
	}

	var req dto.StoreSchemaRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	// If the payload env_id is empty, fall back to the env_id from the API key.
	if req.EnvID == uuid.Nil {
		envIDStr := middleware.GetEnvID(r.Context())
		if envIDStr != "" {
			if parsed, err := uuid.Parse(envIDStr); err == nil {
				req.EnvID = parsed
			}
		}
	}

	if req.EnvID == uuid.Nil {
		h.WriteErr(w, r, api.ErrBadRequest("env_id is required (provide in payload or use an environment-scoped API key)"))
		return
	}

	resp, err := h.svc.StoreSchema(r.Context(), tenantID, &req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	dto.WriteCreated(w, r, resp)
}

// =============================================================================
// GET /api/v1/environments/{env_id}/schema/latest
// =============================================================================

// HandleGetLatest returns the most recent schema snapshot for an environment.
//
//	@Summary      Get latest schema snapshot
//	@Description  Return the most recently captured schema snapshot for the given environment
//	@Tags         schema
//	@Produce      json
//	@Param        env_id  path      string  true  "Environment ID (UUID)"
//	@Success      200     {object}  dto.SchemaSnapshotResponse
//	@Failure      401     {object}  api.Error
//	@Failure      404     {object}  api.Error
//	@Router       /environments/{env_id}/schema/latest [get]
//	@Security     BearerAuth
func (h *SchemaHandler) HandleGetLatest(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.mustTenantID(w, r)
	if !ok {
		return
	}

	envID, ok := h.MustUUIDParam(w, r, "env_id")
	if !ok {
		return
	}

	resp, err := h.svc.GetLatest(r.Context(), tenantID, envID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	dto.WriteSuccess(w, r, resp)
}

// =============================================================================
// GET /api/v1/environments/{env_id}/schema/{snapshot_id}
// =============================================================================

// HandleGetSnapshot returns a single schema snapshot by ID for an environment.
//
//	@Summary      Get schema snapshot by ID
//	@Description  Return a specific schema snapshot for the given environment
//	@Tags         schema
//	@Produce      json
//	@Param        env_id       path      string  true  "Environment ID (UUID)"
//	@Param        snapshot_id  path      string  true  "Snapshot ID (UUID)"
//	@Success      200          {object}  dto.SchemaSnapshotResponse
//	@Failure      401          {object}  api.Error
//	@Failure      404          {object}  api.Error
//	@Router       /environments/{env_id}/schema/{snapshot_id} [get]
//	@Security     BearerAuth
func (h *SchemaHandler) HandleGetSnapshot(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.mustTenantID(w, r)
	if !ok {
		return
	}

	envID, ok := h.MustUUIDParam(w, r, "env_id")
	if !ok {
		return
	}

	snapshotID, ok := h.MustUUIDParam(w, r, "snapshot_id")
	if !ok {
		return
	}

	resp, err := h.svc.GetSnapshot(r.Context(), tenantID, envID, snapshotID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	dto.WriteSuccess(w, r, resp)
}

// =============================================================================
// GET /api/v1/environments/{env_id}/schema
// =============================================================================

// HandleList returns a paginated list of schema snapshot summaries for an environment.
//
//	@Summary      List schema snapshots
//	@Description  Return schema snapshot summaries for the given environment, newest first
//	@Tags         schema
//	@Produce      json
//	@Param        env_id  path      string  true   "Environment ID (UUID)"
//	@Param        limit   query     int     false  "Max results (default 20, max 100)"
//	@Success      200     {object}  dto.SchemaSnapshotListResponse
//	@Failure      401     {object}  api.Error
//	@Failure      404     {object}  api.Error
//	@Router       /environments/{env_id}/schema [get]
//	@Security     BearerAuth
func (h *SchemaHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.mustTenantID(w, r)
	if !ok {
		return
	}

	envID, ok := h.MustUUIDParam(w, r, "env_id")
	if !ok {
		return
	}

	limit := ParseQueryInt32(r, "limit", 20)

	resp, err := h.svc.ListSnapshots(r.Context(), tenantID, envID, limit)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	dto.WriteSuccess(w, r, resp)
}

// =============================================================================
// GET /api/v1/environments/{env_id}/schema/models
// =============================================================================

// HandleSearchModels searches model entries within the latest snapshot.
//
//	@Summary      Search schema models
//	@Description  Filter models in the latest snapshot by technical name or display label
//	@Tags         schema
//	@Produce      json
//	@Param        env_id  path      string  true   "Environment ID (UUID)"
//	@Param        q       query     string  false  "Search term (substring, case-insensitive)"
//	@Param        limit   query     int     false  "Page size (default 50)"
//	@Param        offset  query     int     false  "Offset for pagination"
//	@Success      200     {object}  dto.SearchModelsResponse
//	@Failure      401     {object}  api.Error
//	@Failure      404     {object}  api.Error
//	@Router       /environments/{env_id}/schema/models [get]
//	@Security     BearerAuth
func (h *SchemaHandler) HandleSearchModels(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.mustTenantID(w, r)
	if !ok {
		return
	}

	envID, ok := h.MustUUIDParam(w, r, "env_id")
	if !ok {
		return
	}

	q := r.URL.Query().Get("q")
	limit := ParseQueryInt32(r, "limit", 50)
	offset := ParseQueryInt32(r, "offset", 0)

	resp, err := h.svc.SearchModels(r.Context(), tenantID, envID, q, limit, offset)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	dto.WriteSuccess(w, r, resp)
}
