package server

import (
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	mw "Intelligent_Dev_ToolKit_Odoo/internal/middleware"
	"Intelligent_Dev_ToolKit_Odoo/internal/service"
	"context"
	"fmt"
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
	r.Get("/api/v1/ready", s.handleReady)
	r.Get("/api/v1/not_implement", s.handler.Auth.HandleNotImplement)

	// Root
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		api.WriteJSON(w, http.StatusOK, map[string]string{
			"service": "OdooDevTools API",
			"version": "1.0.0",
			"status":  "running",
		})
	})
	r.Route("/api/v1", func(r chi.Router) {
		// --------------------
		// Public endpoints
		// --------------------

		r.Get("/health", s.handleHealth)
		r.Get("/version", s.handleVersion)

		// Authentication (public)
		r.Route("/auth", func(r chi.Router) {

			// Public auth endpoints (no authentication required)
			if s.handler.Auth != nil {
				r.Post("/register", s.handler.Auth.HandleRegister)
				r.Post("/login", s.handler.Auth.HandleLogin)
				r.Post("/refresh", s.handler.Auth.HandleRefreshToken)
				r.Post("/forgot-password", s.handler.Auth.HandleForgotPassword)
				r.Post("/reset-password", s.handler.Auth.HandleResetPassword)
				r.Post("/verify-email", s.handler.Auth.HandleVerifyEmail)
			} else {
				// Fallback to not implemented
				r.Post("/register", s.handleRegister)
				r.Post("/login", s.handleLogin)
				r.Post("/refresh", s.handleRefreshToken)
				r.Post("/forgot-password", s.handleForgotPassword)
				r.Post("/reset-password", s.handleResetPassword)
			}
		})
	})

	// --------------------
	// Protected endpoints (JWT auth)
	// --------------------

	r.Group(func(r chi.Router) {
		// JWT Authentication - use service-based auth if available (checks Redis blacklist)
		if s.services != nil {
			r.Use(mw.JWTAuthWithService(s.services.Auth))
		} else {
			r.Use(mw.JWTAuth(s.tokenMaker))
		}

		// Tenant Resolution
		r.Use(mw.TenantResolver(mw.DatabaseTenantLookup(s.store)))

		r.Use(mw.TieredRateLimit(mw.DefaultPlanLimits))

		// Authenticated Auth endpoints
		r.Route("/auth", func(r chi.Router) {
			if s.handler.Auth != nil {
				r.Post("/logout", s.handler.Auth.HandleLogout)
				r.Get("/me", s.handler.Auth.HandleGetCurrentUser)
				r.Patch("/me", s.handler.Auth.HandleUpdateCurrentUser)
				r.Post("/change-password", s.handler.Auth.HandleChangePassword)
				r.Get("/sessions", s.handler.Auth.HandleGetSessions)
				r.Delete("/sessions/{session_id}", s.handler.Auth.HandleRevokeSession)
				r.Post("/resend-verification", s.handler.Auth.HandleResendVerification)
			} else {
				r.Post("/logout", s.handleLogout)
				r.Get("/me", s.handleGetCurrentUser)
				r.Patch("/me", s.handleUpdateCurrentUser)
				r.Post("/change-password", s.handleChangePassword)
			}
		})
	})
	s.router = r
}
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	api.WriteJSON(w, http.StatusOK, map[string]string{
		"status": "healthy",
	})
}
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	checks := make(map[string]string)

	// create a cancellable context so that individual probes don't hang
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// ------------------------------------------------------------------
	// database
	// ------------------------------------------------------------------
	if err := s.store.Ping(ctx); err != nil {
		checks["database"] = "unhealthy: " + err.Error()
		api.WriteReady(w, false, checks)
		return
	}
	checks["database"] = "healthy"

	// ------------------------------------------------------------------
	// cache (redis) – optional component
	// ------------------------------------------------------------------
	if s.cache != nil {
		if err := s.cache.Client().Ping(ctx).Err(); err != nil {
			checks["cache"] = "unhealthy: " + err.Error()
			api.WriteReady(w, false, checks)
			return
		}
		checks["cache"] = "healthy"
	}

	// ------------------------------------------------------------------
	// external services (example)
	// if your application depends on a third party API you can perform
	// a lightweight GET/HEAD request and report its status here.
	// ------------------------------------------------------------------
	if u := s.config.AgentCloudURL; u != "" {
		client := http.Client{Timeout: 2 * time.Second}
		if resp, err := client.Get(u); err != nil || resp.StatusCode >= 400 {
			checks["agent_cloud"] = "unhealthy"
			if err != nil {
				checks["agent_cloud"] += ": " + err.Error()
			} else {
				checks["agent_cloud"] += fmt.Sprintf(" status=%d", resp.StatusCode)
			}
			api.WriteReady(w, false, checks)
			return
		} else {
			checks["agent_cloud"] = "healthy"
		}
	}

	api.WriteReady(w, true, checks)
}
func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	api.WriteJSON(w, http.StatusOK, map[string]string{
		"version":     "1.0.0",
		"api_version": "v1",
		"go_version":  "1.24.4",
	})
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {

	var req service.RegisterRequest
	if apiErr := s.handler.Auth.DecodeJSON(r, &req); apiErr != nil {
		s.handler.Auth.WriteErr(w, r, apiErr)
	}

	if apiErr := s.handler.Auth.ValidateRequest(&req); apiErr != nil {
		s.handler.Auth.WriteErr(w, r, apiErr)
		return
	}

	resp, err := s.services.Auth.Register(r.Context(), &req, s.handler.Auth.ClientIP(r), r.Header.Get("User-Agent"))

	if err != nil {
		s.handler.Auth.HandleErr(w, r, err)
		return
	}

	api.WriteCreated(w, r, resp)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req service.LoginRequest
	if apiErr := s.handler.Auth.DecodeJSON(r, &req); apiErr != nil {
		s.handler.Auth.WriteErr(w, r, apiErr)
		return
	}
	if apiErr := s.handler.Auth.ValidateRequest(&req); apiErr != nil {
		s.handler.Auth.WriteErr(w, r, apiErr)
		return
	}

	resp, err := s.services.Auth.Login(r.Context(), &req, s.handler.Auth.ClientIP(r), r.Header.Get("User-Agent"))
	if err != nil {
		s.handler.Auth.HandleErr(w, r, err)
		return
	}

	api.WriteSuccess(w, r, resp)
}

