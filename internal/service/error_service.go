package service

import (
	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"
	"Intelligent_Dev_ToolKit_Odoo/internal/storage"
	"context"

	"github.com/google/uuid"
)

// ErrorService handles agent error ingestion and error group queries.
type ErrorService struct {
	store db.Store
	s3    *storage.S3Client // nil = raw trace fetching disabled
}

func NewErrorService(store db.Store) *ErrorService {
	return &ErrorService{store: store}
}

// SetS3Client attaches an S3 client for fetching raw tracebacks.
func (s *ErrorService) SetS3Client(s3 *storage.S3Client) {
	s.s3 = s3
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
		if ev.Context != nil && ev.Context.UID != 0 {
			affectedUIDs = []int32{ev.Context.UID}
		}

		if err := s.store.IngestErrorBatchTx(ctx, db.IngestErrorBatchParams{
			EnvID:          envID,
			TenantID:       tenantID,
			Signature:      ev.Signature,
			ErrorType:      ev.Type,
			Message:        ev.Message,
			Module:         module,
			Model:          model,
			Timestamp:      ev.Timestamp,
			AffectedUIDs:   affectedUIDs,
			RawTraceRef:    traceRef,
			SpikeThreshold: req.SpikeThreshold,
		}); err != nil {
			return api.FromPgError(err)
		}
	}

	return nil
}

// ListErrorGroups returns a paginated list of error groups for an environment.
// Supports filtering by status, error_type, and free-text search.
func (s *ErrorService) ListErrorGroups(
	ctx context.Context,
	tenantID, envID uuid.UUID,
	req *dto.ListErrorGroupsRequest,
) (*dto.ErrorGroupListResponse, error) {
	// Verify env belongs to tenant.
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	pg := dto.NewPagination(req.Page, req.PerPage)
	limit := int32(pg.PerPage)   //nolint:gosec // PerPage is bounded by validation, safe to convert
	offset := int32(pg.Offset()) //nolint:gosec // Offset derived from bounded PerPage and Page

	var rows []db.ErrorGroup
	var err error

	switch {
	case req.Search != "":
		rows, err = s.store.SearchErrorGroups(ctx, db.SearchErrorGroupsParams{
			EnvID:   envID,
			Column2: &req.Search,
			Limit:   limit,
			Offset:  offset,
		})
	case req.Status != "":
		rows, err = s.store.ListErrorGroupsByStatus(ctx, db.ListErrorGroupsByStatusParams{
			EnvID:  envID,
			Status: req.Status,
			Limit:  limit,
			Offset: offset,
		})
	case req.ErrorType != "":
		rows, err = s.store.ListErrorGroupsByType(ctx, db.ListErrorGroupsByTypeParams{
			EnvID:     envID,
			ErrorType: req.ErrorType,
			Limit:     limit,
			Offset:    offset,
		})
	default:
		rows, err = s.store.ListErrorGroups(ctx, db.ListErrorGroupsParams{
			EnvID:  envID,
			Limit:  limit,
			Offset: offset,
		})
	}

	if err != nil {
		return nil, api.FromPgError(err)
	}

	// Get total count for pagination.
	total, err := s.store.CountErrorGroupsByEnv(ctx, envID)
	if err != nil {
		return nil, api.FromPgError(err)
	}
	pg.Total = total

	resp := &dto.ErrorGroupListResponse{
		Errors:     make([]dto.ErrorGroupResponse, len(rows)),
		Pagination: pg.ToMeta(),
	}
	for i, row := range rows {
		resp.Errors[i] = toErrorGroupResponse(row)
	}

	return resp, nil
}

// GetErrorGroup returns a single error group by its ID. Optionally fetches
// the raw traceback from S3 when include_trace is true.
func (s *ErrorService) GetErrorGroup(
	ctx context.Context,
	tenantID, envID, errorID uuid.UUID,
	includeTrace bool,
) (*dto.ErrorGroupDetailResponse, error) {
	// Verify env belongs to tenant.
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	eg, err := s.store.GetErrorGroupByID(ctx, db.GetErrorGroupByIDParams{
		ID:    errorID,
		EnvID: envID,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	resp := &dto.ErrorGroupDetailResponse{
		ErrorGroupResponse: toErrorGroupResponse(eg),
	}

	// Fetch raw traceback from S3 if requested and ref exists.
	if includeTrace && eg.RawTraceRef != nil && s.s3 != nil {
		data, err := s.s3.Get(ctx, *eg.RawTraceRef)
		if err == nil {
			trace := string(data)
			resp.RawTrace = &trace
		}
		// Non-fatal: if S3 fetch fails we still return the metadata.
	}

	return resp, nil
}

// GetErrorGroupBySignature returns a single error group by its signature.
func (s *ErrorService) GetErrorGroupBySignature(
	ctx context.Context,
	tenantID, envID uuid.UUID,
	signature string,
) (*dto.ErrorGroupDetailResponse, error) {
	// Verify env belongs to tenant.
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	eg, err := s.store.GetErrorGroupBySignature(ctx, db.GetErrorGroupBySignatureParams{
		EnvID:     envID,
		Signature: signature,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	return &dto.ErrorGroupDetailResponse{
		ErrorGroupResponse: toErrorGroupResponse(eg),
	}, nil
}

// toErrorGroupResponse converts a DB model to an API response DTO.
func toErrorGroupResponse(eg db.ErrorGroup) dto.ErrorGroupResponse {
	return dto.ErrorGroupResponse{
		ID:              eg.ID,
		EnvID:           eg.EnvID,
		Signature:       eg.Signature,
		ErrorType:       eg.ErrorType,
		Message:         eg.Message,
		Module:          eg.Module,
		Model:           eg.Model,
		FirstSeen:       eg.FirstSeen,
		LastSeen:        eg.LastSeen,
		OccurrenceCount: eg.OccurrenceCount,
		AffectedUsers:   eg.AffectedUsers,
		Status:          eg.Status,
		ResolvedBy:      eg.ResolvedBy,
		ResolvedAt:      eg.ResolvedAt,
		RawTraceRef:     eg.RawTraceRef,
		CreatedAt:       eg.CreatedAt,
	}
}
