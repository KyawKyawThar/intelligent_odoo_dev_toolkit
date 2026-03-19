// Package middleware provides HTTP middleware for the application.
package middleware

import (
	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/utils"
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	// AuthorizationHeaderKey is the header key for the authorization token.
	AuthorizationHeaderKey = "Authorization"
	// AuthorizationTypeAPIKey is the authorization type for API keys.
	AuthorizationTypeAPIKey = "ApiKey"
	// AuthorizationPayloadKey is the context key for the authorization payload.
	AuthorizationPayloadKey = "authorization_payload"
)

// APIKeyAuthenticator is an interface for authenticating API keys.
type APIKeyAuthenticator interface {
	GetAPIKeyByHash(ctx context.Context, keyHash string) (db.ApiKey, error)
	TouchAPIKey(ctx context.Context, id uuid.UUID) error
}

// AgentAPIKeyAuth is a middleware for authenticating agent API keys.
func AgentAPIKeyAuth(store APIKeyAuthenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey, ok := extractAPIKey(w, r)
			if !ok {
				return
			}

			apiKeyRecord, ok := lookupAPIKey(w, r, store, apiKey)
			if !ok {
				return
			}

			if err := validateAPIKeyRecord(apiKeyRecord); err != nil {
				api.HandleError(w, r, err)
				return
			}

			go touchAPIKeyAsync(store, apiKeyRecord.ID) //nolint:gosec,contextcheck // G118: intentionally outlives request

			ctx := SetTenantID(r.Context(), apiKeyRecord.TenantID.String())
			ctx = SetAPIKeyID(ctx, apiKeyRecord.ID.String())
			if apiKeyRecord.EnvironmentID != nil {
				ctx = SetEnvID(ctx, apiKeyRecord.EnvironmentID.String())
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// extractAPIKey parses the Authorization header and returns the raw API key.
// Returns false (and writes an error response) if the header is missing or malformed.
func extractAPIKey(w http.ResponseWriter, r *http.Request) (string, bool) {
	authorizationHeader := r.Header.Get(AuthorizationHeaderKey)
	if authorizationHeader == "" {
		api.HandleError(w, r, api.ErrMissingAuthHeader())
		return "", false
	}

	fields := strings.Fields(authorizationHeader)
	if len(fields) < 2 {
		api.HandleError(w, r, api.ErrInvalidAuthHeaderFormat())
		return "", false
	}

	authorizationType := strings.ToLower(fields[0])
	if !strings.EqualFold(authorizationType, AuthorizationTypeAPIKey) {
		api.HandleError(w, r, api.ErrUnsupportedAuthType(AuthorizationTypeAPIKey))
		return "", false
	}

	return fields[1], true
}

// lookupAPIKey hashes the key and looks it up in the store.
// Returns false (and writes an error response) on failure.
func lookupAPIKey(w http.ResponseWriter, r *http.Request, store APIKeyAuthenticator, apiKey string) (db.ApiKey, bool) {
	hashedAPIKey := utils.HashAPIKey(apiKey)

	apiKeyRecord, err := store.GetAPIKeyByHash(r.Context(), hashedAPIKey)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.HandleError(w, r, api.ErrInvalidAPIKey())
			return db.ApiKey{}, false
		}
		api.HandleError(w, r, api.ErrInternal(err))
		return db.ApiKey{}, false
	}
	return apiKeyRecord, true
}

// validateAPIKeyRecord checks that the key is active and not expired.
func validateAPIKeyRecord(rec db.ApiKey) *api.Error {
	if !rec.IsActive {
		return api.ErrInvalidAPIKey()
	}
	if rec.ExpiresAt != nil && rec.ExpiresAt.Before(time.Now()) {
		return api.ErrExpiredAPIKey()
	}
	return nil
}

// touchAPIKeyAsync updates the last-used timestamp in the background.
func touchAPIKeyAsync(store APIKeyAuthenticator, id uuid.UUID) {
	bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if touchErr := store.TouchAPIKey(bgCtx, id); touchErr != nil {
		log.Printf("failed to touch API key: %v", touchErr)
	}
}
