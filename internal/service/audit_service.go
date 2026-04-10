package service

import (
	"context"
	"time"

	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"

	"github.com/google/uuid"
)

// AuditService implements AuditServicer.
type AuditService struct {
	store db.Store
}

// NewAuditService creates a new AuditService.
func NewAuditService(store db.Store) *AuditService {
	return &AuditService{store: store}
}

// List returns a paginated list of audit logs for the tenant.
func (s *AuditService) List(
	ctx context.Context,
	tenantID uuid.UUID,
	limit, offset int32,
) (*dto.AuditLogListResponse, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	rows, err := s.store.ListAuditLogs(ctx, db.ListAuditLogsParams{
		TenantID: tenantID,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		return nil, err
	}

	total, err := s.store.CountAuditLogs(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	return &dto.AuditLogListResponse{
		Items: dto.MapAuditLogs(rows),
		Total: total,
		Limit: int(limit),
	}, nil
}

// ListByAction returns audit logs filtered to a specific action string.
func (s *AuditService) ListByAction(
	ctx context.Context,
	tenantID uuid.UUID,
	action string,
	limit, offset int32,
) (*dto.AuditLogListResponse, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	rows, err := s.store.ListAuditLogsByAction(ctx, db.ListAuditLogsByActionParams{
		TenantID: tenantID,
		Action:   action,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		return nil, err
	}

	return &dto.AuditLogListResponse{
		Items: dto.MapAuditLogs(rows),
		Total: int64(len(rows)),
		Limit: int(limit),
	}, nil
}

// ListByUser returns audit logs for a specific user within the tenant.
func (s *AuditService) ListByUser(
	ctx context.Context,
	tenantID, userID uuid.UUID,
	limit, offset int32,
) (*dto.AuditLogListResponse, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	rows, err := s.store.ListAuditLogsByUser(ctx, db.ListAuditLogsByUserParams{
		TenantID: tenantID,
		UserID:   &userID,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		return nil, err
	}

	return &dto.AuditLogListResponse{
		Items: dto.MapAuditLogs(rows),
		Total: int64(len(rows)),
		Limit: int(limit),
	}, nil
}

// ListByResource returns audit logs for a specific resource type + ID.
func (s *AuditService) ListByResource(
	ctx context.Context,
	tenantID uuid.UUID,
	resource, resourceID string,
	limit int32,
) (*dto.AuditLogListResponse, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	rows, err := s.store.ListAuditLogsByResource(ctx, db.ListAuditLogsByResourceParams{
		TenantID:   tenantID,
		Resource:   &resource,
		ResourceID: &resourceID,
		Limit:      limit,
	})
	if err != nil {
		return nil, err
	}

	return &dto.AuditLogListResponse{
		Items: dto.MapAuditLogs(rows),
		Total: int64(len(rows)),
		Limit: int(limit),
	}, nil
}

// ListBetween returns audit logs within a time range.
func (s *AuditService) ListBetween(
	ctx context.Context,
	tenantID uuid.UUID,
	from, to time.Time,
	limit, offset int32,
) (*dto.AuditLogListResponse, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	rows, err := s.store.ListAuditLogsBetween(ctx, db.ListAuditLogsBetweenParams{
		TenantID:    tenantID,
		CreatedAt:   from,
		CreatedAt_2: to,
		Limit:       limit,
		Offset:      offset,
	})
	if err != nil {
		return nil, err
	}

	return &dto.AuditLogListResponse{
		Items: dto.MapAuditLogs(rows),
		Total: int64(len(rows)),
		Limit: int(limit),
	}, nil
}
