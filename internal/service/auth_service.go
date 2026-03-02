package service

import (
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"net/http"
)

type Service struct {
}

func NewService() *Service {
	return &Service{}
}

// Helper for not-yet-implemented handlers
func (s *Service) NotImplemented(w http.ResponseWriter, r *http.Request) {
	api.HandleError(w, r, api.NewAPIError(
		api.ErrCodeInternal,
		"This endpoint is not yet implemented",
		http.StatusNotImplemented,
	))
}
