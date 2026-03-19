package dto

import (
	"time"

	"github.com/google/uuid"
)

// ErrorEventContext holds request-scoped metadata captured at the time of the error.
type ErrorEventContext struct {
	UID        int32  `json:"uid,omitempty"`
	RequestURL string `json:"request_url,omitempty"`
}

// ErrorEventPayload is a single error occurrence sent by the agent.
type ErrorEventPayload struct {
	Signature string             `json:"signature" validate:"required"`
	Type      string             `json:"type" validate:"required"`
	Message   string             `json:"message" validate:"required"`
	Module    string             `json:"module,omitempty"`
	Model     string             `json:"model,omitempty"`
	Timestamp time.Time          `json:"timestamp" validate:"required"`
	Traceback string             `json:"traceback,omitempty"`
	Context   *ErrorEventContext `json:"context,omitempty"`
}

// IngestErrorsRequest is the body for POST /api/v1/agent/errors.
type IngestErrorsRequest struct {
	EnvID          string              `json:"env_id,omitempty" validate:"omitempty,uuid"`
	Events         []ErrorEventPayload `json:"events" validate:"required,min=1,dive"`
	SpikeThreshold int                 `json:"spike_threshold,omitempty"`
}

// ─── Response DTOs ──────────────────────────────────────────────────────────

// ErrorGroupResponse is the API representation of a single error group.
type ErrorGroupResponse struct {
	ID              uuid.UUID  `json:"id"`
	EnvID           uuid.UUID  `json:"env_id"`
	Signature       string     `json:"signature"`
	ErrorType       string     `json:"error_type"`
	Message         string     `json:"message"`
	Module          *string    `json:"module,omitempty"`
	Model           *string    `json:"model,omitempty"`
	FirstSeen       time.Time  `json:"first_seen"`
	LastSeen        time.Time  `json:"last_seen"`
	OccurrenceCount int32      `json:"occurrence_count"`
	AffectedUsers   []int32    `json:"affected_users"`
	Status          string     `json:"status"`
	ResolvedBy      *uuid.UUID `json:"resolved_by,omitempty"`
	ResolvedAt      *time.Time `json:"resolved_at,omitempty"`
	RawTraceRef     *string    `json:"raw_trace_ref,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

// ErrorGroupListResponse wraps a paginated list of error groups.
type ErrorGroupListResponse struct {
	Errors     []ErrorGroupResponse `json:"errors"`
	Pagination *Meta                `json:"pagination,omitempty"`
}

// ErrorGroupDetailResponse includes the error group plus optionally the
// raw traceback fetched from S3.
type ErrorGroupDetailResponse struct {
	ErrorGroupResponse
	RawTrace *string `json:"raw_trace,omitempty"`
}

// ListErrorGroupsRequest holds query parameters for listing error groups.
type ListErrorGroupsRequest struct {
	Status    string `json:"status,omitempty"` // open | acknowledged | resolved
	ErrorType string `json:"error_type,omitempty"`
	Search    string `json:"search,omitempty"` // free-text search across message/module/model
	Page      int    `json:"page,omitempty"`
	PerPage   int    `json:"per_page,omitempty"`
}
