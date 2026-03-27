package dto

import "time"

// ─── N+1 Detector DTOs ─────────────────────────────────────────────────────

// N1DetectionRequest is the query parameters for the N+1 analysis endpoint.
type N1DetectionRequest struct {
	// Since limits analysis to data from this timestamp onward.
	Since time.Time `json:"since"`
	// Limit caps the number of results (default 50).
	Limit int32 `json:"limit,omitempty"`
}

// N1Pattern represents a single detected N+1 query pattern with impact analysis.
type N1Pattern struct {
	// Model is the Odoo model (e.g. "res.partner").
	Model string `json:"model"`
	// Method is the ORM method (e.g. "search_read").
	Method string `json:"method"`
	// Signature is the normalized SQL signature (literals replaced with ?).
	Signature string `json:"signature,omitempty"`
	// SampleSQL is a concrete SQL example from the pattern.
	SampleSQL string `json:"sample_sql,omitempty"`
	// TotalCalls is the cumulative call count across all detected windows.
	TotalCalls int `json:"total_calls"`
	// TotalMS is the cumulative duration in milliseconds.
	TotalMS int `json:"total_ms"`
	// AvgCallsPerWindow is the average call count per detection window.
	AvgCallsPerWindow float64 `json:"avg_calls_per_window"`
	// PeakCalls is the highest call count seen in a single window.
	PeakCalls int `json:"peak_calls"`
	// PeakMS is the highest duration seen in a single window.
	PeakMS int `json:"peak_ms"`
	// Occurrences is how many time windows this pattern was detected in.
	Occurrences int `json:"occurrences"`
	// ImpactScore ranks the pattern: higher = more damaging. Computed as
	// total_ms × occurrences / 1000 (normalized to seconds of wasted time).
	ImpactScore float64 `json:"impact_score"`
	// Severity categorizes the impact: "critical", "high", "medium", "low".
	Severity string `json:"severity"`
	// Suggestion is an actionable fix recommendation.
	Suggestion string `json:"suggestion"`
	// FirstSeen is the earliest detection window.
	FirstSeen time.Time `json:"first_seen"`
	// LastSeen is the most recent detection window.
	LastSeen time.Time `json:"last_seen"`
}

// N1Summary provides aggregate statistics across all detected patterns.
type N1Summary struct {
	// TotalPatterns is the number of distinct model:method patterns.
	TotalPatterns int `json:"total_patterns"`
	// TotalWastedMS is the cumulative time spent in N+1 queries.
	TotalWastedMS int `json:"total_wasted_ms"`
	// CriticalCount is patterns with severity "critical".
	CriticalCount int `json:"critical_count"`
	// HighCount is patterns with severity "high".
	HighCount int `json:"high_count"`
	// TopModel is the model with the most N+1 impact.
	TopModel string `json:"top_model,omitempty"`
	// TopMethod is the method with the most N+1 impact.
	TopMethod string `json:"top_method,omitempty"`
}

// N1DetectionResponse is the full N+1 analysis result.
type N1DetectionResponse struct {
	Patterns []N1Pattern `json:"patterns"`
	Summary  N1Summary   `json:"summary"`
}

// N1TimelinePoint is a single data point for N+1 trend visualization.
type N1TimelinePoint struct {
	Period       time.Time `json:"period"`
	PatternCount int       `json:"pattern_count"`
	TotalCalls   int       `json:"total_calls"`
	TotalMS      int       `json:"total_ms"`
}

// N1TimelineResponse wraps the timeline data.
type N1TimelineResponse struct {
	Points []N1TimelinePoint `json:"points"`
}

// N1RecordingPattern is an N+1 pattern extracted from a profiler recording.
type N1RecordingPattern struct {
	RecordingID   string    `json:"recording_id"`
	RecordingName string    `json:"recording_name"`
	Model         string    `json:"model"`
	Method        string    `json:"method"`
	SQL           string    `json:"sql,omitempty"`
	Count         int       `json:"count"`
	TotalMS       int       `json:"total_ms"`
	RecordedAt    time.Time `json:"recorded_at"`
}
