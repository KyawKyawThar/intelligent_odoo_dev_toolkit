package handler

import (
	"Intelligent_Dev_ToolKit_Odoo/internal/service"
	"net/http"
)

// OverviewHandler handles the environment overview endpoint.
type OverviewHandler struct {
	*BaseHandler
	svc *service.OverviewService
}

// NewOverviewHandler creates an OverviewHandler.
func NewOverviewHandler(svc *service.OverviewService, base *BaseHandler) *OverviewHandler {
	return &OverviewHandler{BaseHandler: base, svc: svc}
}

// HandleGet returns the cross-feature summary for an environment.
//
//	@Summary      Environment overview
//	@Description  Returns aggregated stats across all features (errors, profiler, N+1, alerts, budgets, agent) for a single environment
//	@Tags         overview
//	@Produce      json
//	@Param        env_id  path  string  true  "Environment ID"
//	@Success      200  {object}  dto.OverviewResponse
//	@Failure      401  {object}  api.Error
//	@Failure      404  {object}  api.Error
//	@Router       /environments/{env_id}/overview [get]
//	@Security     BearerAuth
func (h *OverviewHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	tenantID, envID, ok := h.MustTenantAndEnvID(w, r)
	if !ok {
		return
	}

	result, err := h.svc.GetOverview(r.Context(), tenantID, envID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, result)
}
