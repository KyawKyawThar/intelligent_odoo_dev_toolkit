package dto

import (
	"time"

	"github.com/google/uuid"
)

// ─── Request DTOs ────────────────────────────────────────────────────────────

// ListAlertsRequest holds query parameters for the list alerts endpoint.
type ListAlertsRequest struct {
	// Limit is the maximum number of alerts to return (default 50, max 200).
	Limit int32
	// Offset is the number of alerts to skip for pagination.
	Offset int32
	// Unacknowledged filters to only unacknowledged alerts when true.
	Unacknowledged bool
}

// AcknowledgeAlertRequest is the body for acknowledging a single alert.
// (empty body — user ID comes from JWT context)
type AcknowledgeAlertRequest struct{}

// ─── Response DTOs ───────────────────────────────────────────────────────────

// AlertResponse represents a single alert.
type AlertResponse struct {
	ID             uuid.UUID  `json:"id"`
	EnvID          uuid.UUID  `json:"env_id"`
	Type           string     `json:"type"`
	Severity       string     `json:"severity"`
	Message        string     `json:"message"`
	Metadata       any        `json:"metadata,omitempty"`
	Acknowledged   bool       `json:"acknowledged"`
	AcknowledgedBy *uuid.UUID `json:"acknowledged_by,omitempty"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// AlertListResponse is the paginated response for listing alerts.
type AlertListResponse struct {
	Alerts []AlertResponse `json:"alerts"`
	Total  int64           `json:"total"`
}

// AlertCountResponse is returned by the unacknowledged count endpoint.
type AlertCountResponse struct {
	Count int64 `json:"count"`
}

// AcknowledgeAllResponse reports how many alerts were acknowledged.
type AcknowledgeAllResponse struct {
	Acknowledged int64 `json:"acknowledged"`
}
