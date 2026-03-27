package service

import (
	"encoding/json"
	"testing"
	"time"
)

func TestBuildWaterfall_SimpleFormat(t *testing.T) {
	// Simple format: [{"label":"ORM","ms":100},{"label":"SQL","ms":500}]
	raw := json.RawMessage(`[
		{"label":"ORM","ms":100},
		{"label":"SQL","ms":500},
		{"label":"Python","ms":200}
	]`)

	wf, err := BuildWaterfall(raw, 800, nil)
	if err != nil {
		t.Fatalf("BuildWaterfall returned error: %v", err)
	}

	if len(wf.Spans) != 3 {
		t.Fatalf("expected 3 spans, got %d", len(wf.Spans))
	}

	// Verify sequential offsets.
	if wf.Spans[0].StartMS != 0 {
		t.Errorf("span 0 start_ms = %d, want 0", wf.Spans[0].StartMS)
	}
	if wf.Spans[1].StartMS != 100 {
		t.Errorf("span 1 start_ms = %d, want 100", wf.Spans[1].StartMS)
	}
	if wf.Spans[2].StartMS != 600 {
		t.Errorf("span 2 start_ms = %d, want 600", wf.Spans[2].StartMS)
	}

	// Verify categories inferred from labels.
	if wf.Spans[0].Category != "orm" {
		t.Errorf("span 0 category = %q, want orm", wf.Spans[0].Category)
	}
	if wf.Spans[1].Category != "sql" {
		t.Errorf("span 1 category = %q, want sql", wf.Spans[1].Category)
	}

	// Verify summary.
	if wf.Summary.TotalMS != 800 {
		t.Errorf("summary total_ms = %d, want 800", wf.Summary.TotalMS)
	}
	if wf.Summary.SpanCount != 3 {
		t.Errorf("summary span_count = %d, want 3", wf.Summary.SpanCount)
	}
	if wf.Summary.SQLMS != 500 {
		t.Errorf("summary sql_ms = %d, want 500", wf.Summary.SQLMS)
	}
	if wf.Summary.SQLCount != 1 {
		t.Errorf("summary sql_count = %d, want 1", wf.Summary.SQLCount)
	}

	// Verify lanes are present and sorted by duration desc.
	if len(wf.Lanes) != 3 {
		t.Fatalf("expected 3 lanes, got %d", len(wf.Lanes))
	}
	if wf.Lanes[0].Category != "sql" {
		t.Errorf("top lane = %q, want sql (highest duration)", wf.Lanes[0].Category)
	}
}

func TestBuildWaterfall_DetailedFormat(t *testing.T) {
	raw := json.RawMessage(`[
		{"label":"res.partner.search_read","category":"orm","start_ms":0,"ms":120,"model":"res.partner","method":"search_read","module":"base"},
		{"label":"SELECT * FROM res_partner","category":"sql","start_ms":10,"ms":80,"sql":"SELECT * FROM res_partner WHERE active = true","parent_id":"span-0"},
		{"label":"res.partner.write","category":"orm","start_ms":120,"ms":50,"model":"res.partner","method":"write","module":"base","is_n1":true}
	]`)

	wf, err := BuildWaterfall(raw, 170, nil)
	if err != nil {
		t.Fatalf("BuildWaterfall returned error: %v", err)
	}

	if len(wf.Spans) != 3 {
		t.Fatalf("expected 3 spans, got %d", len(wf.Spans))
	}

	// SQL span should be nested (depth=1 due to parent_id).
	sqlSpan := wf.Spans[1]
	if sqlSpan.Depth != 1 {
		t.Errorf("sql span depth = %d, want 1", sqlSpan.Depth)
	}
	if sqlSpan.ParentID != "span-0" {
		t.Errorf("sql span parent_id = %q, want span-0", sqlSpan.ParentID)
	}

	// N+1 span should be flagged.
	n1Span := wf.Spans[2]
	if !n1Span.IsN1 {
		t.Error("expected span 2 to be flagged as N+1")
	}

	// Verify summary N+1 stats.
	if wf.Summary.N1Count != 1 {
		t.Errorf("summary n1_count = %d, want 1", wf.Summary.N1Count)
	}
	if wf.Summary.N1MS != 50 {
		t.Errorf("summary n1_ms = %d, want 50", wf.Summary.N1MS)
	}
}

