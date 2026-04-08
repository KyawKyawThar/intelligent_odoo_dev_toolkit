package dto

import (
	"time"

	"github.com/google/uuid"
)

// OverviewResponse is the top-level dashboard summary for an environment.
type OverviewResponse struct {
	Agent    AgentOverview    `json:"agent"`
	Errors   ErrorsOverview   `json:"errors"`
	Profiler ProfilerOverview `json:"profiler"`
	N1       N1Overview       `json:"n1"`
	Alerts   AlertsOverview   `json:"alerts"`
	Budgets  BudgetsOverview  `json:"budgets"`
}

// AgentOverview summarizes agent connectivity.
type AgentOverview struct {
	// Status is the latest heartbeat status ("healthy", "degraded", "offline", "unknown").
	Status string `json:"status"`
	// LastHeartbeatAt is the time of the most recent heartbeat (nil if never seen).
	LastHeartbeatAt *time.Time `json:"last_heartbeat_at,omitempty"`
}

// ErrorsOverview summarizes error group counts.
type ErrorsOverview struct {
	// Total is the total number of error groups for this environment.
	Total int64 `json:"total"`
	// Open is the count of error groups still in "open" status.
	Open int64 `json:"open"`
}

// ProfilerOverview summarizes profiler recording activity.
type ProfilerOverview struct {
	// TotalRecordings is the total number of profiler recordings.
	TotalRecordings int64 `json:"total_recordings"`
	// WithComputeChain is the count of recordings that contain compute-chain data.
	WithComputeChain int64 `json:"with_compute_chain"`
}

// N1Overview summarizes N+1 detection results over the last 24 hours.
type N1Overview struct {
	// PatternsDetected is the number of distinct N+1 patterns found in the last 24h.
	PatternsDetected int `json:"patterns_detected"`
	// CriticalCount is the number of patterns classified as critical severity.
	CriticalCount int `json:"critical_count"`
}

// AlertsOverview summarizes alert activity.
type AlertsOverview struct {
	// Unacknowledged is the number of alerts not yet acknowledged.
	Unacknowledged int64 `json:"unacknowledged"`
}

// BudgetsOverview summarizes performance budget configuration.
type BudgetsOverview struct {
	// Total is the total number of configured budgets (active + inactive).
	Total int `json:"total"`
	// Active is the number of currently active budgets.
	Active int `json:"active"`
}

// ChainRecordingItem is a lightweight list entry for the compute-chain page.
type ChainRecordingItem struct {
	ID           uuid.UUID     `json:"id"`
	EnvID        uuid.UUID     `json:"env_id"`
	TriggeredBy  *uuid.UUID    `json:"triggered_by,omitempty"`
	Name         string        `json:"name"`
	Endpoint     *string       `json:"endpoint,omitempty"`
	TotalMS      int32         `json:"total_ms"`
	RecordedAt   time.Time     `json:"recorded_at"`
	ComputeChain *ComputeChain `json:"compute_chain"`
}

// ChainRecordingListResponse is the paginated list for the chain page.
type ChainRecordingListResponse struct {
	Recordings []ChainRecordingItem `json:"recordings"`
	Total      int64                `json:"total"`
}

// ChainResponse returns just the compute-chain portion of a recording.
type ChainResponse struct {
	RecordingID  uuid.UUID     `json:"recording_id"`
	Name         string        `json:"name"`
	RecordedAt   time.Time     `json:"recorded_at"`
	ComputeChain *ComputeChain `json:"compute_chain"`
}
