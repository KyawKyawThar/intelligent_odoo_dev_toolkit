package dto

import (
	"time"

	"github.com/google/uuid"
)

// ─── Waterfall Builder DTOs ─────────────────────────────────────────────────

// WaterfallSpan represents a single span in the waterfall timeline.
// Each span has a start offset, duration, category, and optional metadata.
type WaterfallSpan struct {
	// ID is a unique identifier for this span within the waterfall.
	ID string `json:"id"`
	// Category classifies the span: "sql", "orm", "python", "rpc", "http".
	Category string `json:"category"`
	// Label is a human-readable description (e.g. "res.partner.search_read").
	Label string `json:"label"`
	// StartMS is the offset from the recording start in milliseconds.
	StartMS int `json:"start_ms"`
	// DurationMS is the span duration in milliseconds.
	DurationMS int `json:"duration_ms"`
	// Module is the Odoo module that originated this span (optional).
	Module string `json:"module,omitempty"`
	// Model is the Odoo model involved (optional).
	Model string `json:"model,omitempty"`
	// Method is the ORM method name (optional).
	Method string `json:"method,omitempty"`
	// SQL is the query text for SQL spans (optional, truncated).
	SQL string `json:"sql,omitempty"`
	// IsN1 flags spans that are part of an N+1 pattern.
	IsN1 bool `json:"is_n1,omitempty"`
	// IsError flags spans that resulted in an error.
	IsError bool `json:"is_error,omitempty"`
	// ParentID links child spans to their parent (e.g. ORM → SQL).
	ParentID string `json:"parent_id,omitempty"`
	// Depth is the nesting level (0 = top-level).
	Depth int `json:"depth"`
}

// WaterfallSummary provides aggregate statistics for the waterfall view.
type WaterfallSummary struct {
	TotalMS     int    `json:"total_ms"`
	SQLMS       int    `json:"sql_ms"`
	SQLCount    int    `json:"sql_count"`
	PythonMS    int    `json:"python_ms"`
	ORMMS       int    `json:"orm_ms"`
	ORMCount    int    `json:"orm_count"`
	N1Count     int    `json:"n1_count"`
	N1MS        int    `json:"n1_ms"`
	ErrorCount  int    `json:"error_count"`
	SpanCount   int    `json:"span_count"`
	CriticalSQL string `json:"critical_sql,omitempty"`
}

// WaterfallLane groups spans by category for stacked rendering.
type WaterfallLane struct {
	Category string  `json:"category"`
	Label    string  `json:"label"`
	Pct      float64 `json:"pct"`
	TotalMS  int     `json:"total_ms"`
}

// Waterfall is the complete waterfall timeline for a profiler recording.
type Waterfall struct {
	Spans   []WaterfallSpan  `json:"spans"`
	Lanes   []WaterfallLane  `json:"lanes"`
	Summary WaterfallSummary `json:"summary"`
}

// ─── Compute Chain DTOs ─────────────────────────────────────────────────────

// ComputeNode represents a single computed field evaluation in the chain.
type ComputeNode struct {
	// ID is a unique identifier for this node within the chain.
	ID string `json:"id"`
	// Model is the Odoo model (e.g. "sale.order").
	Model string `json:"model"`
	// FieldName is the computed field (e.g. "amount_total").
	FieldName string `json:"field_name"`
	// Method is the compute method (e.g. "_compute_amount_total").
	Method string `json:"method,omitempty"`
	// Module is the Odoo module that defines this compute (e.g. "sale").
	Module string `json:"module,omitempty"`
	// DurationMS is how long this computation took.
	DurationMS int `json:"duration_ms"`
	// DependsOn lists the field names this computation depends on.
	DependsOn []string `json:"depends_on,omitempty"`
	// ParentID links to the node that triggered this computation.
	ParentID string `json:"parent_id,omitempty"`
	// Depth is the nesting level in the dependency chain (0 = root trigger).
	Depth int `json:"depth"`
	// SQLCount is the number of SQL queries executed during this computation.
	SQLCount int `json:"sql_count,omitempty"`
	// IsBottleneck flags nodes that are disproportionately slow.
	IsBottleneck bool `json:"is_bottleneck,omitempty"`
}

