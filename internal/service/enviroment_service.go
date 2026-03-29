package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"time"

	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"

	"github.com/google/uuid"
)

func (s *EnvironmentService) Create(ctx context.Context, tenantID uuid.UUID, req *dto.CreateEnvironmentRequest) (*dto.EnvironmentResponse, error) {

	exists, err := s.store.CheckEnvironmentNameExists(ctx, db.CheckEnvironmentNameExistsParams{
		TenantID: tenantID,
		Name:     req.Name,
		ID:       uuid.Nil, // no env to exclude on create
	})
	if err != nil {
		return nil, api.ErrInternal(err)
	}
	if exists {
		return nil, api.NewError(api.ErrCodeConflict, "An environment with this name already exists", http.StatusConflict)
	}
	// Default feature flags based on env type if not provided
	featureFlags := req.FeatureFlags

	if featureFlags == nil {
		featureFlags = defaultFeatureFlags(req.EnvType)
	}

	env, err := s.store.CreateEnvironment(ctx, db.CreateEnvironmentParams{
		TenantID:     tenantID,
		Name:         req.Name,
		OdooUrl:      req.OdooURL,
		DbName:       req.DbName,
		OdooVersion:  req.OdooVersion,
		EnvType:      req.EnvType,
		Status:       "active",
		FeatureFlags: featureFlags,
	})

	if err != nil {
		return nil, api.FromPgError(err)
	}

	return dto.ToEnvironmentResponse(&env), nil

}

func (s *EnvironmentService) GetByID(ctx context.Context, tenantID, envID uuid.UUID) (*dto.EnvironmentResponse, error) {
	env, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	return dto.ToEnvironmentResponse(&env), nil
}

func (s *EnvironmentService) List(ctx context.Context, tenantID uuid.UUID, req *dto.ListEnvironmentsRequest) (*dto.EnvironmentListResponse, error) {

	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	var (
		envs  []db.Environment
		total int64
		err   error
	)

	hasType := req.EnvType != ""
	hasStatus := req.Status != ""

	switch {
	case hasType && hasStatus:
		envs, err = s.store.ListEnvironmentsByTenantTypeAndStatus(ctx, db.ListEnvironmentsByTenantTypeAndStatusParams{
			TenantID: tenantID,
			EnvType:  req.EnvType,
			Status:   req.Status,
			Limit:    limit,
			Offset:   offset,
		})
		if err == nil {
			total, err = s.store.CountEnvironmentsByTenantTypeAndStatus(ctx, db.CountEnvironmentsByTenantTypeAndStatusParams{
				TenantID: tenantID,
				EnvType:  req.EnvType,
				Status:   req.Status,
			})
		}

	case hasType:
		envs, err = s.store.ListEnvironmentsByTenantAndType(ctx, db.ListEnvironmentsByTenantAndTypeParams{
			TenantID: tenantID,
			EnvType:  req.EnvType,
			Limit:    limit,
			Offset:   offset,
		})
		if err == nil {
			total, err = s.store.CountEnvironmentsByTenantAndType(ctx, db.CountEnvironmentsByTenantAndTypeParams{
				TenantID: tenantID,
				EnvType:  req.EnvType,
			})
		}

	case hasStatus:
		envs, err = s.store.ListEnvironmentsByTenantAndStatus(ctx, db.ListEnvironmentsByTenantAndStatusParams{
			TenantID: tenantID,
			Status:   req.Status,
			Limit:    limit,
			Offset:   offset,
		})
		if err == nil {
			total, err = s.store.CountEnvironmentsByTenantAndStatus(ctx, db.CountEnvironmentsByTenantAndStatusParams{
				TenantID: tenantID,
				Status:   req.Status,
			})
		}
	default:
		envs, err = s.store.ListEnvironmentsByTenant(ctx, tenantID)
		if err == nil {
			total, err = s.store.CountEnvironmentsByTenant(ctx, tenantID)
		}
	}

	if err != nil {
		return nil, api.FromPgError(err)
	}

	responses := make([]dto.EnvironmentResponse, 0, len(envs))
	for i := range envs {
		responses = append(responses, *dto.ToEnvironmentResponse(&envs[i]))
	}

	return &dto.EnvironmentListResponse{
		Environments: responses,
		Total:        total,
		Limit:        limit,
		Offset:       offset,
	}, nil
}

