package dto

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ─── Budget Request DTOs ────────────────────────────────────────────────────

// CreateBudgetRequest is the payload for creating a performance budget.
type CreateBudgetRequest struct {
	Module       string `json:"module" validate:"required,min=1,max=128"`
	Endpoint     string `json:"endpoint" validate:"required,min=1,max=256"`
	ThresholdPct int32  `json:"threshold_pct" validate:"required,min=1,max=100"`
}

// UpdateBudgetRequest is the payload for updating a performance budget.
type UpdateBudgetRequest struct {
	ThresholdPct int32 `json:"threshold_pct" validate:"required,min=1,max=100"`
	IsActive     *bool `json:"is_active" validate:"required"`
}

// ListBudgetSamplesRequest is the query parameters for listing budget samples.
type ListBudgetSamplesRequest struct {
	Limit int32 `json:"limit,omitempty"`
}

// ListBudgetSamplesBetweenRequest is the query parameters for listing samples in a date range.
type ListBudgetSamplesBetweenRequest struct {
	From time.Time `json:"from" validate:"required"`
	To   time.Time `json:"to" validate:"required"`
}

// ─── Budget Response DTOs ───────────────────────────────────────────────────

// BudgetResponse is the full budget configuration.
type BudgetResponse struct {
	ID           uuid.UUID `json:"id"`
	EnvID        uuid.UUID `json:"env_id"`
	Module       string    `json:"module"`
	Endpoint     string    `json:"endpoint"`
	ThresholdPct int32     `json:"threshold_pct"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
}

// BudgetListResponse is the list of budgets for an environment.
type BudgetListResponse struct {
	Budgets []BudgetResponse `json:"budgets"`
	Total   int              `json:"total"`
}

// BudgetSampleResponse represents a single budget performance sample.
type BudgetSampleResponse struct {
	ID          uuid.UUID        `json:"id"`
	BudgetID    uuid.UUID        `json:"budget_id"`
	OverheadPct string           `json:"overhead_pct"`
	TotalMS     *int32           `json:"total_ms,omitempty"`
	ModuleMS    *int32           `json:"module_ms,omitempty"`
	Breakdown   *json.RawMessage `json:"breakdown,omitempty"`
	SampledAt   time.Time        `json:"sampled_at"`
}

// BudgetSampleListResponse is the list of budget samples.
type BudgetSampleListResponse struct {
	Samples []BudgetSampleResponse `json:"samples"`
	Total   int                    `json:"total"`
}

// BudgetTrendResponse is the 7-day trend for a budget.
type BudgetTrendResponse struct {
	BudgetID    uuid.UUID `json:"budget_id"`
	AvgOverhead float64   `json:"avg_overhead"`
	MaxOverhead int32     `json:"max_overhead"`
	SampleCount int64     `json:"sample_count"`
}

// BudgetDetailResponse combines the budget config with its latest sample and trend.
type BudgetDetailResponse struct {
	Budget       BudgetResponse        `json:"budget"`
	LatestSample *BudgetSampleResponse `json:"latest_sample,omitempty"`
	Trend        *BudgetTrendResponse  `json:"trend,omitempty"`
}

// ─── Function-Level Breakdown DTOs ──────────────────────────────────────────

// FunctionStat represents the performance stats for a single function (model.method).
type FunctionStat struct {
	Model      string  `json:"model"`
	Method     string  `json:"method"`
	Category   string  `json:"category"`
	DurationMS int     `json:"duration_ms"`
	CallCount  int     `json:"call_count"`
	Pct        float64 `json:"pct"`
}

// FunctionBreakdownResponse is the full function-level drill-down for a budget sample.
type FunctionBreakdownResponse struct {
	SampleID    uuid.UUID      `json:"sample_id"`
	BudgetID    uuid.UUID      `json:"budget_id"`
	OverheadPct string         `json:"overhead_pct"`
	TotalMS     *int32         `json:"total_ms,omitempty"`
	ModuleMS    *int32         `json:"module_ms,omitempty"`
	SQLMS       int            `json:"sql_ms"`
	SQLCount    int            `json:"sql_count"`
	ORMMS       int            `json:"orm_ms"`
	ORMCount    int            `json:"orm_count"`
	PythonMS    int            `json:"python_ms"`
	Functions   []FunctionStat `json:"functions"`
	SampledAt   time.Time      `json:"sampled_at"`
}