// ComputeEdge represents a dependency link between two compute nodes.
type ComputeEdge struct {
	// From is the source node ID (the field that was written/changed).
	From string `json:"from"`
	// To is the target node ID (the computed field that was triggered).
	To string `json:"to"`
	// TriggerField is the specific field that triggered the recomputation.
	TriggerField string `json:"trigger_field,omitempty"`
}

// ComputeChainSummary provides aggregate stats for the compute chain.
type ComputeChainSummary struct {
	// TotalMS is the total time spent in compute field evaluations.
	TotalMS int `json:"total_ms"`
	// NodeCount is the number of computed field evaluations.
	NodeCount int `json:"node_count"`
	// MaxDepth is the deepest dependency chain depth.
	MaxDepth int `json:"max_depth"`
	// BottleneckCount is the number of nodes flagged as bottlenecks.
	BottleneckCount int `json:"bottleneck_count"`
	// SlowestNode is the ID of the slowest compute node.
	SlowestNode string `json:"slowest_node,omitempty"`
	// SlowestMS is the duration of the slowest compute node.
	SlowestMS int `json:"slowest_ms"`
	// TriggerField is the root field write that started the chain.
	TriggerField string `json:"trigger_field,omitempty"`
}

// ComputeChain is the full dependency graph for computed field evaluations.
type ComputeChain struct {
	// Nodes are the individual computed field evaluations.
	Nodes []ComputeNode `json:"nodes"`
	// Edges are the dependency links between nodes.
	Edges []ComputeEdge `json:"edges"`
	// Summary provides aggregate statistics.
	Summary ComputeChainSummary `json:"summary"`
}

// ─── Profiler Recording Response DTOs ───────────────────────────────────────

// ProfilerRecordingResponse is the full recording with a built waterfall.
type ProfilerRecordingResponse struct {
	ID           uuid.UUID     `json:"id"`
	EnvID        uuid.UUID     `json:"env_id"`
	TriggeredBy  *uuid.UUID    `json:"triggered_by,omitempty"`
	Name         string        `json:"name"`
	Endpoint     *string       `json:"endpoint,omitempty"`
	TotalMS      int32         `json:"total_ms"`
	SQLCount     *int32        `json:"sql_count,omitempty"`
	SQLMS        *int32        `json:"sql_ms,omitempty"`
	PythonMS     *int32        `json:"python_ms,omitempty"`
	Waterfall    *Waterfall    `json:"waterfall"`
	ComputeChain *ComputeChain `json:"compute_chain,omitempty"`
	RecordedAt   time.Time     `json:"recorded_at"`
}

// ProfilerRecordingListItem is a lightweight list entry (no waterfall).
type ProfilerRecordingListItem struct {
	ID              uuid.UUID  `json:"id"`
	EnvID           uuid.UUID  `json:"env_id"`
	TriggeredBy     *uuid.UUID `json:"triggered_by,omitempty"`
	Name            string     `json:"name"`
	Endpoint        *string    `json:"endpoint,omitempty"`
	TotalMS         int32      `json:"total_ms"`
	SQLCount        *int32     `json:"sql_count,omitempty"`
	SQLMS           *int32     `json:"sql_ms,omitempty"`
	PythonMS        *int32     `json:"python_ms,omitempty"`
	HasN1           bool       `json:"has_n1"`
	HasComputeChain bool       `json:"has_compute_chain"`
	RecordedAt      time.Time  `json:"recorded_at"`
}

// ProfilerRecordingListResponse is the paginated list of recordings.
type ProfilerRecordingListResponse struct {
	Recordings []ProfilerRecordingListItem `json:"recordings"`
	Total      int64                       `json:"total"`
}

// ListProfilerRecordingsRequest is the query parameters for listing recordings.
type ListProfilerRecordingsRequest struct {
	Limit  int32 `json:"limit,omitempty"`
	Offset int32 `json:"offset,omitempty"`
}

// ListSlowRecordingsRequest is the query parameters for listing slow recordings.
type ListSlowRecordingsRequest struct {
	ThresholdMS int32 `json:"threshold_ms" validate:"required,min=1"`
	Limit       int32 `json:"limit,omitempty"`
}