func TestBuildWaterfall_WithN1Patterns(t *testing.T) {
	raw := json.RawMessage(`[
		{"label":"res.partner.search_read","category":"orm","start_ms":0,"ms":890,"model":"res.partner","method":"search_read"}
	]`)

	n1Raw := json.RawMessage(`[
		{"model":"res.partner","method":"search_read","count":50,"total_ms":890}
	]`)

	wf, err := BuildWaterfall(raw, 890, &n1Raw)
	if err != nil {
		t.Fatalf("BuildWaterfall returned error: %v", err)
	}

	// Span should be cross-referenced with N+1 patterns.
	if !wf.Spans[0].IsN1 {
		t.Error("expected span to be flagged as N+1 via n1_patterns cross-reference")
	}
	if wf.Summary.N1Count != 1 {
		t.Errorf("summary n1_count = %d, want 1", wf.Summary.N1Count)
	}
	if wf.Summary.N1MS != 890 {
		t.Errorf("summary n1_ms = %d, want 890", wf.Summary.N1MS)
	}
}

func TestBuildWaterfall_EmptyInput(t *testing.T) {
	tests := []struct {
		name string
		raw  json.RawMessage
	}{
		{"nil", nil},
		{"null", json.RawMessage(`null`)},
		{"empty array", json.RawMessage(`[]`)},
		{"empty bytes", json.RawMessage{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wf, err := BuildWaterfall(tt.raw, 100, nil)
			if err != nil {
				t.Fatalf("BuildWaterfall returned error: %v", err)
			}
			if len(wf.Spans) != 0 {
				t.Errorf("expected 0 spans, got %d", len(wf.Spans))
			}
			if wf.Summary.TotalMS != 100 {
				t.Errorf("summary total_ms = %d, want 100", wf.Summary.TotalMS)
			}
		})
	}
}

func TestBuildWaterfall_InvalidJSON(t *testing.T) {
	// Invalid JSON should not error — returns empty waterfall.
	raw := json.RawMessage(`{not valid json}`)
	wf, err := BuildWaterfall(raw, 200, nil)
	if err != nil {
		t.Fatalf("BuildWaterfall returned error: %v", err)
	}
	if len(wf.Spans) != 0 {
		t.Errorf("expected 0 spans for invalid JSON, got %d", len(wf.Spans))
	}
}

func TestBuildWaterfallFromEvents(t *testing.T) {
	base := time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC)

	events := []ProfilerEvent{
		{
			Category:   "orm",
			Model:      "res.partner",
			Method:     "search_read",
			DurationMS: 120,
			Timestamp:  base,
		},
		{
			Category:   "sql",
			SQL:        "SELECT * FROM res_partner WHERE active = true",
			DurationMS: 80,
			Timestamp:  base.Add(10 * time.Millisecond),
		},
		{
			Category:   "orm",
			Model:      "res.partner",
			Method:     "write",
			DurationMS: 50,
			IsN1:       true,
			Timestamp:  base.Add(130 * time.Millisecond),
		},
	}

	waterfallJSON, n1JSON, meta := BuildWaterfallFromEvents(events)

	// Verify waterfall JSON is valid.
	var spans []rawSpan
	if err := json.Unmarshal(waterfallJSON, &spans); err != nil {
		t.Fatalf("waterfall JSON unmarshal error: %v", err)
	}
	if len(spans) != 3 {
		t.Fatalf("expected 3 spans, got %d", len(spans))
	}

	// Verify timing offsets.
	if spans[0].StartMS != 0 {
		t.Errorf("span 0 start_ms = %d, want 0", spans[0].StartMS)
	}
	if spans[1].StartMS != 10 {
		t.Errorf("span 1 start_ms = %d, want 10", spans[1].StartMS)
	}
	if spans[2].StartMS != 130 {
		t.Errorf("span 2 start_ms = %d, want 130", spans[2].StartMS)
	}

	// Verify N+1 patterns were extracted.
	if n1JSON == nil {
		t.Fatal("expected n1_patterns JSON, got nil")
	}
	var n1Patterns []n1Pattern
	if err := json.Unmarshal(*n1JSON, &n1Patterns); err != nil {
		t.Fatalf("n1 JSON unmarshal error: %v", err)
	}
	if len(n1Patterns) != 1 {
		t.Fatalf("expected 1 N+1 pattern, got %d", len(n1Patterns))
	}
	if n1Patterns[0].Model != "res.partner" {
		t.Errorf("n1 pattern model = %q, want res.partner", n1Patterns[0].Model)
	}

	// Verify meta.
	if meta.SQLCount != 1 {
		t.Errorf("meta sql_count = %d, want 1", meta.SQLCount)
	}
	if meta.SQLMS != 80 {
		t.Errorf("meta sql_ms = %d, want 80", meta.SQLMS)
	}
	if meta.TotalMS != 250 {
		t.Errorf("meta total_ms = %d, want 250 (120+80+50)", meta.TotalMS)
	}
}