func (s *EnvironmentService) Update(ctx context.Context, tenantID, envID uuid.UUID, req *dto.UpdateEnvironmentRequest) (*dto.EnvironmentResponse, error) {

	// Check duplicate name if name is being changed
	if req.Name != nil {
		exists, err := s.store.CheckEnvironmentNameExists(ctx, db.CheckEnvironmentNameExistsParams{
			TenantID: tenantID,
			Name:     *req.Name,
			ID:       envID, // exclude current env
		})
		if err != nil {
			return nil, api.ErrInternal(err)
		}
		if exists {
			return nil, api.NewError(api.ErrCodeConflict, "An environment with this name already exists", http.StatusConflict)
		}
	}

	env, err := s.store.UpdateEnvironment(ctx, db.UpdateEnvironmentParams{
		ID:           envID,
		TenantID:     tenantID,
		Name:         req.Name,
		OdooUrl:      req.OdooURL,
		DbName:       req.DbName,
		OdooVersion:  req.OdooVersion,
		EnvType:      req.EnvType,
		Status:       req.Status,
		FeatureFlags: &req.FeatureFlags,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	return dto.ToEnvironmentResponse(&env), nil
}

func (s *EnvironmentService) RegisterAgent(ctx context.Context, tenantID, envID uuid.UUID, _ *dto.RegisterAgentRequest) (*dto.RegisterAgentResponse, error) {
	// Verify environment exists and belongs to tenant.
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.ErrNotFound("environment not found")
	}

	// Generate a one-time registration token.
	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		return nil, api.ErrInternal(err)
	}
	token := "reg_" + base64.RawURLEncoding.EncodeToString(rawBytes)
	expiresAt := time.Now().UTC().Add(1 * time.Hour)

	if _, err := s.store.SetRegistrationToken(ctx, db.SetRegistrationTokenParams{
		ID:                         envID,
		RegistrationToken:          &token,
		RegistrationTokenExpiresAt: &expiresAt,
		TenantID:                   tenantID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	return &dto.RegisterAgentResponse{
		RegistrationToken: token,
		ExpiresAt:         expiresAt,
	}, nil
}

func (s *EnvironmentService) GetLatestHeartbeat(ctx context.Context, tenantID, envID uuid.UUID) (*dto.HeartbeatResponse, error) {
	// Verify environment belongs to tenant.
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.ErrNotFound("environment")
	}

	hb, err := s.store.GetLatestHeartbeat(ctx, envID)
	if err != nil {
		return nil, api.NewError(api.ErrCodeNotFound, "no heartbeats found", http.StatusNotFound)
	}

	return dto.ToHeartbeatResponse(&hb), nil
}

func (s *EnvironmentService) ListHeartbeats(ctx context.Context, tenantID, envID uuid.UUID, limit int32) (*dto.HeartbeatListResponse, error) {
	// Verify environment belongs to tenant.
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.ErrNotFound("environment")
	}

	if limit <= 0 {
		limit = 20
	}

	hbs, err := s.store.ListHeartbeats(ctx, db.ListHeartbeatsParams{
		EnvID: envID,
		Limit: limit,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	resp := &dto.HeartbeatListResponse{
		Heartbeats: make([]dto.HeartbeatResponse, 0, len(hbs)),
		Total:      len(hbs),
	}
	for i := range hbs {
		resp.Heartbeats = append(resp.Heartbeats, *dto.ToHeartbeatResponse(&hbs[i]))
	}
	return resp, nil
}

func (s *EnvironmentService) Delete(ctx context.Context, tenantID, envID uuid.UUID) error {
	rows, err := s.store.DeleteEnvironment(ctx, db.DeleteEnvironmentParams{
		ID:       envID,
		TenantID: tenantID,
	})
	if err != nil {
		return api.FromPgError(err)
	}
	if rows == 0 {
		return api.ErrNotFound("environment")
	}
	return nil
}
