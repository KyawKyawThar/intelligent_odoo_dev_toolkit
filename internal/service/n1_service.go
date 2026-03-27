package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"

	"github.com/google/uuid"
)

// Shared category and severity constants used across service files.
const (
	catSQL    = "sql"
	catORM    = "orm"
	catPython = "python"
	catRPC    = "rpc"
	catHTTP   = "http"

	sevCritical = "critical"
	sevHigh     = "high"
	sevMedium   = "medium"
	sevLow      = "low"

	methodRead = "read"
)

// N1Service implements the N1Servicer interface.
type N1Service struct {
	store db.Store
}

// NewN1Service creates a new N1Service.
func NewN1Service(store db.Store) *N1Service {
	return &N1Service{store: store}
}

// Detect runs the full server-side N+1 analysis pipeline:
//  1. Queries orm_stats for aggregated N+1 patterns by model:method
//  2. Enriches with profiler recording N+1 data (SQL signatures, counts)
//  3. Scores each pattern by impact
//  4. Generates actionable fix suggestions
func (s *N1Service) Detect(
	ctx context.Context,
	tenantID, envID uuid.UUID,
	req *dto.N1DetectionRequest,
) (*dto.N1DetectionResponse, error) {
	// Verify environment belongs to tenant.
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	since := req.Since
	if since.IsZero() {
		since = time.Now().UTC().Add(-24 * time.Hour)
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}

	// 1. Get aggregated N+1 patterns from orm_stats.
	rows, err := s.store.GetN1PatternsSummary(ctx, db.GetN1PatternsSummaryParams{
		EnvID:  envID,
		Period: since,
		Limit:  limit,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	// 2. Enrich with profiler recording data (SQL signatures).
	recordingPatterns, err := s.getRecordingPatterns(ctx, envID, since)
	if err != nil {
		// Non-fatal: recording data is supplementary.
		recordingPatterns = make(map[string]recordingEnrichment)
	}

	// 3. Build patterns with scoring and suggestions.
	patterns := make([]dto.N1Pattern, 0, len(rows))
	for _, row := range rows {
		pattern := s.buildPattern(row, recordingPatterns)
		patterns = append(patterns, pattern)
	}

	// 4. Build summary.
	summary := buildN1Summary(patterns)

	return &dto.N1DetectionResponse{
		Patterns: patterns,
		Summary:  summary,
	}, nil
}

// GetTimeline returns per-period N+1 trend data for visualization.
func (s *N1Service) GetTimeline(
	ctx context.Context,
	tenantID, envID uuid.UUID,
	since time.Time,
	limit int32,
) ([]dto.N1TimelinePoint, error) {
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	if since.IsZero() {
		since = time.Now().UTC().Add(-24 * time.Hour)
	}
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.store.GetN1Timeline(ctx, db.GetN1TimelineParams{
		EnvID:  envID,
		Period: since,
		Limit:  limit,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	points := make([]dto.N1TimelinePoint, 0, len(rows))
	for _, row := range rows {
		points = append(points, dto.N1TimelinePoint{
			Period:       row.Period,
			PatternCount: int(row.PatternCount),
			TotalCalls:   int(row.TotalCalls),
			TotalMS:      int(row.TotalMs),
		})
	}

	return points, nil
}

// ─── Pattern Building ───────────────────────────────────────────────────────

// recordingEnrichment holds supplementary data from profiler recordings.
type recordingEnrichment struct {
	signature string
	sampleSQL string
	count     int
	totalMS   int
}

// getRecordingPatterns extracts N+1 patterns from profiler recordings' JSONB.
func (s *N1Service) getRecordingPatterns(
	ctx context.Context,
	envID uuid.UUID,
	since time.Time,
) (map[string]recordingEnrichment, error) {
	rows, err := s.store.ListN1RecordingPatterns(ctx, db.ListN1RecordingPatternsParams{
		EnvID:      envID,
		RecordedAt: since,
		Limit:      100,
	})
	if err != nil {
		return nil, err
	}

	result := make(map[string]recordingEnrichment)
	for _, row := range rows {
		if row.N1Patterns == nil {
			continue
		}
		var patterns []n1Pattern
		if err := json.Unmarshal(*row.N1Patterns, &patterns); err != nil {
			continue
		}
		for _, p := range patterns {
			key := p.Model + ":" + p.Method
			existing, found := result[key]
			if !found {
				result[key] = recordingEnrichment{
					signature: NormalizeSQL(p.SQL),
					sampleSQL: p.SQL,
					count:     p.Count,
					totalMS:   p.TotalMS,
				}
			} else {
				existing.count += p.Count
				existing.totalMS += p.TotalMS
				if existing.sampleSQL == "" && p.SQL != "" {
					existing.sampleSQL = p.SQL
					existing.signature = NormalizeSQL(p.SQL)
				}
				result[key] = existing
			}
		}
	}

	return result, nil
}

// buildPattern converts a DB row + enrichment data into an N1Pattern DTO.
func (s *N1Service) buildPattern(
	row db.GetN1PatternsSummaryRow,
	enrichments map[string]recordingEnrichment,
) dto.N1Pattern {
	key := row.Model + ":" + row.Method

	pattern := dto.N1Pattern{
		Model:             row.Model,
		Method:            row.Method,
		TotalCalls:        int(row.TotalCalls),
		TotalMS:           int(row.TotalDurationMs),
		AvgCallsPerWindow: math.Round(row.AvgCallsPerWindow*100) / 100,
		PeakCalls:         int(row.PeakCalls),
		PeakMS:            int(row.PeakMs),
		Occurrences:       int(row.Occurrences),
		FirstSeen:         row.FirstSeen,
		LastSeen:          row.LastSeen,
		SampleSQL:         row.SampleSql,
	}

	// Enrich with profiler recording data.
	if enrich, found := enrichments[key]; found {
		if pattern.SampleSQL == "" {
			pattern.SampleSQL = enrich.sampleSQL
		}
		pattern.Signature = enrich.signature
	} else if pattern.SampleSQL != "" {
		pattern.Signature = NormalizeSQL(pattern.SampleSQL)
	}

	// Score and classify.
	pattern.ImpactScore = ComputeImpactScore(pattern.TotalMS, pattern.Occurrences, pattern.PeakCalls)
	pattern.Severity = ClassifySeverity(pattern.ImpactScore)
	pattern.Suggestion = GenerateN1Suggestion(pattern.Model, pattern.Method, pattern.PeakCalls)

	return pattern
}

// ─── Impact Scoring ─────────────────────────────────────────────────────────

// ComputeImpactScore calculates a normalized impact score.
// Formula: (total_ms_wasted × occurrences × peak_factor) / 1000
// Higher score = more damaging pattern.
func ComputeImpactScore(totalMS, occurrences, peakCalls int) float64 {
	if totalMS == 0 || occurrences == 0 {
		return 0
	}
	peakFactor := 1.0
	if peakCalls > 100 {
		peakFactor = 2.0
	} else if peakCalls > 50 {
		peakFactor = 1.5
	}
	score := float64(totalMS) * float64(occurrences) * peakFactor / 1000.0
	return math.Round(score*100) / 100
}

// ClassifySeverity returns a severity string based on impact score.
func ClassifySeverity(score float64) string {
	switch {
	case score >= 100:
		return sevCritical
	case score >= 30:
		return sevHigh
	case score >= 5:
		return sevMedium
	default:
		return sevLow
	}
}

// ─── Fix Suggestions ────────────────────────────────────────────────────────

// GenerateN1Suggestion produces an actionable fix recommendation based on
// the Odoo model:method pattern and call volume.
func GenerateN1Suggestion(model, method string, peakCalls int) string {
	switch method {
	case methodRead, "browse":
		return fmt.Sprintf(
			"Use prefetch_fields or batch read: env['%s'].browse(ids).read([fields]) instead of individual reads. "+
				"Peak: %d calls in one window.",
			model, peakCalls,
		)
	case "search_read":
		return fmt.Sprintf(
			"Combine search+read into a single search_read with domain filter. "+
				"If looping, collect IDs first then batch-read. Peak: %d calls.",
			peakCalls,
		)
	case "search", "search_count":
		return fmt.Sprintf(
			"Cache search results or use a single search with a broader domain. "+
				"Avoid calling search inside loops. Peak: %d calls.",
			peakCalls,
		)
	case "write":
		return fmt.Sprintf(
			"Batch writes: collect vals dicts and call write once on a recordset. "+
				"env['%s'].browse(ids).write(vals). Peak: %d calls.",
			model, peakCalls,
		)
	case "create":
		return fmt.Sprintf(
			"Batch creates: pass a list of dicts to env['%s'].create([vals_list]). "+
				"Peak: %d calls.",
			model, peakCalls,
		)
	case "unlink":
		return fmt.Sprintf(
			"Batch deletes: env['%s'].browse(ids).unlink(). Peak: %d calls.",
			model, peakCalls,
		)
	case "name_get":
		return fmt.Sprintf(
			"Override _compute_display_name or use read(['display_name']) on the recordset. "+
				"Peak: %d calls.",
			peakCalls,
		)
	default:
		return fmt.Sprintf(
			"Review %s.%s for loop-based calls. Consider batch operations or caching. "+
				"Peak: %d calls in one window.",
			model, method, peakCalls,
		)
	}
}

// ─── SQL Normalization (mirrors agent-side logic) ───────────────────────────

// normalizeSQLRe replaces literal values with ? for signature matching.
var normalizeSQLRe = regexp.MustCompile(
	`(?:` +
		`'[^']*'` + // single-quoted strings
		`|` +
		`\b\d+\b` + // integers
		`)`,
)

// NormalizeSQL replaces literal values with '?' to produce a query signature.
// This mirrors the agent-side normalizeSQL function.
func NormalizeSQL(sql string) string {
	s := strings.TrimSpace(sql)
	if s == "" {
		return ""
	}
	return normalizeSQLRe.ReplaceAllString(s, "?")
}

// ─── Summary Builder ────────────────────────────────────────────────────────

func buildN1Summary(patterns []dto.N1Pattern) dto.N1Summary {
	summary := dto.N1Summary{
		TotalPatterns: len(patterns),
	}

	var topScore float64
	for _, p := range patterns {
		summary.TotalWastedMS += p.TotalMS

		switch p.Severity {
		case sevCritical:
			summary.CriticalCount++
		case sevHigh:
			summary.HighCount++
		}

		if p.ImpactScore > topScore {
			topScore = p.ImpactScore
			summary.TopModel = p.Model
			summary.TopMethod = p.Method
		}
	}

	return summary
}
