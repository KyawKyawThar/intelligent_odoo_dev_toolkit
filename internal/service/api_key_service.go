package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"
	"Intelligent_Dev_ToolKit_Odoo/utils"

	"github.com/google/uuid"
)

// APIKeyService manages API keys for agent authentication.
type APIKeyService struct {
	store db.Store
}

func NewAPIKeyService(store db.Store) *APIKeyService {
	return &APIKeyService{store: store}
}

// Create generates a new API key for the given environment, scoped to the caller's tenant.
// The raw key is returned once and is never stored in plaintext.
func (s *APIKeyService) Create(
	ctx context.Context,
	tenantID, envID, userID uuid.UUID,
	req *dto.CreateAPIKeyRequest,
) (*dto.APIKeyCreatedResponse, error) {
	// Verify the environment belongs to this tenant.
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.ErrNotFound("environment not found")
	}

	// Generate a random 32-byte key: prefix "odt_" + base64url.
	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}
	rawKey := "odt_" + base64.RawURLEncoding.EncodeToString(rawBytes)

	// key_prefix = first 12 chars (safe to store and display).
	prefix := rawKey
	if len(prefix) > 12 {
		prefix = prefix[:12]
	}

	keyHash := utils.HashAPIKey(rawKey)

	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			return nil, api.ErrBadRequest("expires_at must be RFC3339 format")
		}
		expiresAt = &t
	} else {
		// Default to 90 days from now when not specified.
		t := time.Now().UTC().Add(90 * 24 * time.Hour)
		expiresAt = &t
	}

	scopes := req.Scopes
	if len(scopes) == 0 {
		scopes = []string{"agent:write"}
	}

	uid := userID
	eid := envID
	row, err := s.store.CreateAPIKey(ctx, db.CreateAPIKeyParams{
		TenantID:      tenantID,
		EnvironmentID: &eid,
		CreatedBy:     &uid,
		KeyHash:       keyHash,
		KeyPrefix:     prefix,
		Name:          req.Name,
		Description:   req.Description,
		Scopes:        scopes,
		ExpiresAt:     expiresAt,
	})
	if err != nil {
		return nil, api.ErrInternal(err)
	}

	return &dto.APIKeyCreatedResponse{
		ID:          row.ID,
		TenantID:    row.TenantID,
		EnvID:       envID,
		Name:        row.Name,
		Description: row.Description,
		KeyPrefix:   row.KeyPrefix,
		RawKey:      rawKey,
		Scopes:      row.Scopes,
		ExpiresAt:   row.ExpiresAt,
		CreatedAt:   row.CreatedAt,
	}, nil
}

// List returns all API keys for the given tenant (raw key never included).
func (s *APIKeyService) List(
	ctx context.Context,
	tenantID, envID uuid.UUID,
) (*dto.APIKeyListResponse, error) {
	// Verify env ownership.
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.ErrNotFound("environment not found")
	}

	eid := envID
	rows, err := s.store.ListAPIKeysByEnvironment(ctx, db.ListAPIKeysByEnvironmentParams{
		TenantID:      tenantID,
		EnvironmentID: &eid,
	})
	if err != nil {
		return nil, api.ErrInternal(err)
	}

	items := make([]dto.APIKeyItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, dto.APIKeyItem{
			ID:          r.ID,
			Name:        r.Name,
			Description: r.Description,
			KeyPrefix:   r.KeyPrefix,
			Scopes:      r.Scopes,
			IsActive:    r.IsActive,
			LastUsed:    r.LastUsed,
			ExpiresAt:   r.ExpiresAt,
			CreatedAt:   r.CreatedAt,
		})
	}

	return &dto.APIKeyListResponse{
		Keys:  items,
		Total: len(items),
	}, nil
}

// Revoke marks an API key as inactive. The AgentAPIKeyAuth middleware will
// immediately reject it on the next request.
func (s *APIKeyService) Revoke(
	ctx context.Context,
	tenantID, keyID uuid.UUID,
) error {
	key, err := s.store.GetAPIKeyByID(ctx, db.GetAPIKeyByIDParams{
		ID:       keyID,
		TenantID: tenantID,
	})
	if err != nil {
		return api.ErrNotFound("API key not found")
	}

	if !key.IsActive {
		return api.ErrBadRequest("API key is already revoked")
	}

	if err := s.store.RevokeAPIKey(ctx, db.RevokeAPIKeyParams{
		ID:       keyID,
		TenantID: tenantID,
	}); err != nil {
		return api.ErrInternal(err)
	}
	return nil
}
