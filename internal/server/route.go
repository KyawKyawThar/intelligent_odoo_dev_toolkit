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
	"github.com/rs/zerolog"
	httpSwagger "github.com/swaggo/http-swagger"
)

func (s *Server) setupRoutes() {
	r := chi.NewRouter()

	s.configureLogger()
	s.setupMiddleware(r)
	s.setupSwagger(r)
	s.setupPublicRoutes(r)
	s.setupProtectedRoutes(r)
	s.setupAgentPublicRoutes(r)
	s.setupAgentRoutes(r)

	s.router = r
}

func (s *Server) configureLogger() {
	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	if s.config.Environment == config.EnvironmentDevelopment {
		logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).With().Timestamp().Logger()
	}
	s.logger = &logger
}

func (s *Server) setupMiddleware(r chi.Router) {
	r.Use(mw.RealIP)
	r.Use(mw.RequestID)
	r.Use(mw.Recoverer(s.logger))

	healthPaths := []string{"/api/v1/ready", "/api/v1/health"}
	r.Use(mw.SkipPaths(s.logger, healthPaths...))

	if s.config.Environment == config.EnvironmentDevelopment {
		r.Use(mw.SimpleCORS)
	} else {
		corsConfig := mw.ProductionCORSConfig(s.config.AllowedOrigins)
		r.Use(mw.CORS(corsConfig))
	}

	r.Use(mw.SecurityHeaders)
	// Timeout and MaxBodySize are NOT applied globally — they are set
	// per-route-group so that long-lived connections (WebSocket) and
	// large payloads (schema push) can use different limits.
}

func (s *Server) setupSwagger(r chi.Router) {
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
}

func (s *Server) setupPublicRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(mw.Timeout(30 * time.Second))
		r.Use(mw.MaxBodySize(1 << 20)) // 1 MB

		r.Get("/api/v1/ready", s.handleReady)
		r.Get("/api/v1/not_implement", s.handler.Auth.HandleNotImplement)

		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			dto.WriteJSON(w, http.StatusOK, map[string]string{
				"service": "OdooDevTools API",
				"version": "1.0.0",
				"status":  "running",
				"docs":    "/docs",
			})
		})

		r.Route("/api/v1", func(r chi.Router) {
			r.Get("/health", s.handleHealth)
			r.Get("/version", s.handleVersion)

			r.Route("/auth", func(r chi.Router) {
				if s.handler.Auth != nil {
					r.Post("/register", s.handler.Auth.HandleRegister)
					r.Post("/login", s.handler.Auth.HandleLogin)
					r.Post("/refresh", s.handler.Auth.HandleRefreshToken)
					r.Post("/forgot-password", s.handler.Auth.HandleForgotPassword)
					r.Post("/reset-password", s.handler.Auth.HandleResetPassword)
					r.Post("/verify-email", s.handler.Auth.HandleVerifyEmail)
				} else {
					r.Post("/register", s.handleRegister)
					r.Post("/login", s.handleLogin)
					r.Post("/refresh", s.handleRefreshToken)
					r.Post("/forgot-password", s.handleForgotPassword)
					r.Post("/reset-password", s.handleResetPassword)
				}
			})
		})
	})
}

func (s *Server) setupProtectedRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(mw.Timeout(30 * time.Second))
		r.Use(mw.MaxBodySize(1 << 20)) // 1 MB

		if s.services != nil {
			r.Use(mw.JWTAuthWithService(s.services.Auth))
		} else {
			r.Use(mw.JWTAuth(s.tokenMaker))
		}

		r.Use(mw.TenantResolver(mw.DatabaseTenantLookup(s.store)))
		r.Use(mw.TieredRateLimit(mw.DefaultPlanLimits))
		r.Use(mw.AuditLog(s.store, *s.logger))

		s.registerAuthRoutes(r)
		s.registerEnvironmentRoutes(r)
		s.registerStandaloneMigrationRoutes(r)
		s.registerAuditRoutes(r)
		s.registerNotificationChannelRoutes(r)
	})
}

// registerNotificationChannelRoutes mounts CRUD endpoints for notification channels.
func (s *Server) registerNotificationChannelRoutes(r chi.Router) {
	if s.handler.NotificationChannel == nil {
		return
	}
	r.Route("/api/v1/notification-channels", func(r chi.Router) {
		r.Get("/", s.handler.NotificationChannel.HandleList)
		r.Post("/", s.handler.NotificationChannel.HandleCreate)
		r.Route("/{channel_id}", func(r chi.Router) {
			r.Get("/", s.handler.NotificationChannel.HandleGet)
			r.Patch("/", s.handler.NotificationChannel.HandleUpdate)
			r.Delete("/", s.handler.NotificationChannel.HandleDelete)
		})
	})
}

