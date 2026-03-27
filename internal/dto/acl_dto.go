package dto

import "Intelligent_Dev_ToolKit_Odoo/internal/acl"

// =============================================================================
// Request DTOs
// =============================================================================

// ACLTraceRequest is the body for POST /api/v1/environments/{env_id}/acl/trace.
// It asks the pipeline: "why can/can't user X perform operation Y on model Z (record R)?"
type ACLTraceRequest struct {
	// UserID is the Odoo uid to trace (e.g. 2 for admin).
	UserID int `json:"user_id" validate:"required,min=1"`
	// Model is the Odoo model technical name (e.g. "sale.order").
	Model string `json:"model" validate:"required"`
	// Operation is the CRUD operation to check: read, write, create, unlink.
	Operation string `json:"operation" validate:"required,oneof=read write create unlink"`
	// RecordID is the optional database ID of a specific record to evaluate
	// domain conditions against. If 0, domain evaluation is skipped.
	RecordID int `json:"record_id,omitempty"`
	// UserData is the raw res.users record as fetched from Odoo by the agent.
	// Must contain at least: id, login, name, active, groups_id, company_id.
	UserData map[string]any `json:"user_data" validate:"required"`
	// GroupData is all res.groups records from the agent. Each must have: id, name, implied_ids.
	GroupData []map[string]any `json:"group_data" validate:"required"`
	// RecordData is the optional field values of the target record (for domain eval).
	// Required when record_id > 0. Keys are field names, values are JSON-decoded Odoo values.
	RecordData map[string]any `json:"record_data,omitempty"`
}

// =============================================================================
// Response DTOs
// =============================================================================

// ACLTraceResponse is the full 5-stage pipeline result.
type ACLTraceResponse struct {
	Verdict     string            `json:"verdict"` // "ALLOWED" or "DENIED"
	Stages      []acl.StageResult `json:"stages"`
	Suggestions []acl.Suggestion  `json:"suggestions,omitempty"` // actionable fix suggestions (only when denied)
}
