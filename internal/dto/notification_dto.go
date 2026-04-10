package dto

import (
	"encoding/json"
	"time"

	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"

	"github.com/google/uuid"
)

// NotificationChannelResponse is the JSON representation of a notification channel.
type NotificationChannelResponse struct {
	ID        uuid.UUID       `json:"id"`
	TenantID  uuid.UUID       `json:"tenant_id"`
	Name      string          `json:"name"`
	Type      string          `json:"type"`
	Config    json.RawMessage `json:"config"`
	IsActive  bool            `json:"is_active"`
	CreatedAt time.Time       `json:"created_at"`
}

// NotificationChannelListResponse wraps a slice of channels.
type NotificationChannelListResponse struct {
	Channels []NotificationChannelResponse `json:"channels"`
	Total    int                           `json:"total"`
}

// CreateNotificationChannelRequest is the request body for creating a channel.
type CreateNotificationChannelRequest struct {
	Name     string          `json:"name"     validate:"required,min=1,max=100"`
	Type     string          `json:"type"     validate:"required,oneof=slack webhook email"`
	Config   json.RawMessage `json:"config"   validate:"required"`
	IsActive *bool           `json:"is_active"` // defaults to true
}

// UpdateNotificationChannelRequest is the request body for updating a channel.
type UpdateNotificationChannelRequest struct {
	Name     string          `json:"name"      validate:"required,min=1,max=100"`
	Type     string          `json:"type"      validate:"required,oneof=slack webhook email"`
	Config   json.RawMessage `json:"config"    validate:"required"`
	IsActive bool            `json:"is_active"`
}

// MapNotificationChannel converts a db.NotificationChannel to the DTO.
func MapNotificationChannel(ch db.NotificationChannel) NotificationChannelResponse {
	return NotificationChannelResponse{
		ID:        ch.ID,
		TenantID:  ch.TenantID,
		Name:      ch.Name,
		Type:      ch.Type,
		Config:    ch.Config,
		IsActive:  ch.IsActive,
		CreatedAt: ch.CreatedAt,
	}
}

// MapNotificationChannels maps a slice of db rows.
func MapNotificationChannels(rows []db.NotificationChannel) []NotificationChannelResponse {
	out := make([]NotificationChannelResponse, len(rows))
	for i, r := range rows {
		out[i] = MapNotificationChannel(r)
	}
	return out
}