// registerAuditRoutes mounts the audit log query endpoints.
func (s *Server) registerAuditRoutes(r chi.Router) {
	if s.handler.Audit == nil {
		return
	}
	r.Route("/api/v1/audit-logs", func(r chi.Router) {
		r.Get("/", s.handler.Audit.HandleList)
		r.Get("/by-action/{action}", s.handler.Audit.HandleListByAction)
		r.Get("/by-user/{user_id}", s.handler.Audit.HandleListByUser)
		r.Get("/by-resource", s.handler.Audit.HandleListByResource)
		r.Get("/between", s.handler.Audit.HandleListBetween)
	})
}

// registerStandaloneMigrationRoutes mounts migration endpoints that do not require an
// environment ID. These are used by the offline Source Scanner and the transitions list.
func (s *Server) registerStandaloneMigrationRoutes(r chi.Router) {
	if s.handler.Migration == nil {
		return
	}
	r.Route("/api/v1/migration", func(r chi.Router) {
		r.Use(mw.MaxBodySize(2 << 20)) // 2 MB — source scan payloads can be large
		r.Post("/scan/source", s.handler.Migration.HandleScanSource)
		r.Get("/transitions", s.handler.Migration.HandleSupportedTransitions)
	})
}

// registerAuthRoutes mounts the authenticated auth endpoints.
func (s *Server) registerAuthRoutes(r chi.Router) {
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
}

// registerEnvironmentRoutes mounts the environment CRUD and nested routes.
func (s *Server) registerEnvironmentRoutes(r chi.Router) {
	if s.handler.Environment == nil {
		return
	}

	r.Route("/api/v1/environments", func(r chi.Router) {
		r.Post("/", s.handler.Environment.HandleCreate)
		r.Get("/", s.handler.Environment.HandleList)

		r.Route("/{env_id}", func(r chi.Router) {
			r.Get("/", s.handler.Environment.HandleGet)
			r.Patch("/", s.handler.Environment.HandleUpdate)
			r.Delete("/", s.handler.Environment.HandleDelete)

			s.registerEnvSubRoutes(r)
		})
	})
}

