package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"

	"github.com/google/uuid"
)

// ProfilerService implements the ProfilerServicer interface.
type ProfilerService struct {
	store db.Store
}

// NewProfilerService creates a new ProfilerService.
func NewProfilerService(store db.Store) *ProfilerService {
	return &ProfilerService{store: store}
}

// GetRecording retrieves a single profiler recording and builds its waterfall.
func (s *ProfilerService) GetRecording(
	ctx context.Context,
	tenantID, envID, recordingID uuid.UUID,
) (*dto.ProfilerRecordingResponse, error) {
	// Verify environment belongs to tenant.
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	rec, err := s.store.GetProfilerRecording(ctx, db.GetProfilerRecordingParams{
		ID:    recordingID,
		EnvID: envID,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	waterfall, err := BuildWaterfall(rec.Waterfall, rec.TotalMs, rec.N1Patterns)
	if err != nil {
		return nil, api.ErrInternal(fmt.Errorf("build waterfall: %w", err))
	}

	computeChain := BuildComputeChain(rec.ComputeChain)

	return &dto.ProfilerRecordingResponse{
		ID:           rec.ID,
		EnvID:        rec.EnvID,
		TriggeredBy:  rec.TriggeredBy,
		Name:         rec.Name,
		Endpoint:     rec.Endpoint,
		TotalMS:      rec.TotalMs,
		SQLCount:     rec.SqlCount,
		SQLMS:        rec.SqlMs,
		PythonMS:     rec.PythonMs,
		Waterfall:    waterfall,
		ComputeChain: computeChain,
		RecordedAt:   rec.RecordedAt,
	}, nil
}

// ListRecordings lists profiler recordings for an environment.
func (s *ProfilerService) ListRecordings(
	ctx context.Context,
	tenantID, envID uuid.UUID,
	req *dto.ListProfilerRecordingsRequest,
) (*dto.ProfilerRecordingListResponse, error) {
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	rows, err := s.store.ListProfilerRecordings(ctx, db.ListProfilerRecordingsParams{
		EnvID:  envID,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	total, err := s.store.CountRecordingsByEnv(ctx, envID)
	if err != nil {
		return nil, api.FromPgError(err)
	}

	items := make([]dto.ProfilerRecordingListItem, 0, len(rows))
	for _, row := range rows {
		hasN1, ok := row.HasN1.(bool)
		if !ok {
			hasN1 = false
		}
		hasComputeChain, ok := row.HasComputeChain.(bool)
		if !ok {
			hasComputeChain = false
		}
		items = append(items, dto.ProfilerRecordingListItem{
			ID:              row.ID,
			EnvID:           row.EnvID,
			TriggeredBy:     row.TriggeredBy,
			Name:            row.Name,
			Endpoint:        row.Endpoint,
			TotalMS:         row.TotalMs,
			SQLCount:        row.SqlCount,
			SQLMS:           row.SqlMs,
			PythonMS:        row.PythonMs,
			HasN1:           hasN1,
			HasComputeChain: hasComputeChain,
			RecordedAt:      row.RecordedAt,
		})
	}

	return &dto.ProfilerRecordingListResponse{
		Recordings: items,
		Total:      total,
	}, nil
}

// ListSlowRecordings lists recordings exceeding a duration threshold.
func (s *ProfilerService) ListSlowRecordings(
	ctx context.Context,
	tenantID, envID uuid.UUID,
	req *dto.ListSlowRecordingsRequest,
) (*dto.ProfilerRecordingListResponse, error) {
	if _, err := s.store.GetEnvironmentByID(ctx, db.GetEnvironmentByIDParams{
		ID:       envID,
		TenantID: tenantID,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.store.ListSlowRecordings(ctx, db.ListSlowRecordingsParams{
		EnvID:   envID,
		TotalMs: req.ThresholdMS,
		Limit:   limit,
	})
	if err != nil {
		return nil, api.FromPgError(err)
	}

	items := make([]dto.ProfilerRecordingListItem, 0, len(rows))
	for _, row := range rows {
		hasN1 := row.N1Patterns != nil
		items = append(items, dto.ProfilerRecordingListItem{
			ID:          row.ID,
			EnvID:       row.EnvID,
			TriggeredBy: row.TriggeredBy,
			Name:        row.Name,
			Endpoint:    row.Endpoint,
			TotalMS:     row.TotalMs,
			SQLCount:    row.SqlCount,
			SQLMS:       row.SqlMs,
			PythonMS:    row.PythonMs,
			HasN1:       hasN1,
			RecordedAt:  row.RecordedAt,
		})
	}

	return &dto.ProfilerRecordingListResponse{
		Recordings: items,
		Total:      int64(len(items)),
	}, nil
}

// ─── Waterfall Builder ──────────────────────────────────────────────────────

// rawSpan is the shape of individual span entries stored in the waterfall JSONB.
type rawSpan struct {
	Label      string `json:"label"`
	Category   string `json:"category,omitempty"`
	MS         int    `json:"ms"`
	StartMS    int    `json:"start_ms,omitempty"`
	Model      string `json:"model,omitempty"`
	Method     string `json:"method,omitempty"`
	Module     string `json:"module,omitempty"`
	SQL        string `json:"sql,omitempty"`
	IsN1       bool   `json:"is_n1,omitempty"`
	IsError    bool   `json:"is_error,omitempty"`
	ParentID   string `json:"parent_id,omitempty"`
	DurationMS int    `json:"duration_ms,omitempty"`
}

// n1Pattern is the shape of entries in the n1_patterns JSONB column.
type n1Pattern struct {
	Model   string `json:"model"`
	Method  string `json:"method"`
	SQL     string `json:"sql,omitempty"`
	Count   int    `json:"count"`
	TotalMS int    `json:"total_ms"`
}

// BuildWaterfall constructs a Waterfall from the raw JSONB waterfall data
// stored in profiler_recordings. It handles two formats:
//   - Simple: [{"label":"ORM","ms":100},{"label":"SQL","ms":500}]
//   - Detailed: spans with start_ms, category, model, method, etc.
//
// When detailed spans have start_ms offsets, they are arranged as a true
// timeline. When only simple label+ms pairs exist, the builder synthesizes
// sequential offsets to create a visual timeline.
func BuildWaterfall(waterfallJSON json.RawMessage, totalMS int32, n1JSON *json.RawMessage) (*dto.Waterfall, error) {
	if len(waterfallJSON) == 0 || string(waterfallJSON) == "null" {
		return emptyWaterfall(totalMS), nil
	}

	var rawSpans []rawSpan
	if err := json.Unmarshal(waterfallJSON, &rawSpans); err != nil {
		return emptyWaterfall(totalMS), nil //nolint:nilerr // If waterfall JSON is malformed, gracefully return an empty waterfall.
	}

	if len(rawSpans) == 0 {
		return emptyWaterfall(totalMS), nil
	}

	// Parse N+1 patterns for cross-referencing.
	n1Map := parseN1Patterns(n1JSON)

	// Determine if spans have explicit timing (detailed format).
	hasExplicitTiming := false
	for _, rs := range rawSpans {
		if rs.StartMS > 0 || rs.Category != "" {
			hasExplicitTiming = true
			break
		}
	}

	var spans []dto.WaterfallSpan
	if hasExplicitTiming {
		spans = buildDetailedSpans(rawSpans, n1Map)
	} else {
		spans = buildSimpleSpans(rawSpans, n1Map, totalMS)
	}

	// Sort spans by start time, then by depth.
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].StartMS != spans[j].StartMS {
			return spans[i].StartMS < spans[j].StartMS
		}
		return spans[i].Depth < spans[j].Depth
	})

	lanes := buildLanes(spans, totalMS)
	summary := buildSummary(spans, totalMS)

	return &dto.Waterfall{
		Spans:   spans,
		Lanes:   lanes,
		Summary: summary,
	}, nil
}

// buildDetailedSpans processes spans that have explicit start_ms and category.
func buildDetailedSpans(rawSpans []rawSpan, n1Map map[string]n1Pattern) []dto.WaterfallSpan {
	spans := make([]dto.WaterfallSpan, 0, len(rawSpans))

	for i, rs := range rawSpans {
		category := rs.Category
		if category == "" {
			category = inferCategory(rs.Label)
		}

		duration := rs.DurationMS
		if duration == 0 {
			duration = rs.MS
		}

		isN1 := rs.IsN1
		if !isN1 {
			key := rs.Model + ":" + rs.Method
			if _, found := n1Map[key]; found {
				isN1 = true
			}
		}

		depth := 0
		if rs.ParentID != "" {
			depth = 1
		}

		spans = append(spans, dto.WaterfallSpan{
			ID:         fmt.Sprintf("span-%d", i),
			Category:   category,
			Label:      buildLabel(rs),
			StartMS:    rs.StartMS,
			DurationMS: duration,
			Module:     rs.Module,
			Model:      rs.Model,
			Method:     rs.Method,
			SQL:        truncateSQL(rs.SQL),
			IsN1:       isN1,
			IsError:    rs.IsError,
			ParentID:   rs.ParentID,
			Depth:      depth,
		})
	}

	return spans
}

// buildSimpleSpans converts simple label+ms entries into sequential spans.
func buildSimpleSpans(rawSpans []rawSpan, n1Map map[string]n1Pattern, totalMS int32) []dto.WaterfallSpan {
	spans := make([]dto.WaterfallSpan, 0, len(rawSpans))
	offset := 0

	for i, rs := range rawSpans {
		category := inferCategory(rs.Label)
		duration := rs.MS
		if duration == 0 {
			duration = rs.DurationMS
		}

		isN1 := rs.IsN1
		if !isN1 {
			key := rs.Model + ":" + rs.Method
			if _, found := n1Map[key]; found {
				isN1 = true
			}
		}

		spans = append(spans, dto.WaterfallSpan{
			ID:         fmt.Sprintf("span-%d", i),
			Category:   category,
			Label:      buildLabel(rs),
			StartMS:    offset,
			DurationMS: duration,
			Module:     rs.Module,
			Model:      rs.Model,
			Method:     rs.Method,
			SQL:        truncateSQL(rs.SQL),
			IsN1:       isN1,
			IsError:    rs.IsError,
			Depth:      0,
		})

		offset += duration
	}

	return spans
}

// buildLanes computes per-category aggregated lanes for stacked bar rendering.
func buildLanes(spans []dto.WaterfallSpan, totalMS int32) []dto.WaterfallLane {
	type laneAcc struct {
		label   string
		totalMS int
	}
	acc := make(map[string]*laneAcc)
	order := []string{}

	for _, sp := range spans {
		cat := sp.Category
		if _, exists := acc[cat]; !exists {
			acc[cat] = &laneAcc{label: categoryLabel(cat)}
			order = append(order, cat)
		}
		acc[cat].totalMS += sp.DurationMS
	}

	lanes := make([]dto.WaterfallLane, 0, len(order))
	total := float64(totalMS)
	if total == 0 {
		total = 1
	}

	for _, cat := range order {
		a := acc[cat]
		pct := math.Round(float64(a.totalMS)/total*10000) / 100
		lanes = append(lanes, dto.WaterfallLane{
			Category: cat,
			Label:    a.label,
			TotalMS:  a.totalMS,
			Pct:      pct,
		})
	}

	// Sort by duration descending.
	sort.Slice(lanes, func(i, j int) bool {
		return lanes[i].TotalMS > lanes[j].TotalMS
	})

	return lanes
}

// buildSummary aggregates span statistics.
func buildSummary(spans []dto.WaterfallSpan, totalMS int32) dto.WaterfallSummary {
	s := dto.WaterfallSummary{
		TotalMS:   int(totalMS),
		SpanCount: len(spans),
	}

	var maxSQLDuration int
	var criticalSQL string

	for _, sp := range spans {
		switch sp.Category {
		case catSQL:
			s.SQLMS += sp.DurationMS
			s.SQLCount++
			if sp.DurationMS > maxSQLDuration {
				maxSQLDuration = sp.DurationMS
				criticalSQL = sp.SQL
			}
		case catORM:
			s.ORMMS += sp.DurationMS
			s.ORMCount++
		case catPython:
			s.PythonMS += sp.DurationMS
		}

		if sp.IsN1 {
			s.N1Count++
			s.N1MS += sp.DurationMS
		}
		if sp.IsError {
			s.ErrorCount++
		}
	}

	s.CriticalSQL = criticalSQL
	return s
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func emptyWaterfall(totalMS int32) *dto.Waterfall {
	return &dto.Waterfall{
		Spans: []dto.WaterfallSpan{},
		Lanes: []dto.WaterfallLane{},
		Summary: dto.WaterfallSummary{
			TotalMS: int(totalMS),
		},
	}
}

func parseN1Patterns(n1JSON *json.RawMessage) map[string]n1Pattern {
	m := make(map[string]n1Pattern)
	if n1JSON == nil {
		return m
	}
	var patterns []n1Pattern
	if err := json.Unmarshal(*n1JSON, &patterns); err != nil {
		return m
	}
	for _, p := range patterns {
		key := p.Model + ":" + p.Method
		m[key] = p
	}
	return m
}

func inferCategory(label string) string {
	switch label {
	case "SQL", catSQL:
		return catSQL
	case "ORM", catORM:
		return catORM
	case "Python", catPython:
		return catPython
	case "RPC", catRPC:
		return catRPC
	case "HTTP", catHTTP:
		return catHTTP
	default:
		return catPython
	}
}

func categoryLabel(cat string) string {
	switch cat {
	case catSQL:
		return "SQL Queries"
	case catORM:
		return "ORM Calls"
	case catPython:
		return "Python Execution"
	case catRPC:
		return "RPC Calls"
	case catHTTP:
		return "HTTP Requests"
	default:
		return cat
	}
}

func buildLabel(rs rawSpan) string {
	if rs.Label != "" {
		return rs.Label
	}
	if rs.Model != "" && rs.Method != "" {
		return rs.Model + "." + rs.Method
	}
	if rs.Model != "" {
		return rs.Model
	}
	return rs.Category
}

func truncateSQL(sql string) string {
	if len(sql) <= 200 {
		return sql
	}
	return sql[:200] + "..."
}

// ─── Waterfall Builder from Raw Events ──────────────────────────────────────

// BuildWaterfallFromEvents constructs a waterfall JSONB payload from raw
// profiler events received during batch ingestion. This is used by the
// ingest worker to build the initial waterfall when storing a recording.
func BuildWaterfallFromEvents(events []ProfilerEvent) (waterfallJSON json.RawMessage, n1JSON *json.RawMessage, meta WaterfallMeta) {
	if len(events) == 0 {
		empty, err := json.Marshal([]rawSpan{})
		if err != nil {
			return []byte("[]"), nil, WaterfallMeta{}
		}
		return empty, nil, WaterfallMeta{}
	}

	// Sort events by timestamp.
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})

	baseTime := events[0].Timestamp
	spans := make([]rawSpan, 0, len(events))
	n1Patterns := make(map[string]*n1Pattern)

	for _, ev := range events {
		offsetMS := int(ev.Timestamp.Sub(baseTime).Milliseconds())

		category := ev.Category
		if category == "" {
			category = catPython
		}

		span := rawSpan{
			Label:    buildEventLabel(ev),
			Category: category,
			MS:       ev.DurationMS,
			StartMS:  offsetMS,
			Model:    ev.Model,
			Method:   ev.Method,
			Module:   ev.Module,
			SQL:      truncateSQL(ev.SQL),
			IsN1:     ev.IsN1,
			IsError:  ev.IsError,
		}
		spans = append(spans, span)

		// Accumulate metadata.
		meta.TotalMS += ev.DurationMS
		switch category {
		case catSQL:
			meta.SQLCount++
			meta.SQLMS += ev.DurationMS
		case catPython:
			meta.PythonMS += ev.DurationMS
		}

		if ev.IsN1 {
			key := ev.Model + ":" + ev.Method
			if p, exists := n1Patterns[key]; exists {
				p.Count++
				p.TotalMS += ev.DurationMS
			} else {
				n1Patterns[key] = &n1Pattern{
					Model:   ev.Model,
					Method:  ev.Method,
					SQL:     ev.SQL,
					Count:   1,
					TotalMS: ev.DurationMS,
				}
			}
		}
	}

	var err error
	waterfallJSON, err = json.Marshal(spans)
	if err != nil {
		waterfallJSON = []byte("[]")
	}

	if len(n1Patterns) > 0 {
		patterns := make([]n1Pattern, 0, len(n1Patterns))
		for _, p := range n1Patterns {
			patterns = append(patterns, *p)
		}
		raw, err := json.Marshal(patterns)
		if err == nil {
			n1Raw := json.RawMessage(raw)
			n1JSON = &n1Raw
		}
	}

	return waterfallJSON, n1JSON, meta
}

// ProfilerEvent is the input for waterfall building from raw agent events.
type ProfilerEvent struct {
	Category     string
	Model        string
	Method       string
	DurationMS   int
	IsError      bool
	IsN1         bool
	SQL          string
	Module       string
	Traceback    string
	UserID       int
	Timestamp    time.Time
	FieldName    string
	IsCompute    bool
	DependsOn    []string
	TriggerField string
}

// WaterfallMeta holds computed aggregate values for the profiler recording row.
type WaterfallMeta struct {
	TotalMS  int
	SQLCount int
	SQLMS    int
	PythonMS int
}

// time is needed for ProfilerEvent and imported at the top.
// This import is unused comment — the time package is imported via the
// aggregator.Event → ProfilerEvent.Timestamp field.

// ─── Compute Chain Builder ───────────────────────────────────────────────────

// rawComputeNode is the shape stored in the compute_chain JSONB column.
type rawComputeNode struct {
	Model        string   `json:"model"`
	FieldName    string   `json:"field_name"`
	Method       string   `json:"method,omitempty"`
	Module       string   `json:"module,omitempty"`
	DurationMS   int      `json:"duration_ms"`
	DependsOn    []string `json:"depends_on,omitempty"`
	TriggerField string   `json:"trigger_field,omitempty"`
	SQLCount     int      `json:"sql_count,omitempty"`
	Depth        int      `json:"depth"`
	ParentID     string   `json:"parent_id,omitempty"`
}

// computeChainStats holds intermediate stats collected while building compute nodes.
type computeChainStats struct {
	totalMS       int
	maxDepth      int
	slowestMS     int
	slowestNodeID string
	bottleneckCnt int
}

// buildComputeNodes converts raw nodes into dto nodes and collects stats.
func buildComputeNodes(rawNodes []rawComputeNode, fieldToNodeID map[string]string) ([]dto.ComputeNode, computeChainStats) {
	// Compute bottleneck threshold: > 2x average, minimum 10ms.
	sumMS := 0
	for _, rn := range rawNodes {
		sumMS += rn.DurationMS
	}
	avgMS := sumMS / len(rawNodes)
	bottleneckThreshold := avgMS * 2
	if bottleneckThreshold < 10 {
		bottleneckThreshold = 10
	}

	nodes := make([]dto.ComputeNode, 0, len(rawNodes))
	var stats computeChainStats

	for i, rn := range rawNodes {
		nodeID := fmt.Sprintf("compute-%d", i)
		fieldToNodeID[rn.Model+"."+rn.FieldName] = nodeID

		isBottleneck := rn.DurationMS >= bottleneckThreshold
		if isBottleneck {
			stats.bottleneckCnt++
		}
		stats.totalMS += rn.DurationMS
		if rn.DurationMS > stats.slowestMS {
			stats.slowestMS = rn.DurationMS
			stats.slowestNodeID = nodeID
		}
		if rn.Depth > stats.maxDepth {
			stats.maxDepth = rn.Depth
		}

		nodes = append(nodes, dto.ComputeNode{
			ID:           nodeID,
			Model:        rn.Model,
			FieldName:    rn.FieldName,
			Method:       rn.Method,
			Module:       rn.Module,
			DurationMS:   rn.DurationMS,
			DependsOn:    rn.DependsOn,
			ParentID:     rn.ParentID,
			Depth:        rn.Depth,
			SQLCount:     rn.SQLCount,
			IsBottleneck: isBottleneck,
		})
	}
	return nodes, stats
}

// buildComputeEdges creates edges from parent/dependency relationships.
func buildComputeEdges(rawNodes []rawComputeNode, fieldToNodeID map[string]string) []dto.ComputeEdge {
	var edges []dto.ComputeEdge
	for i, rn := range rawNodes {
		nodeID := fmt.Sprintf("compute-%d", i)
		if rn.ParentID != "" {
			edges = append(edges, dto.ComputeEdge{
				From: rn.ParentID, To: nodeID, TriggerField: rn.TriggerField,
			})
		}
		for _, dep := range rn.DependsOn {
			if fromID, ok := fieldToNodeID[rn.Model+"."+dep]; ok && fromID != nodeID {
				edges = append(edges, dto.ComputeEdge{
					From: fromID, To: nodeID, TriggerField: dep,
				})
			}
		}
	}
	return edges
}

// BuildComputeChain constructs a ComputeChain from the raw JSONB data
// stored in profiler_recordings.compute_chain.
func BuildComputeChain(chainJSON *json.RawMessage) *dto.ComputeChain {
	if chainJSON == nil || len(*chainJSON) == 0 || string(*chainJSON) == "null" {
		return nil
	}

	var rawNodes []rawComputeNode
	if err := json.Unmarshal(*chainJSON, &rawNodes); err != nil || len(rawNodes) == 0 {
		return nil
	}

	fieldToNodeID := make(map[string]string, len(rawNodes))
	nodes, stats := buildComputeNodes(rawNodes, fieldToNodeID)
	edges := buildComputeEdges(rawNodes, fieldToNodeID)

	triggerField := ""
	for _, rn := range rawNodes {
		if rn.Depth == 0 && rn.TriggerField != "" {
			triggerField = rn.TriggerField
			break
		}
	}

	return &dto.ComputeChain{
		Nodes: nodes,
		Edges: edges,
		Summary: dto.ComputeChainSummary{
			TotalMS:         stats.totalMS,
			NodeCount:       len(nodes),
			MaxDepth:        stats.maxDepth,
			BottleneckCount: stats.bottleneckCnt,
			SlowestNode:     stats.slowestNodeID,
			SlowestMS:       stats.slowestMS,
			TriggerField:    triggerField,
		},
	}
}

// BuildComputeChainFromEvents constructs compute_chain JSONB from profiler
// events that have IsCompute=true. Returns nil if no compute events found.
func BuildComputeChainFromEvents(events []ProfilerEvent) *json.RawMessage {
	computeEvents := filterComputeEvents(events)
	if len(computeEvents) == 0 {
		return nil
	}

	// Sort by timestamp to establish ordering.
	sort.Slice(computeEvents, func(i, j int) bool {
		return computeEvents[i].Timestamp.Before(computeEvents[j].Timestamp)
	})

	fieldToNodeID := make(map[string]string) // "model.field" -> node ID
	nodes := make([]rawComputeNode, 0, len(computeEvents))

	for i, ev := range computeEvents {
		nodeID := fmt.Sprintf("compute-%d", i)
		key := ev.Model + "." + ev.FieldName
		fieldToNodeID[key] = nodeID

		depth, parentID := determineDepthAndParent(ev, nodes, fieldToNodeID)

		nodes = append(nodes, rawComputeNode{
			Model:        ev.Model,
			FieldName:    ev.FieldName,
			Method:       ev.Method,
			Module:       ev.Module,
			DurationMS:   ev.DurationMS,
			DependsOn:    ev.DependsOn,
			TriggerField: ev.TriggerField,
			SQLCount:     0, // This was not being calculated correctly before
			Depth:        depth,
			ParentID:     parentID,
		})
	}

	raw, err := json.Marshal(nodes)
	if err != nil {
		return nil
	}
	result := json.RawMessage(raw)
	return &result
}

func filterComputeEvents(events []ProfilerEvent) []ProfilerEvent {
	var computeEvents []ProfilerEvent
	for _, ev := range events {
		if ev.IsCompute && ev.FieldName != "" {
			computeEvents = append(computeEvents, ev)
		}
	}
	return computeEvents
}

func determineDepthAndParent(ev ProfilerEvent, nodes []rawComputeNode, fieldToNodeID map[string]string) (depth int, parentID string) {
	maxParentDepth := -1

	for _, dep := range ev.DependsOn {
		depKey := ev.Model + "." + dep
		if pid, ok := fieldToNodeID[depKey]; ok {
			// Find parent depth.
			for _, prev := range nodes {
				prevKey := prev.Model + "." + prev.FieldName
				if prevKey == depKey && prev.Depth > maxParentDepth {
					maxParentDepth = prev.Depth
					parentID = pid
				}
			}
		}
	}

	if parentID != "" {
		depth = maxParentDepth + 1
	}

	return depth, parentID
}

func buildEventLabel(ev ProfilerEvent) string {
	if ev.Model != "" && ev.Method != "" {
		return ev.Model + "." + ev.Method
	}
	if ev.Model != "" {
		return ev.Model
	}
	if ev.SQL != "" {
		if len(ev.SQL) > 60 {
			return ev.SQL[:60] + "..."
		}
		return ev.SQL
	}
	return ev.Category
}