func TestBuildWaterfall_Lanes(t *testing.T) {
	raw := json.RawMessage(`[
		{"label":"ORM","ms":200},
		{"label":"SQL","ms":500},
		{"label":"SQL","ms":100},
		{"label":"Python","ms":200}
	]`)

	wf, err := BuildWaterfall(raw, 1000, nil)
	if err != nil {
		t.Fatalf("BuildWaterfall returned error: %v", err)
	}

	// Lanes should be sorted by duration descending.
	if len(wf.Lanes) != 3 {
		t.Fatalf("expected 3 lanes, got %d", len(wf.Lanes))
	}

	// SQL lane should be first (600ms total).
	if wf.Lanes[0].Category != "sql" {
		t.Errorf("lane 0 = %q, want sql", wf.Lanes[0].Category)
	}
	if wf.Lanes[0].TotalMS != 600 {
		t.Errorf("sql lane total_ms = %d, want 600", wf.Lanes[0].TotalMS)
	}
	if wf.Lanes[0].Pct != 60.0 {
		t.Errorf("sql lane pct = %.2f, want 60.00", wf.Lanes[0].Pct)
	}
}

func TestBuildWaterfall_SQLTruncation(t *testing.T) {
	longSQL := ""
	for i := range 300 {
		_ = i
		longSQL += "x"
	}

	raw := json.RawMessage(`[
		{"label":"query","category":"sql","start_ms":0,"ms":100,"sql":"` + longSQL + `"}
	]`)

	wf, err := BuildWaterfall(raw, 100, nil)
	if err != nil {
		t.Fatalf("BuildWaterfall returned error: %v", err)
	}

	if len(wf.Spans[0].SQL) > 204 { // 200 + "..."
		t.Errorf("SQL not truncated: len=%d", len(wf.Spans[0].SQL))
	}
}

func TestBuildWaterfall_CriticalSQL(t *testing.T) {
	raw := json.RawMessage(`[
		{"label":"fast query","category":"sql","start_ms":0,"ms":10,"sql":"SELECT 1"},
		{"label":"slow query","category":"sql","start_ms":10,"ms":500,"sql":"SELECT * FROM big_table"}
	]`)

	wf, err := BuildWaterfall(raw, 510, nil)
	if err != nil {
		t.Fatalf("BuildWaterfall returned error: %v", err)
	}

	if wf.Summary.CriticalSQL != "SELECT * FROM big_table" {
		t.Errorf("critical_sql = %q, want 'SELECT * FROM big_table'", wf.Summary.CriticalSQL)
	}
}

func TestBuildWaterfallFromEvents_Empty(t *testing.T) {
	waterfallJSON, n1JSON, meta := BuildWaterfallFromEvents(nil)

	var spans []rawSpan
	if err := json.Unmarshal(waterfallJSON, &spans); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(spans) != 0 {
		t.Errorf("expected 0 spans, got %d", len(spans))
	}
	if n1JSON != nil {
		t.Error("expected nil n1_patterns for empty events")
	}
	if meta.TotalMS != 0 {
		t.Errorf("meta total_ms = %d, want 0", meta.TotalMS)
	}
}

// ─── Compute Chain Builder Tests ────────────────────────────────────────────

func TestBuildComputeChain_Nil(t *testing.T) {
	chain := BuildComputeChain(nil)
	if chain != nil {
		t.Error("expected nil compute chain for nil input")
	}
}

func TestBuildComputeChain_EmptyArray(t *testing.T) {
	raw := json.RawMessage(`[]`)
	chain := BuildComputeChain(&raw)
	if chain != nil {
		t.Error("expected nil compute chain for empty array")
	}
}

func TestBuildComputeChain_SingleNode(t *testing.T) {
	raw := json.RawMessage(`[
		{
			"model": "sale.order",
			"field_name": "amount_total",
			"method": "_compute_amount_total",
			"module": "sale",
			"duration_ms": 45,
			"depends_on": ["order_line.price_subtotal"],
			"trigger_field": "order_line",
			"depth": 0
		}
	]`)

	chain := BuildComputeChain(&raw)
	if chain == nil {
		t.Fatal("expected non-nil compute chain")
	}

	if len(chain.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(chain.Nodes))
	}

	node := chain.Nodes[0]
	if node.Model != "sale.order" {
		t.Errorf("node model = %q, want sale.order", node.Model)
	}
	if node.FieldName != "amount_total" {
		t.Errorf("node field_name = %q, want amount_total", node.FieldName)
	}
	if node.DurationMS != 45 {
		t.Errorf("node duration_ms = %d, want 45", node.DurationMS)
	}
	if node.Method != "_compute_amount_total" {
		t.Errorf("node method = %q, want _compute_amount_total", node.Method)
	}

	if chain.Summary.TotalMS != 45 {
		t.Errorf("summary total_ms = %d, want 45", chain.Summary.TotalMS)
	}
	if chain.Summary.NodeCount != 1 {
		t.Errorf("summary node_count = %d, want 1", chain.Summary.NodeCount)
	}
	if chain.Summary.TriggerField != "order_line" {
		t.Errorf("summary trigger_field = %q, want order_line", chain.Summary.TriggerField)
	}
}

