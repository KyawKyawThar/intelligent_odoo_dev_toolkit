package service

import (
	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
)

type SchemaService struct {
	store db.Store
}

func NewSchemaService(store db.Store) *SchemaService {
	return &SchemaService{store: store}
}

// StoreSchema validates that the environment belongs to the tenant, then
// persists the schema snapshot sent by the agent.
func (s *SchemaService) StoreSchema(ctx context.Context, tenantID uuid.UUID, req *dto.StoreSchemaRequest) (*dto.SchemaSnapshotResponse, error) {
	// Verify env exists and belongs to this tenant.
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       req.EnvID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	snapshot, err := s.store.CreateSchemaSnapshot(ctx, db.CreateSchemaSnapshotParams{
		EnvID:       req.EnvID,
		Models:      req.Models,
		AclRules:    req.ACLRules,
		RecordRules: req.RecordRules,
		ModelCount:  req.ModelCount,
		FieldCount:  req.FieldCount,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	return dto.ToSchemaSnapshotResponse(&snapshot), nil
}

// GetLatest returns the most recent schema snapshot for an environment,
// scoped to the tenant.
func (s *SchemaService) GetLatest(ctx context.Context, tenantID, envID uuid.UUID) (*dto.SchemaSnapshotResponse, error) {
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	snapshot, err := s.store.GetLatestSchema(ctx, envID)
	if err != nil {
		return nil, api.FromPgError(err)
	}

	return dto.ToSchemaSnapshotResponse(&snapshot), nil
}

// ListSnapshots returns a lightweight paginated list of schema snapshots for
// an environment, scoped to the tenant.
func (s *SchemaService) ListSnapshots(ctx context.Context, tenantID, envID uuid.UUID, limit int32) (*dto.SchemaSnapshotListResponse, error) {
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	if limit <= 0 {
		limit = 20
	}

	total, err := s.store.CountSchemasByEnv(ctx, envID)
	if err != nil {
		return nil, api.FromPgError(err)
	}

	rows, err := s.store.ListSchemaSnapshots(ctx, db.ListSchemaSnapshotsParams{
		EnvID: envID,
		Limit: limit,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	items := make([]dto.SchemaSnapshotListItem, 0, len(rows))
	for i := range rows {
		items = append(items, dto.ToSchemaSnapshotListItem(&rows[i]))
	}

	return &dto.SchemaSnapshotListResponse{
		Snapshots: items,
		Total:     total,
		Limit:     limit,
	}, nil
}

// SearchModels searches model entries within the latest snapshot for an
// environment. It filters by case-insensitive substring match on the model's
// "model" (technical name) or "name" (display label) keys.
func (s *SchemaService) SearchModels(ctx context.Context, tenantID, envID uuid.UUID, q string, limit, offset int32) (*dto.SearchModelsResponse, error) {
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	snapshot, err := s.store.GetLatestSchema(ctx, envID)
	if err != nil {
		return nil, api.FromPgError(err)
	}

	// Unmarshal the models JSONB into a generic slice so we can filter in Go.
	var all []json.RawMessage
	if err := json.Unmarshal(snapshot.Models, &all); err != nil {
		return nil, api.ErrInternal(err)
	}

	// Filter: if q is empty return all; otherwise match against "model" / "name".
	q = strings.ToLower(strings.TrimSpace(q))
	filtered := all
	if q != "" {
		filtered = filtered[:0]
		for _, raw := range all {
			if modelMatches(raw, q) {
				filtered = append(filtered, raw)
			}
		}
	}

	total := len(filtered)

	// Apply offset + limit.
	if limit <= 0 {
		limit = 50
	}
	offsetAsInt := int(offset)
	if offsetAsInt >= total {
		return &dto.SearchModelsResponse{
			Models: []json.RawMessage{},
			Total:  total,
			Limit:  limit,
			Offset: offset,
		}, nil
	}

	end := offsetAsInt + int(limit)
	if end > total {
		end = total
	}
	page := filtered[offsetAsInt:end]

	return &dto.SearchModelsResponse{
		Models: page,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}

// modelMatches returns true when the model entry's "model" or "name" field
// contains q (lowercased substring match).
func modelMatches(raw json.RawMessage, q string) bool {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}
	for _, key := range []string{"model", "name"} {
		val, ok := m[key]
		if !ok {
			continue
		}
		var s string
		if err := json.Unmarshal(val, &s); err != nil {
			continue
		}
		if strings.Contains(strings.ToLower(s), q) {
			return true
		}
	}
	return false
}
