package handler

import (
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"
	"Intelligent_Dev_ToolKit_Odoo/internal/middleware"
	"Intelligent_Dev_ToolKit_Odoo/internal/service"
	"net/http"

	"github.com/google/uuid"

	"Intelligent_Dev_ToolKit_Odoo/internal/api"
)

// ErrorHandler handles agent error ingestion endpoints.
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
//	@Router       /api/v1/agent/errors [post]
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

	if err := h.svc.IngestBatch(r.Context(), tenantID, &req); err != nil {
		h.HandleErr(w, r, err)
		return
	}

	dto.WriteNoContent(w)
}
