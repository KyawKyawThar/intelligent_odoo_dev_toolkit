package dto

import "time"

// ErrorEventPayload is a single error occurrence sent by the agent.
type ErrorEventPayload struct {
	Signature   string    `json:"signature" validate:"required"`
	ErrorType   string    `json:"error_type" validate:"required"`
	Message     string    `json:"message" validate:"required"`
	Module      string    `json:"module,omitempty"`
	Model       string    `json:"model,omitempty"`
	Traceback   string    `json:"traceback,omitempty"`
	AffectedUID int32     `json:"affected_uid,omitempty"`
	CapturedAt  time.Time `json:"captured_at" validate:"required"`
}

// IngestErrorsRequest is the body for POST /api/v1/agent/errors.
type IngestErrorsRequest struct {
	EnvID          string              `json:"env_id" validate:"required,uuid"`
	Events         []ErrorEventPayload `json:"events" validate:"required,min=1,dive"`
	SpikeThreshold int                 `json:"spike_threshold,omitempty"`
}
