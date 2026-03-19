package handler

import (
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"
	"Intelligent_Dev_ToolKit_Odoo/internal/service"
	"net/http"
)

// AgentRegisterHandler handles agent self-registration.
type AgentRegisterHandler struct {
	*BaseHandler
	svc service.AgentRegisterServicer
}

func NewAgentRegisterHandler(svc service.AgentRegisterServicer, base *BaseHandler) *AgentRegisterHandler {
	return &AgentRegisterHandler{BaseHandler: base, svc: svc}
}

// HandleSelfRegister exchanges a one-time registration token for agent credentials.
// This endpoint is called automatically by the agent binary — not exposed in Swagger.
func (h *AgentRegisterHandler) HandleSelfRegister(w http.ResponseWriter, r *http.Request) {
	var req dto.AgentSelfRegisterRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	resp, err := h.svc.SelfRegister(r.Context(), &req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	dto.WriteCreated(w, r, resp)
}
