package dto

import (
	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"encoding/json"

	"time"

	"github.com/google/uuid"
)

// =============================================================================
// Request DTOs
// =============================================================================

// CreateEnvironmentRequest is the payload for POST /api/v1/environments.
type CreateEnvironmentRequest struct {
	Name        string  `json:"name" validate:"required,min=1,max=100"`
	OdooURL     string  `json:"odoo_url" validate:"required,url"`
	DbName      string  `json:"db_name" validate:"required,min=1,max=63"`
	OdooVersion *string `json:"odoo_version,omitempty" validate:"omitempty,oneof=14.0 15.0 16.0 17.0 18.0"`
	EnvType     string  `json:"env_type" validate:"required,oneof=development staging production"`
	// FeatureFlags holds arbitrary JSON configuration
	// @Description Feature flags as JSON objec
	FeatureFlags json.RawMessage `json:"feature_flags,omitempty" swaggertype:"object"`
}

// UpdateEnvironmentRequest is the payload for PATCH /api/v1/environments/{env_id}.
// All fields are optional — only non-nil fields are updated.
type UpdateEnvironmentRequest struct {
	Name         *string         `json:"name,omitempty" validate:"omitempty,min=1,max=100"`
	OdooURL      *string         `json:"odoo_url,omitempty" validate:"omitempty,url"`
	DbName       *string         `json:"db_name,omitempty" validate:"omitempty,min=1,max=63"`
	OdooVersion  *string         `json:"odoo_version,omitempty" validate:"omitempty,oneof=14.0 15.0 16.0 17.0 18.0"`
	EnvType      *string         `json:"env_type,omitempty" validate:"omitempty,oneof=development staging production"`
	Status       *string         `json:"status,omitempty" validate:"omitempty,oneof=active inactive maintenance"`
	FeatureFlags json.RawMessage `json:"feature_flags,omitempty" swaggertype:"object"`
}

// ListEnvironmentsRequest holds query parameters for GET /api/v1/environments.
type ListEnvironmentsRequest struct {
	EnvType string `json:"env_type" validate:"omitempty,oneof=development staging production"`
	Status  string `json:"status" validate:"omitempty,oneof=active inactive maintenance"`
	Limit   int32  `json:"limit" validate:"omitempty,min=1,max=100"`
	Offset  int32  `json:"offset" validate:"omitempty,min=0"`
}

// =============================================================================
// Response DTOs
// =============================================================================

// EnvironmentResponse is the public representation of an environment.
type EnvironmentResponse struct {
	ID           uuid.UUID       `json:"id"`
	TenantID     uuid.UUID       `json:"tenant_id"`
	Name         string          `json:"name"`
	OdooURL      string          `json:"odoo_url"`
	DbName       string          `json:"db_name"`
	OdooVersion  *string         `json:"odoo_version,omitempty"`
	EnvType      string          `json:"env_type"`
	Status       string          `json:"status"`
	AgentID      *string         `json:"agent_id,omitempty"`
	FeatureFlags json.RawMessage `json:"feature_flags,omitempty" swaggertype:"object"`
	LastSync     *time.Time      `json:"last_sync,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// EnvironmentListResponse wraps a paginated list of environments.
type EnvironmentListResponse struct {
	Environments []EnvironmentResponse `json:"environments"`
	Total        int64                 `json:"total"`
	Limit        int32                 `json:"limit"`
	Offset       int32                 `json:"offset"`
}

func ToEnvironmentResponse(env *db.Environment) *EnvironmentResponse {
	return &EnvironmentResponse{
		ID:           env.ID,
		TenantID:     env.TenantID,
		Name:         env.Name,
		OdooURL:      env.OdooUrl,
		DbName:       env.DbName,
		OdooVersion:  env.OdooVersion,
		EnvType:      env.EnvType,
		Status:       env.Status,
		AgentID:      env.AgentID,
		FeatureFlags: env.FeatureFlags,
		LastSync:     env.LastSync,
		CreatedAt:    env.CreatedAt,
		UpdatedAt:    env.UpdatedAt,
	}
}