func TestBuildComputeChain_DependencyGraph(t *testing.T) {
	raw := json.RawMessage(`[
		{
			"model": "sale.order.line",
			"field_name": "price_subtotal",
			"method": "_compute_amount",
			"module": "sale",
			"duration_ms": 20,
			"trigger_field": "product_uom_qty",
			"depth": 0
		},
		{
			"model": "sale.order.line",
			"field_name": "price_total",
			"method": "_compute_amount",
			"module": "sale",
			"duration_ms": 15,
			"depends_on": ["price_subtotal"],
			"depth": 1,
			"parent_id": "compute-0"
		},
		{
			"model": "sale.order",
			"field_name": "amount_total",
			"method": "_compute_amount_total",
			"module": "sale",
			"duration_ms": 120,
			"depends_on": ["order_line.price_total"],
			"depth": 2,
			"parent_id": "compute-1",
			"sql_count": 3
		}
	]`)

	chain := BuildComputeChain(&raw)
	if chain == nil {
		t.Fatal("expected non-nil compute chain")
	}

	if len(chain.Nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(chain.Nodes))
	}

	// Verify depths.
	if chain.Nodes[0].Depth != 0 {
		t.Errorf("node 0 depth = %d, want 0", chain.Nodes[0].Depth)
	}
	if chain.Nodes[1].Depth != 1 {
		t.Errorf("node 1 depth = %d, want 1", chain.Nodes[1].Depth)
	}
	if chain.Nodes[2].Depth != 2 {
		t.Errorf("node 2 depth = %d, want 2", chain.Nodes[2].Depth)
	}

	// Verify edges.
	if len(chain.Edges) < 2 {
		t.Fatalf("expected at least 2 edges, got %d", len(chain.Edges))
	}

	// First edge: compute-0 -> compute-1 (parent_id).
	foundParentEdge := false
	for _, edge := range chain.Edges {
		if edge.From == "compute-0" && edge.To == "compute-1" {
			foundParentEdge = true
			break
		}
	}
	if !foundParentEdge {
		t.Error("expected edge from compute-0 to compute-1")
	}

	// Verify summary.
	if chain.Summary.TotalMS != 155 {
		t.Errorf("summary total_ms = %d, want 155", chain.Summary.TotalMS)
	}
	if chain.Summary.MaxDepth != 2 {
		t.Errorf("summary max_depth = %d, want 2", chain.Summary.MaxDepth)
	}
	if chain.Summary.SlowestMS != 120 {
		t.Errorf("summary slowest_ms = %d, want 120", chain.Summary.SlowestMS)
	}
	if chain.Summary.SlowestNode != "compute-2" {
		t.Errorf("summary slowest_node = %q, want compute-2", chain.Summary.SlowestNode)
	}

	// SQL count on the slowest node.
	if chain.Nodes[2].SQLCount != 3 {
		t.Errorf("node 2 sql_count = %d, want 3", chain.Nodes[2].SQLCount)
	}
}

func TestBuildComputeChain_BottleneckDetection(t *testing.T) {
	raw := json.RawMessage(`[
		{
			"model": "sale.order",
			"field_name": "name",
			"method": "_compute_name",
			"duration_ms": 5,
			"depth": 0
		},
		{
			"model": "sale.order",
			"field_name": "amount_tax",
			"method": "_compute_tax",
			"duration_ms": 8,
			"depth": 0
		},
		{
			"model": "sale.order",
			"field_name": "amount_total",
			"method": "_compute_amount_total",
			"duration_ms": 200,
			"depth": 0
		}
	]`)

	chain := BuildComputeChain(&raw)
	if chain == nil {
		t.Fatal("expected non-nil compute chain")
	}

	// amount_total (200ms) should be flagged as bottleneck.
	// Average is ~71ms, threshold is 142ms.
	if !chain.Nodes[2].IsBottleneck {
		t.Error("expected amount_total to be flagged as bottleneck")
	}
	if chain.Nodes[0].IsBottleneck {
		t.Error("expected name (5ms) to NOT be flagged as bottleneck")
	}

	if chain.Summary.BottleneckCount != 1 {
		t.Errorf("summary bottleneck_count = %d, want 1", chain.Summary.BottleneckCount)
	}
}

