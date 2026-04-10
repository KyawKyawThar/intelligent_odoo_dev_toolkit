package service

import (
	"context"

	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"

	"github.com/google/uuid"
)

// NotificationService manages notification channel CRUD.
type NotificationService struct {
	store db.Store
}

// NewNotificationService creates a NotificationService.
func NewNotificationService(store db.Store) *NotificationService {
	return &NotificationService{store: store}
}

// Create adds a new notification channel for the tenant.
func (s *NotificationService) Create(
	ctx context.Context,
	tenantID uuid.UUID,
	req *dto.CreateNotificationChannelRequest,
) (*dto.NotificationChannelResponse, error) {
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	ch, err := s.store.CreateNotificationChannel(ctx, db.CreateNotificationChannelParams{
		TenantID: tenantID,
		Name:     req.Name,
		Type:     req.Type,
		Config:   req.Config,
		IsActive: isActive,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	resp := dto.MapNotificationChannel(ch)
	return &resp, nil
}

// List returns all notification channels for the tenant.
func (s *NotificationService) List(
	ctx context.Context,
	tenantID uuid.UUID,
) (*dto.NotificationChannelListResponse, error) {
	rows, err := s.store.ListNotificationChannels(ctx, tenantID)
	if err != nil {
		return nil, api.FromPgError(err)
	}

	return &dto.NotificationChannelListResponse{
		Channels: dto.MapNotificationChannels(rows),
		Total:    len(rows),
	}, nil
}

// Get returns a single notification channel by ID, scoped to the tenant.
func (s *NotificationService) Get(
	ctx context.Context,
	tenantID, channelID uuid.UUID,
) (*dto.NotificationChannelResponse, error) {
	ch, err := s.store.GetNotificationChannel(ctx, db.GetNotificationChannelParams{
		ID:       channelID,
		TenantID: tenantID,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	resp := dto.MapNotificationChannel(ch)
	return &resp, nil
}

// Update replaces a notification channel's fields.
func (s *NotificationService) Update(
	ctx context.Context,
	tenantID, channelID uuid.UUID,
	req *dto.UpdateNotificationChannelRequest,
) (*dto.NotificationChannelResponse, error) {
	ch, err := s.store.UpdateNotificationChannel(ctx, db.UpdateNotificationChannelParams{
		ID:       channelID,
		TenantID: tenantID,
		Name:     req.Name,
		Type:     req.Type,
		Config:   req.Config,
		IsActive: req.IsActive,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	resp := dto.MapNotificationChannel(ch)
	return &resp, nil
}

// Delete removes a notification channel.
func (s *NotificationService) Delete(
	ctx context.Context,
	tenantID, channelID uuid.UUID,
) error {
	if err := s.store.DeleteNotificationChannel(ctx, db.DeleteNotificationChannelParams{
		ID:       channelID,
		TenantID: tenantID,
	}); err != nil {
		return api.FromPgError(err)
	}
	return nil
}
