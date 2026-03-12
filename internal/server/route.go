// Package server provides the server implementation.
package server

import (
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/config"
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"
	mw "Intelligent_Dev_ToolKit_Odoo/internal/middleware"
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
	httpSwagger "github.com/swaggo/http-swagger"
)

func (s *Server) setupRoutes() {

	r := chi.NewRouter()

	// Create logger
	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	if s.config.Environment == config.EnvironmentDevelopment {
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
	if s.config.Environment == config.EnvironmentDevelopment {
		r.Use(middleware.Logger) // Chi's built-in logger (pretty)
	} else {
		r.Use(mw.RequestLogger(&logger)) // Structured JSON logging
	}

	// 5. CORS - Handle Cross-Origin requests
	if s.config.Environment == config.EnvironmentDevelopment {
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

	// Swagger docs
	r.Group(func(r chi.Router) {
		r.Use(mw.SwaggerCSP)
		r.Get("/docs", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/docs/", http.StatusMovedPermanently)
		})
		r.Get("/docs/*", httpSwagger.Handler(
			httpSwagger.URL("/docs/doc.json"),
			httpSwagger.DeepLinking(true),
			httpSwagger.DocExpansion("list"),
			httpSwagger.UIConfig(map[string]string{

				"responseInterceptor": `function(response) {
					console.log('[swagger] responseInterceptor fired', {
						status: response.status,
						url: response.url,
						obj: response.obj,
						body: response.body,
					});
					if (response.status === 200 || response.status === 201) {
						var url = response.url || '';
						if (url.indexOf('/auth/login') !== -1 || url.indexOf('/auth/register') !== -1) {
							var data = response.obj && response.obj.data;
							console.log('[swagger] auth response data', data);
							if (data && data.access_token) {
								window.ui.preauthorizeApiKey('BearerAuth', 'Bearer ' + data.access_token);
							}
						}
					}
					return response;
				}`,
			}),
		))
	})

	// Root
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		dto.WriteJSON(w, http.StatusOK, map[string]string{
			"service": "OdooDevTools API",
			"version": "1.0.0",
			"status":  "running",
			"docs":    "/docs",
		})
	})

	// ==========================================================================
	// API v1 — Public endpoints
	// ==========================================================================
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", s.handleHealth)
		r.Get("/version", s.handleVersion)

		// Authentication (public — no auth required)
		r.Route("/auth", func(r chi.Router) {
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

	// ==========================================================================
	// Protected endpoints (JWT auth + Tenant resolution)
	// ==========================================================================
	r.Group(func(r chi.Router) {
		// JWT Authentication - use service-based auth if available (checks Redis blacklist)
		if s.services != nil {
			r.Use(mw.JWTAuthWithService(s.services.Auth))
		} else {
			r.Use(mw.JWTAuth(s.tokenMaker))
		}

		// Tenant Resolution
		r.Use(mw.TenantResolver(mw.DatabaseTenantLookup(s.store)))

		// Rate Limiting
		r.Use(mw.TieredRateLimit(mw.DefaultPlanLimits))

		// ------------------------------------------------------------------
		// Auth (protected)
		// ------------------------------------------------------------------
		if s.handler.Auth != nil {
			r.Post("/api/v1/auth/logout", s.handler.Auth.HandleLogout)
			r.Get("/api/v1/auth/me", s.handler.Auth.HandleGetCurrentUser)
			r.Patch("/api/v1/auth/me", s.handler.Auth.HandleUpdateCurrentUser)
			r.Post("/api/v1/auth/change-password", s.handler.Auth.HandleChangePassword)
			r.Get("/api/v1/auth/sessions", s.handler.Auth.HandleGetSessions)
			r.Delete("/api/v1/auth/sessions/{session_id}", s.handler.Auth.HandleRevokeSession)
			r.Post("/api/v1/auth/resend-verification", s.handler.Auth.HandleResendVerification)
		} else {
			r.Post("/api/v1/auth/logout", s.handleLogout)
			r.Get("/api/v1/auth/me", s.handleGetCurrentUser)
			r.Patch("/api/v1/auth/me", s.handleUpdateCurrentUser)
			r.Post("/api/v1/auth/change-password", s.handleChangePassword)
		}

		// ------------------------------------------------------------------
		// Environments (protected — requires JWT + Tenant)
		// ------------------------------------------------------------------
		if s.handler.Environment != nil {

			r.Route("/api/v1/environments", func(r chi.Router) {

				r.Post("/", s.handler.Environment.HandleCreate)
				// r.Get("/", s.handler.Environment.HandleList)

				r.Route("/{env_id}", func(r chi.Router) {
					r.Get("/", s.handler.Environment.HandleGet)
					r.Patch("/", s.handler.Environment.HandleUpdate)
					r.Delete("/", s.handler.Environment.HandleDelete)
				})
			})
		}
	})

	// ==========================================================================
	// Agent endpoints (API Key auth)
	// ==========================================================================
	r.Group(func(r chi.Router) {
		r.Use(mw.AgentAPIKeyAuth(s.store))

		r.Route("/api/v1/agent", func(r chi.Router) {
			// websocket connection
			r.Get("/ws", s.handler.Ws.HandleWebSocket)
		})
	})

	s.router = r
}

// =============================================================================
// Handler Functions
// =============================================================================

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	dto.WriteJSON(w, http.StatusOK, map[string]string{
		"status": config.StatusHealthy,
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
		checks["database"] = config.StatusUnhealthy + ": " + err.Error()
		dto.WriteReady(w, false, checks)
		return
	}
	checks["database"] = config.StatusHealthy

	// ------------------------------------------------------------------
	// cache (redis) – optional component
	// ------------------------------------------------------------------
	if s.cache != nil {
		if err := s.cache.Client.Ping(ctx).Err(); err != nil {
			checks["cache"] = config.StatusUnhealthy + ": " + err.Error()
			dto.WriteReady(w, false, checks)
			return
		}
		checks["cache"] = config.StatusHealthy
	}

	// ------------------------------------------------------------------
	// external services (example)
	// ------------------------------------------------------------------
	if u := s.config.AgentCloudURL; u != "" {
		parsedURL, err := url.Parse(u)
		if err != nil {
			checks["agent_cloud"] = config.StatusUnhealthy + ": invalid URL: " + err.Error()
			dto.WriteReady(w, false, checks)
			return
		}

		if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
			checks["agent_cloud"] = config.StatusUnhealthy + ": invalid URL scheme"
			dto.WriteReady(w, false, checks)
			return
		}

		//gosec:G107
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
		if err != nil {
			checks["agent_cloud"] = config.StatusUnhealthy + ": " + err.Error()
			dto.WriteReady(w, false, checks)
			return
		}
		client := http.Client{
			Timeout: 2 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					MinVersion:         tls.VersionTLS12,
					InsecureSkipVerify: false,
				},
			},
		}
		resp, err := client.Do(req)
		if err != nil {
			checks["agent_cloud"] = config.StatusUnhealthy + ": " + err.Error()
			dto.WriteReady(w, false, checks)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			checks["agent_cloud"] = config.StatusUnhealthy + fmt.Sprintf(" status=%d", resp.StatusCode)
			dto.WriteReady(w, false, checks)
			return
		}

		checks["agent_cloud"] = config.StatusHealthy
	}

	dto.WriteReady(w, true, checks)
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	dto.WriteJSON(w, http.StatusOK, map[string]string{
		"version":     "1.0.0",
		"api_version": "v1",
		"go_version":  "1.26.1",
	})
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req dto.RegisterRequest
	s.handleAuthAction(w, r, &req, func(ctx context.Context) (interface{}, error) {
		return s.services.Auth.Register(ctx, &req, s.handler.Auth.ClientIP(r), r.Header.Get("User-Agent"))
	}, dto.WriteCreated)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req dto.LoginRequest
	s.handleAuthAction(w, r, &req, func(ctx context.Context) (interface{}, error) {
		return s.services.Auth.Login(ctx, &req, s.handler.Auth.ClientIP(r), r.Header.Get("User-Agent"))
	}, dto.WriteSuccess)
}

