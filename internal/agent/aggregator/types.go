// Package aggregator groups raw pipeline events into time-windowed batches.
// Instead of sending 10,000 raw ORM calls, the agent aggregates them into
// per-model:method summaries with counts, avg/max/p95 latencies, and N+1
// detection. Only critical events (errors, slow queries, N+1) are kept as
// raw entries — everything else is represented as aggregated stats.
package aggregator

import "time"

// Event is a single pipeline event fed into the aggregator by collectors
// (ORM hooks, pg_stat_statements, log tailer, profiler, etc.).
type Event struct {
	// Category classifies the event: "orm", "sql", "error", "profiler".
	Category string `json:"category"`

	// Model is the Odoo model name, e.g. "product.product".
	Model string `json:"model,omitempty"`

	// Method is the ORM method name, e.g. "search_read", "write".
	Method string `json:"method,omitempty"`

	// DurationMS is the operation duration in milliseconds.
	DurationMS int `json:"duration_ms"`

	// IsError is true for error/exception events.
	IsError bool `json:"is_error,omitempty"`

	// IsN1 is true when an N+1 query pattern was detected.
	IsN1 bool `json:"is_n1,omitempty"`

	// SQL is a sample SQL query text (for SQL category events).
	SQL string `json:"sql,omitempty"`

	// Module is the Odoo module that originated the event.
	Module string `json:"module,omitempty"`

	// Traceback is the raw Python traceback (for error events).
	Traceback string `json:"traceback,omitempty"`

	// UserID is the Odoo UID associated with the request (0 = unknown).
	UserID int `json:"user_id,omitempty"`

	// Timestamp is when the event was captured.
	Timestamp time.Time `json:"timestamp"`

	// FieldName is the Odoo field being computed (e.g. "amount_total").
	// Only set for compute-category events.
	FieldName string `json:"field_name,omitempty"`

	// IsCompute is true when this event represents a computed field evaluation.
	IsCompute bool `json:"is_compute,omitempty"`

	// DependsOn lists the fields that triggered this computation
	// (from @api.depends decorator).
	DependsOn []string `json:"depends_on,omitempty"`

	// TriggerField is the field whose write triggered this recomputation chain.
	TriggerField string `json:"trigger_field,omitempty"`
}

// ORMModelStat holds aggregated statistics for one model:method combination
// within a single flush window.
type ORMModelStat struct {
	Model      string  `json:"model"`
	Method     string  `json:"method"`
	CallCount  int     `json:"call_count"`
	TotalMS    int     `json:"total_ms"`
	AvgMS      float64 `json:"avg_ms"`
	MaxMS      int     `json:"max_ms"`
	P95MS      int     `json:"p95_ms"`
	N1Detected bool    `json:"n1_detected"`
	SampleSQL  string  `json:"sample_sql,omitempty"`
}

// BatchSummary provides a quick overview of the entire flush window.
type BatchSummary struct {
	TotalQueries    int `json:"total_queries"`
	TotalDurationMS int `json:"total_duration_ms"`
	SlowQueries     int `json:"slow_queries"`
	N1Patterns      int `json:"n1_patterns"`
	Errors          int `json:"errors"`
}

// AggregatedBatch is the payload sent to the cloud server on each flush.
// It contains both aggregated stats and the raw events that passed the
// sampler's critical-event filter.
type AggregatedBatch struct {
	// EnvID identifies the environment this batch belongs to.
	EnvID string `json:"env_id"`

	// Period is the start time of the aggregation window.
	Period time.Time `json:"period"`

	// DurationMS is the window size in milliseconds (typically 30000).
	DurationMS int `json:"duration_ms"`

	// ORMStats are per-model:method aggregated statistics.
	ORMStats []ORMModelStat `json:"orm_stats"`

	// RawEvents are the critical events kept in full (errors, slow, N+1).
	RawEvents []Event `json:"raw_events"`

	// Summary is a pre-computed overview of the entire window.
	Summary BatchSummary `json:"summary"`
}
