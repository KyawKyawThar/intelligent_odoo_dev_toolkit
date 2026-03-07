package dto

import (
	"encoding/json"
	"time"
)

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

type UpdateTenantRequest struct {
	Name string `json:"name" validate:"required,min=2,max=100"`
}

type UpdateTenantSettingsRequest struct {
	Name     string          `json:"name"     validate:"required,min=2,max=100"`
	Settings json.RawMessage `json:"settings" validate:"required"`
}

type UpdateTenantRetentionRequest struct {
	RetentionConfig json.RawMessage `json:"retention_config" validate:"required"`
}