func (s *Server) handleRefreshToken(w http.ResponseWriter, r *http.Request) {
	var req dto.RefreshTokenRequest
	s.handleAuthAction(w, r, &req, func(ctx context.Context) (interface{}, error) {
		return s.services.Auth.RefreshToken(ctx, &req, s.handler.Auth.ClientIP(r), r.Header.Get("User-Agent"))
	}, dto.WriteSuccess)
}

func (s *Server) handleAuthAction(w http.ResponseWriter, r *http.Request, req interface{}, action func(ctx context.Context) (interface{}, error), successWriter func(http.ResponseWriter, *http.Request, interface{})) {
	if !s.handler.Auth.DecodeAndValidate(w, r, req) {
		return
	}

	resp, err := action(r.Context())
	if err != nil {
		s.handler.Auth.HandleErr(w, r, err)
		return
	}

	successWriter(w, r, resp)
}

func (s *Server) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req dto.ForgotPasswordRequest
	if !s.handler.Auth.DecodeAndValidate(w, r, &req) {
		return
	}

	_ = s.services.Auth.ForgotPassword(r.Context(), &req) //nolint:errcheck // Error is ignored to prevent user enumeration attacks. The service handles logging.

	dto.WriteSuccess(w, r, map[string]any{
		"message": "If that email address is registered you will receive a reset link shortly.",
	})
}

func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req dto.ResetPasswordRequest
	if !s.handler.Auth.DecodeAndValidate(w, r, &req) {
		return
	}

	if err := s.services.Auth.ResetPassword(r.Context(), &req); err != nil {
		s.handler.Auth.HandleErr(w, r, err)
		return
	}

	dto.WriteSuccess(w, r, map[string]any{
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
	var req dto.LogoutRequest
	_ = s.handler.Auth.DecodeJSON(r, &req) //nolint:errcheck // intentionally ignoring parse errors

	if err := s.services.Auth.Logout(r.Context(), BearerToken, &req); err != nil {
		s.handler.Auth.HandleErr(w, r, err)
		return
	}

	dto.WriteNoContent(w)
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

	dto.WriteSuccess(w, r, user)
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

	var req dto.UpdateUserRequest
	if !s.handler.Auth.DecodeAndValidate(w, r, &req) {
		return
	}

	user, err := s.services.Auth.UpdateCurrentUser(r.Context(), userID, tenantID, &req)
	if err != nil {
		s.handler.Auth.HandleErr(w, r, err)
		return
	}

	dto.WriteSuccess(w, r, user)
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

	var req dto.ChangePasswordRequest
	if !s.handler.Auth.DecodeAndValidate(w, r, &req) {
		return
	}

	if err := s.services.Auth.ChangePassword(r.Context(), userID, tenantID, &req); err != nil {
		s.handler.Auth.HandleErr(w, r, err)
		return
	}

	dto.WriteSuccess(w, r, map[string]any{
		"message": "Password changed successfully. Please log in again.",
	})
}
