package server

import (
	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/config"
	"Intelligent_Dev_ToolKit_Odoo/internal/handler"
	"Intelligent_Dev_ToolKit_Odoo/internal/service"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
)

type Server struct {
	store   db.Store
	config  config.Config
	router  *chi.Mux
	logger  *zerolog.Logger
	handler *handler.Handler
}

func NewServer(store db.Store, config config.Config) (*Server, error) {

	server := &Server{
		store:  store,
		config: config,
	}

	service := service.NewService()
 	handler := handler.NewHandler(service)
 	server.handler = handler

	server.setupRoutes()
	return server, nil
}

func (s *Server) Start(address string) error {
	return http.ListenAndServe(address, s.router)
}

func (s *Server) Router() http.Handler {
	return s.router
}
