package middleware

import (
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/token"
	"context"
	"net/http"
	"strings"
)

// =============================================================================
// Auth Middleware Configuration
// =============================================================================

type AuthConfig struct {
	TokenMaker  token.Maker
	SkipPaths   []string
	HeaderName  string // Default: "Authorization"
	TokenPrefix string // Default: "Bearer "
}

func DefaultAuthConfig(tokenMaker token.Maker) AuthConfig {
	return AuthConfig{
		TokenMaker:  tokenMaker,
		SkipPaths:   []string{"/health", "/ready", "/api/v1/health"},
		HeaderName:  "Authorization",
		TokenPrefix: "Bearer ",
	}
}

// =============================================================================
// JWT Auth Middleware
// =============================================================================

// Auth validates JWT token and sets user context
func Auth(config AuthConfig) func(http.Handler) http.Handler {
	skipMap := make(map[string]bool)
	for _, path := range config.SkipPaths {
		skipMap[path] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip auth for certain paths
			if skipMap[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			// Get authorization header
			authHeader := r.Header.Get(config.HeaderName)
			if authHeader == "" {
				api.HandleError(w, r, api.ErrMissingAuthHeader())
				return
			}

			// Check prefix
			if !strings.HasPrefix(authHeader, config.TokenPrefix) {
				api.HandleError(w, r, api.ErrInvalidToken("Invalid authorization header format"))
				return
			}

			// Extract token
			tokenString := strings.TrimPrefix(authHeader, config.TokenPrefix)
			if tokenString == "" {
				api.HandleError(w, r, api.ErrInvalidToken("Token is empty"))
				return
			}

			// Verify token
			payload, err := config.TokenMaker.VerifyToken(tokenString)
			if err != nil {
				if err == token.ErrTokenExpired {
					api.HandleError(w, r, api.ErrExpiredToken())
					return
				}
				api.HandleError(w, r, api.ErrInvalidToken(""))
				return
			}

			// Set user context
			ctx := r.Context()
			ctx = context.WithValue(ctx, ContextKeyUserID, payload.ID.String())
			// Note: TenantID should come from the user lookup, not the token
			//ctx = context.WithValue(ctx, ContextKeyTenantID, tenantID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// =============================================================================
// Simple JWT Auth (without config)
// =============================================================================

// JWTAuth is a simpler auth middleware
func JWTAuth(tokenMaker token.Maker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				api.HandleError(w, r, api.ErrMissingAuthHeader())
				return
			}

			// Check prefix
			if !strings.HasPrefix(authHeader, "Bearer ") {
				api.HandleError(w, r, api.ErrInvalidToken("Authorization header must start with 'Bearer '"))
				return
			}

			// Extract and verify token
			tokenString := strings.TrimPrefix(authHeader, "Bearer ")
			payload, err := tokenMaker.VerifyToken(tokenString)
			if err != nil {
				if err == token.ErrTokenExpired {
					api.HandleError(w, r, api.ErrExpiredToken())
					return
				}
				api.HandleError(w, r, api.ErrInvalidToken(""))
				return
			}

			// Set context values
			ctx := SetUserID(r.Context(), payload.ID.String())

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// =============================================================================
// API Key Auth Middleware
// =============================================================================

// APIKeyAuthFunc is a function that validates an API key and returns environment details
type APIKeyAuthFunc func(ctx context.Context, apiKey string) (*APIKeyInfo, error)

type APIKeyInfo struct {
	KeyID         string
	TenantID      string
	EnvironmentID string
	Scopes        []string
}

// APIKeyAuth validates X-API-Key header (for agent endpoints)
func APIKeyAuth(validateFunc APIKeyAuthFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := r.Header.Get("X-API-Key")
			if apiKey == "" {
				api.HandleError(w, r, api.ErrMissingAPIKey())
				return
			}

			keyInfo, err := validateFunc(r.Context(), apiKey)
			if err != nil {
				if apiErr, ok := err.(*api.APIError); ok {
					api.HandleError(w, r, apiErr)
					return
				}
				api.HandleError(w, r, api.ErrInvalidAPIKey())
				return
			}

			// Set context values from API key
			ctx := r.Context()
			ctx = SetAPIKeyID(ctx, keyInfo.KeyID)
			ctx = SetTenantID(ctx, keyInfo.TenantID)
			ctx = SetEnvID(ctx, keyInfo.EnvironmentID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// =============================================================================
// Scope-Based Access Control (for API keys)
// =============================================================================

type scopeContextKey string

const ContextKeyScopes scopeContextKey = "scopes"

// SetScopes sets the API key scopes in context
func SetScopes(ctx context.Context, scopes []string) context.Context {
	return context.WithValue(ctx, ContextKeyScopes, scopes)
}

// GetScopes retrieves the API key scopes from context
func GetScopes(ctx context.Context) []string {
	if scopes, ok := ctx.Value(ContextKeyScopes).([]string); ok {
		return scopes
	}
	return nil
}

// RequireScope ensures the API key has the required scope
func RequireScope(requiredScope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scopes := GetScopes(r.Context())
			if scopes == nil {
				api.HandleError(w, r, api.ErrInsufficientScope(requiredScope))
				return
			}

			hasScope := false
			for _, scope := range scopes {
				if scope == requiredScope || scope == "*" {
					hasScope = true
					break
				}
			}

			if !hasScope {
				api.HandleError(w, r, api.ErrInsufficientScope(requiredScope))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
