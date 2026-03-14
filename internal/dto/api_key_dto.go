package dto

import (
	"time"

	"github.com/google/uuid"
)

// CreateAPIKeyRequest is the request body for POST /environments/{env_id}/api-keys.
type CreateAPIKeyRequest struct {
	Name        string   `json:"name"        validate:"required,min=1,max=100"`
	Description string   `json:"description" validate:"omitempty,max=255"`
	Scopes      []string `json:"scopes"      validate:"omitempty"`
	ExpiresAt   *string  `json:"expires_at"  validate:"omitempty,datetime=2006-01-02T15:04:05Z07:00"` // RFC3339; defaults to 90 days from now if omitted
}

// APIKeyCreatedResponse is returned once on key creation — the raw key is never shown again.
type APIKeyCreatedResponse struct {
	ID          uuid.UUID  `json:"id"`
	TenantID    uuid.UUID  `json:"tenant_id"`
	EnvID       uuid.UUID  `json:"env_id"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	KeyPrefix   string     `json:"key_prefix"`
	RawKey      string     `json:"raw_key"` // shown once; not stored in plaintext
	Scopes      []string   `json:"scopes"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// APIKeyItem is a single row in the list response (raw key omitted).
type APIKeyItem struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	KeyPrefix   string     `json:"key_prefix"`
	Scopes      []string   `json:"scopes"`
	IsActive    bool       `json:"is_active"`
	LastUsed    *time.Time `json:"last_used,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// APIKeyListResponse wraps a slice of APIKeyItem.
type APIKeyListResponse struct {
	Keys  []APIKeyItem `json:"keys"`
	Total int          `json:"total"`
}
