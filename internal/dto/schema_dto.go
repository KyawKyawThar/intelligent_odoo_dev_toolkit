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
	EnvID      uuid.UUID              `json:"env_id,omitempty"`
	Version    string                 `json:"version" validate:"required"`
	Models     map[string]SchemaModel `json:"models" validate:"required"`
	ModelCount *int32                 `json:"model_count,omitempty"`
	FieldCount *int32                 `json:"field_count,omitempty"`
}

// SchemaModel represents the schema of a single Odoo model, including its
// access rules (ir.model.access) and record rules (ir.rule).
type SchemaModel struct {
	Model    string                      `json:"model"`
	Name     string                      `json:"name"`
	Fields   map[string]SchemaModelField `json:"fields"`
	Accesses []SchemaAccessRule          `json:"accesses,omitempty"`
	Rules    []SchemaRecordRule          `json:"rules,omitempty"`
}

// SchemaModelField represents a single field within an Odoo model.
type SchemaModelField struct {
	Type     string `json:"type"`
	String   string `json:"string"`
	Required bool   `json:"required,omitempty"`
	Default  any    `json:"default,omitempty"`
}

// SchemaAccessRule represents a single ir.model.access rule.
type SchemaAccessRule struct {
	GroupID    string `json:"group_id"`
	PermRead   bool   `json:"perm_read"`
	PermWrite  bool   `json:"perm_write"`
	PermCreate bool   `json:"perm_create"`
	PermUnlink bool   `json:"perm_unlink"`
}

// SchemaRecordRule represents a single ir.rule.
type SchemaRecordRule struct {
	Name       string `json:"name"`
	Domain     string `json:"domain"`
	PermRead   bool   `json:"perm_read"`
	PermWrite  bool   `json:"perm_write"`
	PermCreate bool   `json:"perm_create"`
	PermUnlink bool   `json:"perm_unlink"`
}

// =============================================================================
// Response DTOs
// =============================================================================

// SchemaSnapshotResponse is the full representation of a schema snapshot.
type SchemaSnapshotResponse struct {
	ID         uuid.UUID       `json:"id"`
	EnvID      uuid.UUID       `json:"env_id"`
	CapturedAt time.Time       `json:"captured_at"`
	Version    *string         `json:"version,omitempty"`
	Models     json.RawMessage `json:"models" swaggertype:"object"`
	ModelCount *int32          `json:"model_count,omitempty"`
	FieldCount *int32          `json:"field_count,omitempty"`
	DiffRef    *string         `json:"diff_ref,omitempty"`
}

// SchemaSnapshotListItem is a lightweight snapshot entry used in list responses
// (omits the heavy models JSONB blob).
type SchemaSnapshotListItem struct {
	ID         uuid.UUID `json:"id"`
	EnvID      uuid.UUID `json:"env_id"`
	CapturedAt time.Time `json:"captured_at"`
	Version    *string   `json:"version,omitempty"`
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
		ID:         s.ID,
		EnvID:      s.EnvID,
		CapturedAt: s.CapturedAt,
		Version:    s.Version,
		Models:     s.Models,
		ModelCount: s.ModelCount,
		FieldCount: s.FieldCount,
		DiffRef:    s.DiffRef,
	}
}

func ToSchemaSnapshotListItem(row *db.ListSchemaSnapshotsRow) SchemaSnapshotListItem {
	return SchemaSnapshotListItem{
		ID:         row.ID,
		EnvID:      row.EnvID,
		CapturedAt: row.CapturedAt,
		Version:    row.Version,
		ModelCount: row.ModelCount,
		FieldCount: row.FieldCount,
		DiffRef:    row.DiffRef,
	}
}
