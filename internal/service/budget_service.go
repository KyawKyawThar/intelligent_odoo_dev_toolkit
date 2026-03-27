package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"

	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type funcKey struct {
	model    string
	method   string
	category string
}

type funcStat struct {
	durationMS int
	callCount  int
}

type moduleStats struct {
	totalMS   int
	sqlMS     int
	sqlCount  int
	ormMS     int
	ormCount  int
	pythonMS  int
	functions map[funcKey]*funcStat
}

// aggregateModuleStats pre-computes total_ms and per-module stats from events.
func aggregateModuleStats(events []ProfilerEvent) (totalMS int, byModule map[string]*moduleStats) {
	byModule = make(map[string]*moduleStats)
	for i := range events {
		ev := &events[i]
		totalMS += ev.DurationMS

		mod := ev.Module
		if mod == "" {
			continue
		}
		ms, ok := byModule[mod]
		if !ok {
			ms = &moduleStats{functions: make(map[funcKey]*funcStat)}
			byModule[mod] = ms
		}
		ms.totalMS += ev.DurationMS
		switch ev.Category {
		case catSQL:
			ms.sqlMS += ev.DurationMS
			ms.sqlCount++
		case catORM:
			ms.ormMS += ev.DurationMS
			ms.ormCount++
		case catPython, "profiler":
			ms.pythonMS += ev.DurationMS
		}

		if ev.Model != "" || ev.Method != "" {
			fk := funcKey{model: ev.Model, method: ev.Method, category: ev.Category}
			fs, exists := ms.functions[fk]
			if !exists {
				fs = &funcStat{}
				ms.functions[fk] = fs
			}
			fs.durationMS += ev.DurationMS
			fs.callCount++
		}
	}
	return totalMS, byModule
}

// BudgetService implements the BudgetServicer interface.
type BudgetService struct {
	store db.Store
}

// NewBudgetService creates a new BudgetService.
func NewBudgetService(store db.Store) *BudgetService {
	return &BudgetService{store: store}
}

