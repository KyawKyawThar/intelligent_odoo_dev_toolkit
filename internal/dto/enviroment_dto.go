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
	// @example {"enable_profiling": true, "log_level": "info"}
	FeatureFlags json.RawMessage `json:"feature_flags,omitempty" swaggertype:"object"`
}

// UpdateEnvironmentRequest is the payload for PATCH /api/v1/environments/{env_id}.
// All fields are optional — only non-nil fields are updated.
type UpdateEnvironmentRequest struct {
	Name        *string `json:"name,omitempty" validate:"omitempty,min=1,max=100"`
	OdooURL     *string `json:"odoo_url,omitempty" validate:"omitempty,url"`
	DbName      *string `json:"db_name,omitempty" validate:"omitempty,min=1,max=63"`
	OdooVersion *string `json:"odoo_version,omitempty" validate:"omitempty,oneof=14.0 15.0 16.0 17.0 18.0"`
	EnvType     *string `json:"env_type,omitempty" validate:"omitempty,oneof=development staging production"`
	Status      *string `json:"status,omitempty" validate:"omitempty,oneof=active inactive maintenance"`
	// @example {"enable_profiling": true, "log_level": "info"}
	FeatureFlags json.RawMessage `json:"feature_flags,omitempty" swaggertype:"object"`
}

// RegisterAgentRequest is the payload for POST /api/v1/environments/{env_id}/agent.
// No body required — the server generates a one-time registration token.
type RegisterAgentRequest struct{}

// RegisterAgentResponse is returned when a registration token is generated.
type RegisterAgentResponse struct {
	RegistrationToken string    `json:"registration_token"`
	ExpiresAt         time.Time `json:"expires_at"`
}

// AgentSelfRegisterRequest is the payload for POST /api/v1/agent/register.
// The agent sends its one-time registration token to obtain credentials.
type AgentSelfRegisterRequest struct {
	RegistrationToken string `json:"registration_token" validate:"required"`
}

// AgentSelfRegisterResponse is returned after successful self-registration.
type AgentSelfRegisterResponse struct {
	AgentID       string `json:"agent_id"`
	APIKey        string `json:"api_key"`
	EnvironmentID string `json:"environment_id"`
	TenantID      string `json:"tenant_id"`
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

// UpdateFlagsRequest is the payload for PUT /api/v1/environments/{env_id}/flags.
type UpdateFlagsRequest struct {
	// Flags holds the complete feature-flag object to store.
	// @Description Feature flags as JSON object
	// @example {"sampling_mode":"sampled","sample_rate":0.25}
	Flags json.RawMessage `json:"flags" validate:"required" swaggertype:"object"`
}

// UpdateFlagsResponse is returned after updating feature flags.
type UpdateFlagsResponse struct {
	ID             uuid.UUID       `json:"id"`
	FeatureFlags   json.RawMessage `json:"feature_flags" swaggertype:"object"`
	AgentConnected bool            `json:"agent_connected"`
	Pushed         bool            `json:"pushed"`
}

// HeartbeatResponse is the public representation of an agent heartbeat.
type HeartbeatResponse struct {
	ID           uuid.UUID       `json:"id"`
	EnvID        uuid.UUID       `json:"env_id"`
	AgentID      string          `json:"agent_id"`
	AgentVersion *string         `json:"agent_version,omitempty"`
	Status       string          `json:"status"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
	ReceivedAt   time.Time       `json:"received_at"`
}

// HeartbeatListResponse wraps a list of heartbeats with the latest one highlighted.
type HeartbeatListResponse struct {
	Heartbeats []HeartbeatResponse `json:"heartbeats"`
	Total      int                 `json:"total"`
}

func ToHeartbeatResponse(hb *db.AgentHeartbeat) *HeartbeatResponse {
	return &HeartbeatResponse{
		ID:           hb.ID,
		EnvID:        hb.EnvID,
		AgentID:      hb.AgentID,
		AgentVersion: hb.AgentVersion,
		Status:       hb.Status,
		Metadata:     hb.Metadata,
		ReceivedAt:   hb.ReceivedAt,
	}
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
