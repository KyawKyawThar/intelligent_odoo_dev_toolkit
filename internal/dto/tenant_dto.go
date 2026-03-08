package dto

import (
	"encoding/json"
	"time"
)

// TenantResponse is the public representation of a tenant returned by the API.
type TenantResponse struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Slug            string          `json:"slug"`
	Plan            string          `json:"plan"`
	PlanStatus      string          `json:"plan_status"`
	TrialEndsAt     *time.Time      `json:"trial_ends_at,omitempty"`
	Settings        json.RawMessage `json:"settings,omitempty"`
	RetentionConfig json.RawMessage `json:"retention_config,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// TenantListItem is a trimmed tenant representation used in list endpoints.
type TenantListItem struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Slug        string     `json:"slug"`
	Plan        string     `json:"plan"`
	PlanStatus  string     `json:"plan_status"`
	UserCount   int64      `json:"user_count,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	TrialEndsAt *time.Time `json:"trial_ends_at,omitempty"`
}

// UpdateTenantRequest carries the fields for updating a tenant's core info
type UpdateTenantRequest struct {
	Name string `json:"name" validate:"required,min=2,max=100"`
}

// UpdateTenantSettingsRequest carries a tenant name and arbitrary settings JSON.
type UpdateTenantSettingsRequest struct {
	Name     string          `json:"name"     validate:"required,min=2,max=100"`
	Settings json.RawMessage `json:"settings" validate:"required"`
}

// UpdateTenantRetentionRequest carries the retention configuration to apply to a tenant.
type UpdateTenantRetentionRequest struct {
	RetentionConfig json.RawMessage `json:"retention_config" validate:"required"`
}
