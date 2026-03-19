package handler

import (
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"
	"Intelligent_Dev_ToolKit_Odoo/internal/middleware"
	"Intelligent_Dev_ToolKit_Odoo/internal/service"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"Intelligent_Dev_ToolKit_Odoo/internal/api"
)

// ErrorHandler handles agent error ingestion and error group query endpoints.
type ErrorHandler struct {
	*BaseHandler
	svc *service.ErrorService
}

func NewErrorHandler(svc *service.ErrorService, base *BaseHandler) *ErrorHandler {
	return &ErrorHandler{BaseHandler: base, svc: svc}
}

// mustTenantID reads the tenant_id set by either JWT or API-key middleware.
func (h *ErrorHandler) mustTenantID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
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

// HandleIngestErrors ingests a batch of error events from an agent.
//
//	@Summary      Ingest error events
//	@Description  Agent endpoint: receive a batch of Odoo error/exception events
//	@Tags         agent
//	@Accept       json
//	@Produce      json
//	@Param        body  body  dto.IngestErrorsRequest  true  "Error batch"
//	@Success      204
//	@Failure      400  {object}  api.Error
//	@Failure      401  {object}  api.Error
//	@Failure      404  {object}  api.Error
//	@Router       /agent/errors [post]
//	@Security     ApiKeyAuth
func (h *ErrorHandler) HandleIngestErrors(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.mustTenantID(w, r)
	if !ok {
		return
	}

	var req dto.IngestErrorsRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	// If the payload env_id is empty, fall back to the env_id from the API key.
	if req.EnvID == "" {
		req.EnvID = middleware.GetEnvID(r.Context())
	}

	if err := h.svc.IngestBatch(r.Context(), tenantID, &req); err != nil {
		h.HandleErr(w, r, err)
		return
	}

	dto.WriteNoContent(w)
}

// HandleListErrors returns a paginated list of error groups for an environment.
//
//	@Summary      List error groups
//	@Description  Returns aggregated error groups for the given environment, sorted by occurrence count
//	@Tags         errors
//	@Produce      json
//	@Param        env_id      path   string  true  "Environment ID"
//	@Param        page        query  int     false "Page number (default 1)"
//	@Param        per_page    query  int     false "Items per page (default 25, max 100)"
//	@Param        status      query  string  false "Filter by status: open, acknowledged, resolved"
//	@Param        error_type  query  string  false "Filter by error type"
//	@Param        search      query  string  false "Free-text search in message/module/model"
//	@Success      200  {object}  dto.ErrorGroupListResponse
//	@Failure      400  {object}  api.Error
//	@Failure      401  {object}  api.Error
//	@Failure      404  {object}  api.Error
//	@Router       /environments/{env_id}/errors [get]
//	@Security     BearerAuth
func (h *ErrorHandler) HandleListErrors(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.MustTenantID(w, r)
	if !ok {
		return
	}
	envID, ok := h.MustUUIDParam(w, r, "env_id")
	if !ok {
		return
	}

	pg := dto.GetPaginationFromRequest(r)
	req := &dto.ListErrorGroupsRequest{
		Status:    r.URL.Query().Get("status"),
		ErrorType: r.URL.Query().Get("error_type"),
		Search:    r.URL.Query().Get("search"),
		Page:      pg.Page,
		PerPage:   pg.PerPage,
	}

	resp, err := h.svc.ListErrorGroups(r.Context(), tenantID, envID, req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	dto.WriteSuccess(w, r, resp)
}

// HandleGetErrorGroup returns a single error group by ID.
//
//	@Summary      Get error group
//	@Description  Returns a single error group with optional raw traceback from S3
//	@Tags         errors
//	@Produce      json
//	@Param        env_id    path   string  true  "Environment ID"
//	@Param        error_id  path   string  true  "Error group ID"
//	@Param        trace     query  bool    false "Include raw traceback from S3 (default false)"
//	@Success      200  {object}  dto.ErrorGroupDetailResponse
//	@Failure      400  {object}  api.Error
//	@Failure      401  {object}  api.Error
//	@Failure      404  {object}  api.Error
//	@Router       /environments/{env_id}/errors/{error_id} [get]
//	@Security     BearerAuth
func (h *ErrorHandler) HandleGetErrorGroup(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.MustTenantID(w, r)
	if !ok {
		return
	}
	envID, ok := h.MustUUIDParam(w, r, "env_id")
	if !ok {
		return
	}

	// Support both UUID and signature-based lookup.
	errorParam := chi.URLParam(r, "error_id")
	if errorParam == "" {
		h.WriteErr(w, r, api.ErrBadRequest("missing error_id path parameter"))
		return
	}

	includeTrace := r.URL.Query().Get("trace") == "true"

	// Try UUID parse first; fall back to signature lookup.
	errorID, parseErr := uuid.Parse(errorParam)
	if parseErr == nil {
		resp, err := h.svc.GetErrorGroup(r.Context(), tenantID, envID, errorID, includeTrace)
		if err != nil {
			h.HandleErr(w, r, err)
			return
		}
		dto.WriteSuccess(w, r, resp)
		return
	}

	// Treat as signature.
	resp, err := h.svc.GetErrorGroupBySignature(r.Context(), tenantID, envID, errorParam)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}
	dto.WriteSuccess(w, r, resp)
}