// registerEnvSubRoutes mounts feature-specific sub-routes under /{env_id}.
func (s *Server) registerEnvSubRoutes(r chi.Router) {
	if s.handler.Ws != nil {
		r.Put("/flags", s.handler.Ws.HandleUpdateFlags)
	}

	if s.handler.Schema != nil {
		r.Route("/errors", func(r chi.Router) {
			r.Get("/", s.handler.Error.HandleListErrors)
			r.Route("/{error_id}", func(r chi.Router) {
				r.Get("/", s.handler.Error.HandleGetErrorGroup)
				r.Patch("/", s.handler.Error.HandleUpdateErrorGroup)
			})
		})
		r.Route("/schema", func(r chi.Router) {
			r.Get("/", s.handler.Schema.HandleList)
			r.Get("/latest", s.handler.Schema.HandleGetLatest)
			r.Get("/models", s.handler.Schema.HandleSearchModels)
			r.Get("/{snapshot_id}", s.handler.Schema.HandleGetSnapshot)
		})
	}

	if s.handler.ACL != nil {
		r.Post("/acl/trace", s.handler.ACL.HandleTraceAccess)
	}

	if s.handler.Overview != nil {
		r.Get("/overview", s.handler.Overview.HandleGet)
	}
	//nolint:dupl // similar routing structures are expected
	if s.handler.Profiler != nil {
		r.Route("/profiler/recordings", func(r chi.Router) {
			r.Get("/", s.handler.Profiler.HandleListRecordings)
			r.Get("/slow", s.handler.Profiler.HandleListSlowRecordings)
			r.Get("/{recording_id}", s.handler.Profiler.HandleGetRecording)
		})
		r.Route("/profiler/chain", func(r chi.Router) {
			r.Get("/", s.handler.Profiler.HandleListChainRecordings)
			r.Get("/{recording_id}", s.handler.Profiler.HandleGetChain)
		})
	}

	if s.handler.N1 != nil {
		r.Route("/n1", func(r chi.Router) {
			r.Get("/detect", s.handler.N1.HandleDetect)
			r.Get("/timeline", s.handler.N1.HandleTimeline)
		})
	}
	//nolint:dupl // similar routing structures are expected
	if s.handler.Alert != nil {
		r.Route("/alerts", func(r chi.Router) {
			r.Get("/", s.handler.Alert.HandleList)
			r.Get("/count", s.handler.Alert.HandleCount)
			r.Post("/acknowledge-all", s.handler.Alert.HandleAcknowledgeAll)
			r.Route("/{alert_id}", func(r chi.Router) {
				r.Get("/", s.handler.Alert.HandleGet)
				r.Post("/acknowledge", s.handler.Alert.HandleAcknowledge)
			})
		})
	}

	if s.handler.Budget != nil {
		r.Route("/budgets", func(r chi.Router) {
			r.Post("/", s.handler.Budget.HandleCreate)
			r.Get("/", s.handler.Budget.HandleList)
			r.Route("/{budget_id}", func(r chi.Router) {
				r.Get("/", s.handler.Budget.HandleGet)
				r.Patch("/", s.handler.Budget.HandleUpdate)
				r.Delete("/", s.handler.Budget.HandleDelete)
				r.Get("/samples", s.handler.Budget.HandleListSamples)
				r.Get("/samples/{sample_id}/breakdown", s.handler.Budget.HandleGetBreakdown)
				r.Get("/trend", s.handler.Budget.HandleGetTrend)
			})
		})
	}

	if s.handler.Migration != nil {
		r.Route("/migration", func(r chi.Router) {
			r.Post("/scan", s.handler.Migration.HandleRunScan)
			r.Post("/scan/source", s.handler.Migration.HandleScanSource)
			r.Get("/transitions", s.handler.Migration.HandleSupportedTransitions)
			r.Route("/scans", func(r chi.Router) {
				r.Get("/", s.handler.Migration.HandleListScans)
				r.Get("/latest", s.handler.Migration.HandleGetLatestScan)
				r.Route("/{scan_id}", func(r chi.Router) {
					r.Get("/", s.handler.Migration.HandleGetScan)
					r.Delete("/", s.handler.Migration.HandleDeleteScan)
				})
			})
		})
	}

	r.Post("/agent", s.handler.Environment.HandleRegisterAgent)

	r.Route("/heartbeats", func(r chi.Router) {
		r.Get("/", s.handler.Environment.HandleListHeartbeats)
		r.Get("/latest", s.handler.Environment.HandleGetLatestHeartbeat)
	})

	if s.handler.APIKey != nil {
		r.Route("/api-keys", func(r chi.Router) {
			r.Post("/", s.handler.APIKey.HandleCreate)
			r.Get("/", s.handler.APIKey.HandleList)
			r.Delete("/{key_id}", s.handler.APIKey.HandleRevoke)
		})
	}
}

func (s *Server) setupAgentPublicRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(mw.Timeout(30 * time.Second))
		r.Use(mw.MaxBodySize(1 << 20)) // 1 MB
		// Agent self-registration: no auth required (protected by one-time token).
		r.Post("/api/v1/agent/register", s.handler.AgentRegister.HandleSelfRegister)
	})
}

func (s *Server) setupAgentRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(mw.AgentAPIKeyAuth(s.store))

		r.Route("/api/v1/agent", func(r chi.Router) {
			// WebSocket: no timeout, no body-size limit (long-lived connection).
			r.Group(func(r chi.Router) {
				r.Get("/ws", s.handler.Ws.HandleWebSocket)
			})

			// Schema push: larger body limit (Odoo schemas can be several MB).
			if s.handler.Schema != nil {
				r.Group(func(r chi.Router) {
					r.Use(mw.Timeout(30 * time.Second))
					r.Use(mw.MaxBodySize(10 << 20)) // 10 MB
					r.Post("/schema", s.handler.Schema.HandleStore)
				})
			}

			// Other agent endpoints: standard 1 MB limit.
			r.Group(func(r chi.Router) {
				r.Use(mw.Timeout(30 * time.Second))
				r.Use(mw.MaxBodySize(1 << 20)) // 1 MB

				if s.handler.Error != nil {
					r.Post("/errors", s.handler.Error.HandleIngestErrors)
				}

				if s.handler.Batch != nil {
					r.Post("/batch", s.handler.Batch.HandleIngestBatch)
				}
			})
		})
	})
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

		if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" && parsedURL.Scheme != "ws" {
			checks["agent_cloud"] = config.StatusUnhealthy + ": invalid URL scheme"
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
		if parsedURL.Scheme == "ws" {
			// For WebSocket, we can't make a simple HTTP request.
			// We can either try to establish a connection or just assume it's fine if the URL is valid.
			// For a readiness probe, assuming it's fine is acceptable.
			checks["agent_cloud"] = config.StatusHealthy
		} else {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
			if err != nil {
				checks["agent_cloud"] = config.StatusUnhealthy + ": " + err.Error()
				dto.WriteReady(w, false, checks)
				return
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
