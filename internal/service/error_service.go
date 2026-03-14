package service

import (
	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"
	"context"

	"github.com/google/uuid"
)

// ErrorService handles agent error ingestion.
type ErrorService struct {
	store db.Store
}

func NewErrorService(store db.Store) *ErrorService {
	return &ErrorService{store: store}
}

// IngestBatch persists a batch of error events sent by an agent.
// Each event is upserted by (env_id, signature); occurrence counts are
// incremented server-side and spike alerts are raised when the threshold is hit.
func (s *ErrorService) IngestBatch(ctx context.Context, tenantID uuid.UUID, req *dto.IngestErrorsRequest) error {
	envID, err := uuid.Parse(req.EnvID)
	if err != nil {
		return api.ErrBadRequest("env_id must be a valid UUID")
	}

	// Verify env belongs to the authenticated tenant.
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return api.FromPgError(err)
	}

	for i := range req.Events {
		ev := &req.Events[i]

		var module *string
		if ev.Module != "" {
			module = &ev.Module
		}
		var model *string
		if ev.Model != "" {
			model = &ev.Model
		}
		var traceRef *string
		if ev.Traceback != "" {
			traceRef = &ev.Traceback
		}

		var affectedUIDs []int32
		if ev.AffectedUID != 0 {
			affectedUIDs = []int32{ev.AffectedUID}
		}

		if err := s.store.IngestErrorBatchTx(ctx, db.IngestErrorBatchParams{
			EnvID:          envID,
			TenantID:       tenantID,
			Signature:      ev.Signature,
			ErrorType:      ev.ErrorType,
			Message:        ev.Message,
			Module:         module,
			Model:          model,
			Timestamp:      ev.CapturedAt,
			AffectedUIDs:   affectedUIDs,
			RawTraceRef:    traceRef,
			SpikeThreshold: req.SpikeThreshold,
		}); err != nil {
			return api.FromPgError(err)
		}
	}

	return nil
}
