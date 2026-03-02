package handler

import (
	"Intelligent_Dev_ToolKit_Odoo/internal/service"
	"net/http"
)

type Handler struct {
	service *service.Service
}

func NewHandler(service *service.Service) *Handler {
	return &Handler{
		service: service,
	}
}

// =============================================================================
// AUTH HANDLERS
// =============================================================================

func (h *Handler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	h.service.NotImplemented(w, r)
}
