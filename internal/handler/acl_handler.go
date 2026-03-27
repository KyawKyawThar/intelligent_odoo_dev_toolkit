package handler

import (
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"
	"Intelligent_Dev_ToolKit_Odoo/internal/service"
	"net/http"
)

// ACLHandler handles the ACL debugger endpoints.
type ACLHandler struct {
	*BaseHandler
	svc *service.ACLService
}

// NewACLHandler creates an ACLHandler.
func NewACLHandler(svc *service.ACLService, base *BaseHandler) *ACLHandler {
	return &ACLHandler{BaseHandler: base, svc: svc}
}

// HandleTraceAccess runs the 5-stage ACL pipeline and returns the result.
//
//	@Summary      Trace ACL access
//	@Description  Run the 5-stage ACL pipeline: "why can't user X see record Y?"
//	@Tags         acl
//	@Accept       json
//	@Produce      json
//	@Param        env_id  path  string               true  "Environment ID"
//	@Param        body    body  dto.ACLTraceRequest   true  "ACL trace request"
//	@Success      200  {object}  dto.ACLTraceResponse
//	@Failure      400  {object}  api.Error
//	@Failure      401  {object}  api.Error
//	@Failure      404  {object}  api.Error
//	@Router       /environments/{env_id}/acl/trace [post]
//	@Security     BearerAuth
func (h *ACLHandler) HandleTraceAccess(w http.ResponseWriter, r *http.Request) {
	tenantID, envID, ok := h.MustTenantAndEnvID(w, r)
	if !ok {
		return
	}

	var req dto.ACLTraceRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	result, err := h.svc.TraceAccess(r.Context(), tenantID, envID, &req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, result)
}
