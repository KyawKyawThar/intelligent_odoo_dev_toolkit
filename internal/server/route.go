package server

import (
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	mw "Intelligent_Dev_ToolKit_Odoo/internal/middleware"
	"context"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
)

func (s *Server) setupRoutes() {

	r := chi.NewRouter()

	// Create logger
	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	if s.config.Environment == "development" {
		logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).With().Timestamp().Logger()
	}
	s.logger = &logger

	// ==========================================================================
	// Global Middleware Stack (order matters!)
	// ==========================================================================

	// 1. Real IP - Extract real client IP from headers
	r.Use(mw.RealIP)

	// 2. Request ID - Add unique ID to each request
	r.Use(mw.RequestID)

	// 3. Panic Recovery - Catch panics and return 500
	r.Use(mw.Recoverer(&logger))

	// 4. Request Logging - Log all requests
	if s.config.Environment == "development" {
		r.Use(middleware.Logger) // Chi's built-in logger (pretty)
	} else {
		r.Use(mw.RequestLogger(&logger)) // Structured JSON logging
	}

	// 5. CORS - Handle Cross-Origin requests
	if s.config.Environment == "development" {
		r.Use(mw.SimpleCORS) // Allow all origins in development
	} else {
		corsConfig := mw.ProductionCORSConfig(s.config.AllowedOrigins)
		r.Use(mw.CORS(corsConfig))
	}

	// 6. Security Headers
	r.Use(mw.SecurityHeaders)

	// 7. Request Timeout - Prevent long-running requests
	r.Use(mw.Timeout(30 * time.Second))

	// 8. Max Body Size - Prevent large payloads (1MB default)
	r.Use(mw.MaxBodySize(1 << 20)) // 1MB

	// ==========================================================================
	// Public Routes (no authentication required)
	// ==========================================================================

	// Health checks
	r.Get("/api/v1/health", s.handleHealth)
	r.Get("/api/v1/ready", s.handleReady)
	r.Get("/api/v1/not_implement", s.handler.HandleRegister)

	s.router = r
}
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	api.WriteJSON(w, http.StatusOK, map[string]string{
		"status": "healthy",
	})
}
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	checks := make(map[string]string)
	// Check database connectivity
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := s.store.Ping(ctx); err != nil {
		checks["database"] = "unhealthy: " + err.Error()
		api.WriteReady(w, false, checks)
		return
	}
	checks["database"] = "healthy"

	// TODO: Add other checks (e.g., cache, external services)

	api.WriteReady(w, true, checks)
}
func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	api.WriteJSON(w, http.StatusOK, map[string]string{
		"version":     "1.0.0",
		"api_version": "v1",
		"go_version":  "1.21",
	})
}
