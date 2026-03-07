package server

import (
	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	_ "Intelligent_Dev_ToolKit_Odoo/docs"
	"Intelligent_Dev_ToolKit_Odoo/internal/cache"
	"Intelligent_Dev_ToolKit_Odoo/internal/config"
	"Intelligent_Dev_ToolKit_Odoo/internal/handler"
	"Intelligent_Dev_ToolKit_Odoo/internal/service"
	"Intelligent_Dev_ToolKit_Odoo/internal/token"
	"crypto/tls"
	"fmt"
	"html/template"
	"net/http"
	"time"

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
	// Safe HTML template rendering
	template *template.Template
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
		template:   template.New(""),
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
	// Good practice: enforce timeouts for servers you create!
	srv := &http.Server{
		Addr:         address,
		Handler:      s.router,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
		TLSConfig: &tls.Config{
			//gosec:G402
			// Force clients to use secure TLS versions
			MinVersion: tls.VersionTLS12,
			// Best practice: enforce server-side cipher suite preferences
			PreferServerCipherSuites: true,
			CurvePreferences: []tls.CurveID{
				tls.X25519,
				tls.CurveP256,
			},
			CipherSuites: []uint16{
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
				tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			},
		},
	}
	// Use ListenAndServeTLS instead of ListenAndServe
	s.router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
			next.ServeHTTP(w, r)
		})
	})
	return srv.ListenAndServeTLS("cert.pem", "key.pem")
}

func (s *Server) Router() http.Handler {
	return s.router
}
