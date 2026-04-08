package service

import (
	"context"
	"encoding/json"

	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"

	"github.com/google/uuid"
)

// AlertService implements AlertServicer.
type AlertService struct {
	store db.Store
}

// NewAlertService creates a new AlertService.
func NewAlertService(store db.Store) *AlertService {
	return &AlertService{store: store}
}

// List returns alerts for an environment with optional unacknowledged filtering.
func (s *AlertService) List(
	ctx context.Context,
	tenantID, envID uuid.UUID,
	req *dto.ListAlertsRequest,
) (*dto.AlertListResponse, error) {
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	limit := req.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	total, err := s.store.CountUnacknowledgedAlerts(ctx, envID)
	if err != nil {
		return nil, api.FromPgError(err)
	}

	var rows []db.Alert

	if req.Unacknowledged {
		rows, err = s.store.ListUnacknowledgedAlerts(ctx, db.ListUnacknowledgedAlertsParams{
			EnvID: envID,
			Limit: limit,
		})
	} else {
		rows, err = s.store.ListAlerts(ctx, db.ListAlertsParams{
			EnvID:  envID,
			Limit:  limit,
			Offset: offset,
		})
	}
	if err != nil {
		return nil, api.FromPgError(err)
	}

	alerts := make([]dto.AlertResponse, 0, len(rows))
	for _, r := range rows {
		alerts = append(alerts, toAlertResponse(r))
	}

	return &dto.AlertListResponse{
		Alerts: alerts,
		Total:  total,
	}, nil
}

// GetByID returns a single alert scoped to an environment.
func (s *AlertService) GetByID(
	ctx context.Context,
	tenantID, envID, alertID uuid.UUID,
) (*dto.AlertResponse, error) {
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	row, err := s.store.GetAlertByID(ctx, db.GetAlertByIDParams{
		ID:    alertID,
		EnvID: envID,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	resp := toAlertResponse(row)
	return &resp, nil
}

// Acknowledge marks a single alert as acknowledged by the given user.
func (s *AlertService) Acknowledge(
	ctx context.Context,
	tenantID, envID, alertID, userID uuid.UUID,
) (*dto.AlertResponse, error) {
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	row, err := s.store.AcknowledgeAlert(ctx, db.AcknowledgeAlertParams{
		ID:             alertID,
		AcknowledgedBy: &userID,
		EnvID:          envID,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	resp := toAlertResponse(row)
	return &resp, nil
}

// AcknowledgeAll marks every unacknowledged alert in the environment as acknowledged.
func (s *AlertService) AcknowledgeAll(
	ctx context.Context,
	tenantID, envID, userID uuid.UUID,
) (*dto.AcknowledgeAllResponse, error) {
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	tag, err := s.store.AcknowledgeAllAlerts(ctx, db.AcknowledgeAllAlertsParams{
		EnvID:          envID,
		AcknowledgedBy: &userID,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	return &dto.AcknowledgeAllResponse{Acknowledged: tag.RowsAffected()}, nil
}

// CountUnacknowledged returns the number of unacknowledged alerts for an environment.
func (s *AlertService) CountUnacknowledged(
	ctx context.Context,
	tenantID, envID uuid.UUID,
) (*dto.AlertCountResponse, error) {
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	count, err := s.store.CountUnacknowledgedAlerts(ctx, envID)
	if err != nil {
		return nil, api.FromPgError(err)
	}

	return &dto.AlertCountResponse{Count: count}, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func toAlertResponse(a db.Alert) dto.AlertResponse {
	var meta any
	if len(a.Metadata) > 0 {
		if err := json.Unmarshal(a.Metadata, &meta); err != nil {
			meta = nil // ignore invalid json
		}
	}
	return dto.AlertResponse{
		ID:             a.ID,
		EnvID:          a.EnvID,
		Type:           a.Type,
		Severity:       a.Severity,
		Message:        a.Message,
		Metadata:       meta,
		Acknowledged:   a.Acknowledged,
		AcknowledgedBy: a.AcknowledgedBy,
		AcknowledgedAt: a.AcknowledgedAt,
		CreatedAt:      a.CreatedAt,
	}
}
