package handler

import (
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"
	"Intelligent_Dev_ToolKit_Odoo/internal/service"
	"net/http"
)

type EnvironmentHandler struct {
	*BaseHandler
	svc service.EnvironmentService
}

func NewEnviromentHandler(envService service.EnvironmentService, base *BaseHandler) *EnvironmentHandler {

	return &EnvironmentHandler{
		BaseHandler: base,
		svc:         envService,
	}
}

// =============================================================================
// POST /api/v1/environments
// =============================================================================

// HandleCreate creates a new environment for the authenticated tenant.
//
//	@Summary      Create environment
//	@Description  Register a new Odoo environment under the current tenant
//	@Tags         environments
//	@Accept       json
//	@Produce      json
//	@Param        body  body      dto.CreateEnvironmentRequest  true  "Environment details"
//	@Success      201   {object}  dto.EnvironmentResponse
//	@Failure      400   {object}  api.Error
//	@Failure      401   {object}  api.Error
//	@Failure      409   {object}  api.Error
//	@Router       /environments [post]
//	@Security     BearerAuth
func (h *EnvironmentHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	tennantID, ok := h.MustTenantID(w, r)

	if !ok {
		return
	}
	var req dto.CreateEnvironmentRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	resp, err := h.svc.Create(r.Context(), tennantID, &req)

	if err != nil {
		// h.JSON(w, http.StatusBadRequest, api.Error{Message: err.Error()})\\
		h.HandleErr(w, r, err)
		return
	}
	dto.WriteCreated(w, r, resp)
}

// =============================================================================
// GET /api/v1/environments
// =============================================================================

// HandleList returns a paginated list of environments for the tenant.
//
//	@Summary      List environments
//	@Description  Get all environments for the current tenant
//	@Tags         environments
//	@Produce      json
//	@Param        env_type  query     string  false  "Filter by type"  Enums(development, staging, production)
//	@Param        status    query     string  false  "Filter by status"  Enums(active, inactive, maintenance)
//	@Param        limit     query     int     false  "Page size (default 20, max 100)"
//	@Param        offset    query     int     false  "Offset for pagination"
//	@Success      200       {object}  dto.EnvironmentListResponse
//	@Failure      401       {object}  api.Error
//	@Router       /environments [get]
//	@Security     BearerAuth
func (h *EnvironmentHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.MustTenantID(w, r)

	if !ok {
		return
	}

	req := dto.ListEnvironmentsRequest{
		EnvType: r.URL.Query().Get("env_type"),
		Status:  r.URL.Query().Get("status"),
		Limit:   ParseQueryInt32(r, "limit", 20),
		Offset:  ParseQueryInt32(r, "offset", 0),
	}
	resp, err := h.svc.List(r.Context(), tenantID, &req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	dto.WriteSuccess(w, r, resp)
}

// =============================================================================
// GET /api/v1/environments/{env_id}
// =============================================================================

// HandleGet returns a single environment by ID.
//
//	@Summary      Get environment
//	@Description  Get details of a specific environment
//	@Tags         environments
//	@Produce      json
//	@Param        env_id  path      string  true  "Environment ID (UUID)"
//	@Success      200     {object}  dto.EnvironmentResponse
//	@Failure      401     {object}  api.Error
//	@Failure      404     {object}  api.Error
//	@Router       /environments/{env_id} [get]
//	@Security     BearerAuth
func (h *EnvironmentHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.MustTenantID(w, r)
	if !ok {
		return
	}

	envID, ok := h.MustUUIDParam(w, r, "env_id")
	if !ok {
		return
	}

	resp, err := h.svc.GetByID(r.Context(), tenantID, envID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	dto.WriteSuccess(w, r, resp)
}

// =============================================================================
// PATCH /api/v1/environments/{env_id}
// =============================================================================

// HandleUpdate partially updates an environment.
//
//	@Summary      Update environment
//	@Description  Partially update an environment's configuration
//	@Tags         environments
//	@Accept       json
//	@Produce      json
//	@Param        env_id  path      string                        true  "Environment ID (UUID)"
//	@Param        body    body      dto.UpdateEnvironmentRequest  true  "Fields to update"
//	@Success      200     {object}  dto.EnvironmentResponse
//	@Failure      400     {object}  api.Error
//	@Failure      401     {object}  api.Error
//	@Failure      404     {object}  api.Error
//	@Failure      409     {object}  api.Error
//	@Router       /environments/{env_id} [patch]
//	@Security     BearerAuth
func (h *EnvironmentHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.MustTenantID(w, r)

	if !ok {
		return
	}

	envID, ok := h.MustUUIDParam(w, r, "env_id")

	if !ok {
		return
	}
	var req dto.UpdateEnvironmentRequest

	if !h.DecodeAndValidate(w, r, &req) {
		return
	}
	resp, err := h.svc.Update(r.Context(), tenantID, envID, &req)

	if err != nil {
		h.HandleErr(w, r, err)
		return
	}
	dto.WriteSuccess(w, r, resp)
}

// =============================================================================
// DELETE /api/v1/environments/{env_id}
// =============================================================================

// HandleDelete removes an environment.
//
//	@Summary      Delete environment
//	@Description  Permanently delete an environment and its associated data
//	@Tags         environments
//	@Produce      json
//	@Param        env_id  path  string  true  "Environment ID (UUID)"
//	@Success      204
//	@Failure      401  {object}  api.Error
//	@Failure      404  {object}  api.Error
//	@Router       /environments/{env_id} [delete]
//	@Security     BearerAuth
func (h *EnvironmentHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.MustTenantID(w, r)
	if !ok {
		return
	}
	envID, ok := h.MustUUIDParam(w, r, "env_id")
	if !ok {
		return
	}

	if err := h.svc.Delete(r.Context(), tenantID, envID); err != nil {
		h.HandleErr(w, r, err)
		return
	}
	dto.WriteNoContent(w)
}
