package service

import (
	"context"
	"time"

	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"

	"github.com/google/uuid"
)

// OverviewService implements OverviewServicer.
type OverviewService struct {
	store db.Store
}

// NewOverviewService creates a new OverviewService.
func NewOverviewService(store db.Store) *OverviewService {
	return &OverviewService{store: store}
}

// GetOverview aggregates cross-feature stats for a single environment.
func (s *OverviewService) GetOverview(
	ctx context.Context,
	tenantID, envID uuid.UUID,
) (*dto.OverviewResponse, error) {
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	resp := &dto.OverviewResponse{}

	if err := s.populateAgent(ctx, envID, resp); err != nil {
		return nil, err
	}
	if err := s.populateErrors(ctx, envID, resp); err != nil {
		return nil, err
	}
	if err := s.populateProfiler(ctx, envID, resp); err != nil {
		return nil, err
	}
	if err := s.populateN1(ctx, envID, resp); err != nil {
		return nil, err
	}
	if err := s.populateAlerts(ctx, envID, resp); err != nil {
		return nil, err
	}
	if err := s.populateBudgets(ctx, envID, resp); err != nil {
		return nil, err
	}

	return resp, nil
}

func (s *OverviewService) populateAgent(ctx context.Context, envID uuid.UUID, resp *dto.OverviewResponse) error {
	hb, err := s.store.GetLatestHeartbeat(ctx, envID)
	if err != nil && !api.IsRecordNotFound(err) {
		return api.FromPgError(err)
	}
	if api.IsRecordNotFound(err) {
		resp.Agent = dto.AgentOverview{Status: "unknown"}
	} else {
		resp.Agent = dto.AgentOverview{
			Status:          hb.Status,
			LastHeartbeatAt: &hb.ReceivedAt,
		}
	}
	return nil
}

func (s *OverviewService) populateErrors(ctx context.Context, envID uuid.UUID, resp *dto.OverviewResponse) error {
	totalErrors, err := s.store.CountErrorGroupsByEnv(ctx, envID)
	if err != nil {
		return api.FromPgError(err)
	}
	openErrors, err := s.store.CountErrorGroupsByStatus(ctx, db.CountErrorGroupsByStatusParams{
		EnvID:  envID,
		Status: "open",
	})
	if err != nil {
		return api.FromPgError(err)
	}
	resp.Errors = dto.ErrorsOverview{Total: totalErrors, Open: openErrors}
	return nil
}

func (s *OverviewService) populateProfiler(ctx context.Context, envID uuid.UUID, resp *dto.OverviewResponse) error {
	totalRecs, err := s.store.CountRecordingsByEnv(ctx, envID)
	if err != nil {
		return api.FromPgError(err)
	}
	chainRows, err := s.store.ListRecordingsWithChain(ctx, db.ListRecordingsWithChainParams{
		EnvID:  envID,
		Limit:  1000,
		Offset: 0,
	})
	if err != nil {
		return api.FromPgError(err)
	}
	resp.Profiler = dto.ProfilerOverview{
		TotalRecordings:  totalRecs,
		WithComputeChain: int64(len(chainRows)),
	}
	return nil
}

func (s *OverviewService) populateN1(ctx context.Context, envID uuid.UUID, resp *dto.OverviewResponse) error {
	n1Rows, err := s.store.GetN1PatternsSummary(ctx, db.GetN1PatternsSummaryParams{
		EnvID:  envID,
		Period: time.Now().UTC().Add(-24 * time.Hour),
		Limit:  200,
	})
	if err != nil {
		return api.FromPgError(err)
	}
	n1Overview := dto.N1Overview{PatternsDetected: len(n1Rows)}
	for _, row := range n1Rows {
		score := ComputeImpactScore(int(row.TotalDurationMs), int(row.Occurrences), int(row.PeakCalls))
		if ClassifySeverity(score) == sevCritical {
			n1Overview.CriticalCount++
		}
	}
	resp.N1 = n1Overview
	return nil
}

func (s *OverviewService) populateAlerts(ctx context.Context, envID uuid.UUID, resp *dto.OverviewResponse) error {
	unacked, err := s.store.CountUnacknowledgedAlerts(ctx, envID)
	if err != nil {
		return api.FromPgError(err)
	}
	resp.Alerts = dto.AlertsOverview{Unacknowledged: unacked}
	return nil
}

func (s *OverviewService) populateBudgets(ctx context.Context, envID uuid.UUID, resp *dto.OverviewResponse) error {
	budgets, err := s.store.ListAllPerfBudgetsByEnv(ctx, envID)
	if err != nil {
		return api.FromPgError(err)
	}
	activeBudgets := 0
	for _, b := range budgets {
		if b.IsActive {
			activeBudgets++
		}
	}
	resp.Budgets = dto.BudgetsOverview{
		Total:  len(budgets),
		Active: activeBudgets,
	}
	return nil
}
