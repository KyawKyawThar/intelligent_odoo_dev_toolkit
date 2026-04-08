package dto

import (
	"time"

	"github.com/google/uuid"
)

// ─── Request DTOs ────────────────────────────────────────────────────────────

// RunMigrationScanRequest is the body for POST /migration/scan.
type RunMigrationScanRequest struct {
	// FromVersion is the current Odoo version, e.g. "16.0".
	FromVersion string `json:"from_version" validate:"required"`
	// ToVersion is the target Odoo version, e.g. "17.0".
	ToVersion string `json:"to_version" validate:"required"`
	// SnapshotID optionally pins the scan to a specific schema snapshot UUID.
	// When omitted the latest snapshot for the environment is used.
	SnapshotID *uuid.UUID `json:"snapshot_id,omitempty"`
}

// ListMigrationScansRequest carries pagination params for GET /migration/scans.
type ListMigrationScansRequest struct {
	Limit  int32 `json:"limit"`
	Offset int32 `json:"offset"`
}

// ─── Response DTOs ───────────────────────────────────────────────────────────

// MigrationIssue is one detected deprecation finding.
type MigrationIssue struct {
	// Severity is "breaking", "warning", or "minor".
	Severity string `json:"severity"`
	// Kind is the type of change: "model_removed", "field_removed", etc.
	Kind string `json:"kind"`
	// Model is the Odoo technical model name.
	Model string `json:"model,omitempty"`
	// Field is the affected field name.
	Field string `json:"field,omitempty"`
	// OldName / NewName for rename issues.
	OldName string `json:"old_name,omitempty"`
	NewName string `json:"new_name,omitempty"`
	// Message is a human-readable summary.
	Message string `json:"message"`
	// Fix is the recommended remediation.
	Fix string `json:"fix,omitempty"`
	// FromVersion / ToVersion shows which transition this issue belongs to.
	FromVersion string `json:"from_version"`
	ToVersion   string `json:"to_version"`
}

// MigrationScanSummary aggregates counts across all issues in a scan.
type MigrationScanSummary struct {
	// BreakingCount is the number of breaking changes detected.
	BreakingCount int `json:"breaking_count"`
	// WarningCount is the number of warnings detected.
	WarningCount int `json:"warning_count"`
	// MinorCount is the number of minor deprecations.
	MinorCount int `json:"minor_count"`
	// TotalIssues is the total number of issues.
	TotalIssues int `json:"total_issues"`
	// FromVersion / ToVersion of the scan path.
	FromVersion string `json:"from_version"`
	ToVersion   string `json:"to_version"`
}

// MigrationScanResponse is returned by POST /migration/scan and
// GET /migration/scans/{scan_id}.
type MigrationScanResponse struct {
	ID          uuid.UUID            `json:"id"`
	EnvID       uuid.UUID            `json:"env_id"`
	SnapshotID  *uuid.UUID           `json:"snapshot_id,omitempty"`
	FromVersion string               `json:"from_version"`
	ToVersion   string               `json:"to_version"`
	Status      string               `json:"status"`
	Issues      []MigrationIssue     `json:"issues"`
	Summary     MigrationScanSummary `json:"summary"`
	ScannedAt   time.Time            `json:"scanned_at"`
}

// MigrationScanListItem is the lightweight row returned in list responses.
type MigrationScanListItem struct {
	ID            uuid.UUID `json:"id"`
	EnvID         uuid.UUID `json:"env_id"`
	FromVersion   string    `json:"from_version"`
	ToVersion     string    `json:"to_version"`
	Status        string    `json:"status"`
	BreakingCount int       `json:"breaking_count"`
	WarningCount  int       `json:"warning_count"`
	MinorCount    int       `json:"minor_count"`
	ScannedAt     time.Time `json:"scanned_at"`
}

// MigrationScanListResponse wraps a paginated list of scan records.
type MigrationScanListResponse struct {
	Scans  []MigrationScanListItem `json:"scans"`
	Total  int64                   `json:"total"`
	Limit  int32                   `json:"limit"`
	Offset int32                   `json:"offset"`
}

// SupportedTransitionsResponse lists the version pairs the deprecation DB covers.
type SupportedTransitionsResponse struct {
	Transitions []VersionTransition `json:"transitions"`
}

// VersionTransition describes one supported upgrade path.
type VersionTransition struct {
	FromVersion string `json:"from_version"`
	ToVersion   string `json:"to_version"`
	RuleCount   int    `json:"rule_count"`
}

// ─── Source-code scan DTOs ────────────────────────────────────────────────────

// ScanSourceRequest is the body for POST /migration/scan/source.
// Clients submit the raw text content of each Odoo module file they want
// scanned. Only .py and .xml files are analyzed; others are ignored.
type ScanSourceRequest struct {
	// FromVersion is the current Odoo version, e.g. "16.0".
	FromVersion string `json:"from_version" validate:"required"`
	// ToVersion is the target Odoo version, e.g. "17.0".
	ToVersion string `json:"to_version" validate:"required"`
	// Files maps relative file path → UTF-8 file content.
	// Maximum 200 files, 512 KB each.
	Files map[string]string `json:"files" validate:"required,min=1"`
}

// CodeFinding is one deprecated-pattern hit inside a source file.
type CodeFinding struct {
	// File is the relative path as provided in the request.
	File string `json:"file"`
	// Line is the 1-based line number.
	Line int `json:"line"`
	// Snippet is the matching line (trimmed, ≤ 200 chars).
	Snippet string `json:"snippet"`

	Severity    string `json:"severity"`
	Kind        string `json:"kind"`
	Model       string `json:"model,omitempty"`
	Field       string `json:"field,omitempty"`
	OldName     string `json:"old_name,omitempty"`
	NewName     string `json:"new_name,omitempty"`
	Message     string `json:"message"`
	Fix         string `json:"fix,omitempty"`
	FromVersion string `json:"from_version"`
	ToVersion   string `json:"to_version"`
}

// CodeScanSummary aggregates finding counts.
type CodeScanSummary struct {
	BreakingCount int `json:"breaking_count"`
	WarningCount  int `json:"warning_count"`
	MinorCount    int `json:"minor_count"`
	TotalFindings int `json:"total_findings"`
	FilesScanned  int `json:"files_scanned"`
}

// ScanSourceResponse is returned by POST /migration/scan/source.
type ScanSourceResponse struct {
	FromVersion string          `json:"from_version"`
	ToVersion   string          `json:"to_version"`
	Findings    []CodeFinding   `json:"findings"`
	Summary     CodeScanSummary `json:"summary"`
}
