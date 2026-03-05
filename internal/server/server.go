package server

import (
	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	_ "Intelligent_Dev_ToolKit_Odoo/docs"
	"Intelligent_Dev_ToolKit_Odoo/internal/cache"
	"Intelligent_Dev_ToolKit_Odoo/internal/config"
	"Intelligent_Dev_ToolKit_Odoo/internal/handler"
	"Intelligent_Dev_ToolKit_Odoo/internal/service"
	"Intelligent_Dev_ToolKit_Odoo/internal/token"
	"fmt"
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
	handler    *handler.Handlers
	services   *service.Services
	tokenMaker token.Maker
}

func NewServer(store db.Store, cache *cache.RedisClient, config config.Config) (*Server, error) {

	tokenMaker, err := token.NewJWTMaker(config.TokenSymmetricKey)
	if err != nil {
		return nil, fmt.Errorf("cannot create token maker: %w", err)
	}

	server := &Server{
		store:      store,
		cache:      cache,
		config:     config,
		tokenMaker: tokenMaker,
	}

	// Instantiate services container
	serviceConfig := service.DefaultConfig()
	// Customize service config from main config
	serviceConfig.Auth.AccessTokenDuration = config.AccessTokenDuration
	serviceConfig.Auth.RefreshTokenDuration = config.RefreshTokenDuration
	serviceConfig.Auth.SMTPHost = config.SMTPHost
	serviceConfig.Auth.SMTPPort = config.SMTPPort
	serviceConfig.Auth.SMTPFrom = config.EmailFrom
	serviceConfig.Auth.AppBaseURL = config.ClientAppURL

	services := service.NewServices(server.store, server.cache, server.tokenMaker, serviceConfig)
	server.services = services

	handler := handler.NewHandlers(services)
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
