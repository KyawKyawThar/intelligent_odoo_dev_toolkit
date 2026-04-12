package dto

import (
	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// AuditLogResponse is the JSON shape returned for a single audit log entry.
type AuditLogResponse struct {
	ID         uuid.UUID        `json:"id"`
	TenantID   uuid.UUID        `json:"tenant_id"`
	UserID     *uuid.UUID       `json:"user_id,omitempty"`
	IPAddress  *string          `json:"ip_address,omitempty"`
	Action     string           `json:"action"`
	Resource   *string          `json:"resource,omitempty"`
	ResourceID *string          `json:"resource_id,omitempty"`
	StatusCode *int             `json:"status_code,omitempty"` // extracted from metadata
	Before     *json.RawMessage `json:"before,omitempty"`
	After      *json.RawMessage `json:"after,omitempty"`
	Metadata   json.RawMessage  `json:"metadata,omitempty"`
	CreatedAt  time.Time        `json:"created_at"`
}

// AuditLogListResponse wraps a paginated list of audit log entries.
type AuditLogListResponse struct {
	Items []AuditLogResponse `json:"items"`
	Total int64              `json:"total"`
	Limit int                `json:"limit"`
}

// MapAuditLog converts a db.AuditLog to an AuditLogResponse.
func MapAuditLog(row db.AuditLog) AuditLogResponse {
	resp := AuditLogResponse{
		ID:         row.ID,
		TenantID:   row.TenantID,
		UserID:     row.UserID,
		Action:     row.Action,
		Resource:   row.Resource,
		ResourceID: row.ResourceID,
		Before:     row.Before,
		After:      row.After,
		Metadata:   row.Metadata,
		CreatedAt:  row.CreatedAt,
	}
	if row.IpAddress != nil {
		s := row.IpAddress.String()
		resp.IPAddress = &s
	}
	// Extract status_code from the metadata JSONB so the frontend gets it as a
	// top-level field instead of having to parse raw JSON.
	if len(row.Metadata) > 0 {
		var meta map[string]json.RawMessage
		if err := json.Unmarshal(row.Metadata, &meta); err == nil {
			if raw, ok := meta["status_code"]; ok {
				var code int
				if json.Unmarshal(raw, &code) == nil {
					resp.StatusCode = &code
				}
			}
		}
	}
	return resp
}

// MapAuditLogs converts a slice of db.AuditLog rows.
func MapAuditLogs(rows []db.AuditLog) []AuditLogResponse {
	out := make([]AuditLogResponse, len(rows))
	for i, r := range rows {
		out[i] = MapAuditLog(r)
	}
	return out
}
