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

// StoreSchemaRequest is the payload agents POST to /api/v1/agent/schema.
type StoreSchemaRequest struct {
	EnvID       uuid.UUID       `json:"env_id" validate:"required"`
	Models      json.RawMessage `json:"models" validate:"required" swaggertype:"object"`
	AclRules    json.RawMessage `json:"acl_rules" validate:"required" swaggertype:"object"`
	RecordRules json.RawMessage `json:"record_rules" validate:"required" swaggertype:"object"`
	ModelCount  *int32          `json:"model_count,omitempty"`
	FieldCount  *int32          `json:"field_count,omitempty"`
}

// =============================================================================
// Response DTOs
// =============================================================================

// SchemaSnapshotResponse is the full representation of a schema snapshot.
type SchemaSnapshotResponse struct {
	ID          uuid.UUID       `json:"id"`
	EnvID       uuid.UUID       `json:"env_id"`
	CapturedAt  time.Time       `json:"captured_at"`
	Models      json.RawMessage `json:"models" swaggertype:"object"`
	AclRules    json.RawMessage `json:"acl_rules" swaggertype:"object"`
	RecordRules json.RawMessage `json:"record_rules" swaggertype:"object"`
	ModelCount  *int32          `json:"model_count,omitempty"`
	FieldCount  *int32          `json:"field_count,omitempty"`
	DiffRef     *string         `json:"diff_ref,omitempty"`
}

// SchemaSnapshotListItem is a lightweight snapshot entry used in list responses
// (omits the heavy models/acl_rules/record_rules JSONB blobs).
type SchemaSnapshotListItem struct {
	ID         uuid.UUID `json:"id"`
	EnvID      uuid.UUID `json:"env_id"`
	CapturedAt time.Time `json:"captured_at"`
	ModelCount *int32    `json:"model_count,omitempty"`
	FieldCount *int32    `json:"field_count,omitempty"`
	DiffRef    *string   `json:"diff_ref,omitempty"`
}

// SchemaSnapshotListResponse wraps a paginated list of schema snapshot summaries.
type SchemaSnapshotListResponse struct {
	Snapshots []SchemaSnapshotListItem `json:"snapshots"`
	Total     int64                    `json:"total"`
	Limit     int32                    `json:"limit"`
}

// SearchModelsResponse is the response for the GET /schema/models endpoint.
type SearchModelsResponse struct {
	Models []json.RawMessage `json:"models" swaggertype:"array,object"`
	Total  int               `json:"total"`
	Limit  int32             `json:"limit"`
	Offset int32             `json:"offset"`
}

// =============================================================================
// Mappers
// =============================================================================

func ToSchemaSnapshotResponse(s *db.SchemaSnapshot) *SchemaSnapshotResponse {
	return &SchemaSnapshotResponse{
		ID:          s.ID,
		EnvID:       s.EnvID,
		CapturedAt:  s.CapturedAt,
		Models:      s.Models,
		AclRules:    s.AclRules,
		RecordRules: s.RecordRules,
		ModelCount:  s.ModelCount,
		FieldCount:  s.FieldCount,
		DiffRef:     s.DiffRef,
	}
}

func ToSchemaSnapshotListItem(row *db.ListSchemaSnapshotsRow) SchemaSnapshotListItem {
	return SchemaSnapshotListItem{
		ID:         row.ID,
		EnvID:      row.EnvID,
		CapturedAt: row.CapturedAt,
		ModelCount: row.ModelCount,
		FieldCount: row.FieldCount,
		DiffRef:    row.DiffRef,
	}
}
