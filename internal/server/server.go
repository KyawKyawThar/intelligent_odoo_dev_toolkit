package server

import (
	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/cache"
	"Intelligent_Dev_ToolKit_Odoo/internal/config"
	"Intelligent_Dev_ToolKit_Odoo/internal/handler"
	"Intelligent_Dev_ToolKit_Odoo/internal/service"
	"Intelligent_Dev_ToolKit_Odoo/internal/token"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
)

type Server struct {
	store      db.Store
	cache      *cache.RedisClient // optional in-memory cache (Redis)
	config     config.Config
	router     *chi.Mux
	logger     *zerolog.Logger
	handler    *handler.Handler
	services   *service.Services
	tokenMaker token.Maker
}

func NewServer(store db.Store, cache *cache.RedisClient, config config.Config) (*Server, error) {

	server := &Server{
		store:  store,
		cache:  cache,
		config: config,
	}

	// Instantiate services container. Token maker is still nil here but can be
	// injected later when we add JWT support.
	services := service.NewServices(server.store, server.cache, nil, nil)
	server.services = services

	handler := handler.NewHandler(services)
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
