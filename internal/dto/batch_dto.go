package dto

import "time"

// ─── Batch Ingest DTOs (POST /api/v1/agent/batch) ───────────────────────────

// BatchORMStat holds aggregated ORM statistics for one model:method pair
// within a single flush window.
type BatchORMStat struct {
	Model      string  `json:"model" example:"res.partner"`
	Method     string  `json:"method" example:"search_read"`
	CallCount  int     `json:"call_count" example:"148"`
	TotalMS    int     `json:"total_ms" example:"890"`
	AvgMS      float64 `json:"avg_ms" example:"6.01"`
	MaxMS      int     `json:"max_ms" example:"42"`
	P95MS      int     `json:"p95_ms" example:"12"`
	N1Detected bool    `json:"n1_detected" example:"true"`
	SampleSQL  string  `json:"sample_sql,omitempty" example:"SELECT id, name FROM res_partner WHERE active = true"`
}

// BatchRawEvent is a single critical event (error, slow query, N+1) kept in
// full detail rather than aggregated.
type BatchRawEvent struct {
	Category   string    `json:"category" example:"error"`
	Model      string    `json:"model,omitempty" example:"sale.order"`
	Method     string    `json:"method,omitempty" example:"action_confirm"`
	DurationMS int       `json:"duration_ms,omitempty" example:"0"`
	IsError    bool      `json:"is_error,omitempty" example:"true"`
	IsN1       bool      `json:"is_n1,omitempty" example:"false"`
	SQL        string    `json:"sql,omitempty"`
	Module     string    `json:"module,omitempty" example:"sale"`
	Traceback  string    `json:"traceback,omitempty" example:"Traceback (most recent call last):\n  File ..."`
	UserID     int       `json:"user_id,omitempty" example:"2"`
	Message    string    `json:"message,omitempty" example:"ValidationError: No warehouse configured"`
	Timestamp  time.Time `json:"timestamp" example:"2026-03-19T10:00:15Z"`
}

// BatchSummary provides a quick overview of the entire flush window.
type BatchSummary struct {
	TotalQueries    int `json:"total_queries" example:"200"`
	TotalDurationMS int `json:"total_duration_ms" example:"1200"`
	SlowQueries     int `json:"slow_queries" example:"3"`
	N1Patterns      int `json:"n1_patterns" example:"1"`
	Errors          int `json:"errors" example:"1"`
}

// IngestBatchRequest is the body for POST /api/v1/agent/batch.
// It represents an aggregated batch sent by the agent every flush window
// (typically 30 seconds). Contains both aggregated ORM stats and raw
// critical events that passed the sampler's filter.
type IngestBatchRequest struct {
	EnvID      string          `json:"env_id" validate:"required,uuid" example:"550e8400-e29b-41d4-a716-446655440000"`
	Period     time.Time       `json:"period" example:"2026-03-19T10:00:00Z"`
	DurationMS int             `json:"duration_ms" example:"30000"`
	ORMStats   []BatchORMStat  `json:"orm_stats"`
	RawEvents  []BatchRawEvent `json:"raw_events"`
	Summary    BatchSummary    `json:"summary"`
}

// IngestBatchResponse is returned on successful batch queuing.
type IngestBatchResponse struct {
	Status string `json:"status" example:"queued"`
}