func (s *Server) handleRefreshToken(w http.ResponseWriter, r *http.Request) {

	var req service.RefreshTokenRequest
	if apiErr := s.handler.Auth.DecodeJSON(r, &req); apiErr != nil {
		s.handler.Auth.WriteErr(w, r, apiErr)
		return
	}
	if apiErr := s.handler.Auth.ValidateRequest(&req); apiErr != nil {
		s.handler.Auth.WriteErr(w, r, apiErr)
		return
	}

	resp, err := s.services.Auth.RefreshToken(r.Context(), &req, s.handler.Auth.ClientIP(r), r.Header.Get("User-Agent"))
	if err != nil {
		s.handler.Auth.HandleErr(w, r, err)
		return
	}

	api.WriteSuccess(w, r, resp)
}

func (s *Server) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req service.ForgotPasswordRequest
	if apiErr := s.handler.Auth.DecodeJSON(r, &req); apiErr != nil {
		s.handler.Auth.WriteErr(w, r, apiErr)
		return
	}
	if apiErr := s.handler.Auth.ValidateRequest(&req); apiErr != nil {
		s.handler.Auth.WriteErr(w, r, apiErr)
		return
	}

	// Intentionally ignore the error — security requirement
	_ = s.services.Auth.ForgotPassword(r.Context(), &req)

	api.WriteSuccess(w, r, map[string]any{
		"message": "If that email address is registered you will receive a reset link shortly.",
	})
}

func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {

	var req service.ResetPasswordRequest
	if apiErr := s.handler.Auth.DecodeJSON(r, &req); apiErr != nil {
		s.handler.Auth.WriteErr(w, r, apiErr)
		return
	}
	if apiErr := s.handler.Auth.ValidateRequest(&req); apiErr != nil {
		s.handler.Auth.WriteErr(w, r, apiErr)
		return
	}

	if err := s.services.Auth.ResetPassword(r.Context(), &req); err != nil {
		s.handler.Auth.HandleErr(w, r, err)
		return
	}

	api.WriteSuccess(w, r, map[string]any{
		"message": "Password has been reset. Please log in with your new password.",
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	BearerToken := s.handler.Auth.BearerToken(r)
	if BearerToken == "" {
		s.handler.Auth.WriteErr(w, r, api.ErrMissingAuthHeader())
		return
	}

	// Body is optional — default to single-token logout
	var req service.LogoutRequest
	_ = s.handler.Auth.DecodeJSON(r, &req) // intentionally ignoring parse errors

	if err := s.services.Auth.Logout(r.Context(), BearerToken, &req); err != nil {
		s.handler.Auth.HandleErr(w, r, err)
		return
	}

	api.WriteNoContent(w)
}

func (s *Server) handleGetCurrentUser(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.handler.Auth.MustUserID(w, r)
	if !ok {
		return
	}
	tenantID, ok := s.handler.Auth.MustTenantID(w, r)
	if !ok {
		return
	}

	user, err := s.services.Auth.GetCurrentUser(r.Context(), userID, tenantID)
	if err != nil {
		s.handler.Auth.HandleErr(w, r, err)
		return
	}

	api.WriteSuccess(w, r, user)
}
func (s *Server) handleUpdateCurrentUser(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.handler.Auth.MustUserID(w, r)
	if !ok {
		return
	}
	tenantID, ok := s.handler.Auth.MustTenantID(w, r)
	if !ok {
		return
	}

	var req service.UpdateUserRequest
	if apiErr := s.handler.Auth.DecodeJSON(r, &req); apiErr != nil {
		s.handler.Auth.WriteErr(w, r, apiErr)
		return
	}
	if apiErr := s.handler.Auth.ValidateRequest(&req); apiErr != nil {
		s.handler.Auth.WriteErr(w, r, apiErr)
		return
	}

	user, err := s.services.Auth.UpdateCurrentUser(r.Context(), userID, tenantID, &req)
	if err != nil {
		s.handler.Auth.HandleErr(w, r, err)
		return
	}

	api.WriteSuccess(w, r, user)
}
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {

	userID, ok := s.handler.Auth.MustUserID(w, r)
	if !ok {
		return
	}
	tenantID, ok := s.handler.Auth.MustTenantID(w, r)
	if !ok {
		return
	}

	var req service.ChangePasswordRequest
	if apiErr := s.handler.Auth.DecodeJSON(r, &req); apiErr != nil {
		s.handler.Auth.WriteErr(w, r, apiErr)
		return
	}
	if apiErr := s.handler.Auth.ValidateRequest(&req); apiErr != nil {
		s.handler.Auth.WriteErr(w, r, apiErr)
		return
	}

	if err := s.services.Auth.ChangePassword(r.Context(), userID, tenantID, &req); err != nil {
		s.handler.Auth.HandleErr(w, r, err)
		return
	}

	api.WriteSuccess(w, r, map[string]any{
		"message": "Password changed successfully. Please log in again.",
	})
}
