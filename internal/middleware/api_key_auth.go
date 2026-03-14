// Package middleware provides HTTP middleware for the application.
package middleware

import (
	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

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
			authorizationHeader := r.Header.Get(AuthorizationHeaderKey)
			if authorizationHeader == "" {
				api.HandleError(w, r, api.ErrMissingAuthHeader())
				return
			}

			fields := strings.Fields(authorizationHeader)
			if len(fields) < 2 {
				api.HandleError(w, r, api.ErrInvalidAuthHeaderFormat())
				return
			}

			authorizationType := strings.ToLower(fields[0])
			if !strings.EqualFold(authorizationType, AuthorizationTypeAPIKey) {
				api.HandleError(w, r, api.ErrUnsupportedAuthType(AuthorizationTypeAPIKey))
				return
			}

			apiKey := fields[1]
			// TODO: We need to hash the api key before looking it up in the database.
			// For now, we will assume the key is the hash.
			hashedAPIKey := apiKey

			apiKeyRecord, err := store.GetAPIKeyByHash(r.Context(), hashedAPIKey)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					api.HandleError(w, r, api.ErrInvalidAPIKey())
					return
				}
				api.HandleError(w, r, api.ErrInternal(err))
				return
			}

			// Touch the API key to update the last used timestamp.
			go func(ctx context.Context) {
				if err := store.TouchAPIKey(ctx, apiKeyRecord.ID); err != nil {
					log.Printf("failed to touch API key: %v", err)
				}
			}(r.Context())

			ctx := SetTenantID(r.Context(), apiKeyRecord.TenantID.String())
			ctx = SetAPIKeyID(ctx, apiKeyRecord.ID.String())
			// Assuming environment is associated with API key, but the db schema doesn't reflect this yet.
			// For now, we will leave it empty.
			// ctx = SetEnvID(ctx, apiKeyRecord.EnvironmentID.String())

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