func TestBuildComputeChain_InvalidJSON(t *testing.T) {
	raw := json.RawMessage(`{not valid}`)
	chain := BuildComputeChain(&raw)
	if chain != nil {
		t.Error("expected nil compute chain for invalid JSON")
	}
}

func TestBuildComputeChainFromEvents_NoComputeEvents(t *testing.T) {
	events := []ProfilerEvent{
		{
			Category:   "orm",
			Model:      "res.partner",
			Method:     "search_read",
			DurationMS: 50,
			Timestamp:  time.Now(),
		},
	}

	result := BuildComputeChainFromEvents(events)
	if result != nil {
		t.Error("expected nil for events with no compute fields")
	}
}

func TestBuildComputeChainFromEvents_WithComputeEvents(t *testing.T) {
	base := time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC)

	events := []ProfilerEvent{
		{
			Category:     "orm",
			Model:        "sale.order.line",
			Method:       "_compute_amount",
			FieldName:    "price_subtotal",
			IsCompute:    true,
			DurationMS:   20,
			Module:       "sale",
			TriggerField: "product_uom_qty",
			Timestamp:    base,
		},
		{
			Category:   "sql",
			Model:      "sale.order.line",
			SQL:        "SELECT * FROM sale_order_line",
			DurationMS: 10,
			Timestamp:  base.Add(5 * time.Millisecond),
		},
		{
			Category:     "orm",
			Model:        "sale.order.line",
			Method:       "_compute_amount",
			FieldName:    "price_total",
			IsCompute:    true,
			DurationMS:   15,
			Module:       "sale",
			DependsOn:    []string{"price_subtotal"},
			TriggerField: "price_subtotal",
			Timestamp:    base.Add(25 * time.Millisecond),
		},
		{
			Category:     "orm",
			Model:        "sale.order",
			Method:       "_compute_amount_total",
			FieldName:    "amount_total",
			IsCompute:    true,
			DurationMS:   50,
			Module:       "sale",
			DependsOn:    []string{"order_line.price_total"},
			TriggerField: "order_line",
			Timestamp:    base.Add(45 * time.Millisecond),
		},
	}

	result := BuildComputeChainFromEvents(events)
	if result == nil {
		t.Fatal("expected non-nil compute chain JSON")
	}

	// Verify the JSON can be deserialized.
	var nodes []rawComputeNode
	if err := json.Unmarshal(*result, &nodes); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if len(nodes) != 3 {
		t.Fatalf("expected 3 compute nodes, got %d", len(nodes))
	}

	// First node should be the root (depth 0).
	if nodes[0].FieldName != "price_subtotal" {
		t.Errorf("node 0 field_name = %q, want price_subtotal", nodes[0].FieldName)
	}
	if nodes[0].Depth != 0 {
		t.Errorf("node 0 depth = %d, want 0", nodes[0].Depth)
	}
	if nodes[0].TriggerField != "product_uom_qty" {
		t.Errorf("node 0 trigger_field = %q, want product_uom_qty", nodes[0].TriggerField)
	}

	// Second node depends on price_subtotal → depth 1.
	if nodes[1].FieldName != "price_total" {
		t.Errorf("node 1 field_name = %q, want price_total", nodes[1].FieldName)
	}
	if nodes[1].Depth != 1 {
		t.Errorf("node 1 depth = %d, want 1", nodes[1].Depth)
	}

	// Verify round-trip: BuildComputeChain can read what BuildComputeChainFromEvents wrote.
	chain := BuildComputeChain(result)
	if chain == nil {
		t.Fatal("round-trip: expected non-nil compute chain")
	}
	if chain.Summary.NodeCount != 3 {
		t.Errorf("round-trip: node_count = %d, want 3", chain.Summary.NodeCount)
	}
	if chain.Summary.TotalMS != 85 {
		t.Errorf("round-trip: total_ms = %d, want 85", chain.Summary.TotalMS)
	}
}

func TestBuildComputeChainFromEvents_Empty(t *testing.T) {
	result := BuildComputeChainFromEvents(nil)
	if result != nil {
		t.Error("expected nil for nil events")
	}

	result = BuildComputeChainFromEvents([]ProfilerEvent{})
	if result != nil {
		t.Error("expected nil for empty events")
	}
}