// Create creates a new performance budget for an environment.
func (s *BudgetService) Create(
	ctx context.Context,
	tenantID, envID uuid.UUID,
	req *dto.CreateBudgetRequest,
) (*dto.BudgetResponse, error) {
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	budget, err := s.store.CreatePerfBudget(ctx, db.CreatePerfBudgetParams{
		EnvID:        envID,
		Module:       req.Module,
		Endpoint:     req.Endpoint,
		ThresholdPct: req.ThresholdPct,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	return toBudgetResponse(budget), nil
}

// GetByID retrieves a budget with its latest sample and 7-day trend.
func (s *BudgetService) GetByID(
	ctx context.Context,
	tenantID, envID, budgetID uuid.UUID,
) (*dto.BudgetDetailResponse, error) {
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	budget, err := s.store.GetPerfBudget(ctx, db.GetPerfBudgetParams{
		ID:    budgetID,
		EnvID: envID,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	resp := &dto.BudgetDetailResponse{
		Budget: *toBudgetResponse(budget),
	}

	// Attach latest sample (optional — may not exist yet).
	if sample, err := s.store.GetLatestBudgetSample(ctx, budgetID); err == nil {
		resp.LatestSample = toBudgetSampleResponse(sample)
	}

	// Attach 7-day trend (optional — may not have data yet).
	if trend, err := s.store.GetBudgetAverage7d(ctx, budgetID); err == nil && trend.SampleCount > 0 {
		resp.Trend = &dto.BudgetTrendResponse{
			BudgetID:    budgetID,
			AvgOverhead: trend.AvgOverhead,
			MaxOverhead: trend.MaxOverhead,
			SampleCount: trend.SampleCount,
		}
	}

	return resp, nil
}

// List lists budgets for an environment. If includeInactive is true, all
// budgets are returned; otherwise only active ones.
func (s *BudgetService) List(
	ctx context.Context,
	tenantID, envID uuid.UUID,
	includeInactive bool,
) (*dto.BudgetListResponse, error) {
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	var budgets []db.PerfBudget
	var err error

	if includeInactive {
		budgets, err = s.store.ListAllPerfBudgetsByEnv(ctx, envID)
	} else {
		budgets, err = s.store.ListPerfBudgets(ctx, envID)
	}
	if err != nil {
		return nil, api.FromPgError(err)
	}

	items := make([]dto.BudgetResponse, 0, len(budgets))
	for _, b := range budgets {
		items = append(items, *toBudgetResponse(b))
	}

	return &dto.BudgetListResponse{
		Budgets: items,
		Total:   len(items),
	}, nil
}

// Update updates a budget's threshold and active status.
func (s *BudgetService) Update(
	ctx context.Context,
	tenantID, envID, budgetID uuid.UUID,
	req *dto.UpdateBudgetRequest,
) (*dto.BudgetResponse, error) {
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	budget, err := s.store.UpdatePerfBudget(ctx, db.UpdatePerfBudgetParams{
		ID:           budgetID,
		EnvID:        envID,
		ThresholdPct: req.ThresholdPct,
		IsActive:     isActive,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	return toBudgetResponse(budget), nil
}

// Delete removes a budget from the environment.
func (s *BudgetService) Delete(
	ctx context.Context,
	tenantID, envID, budgetID uuid.UUID,
) error {
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return api.FromPgError(err)
	}

	if err := s.store.DeletePerfBudget(ctx, db.DeletePerfBudgetParams{
		ID:    budgetID,
		EnvID: envID,
	}); err != nil {
		return api.FromPgError(err)
	}

	return nil
}

// ListSamples lists performance samples for a budget.
func (s *BudgetService) ListSamples(
	ctx context.Context,
	tenantID, envID, budgetID uuid.UUID,
	limit int32,
) (*dto.BudgetSampleListResponse, error) {
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	// Verify the budget belongs to this environment.
	if _, err := s.store.GetPerfBudget(ctx, db.GetPerfBudgetParams{
		ID:    budgetID,
		EnvID: envID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	if limit <= 0 {
		limit = 50
	}

	samples, err := s.store.ListBudgetSamples(ctx, db.ListBudgetSamplesParams{
		BudgetID: budgetID,
		Limit:    limit,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	items := make([]dto.BudgetSampleResponse, 0, len(samples))
	for _, sample := range samples {
		items = append(items, *toBudgetSampleResponse(sample))
	}

	return &dto.BudgetSampleListResponse{
		Samples: items,
		Total:   len(items),
	}, nil
}

// GetTrend returns the 7-day performance trend for a budget.
func (s *BudgetService) GetTrend(
	ctx context.Context,
	tenantID, envID, budgetID uuid.UUID,
) (*dto.BudgetTrendResponse, error) {
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	// Verify the budget belongs to this environment.
	if _, err := s.store.GetPerfBudget(ctx, db.GetPerfBudgetParams{
		ID:    budgetID,
		EnvID: envID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	trend, err := s.store.GetBudgetAverage7d(ctx, budgetID)
	if err != nil {
		return nil, fmt.Errorf("get budget trend: %w", err)
	}

	return &dto.BudgetTrendResponse{
		BudgetID:    budgetID,
		AvgOverhead: trend.AvgOverhead,
		MaxOverhead: trend.MaxOverhead,
		SampleCount: trend.SampleCount,
	}, nil
}

// OverheadBreach describes a budget whose threshold was exceeded.
type OverheadBreach struct {
	BudgetID     uuid.UUID
	Module       string
	Endpoint     string
	OverheadPct  float64
	ThresholdPct int32
	TotalMS      int
	ModuleMS     int
	Breakdown    map[string]any
}

// OverheadResult holds the outcome of an overhead calculation cycle.
type OverheadResult struct {
	SamplesInserted int
	Breaches        []OverheadBreach
}

// CalculateOverhead computes overhead_pct for each active budget matching the
// profiler events in this batch, then inserts a budget sample per matched budget.
//
// Overhead formula: overhead_pct = (module_ms / total_ms) * 100
//   - total_ms  = sum of DurationMS across ALL events in the batch
//   - module_ms = sum of DurationMS for events whose Module matches the budget
//
// A per-category breakdown is stored as JSONB for drill-down.
// Breaches (overhead > threshold) are returned so the caller can publish alerts.
// CalculateOverhead computes overhead_pct for each active budget matching the
// profiler events in this batch, then inserts a budget sample per matched budget.
//
// Overhead formula: overhead_pct = (module_ms / total_ms) * 100
//   - total_ms  = sum of DurationMS across ALL events in the batch
//   - module_ms = sum of DurationMS for events whose Module matches the budget
//
// A per-category breakdown is stored as JSONB for drill-down.
// Breaches (overhead > threshold) are returned so the caller can publish alerts.
func (s *BudgetService) CalculateOverhead(
	ctx context.Context,
	envID uuid.UUID,
	events []ProfilerEvent,
	logger zerolog.Logger,
) (*OverheadResult, error) {
	result := &OverheadResult{}

	if len(events) == 0 {
		return result, nil
	}

	// Fetch active budgets for this environment.
	budgets, err := s.store.ListPerfBudgets(ctx, envID)
	if err != nil {
		return nil, fmt.Errorf("list active budgets: %w", err)
	}
	if len(budgets) == 0 {
		return result, nil
	}

	// Pre-compute total_ms and per-module stats from events.
	totalMS, byModule := aggregateModuleStats(events)
	if totalMS == 0 {
		return result, nil
	}

	// For each active budget, compute overhead and insert a sample.
	for _, budget := range budgets {
		ms, ok := byModule[budget.Module]
		if !ok {
			// No events for this module in this batch — skip.
			continue
		}

		overheadPct := float64(ms.totalMS) / float64(totalMS) * 100
		overheadStr := fmt.Sprintf("%.2f", overheadPct)

		totalMSi32 := int32(totalMS)     //nolint:gosec // profiler durations fit int32
		moduleMSi32 := int32(ms.totalMS) //nolint:gosec // profiler durations fit int32

		breakdown, breakdownJSON, err := createBreakdown(ms)
		if err != nil {
			logger.Error().Err(err).
				Str("budget_id", budget.ID.String()).
				Msg("failed to marshal overhead breakdown")
			continue
		}
		breakdownRaw := json.RawMessage(breakdownJSON)

		if _, err := s.store.InsertBudgetSample(ctx, db.InsertBudgetSampleParams{
			BudgetID:    budget.ID,
			OverheadPct: overheadStr,
			TotalMs:     &totalMSi32,
			ModuleMs:    &moduleMSi32,
			Breakdown:   &breakdownRaw,
		}); err != nil {
			logger.Error().Err(err).
				Str("budget_id", budget.ID.String()).
				Str("module", budget.Module).
				Str("overhead_pct", overheadStr).
				Msg("failed to insert budget sample")
			continue
		}

		exceeded := overheadPct > float64(budget.ThresholdPct)

		logger.Debug().
			Str("budget_id", budget.ID.String()).
			Str("module", budget.Module).
			Str("overhead_pct", overheadStr).
			Bool("exceeded", exceeded).
			Msg("budget sample recorded")

		result.SamplesInserted++

		if exceeded {
			result.Breaches = append(result.Breaches, OverheadBreach{
				BudgetID:     budget.ID,
				Module:       budget.Module,
				Endpoint:     budget.Endpoint,
				OverheadPct:  overheadPct,
				ThresholdPct: budget.ThresholdPct,
				TotalMS:      totalMS,
				ModuleMS:     ms.totalMS,
				Breakdown:    breakdown,
			})
		}
	}

	return result, nil
}

// createBreakdown generates the JSON breakdown for a module's stats.
func createBreakdown(ms *moduleStats) (breakdown map[string]any, jsonBytes []byte, err error) {
	// Build per-function breakdown entries.
	functions := make([]map[string]any, 0, len(ms.functions))
	for fk, fs := range ms.functions {
		pct := 0.0
		if ms.totalMS > 0 {
			pct = float64(fs.durationMS) / float64(ms.totalMS) * 100
		}
		functions = append(functions, map[string]any{
			"model":       fk.model,
			"method":      fk.method,
			"category":    fk.category,
			"duration_ms": fs.durationMS,
			"call_count":  fs.callCount,
			"pct":         math.Round(pct*100) / 100,
		})
	}

	// Sort functions by duration descending for consistent output.
	sort.Slice(functions, func(i, j int) bool {
		di, ok := functions[i]["duration_ms"].(int)
		if !ok {
			return false
		}
		dj, ok := functions[j]["duration_ms"].(int)
		if !ok {
			return true
		}
		return di > dj
	})

	// Build per-category + function-level breakdown.
	breakdown = map[string]any{
		"sql_ms":    ms.sqlMS,
		"sql_count": ms.sqlCount,
		"orm_ms":    ms.ormMS,
		"orm_count": ms.ormCount,
		"python_ms": ms.pythonMS,
		"functions": functions,
	}
	jsonBytes, err = json.Marshal(breakdown)
	return breakdown, jsonBytes, err
}

// GetBreakdown returns the function-level breakdown for a specific budget sample.
func (s *BudgetService) GetBreakdown(
	ctx context.Context,
	tenantID, envID, budgetID, sampleID uuid.UUID,
) (*dto.FunctionBreakdownResponse, error) {
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	// Verify the budget belongs to this environment.
	if _, err := s.store.GetPerfBudget(ctx, db.GetPerfBudgetParams{
		ID:    budgetID,
		EnvID: envID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	sample, err := s.store.GetBudgetSampleByID(ctx, db.GetBudgetSampleByIDParams{
		ID:       sampleID,
		BudgetID: budgetID,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	resp := &dto.FunctionBreakdownResponse{
		SampleID:    sample.ID,
		BudgetID:    sample.BudgetID,
		OverheadPct: sample.OverheadPct,
		TotalMS:     sample.TotalMs,
		ModuleMS:    sample.ModuleMs,
		SampledAt:   sample.SampledAt,
	}

	parseBreakdownJSON(sample.Breakdown, resp)

	return resp, nil
}

// ─── Helpers ────────────────────────────────────────────────────────────────

// unmarshalField is a helper to reduce boilerplate in parseBreakdownJSON.
func unmarshalField(raw map[string]json.RawMessage, key string, target any) {
	if v, ok := raw[key]; ok {
		//nolint:errcheck // best-effort field extraction from JSONB
		json.Unmarshal(v, target)
	}
}

// parseBreakdownJSON extracts category-level and function-level data from
// the breakdown JSONB column into the response struct.
func parseBreakdownJSON(breakdown *json.RawMessage, resp *dto.FunctionBreakdownResponse) {
	if breakdown != nil {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(*breakdown, &raw); err == nil {
			unmarshalField(raw, "sql_ms", &resp.SQLMS)
			unmarshalField(raw, "sql_count", &resp.SQLCount)
			unmarshalField(raw, "orm_ms", &resp.ORMMS)
			unmarshalField(raw, "orm_count", &resp.ORMCount)
			unmarshalField(raw, "python_ms", &resp.PythonMS)
			unmarshalField(raw, "functions", &resp.Functions)
		}
	}

	if resp.Functions == nil {
		resp.Functions = []dto.FunctionStat{}
	}
}

func toBudgetResponse(b db.PerfBudget) *dto.BudgetResponse {
	return &dto.BudgetResponse{
		ID:           b.ID,
		EnvID:        b.EnvID,
		Module:       b.Module,
		Endpoint:     b.Endpoint,
		ThresholdPct: b.ThresholdPct,
		IsActive:     b.IsActive,
		CreatedAt:    b.CreatedAt,
	}
}

func toBudgetSampleResponse(s db.PerfBudgetSample) *dto.BudgetSampleResponse {
	return &dto.BudgetSampleResponse{
		ID:          s.ID,
		BudgetID:    s.BudgetID,
		OverheadPct: s.OverheadPct,
		TotalMS:     s.TotalMs,
		ModuleMS:    s.ModuleMs,
		Breakdown:   s.Breakdown,
		SampledAt:   s.SampledAt,
	}
}
