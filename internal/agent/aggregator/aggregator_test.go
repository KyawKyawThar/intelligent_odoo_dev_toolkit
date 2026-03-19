package aggregator

import (
	"context"
	"testing"
	"time"

	"Intelligent_Dev_ToolKit_Odoo/internal/agent/sampler"

	"github.com/rs/zerolog"
)

// helper: build a test aggregator with short flush interval.
func testAggregator(smp *sampler.Sampler) *Aggregator {
	cfg := Config{
		FlushInterval:    50 * time.Millisecond,
		EnvID:            "test-env",
		EventChannelSize: 256,
		MaxRawPerFlush:   100,
	}
	return New(cfg, smp, zerolog.Nop())
}

// ── Ingest + Flush ──────────────────────────────────────────────────────────

func TestIngest_AggregatesStats(t *testing.T) {
	agg := testAggregator(nil) // no sampler = keep all

	agg.Ingest(Event{Model: "res.partner", Method: "search_read", DurationMS: 10})
	agg.Ingest(Event{Model: "res.partner", Method: "search_read", DurationMS: 20})
	agg.Ingest(Event{Model: "res.partner", Method: "search_read", DurationMS: 50})
	agg.Ingest(Event{Model: "product.product", Method: "write", DurationMS: 5})

	batch := agg.flush()
	if batch == nil {
		t.Fatal("expected non-nil batch")
	}

	if batch.EnvID != "test-env" {
		t.Errorf("EnvID = %q, want %q", batch.EnvID, "test-env")
	}
	if len(batch.ORMStats) != 2 {
		t.Fatalf("expected 2 ORM stats, got %d", len(batch.ORMStats))
	}

	// Find the res.partner stat.
	var partnerStat *ORMModelStat
	for i := range batch.ORMStats {
		if batch.ORMStats[i].Model == "res.partner" {
			partnerStat = &batch.ORMStats[i]
			break
		}
	}
	if partnerStat == nil {
		t.Fatal("missing res.partner stat")
	}

	if partnerStat.CallCount != 3 {
		t.Errorf("CallCount = %d, want 3", partnerStat.CallCount)
	}
	if partnerStat.TotalMS != 80 {
		t.Errorf("TotalMS = %d, want 80", partnerStat.TotalMS)
	}
	if partnerStat.MaxMS != 50 {
		t.Errorf("MaxMS = %d, want 50", partnerStat.MaxMS)
	}
	// AvgMS = 80/3 ≈ 26.67
	if partnerStat.AvgMS < 26.0 || partnerStat.AvgMS > 27.0 {
		t.Errorf("AvgMS = %.2f, want ~26.67", partnerStat.AvgMS)
	}
}

func TestIngest_SummaryCounters(t *testing.T) {
	smp := sampler.New(sampler.Config{
		Mode:            sampler.ModeFull,
		SlowThresholdMS: 100,
	})
	agg := testAggregator(smp)

	agg.Ingest(Event{Model: "res.partner", Method: "read", DurationMS: 5})
	agg.Ingest(Event{Model: "res.partner", Method: "read", DurationMS: 150, IsError: false}) // slow
	agg.Ingest(Event{Model: "sale.order", Method: "create", DurationMS: 10, IsError: true})
	agg.Ingest(Event{Model: "res.partner", Method: "search", DurationMS: 3, IsN1: true})

	batch := agg.flush()
	if batch == nil {
		t.Fatal("expected batch")
	}

	s := batch.Summary
	if s.TotalQueries != 4 {
		t.Errorf("TotalQueries = %d, want 4", s.TotalQueries)
	}
	if s.TotalDurationMS != 168 {
		t.Errorf("TotalDurationMS = %d, want 168", s.TotalDurationMS)
	}
	if s.Errors != 1 {
		t.Errorf("Errors = %d, want 1", s.Errors)
	}
	if s.SlowQueries != 1 {
		t.Errorf("SlowQueries = %d, want 1", s.SlowQueries)
	}
	if s.N1Patterns != 1 {
		t.Errorf("N1Patterns = %d, want 1", s.N1Patterns)
	}
}

func TestFlush_EmptyWindowReturnsNil(t *testing.T) {
	agg := testAggregator(nil)
	batch := agg.flush()
	if batch != nil {
		t.Errorf("expected nil batch for empty window")
	}
}

func TestFlush_ResetsState(t *testing.T) {
	agg := testAggregator(nil)

	agg.Ingest(Event{Model: "res.partner", Method: "read", DurationMS: 10})
	batch1 := agg.flush()
	if batch1 == nil {
		t.Fatal("expected batch")
	}
	if batch1.Summary.TotalQueries != 1 {
		t.Errorf("batch1 TotalQueries = %d, want 1", batch1.Summary.TotalQueries)
	}

	// Second flush without new events should be nil.
	batch2 := agg.flush()
	if batch2 != nil {
		t.Error("expected nil batch after reset")
	}
}

// ── N+1 detection ───────────────────────────────────────────────────────────

func TestIngest_N1DetectedPropagates(t *testing.T) {
	agg := testAggregator(nil)

	agg.Ingest(Event{Model: "res.partner", Method: "read", DurationMS: 1})
	agg.Ingest(Event{Model: "res.partner", Method: "read", DurationMS: 2, IsN1: true})

	batch := agg.flush()
	for _, st := range batch.ORMStats {
		if st.Model == "res.partner" && st.Method == "read" {
			if !st.N1Detected {
				t.Error("expected N1Detected = true")
			}
			return
		}
	}
	t.Error("missing res.partner:read stat")
}

// ── P95 calculation ─────────────────────────────────────────────────────────

func TestPercentile(t *testing.T) {
	tests := []struct {
		name   string
		values []int
		p      int
		want   int
	}{
		{"empty", nil, 95, 0},
		{"single", []int{42}, 95, 42},
		{"two", []int{10, 20}, 95, 20},
		{"100 values", make100(), 95, 96},
		{"p50 of 100", make100(), 50, 51},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := percentile(tt.values, tt.p)
			if got != tt.want {
				t.Errorf("percentile(%v, %d) = %d, want %d", tt.values, tt.p, got, tt.want)
			}
		})
	}
}

func make100() []int {
	v := make([]int, 100)
	for i := range v {
		v[i] = i + 1 // 1..100
	}
	return v
}

// ── SampleSQL ───────────────────────────────────────────────────────────────

func TestIngest_SampleSQL_TakesFirst(t *testing.T) {
	agg := testAggregator(nil)

	agg.Ingest(Event{Model: "res.partner", Method: "read", DurationMS: 1, SQL: "SELECT 1"})
	agg.Ingest(Event{Model: "res.partner", Method: "read", DurationMS: 2, SQL: "SELECT 2"})

	batch := agg.flush()
	for _, st := range batch.ORMStats {
		if st.Model == "res.partner" {
			if st.SampleSQL != "SELECT 1" {
				t.Errorf("SampleSQL = %q, want %q", st.SampleSQL, "SELECT 1")
			}
			return
		}
	}
	t.Error("missing stat")
}

// ── Raw event filtering with sampler ────────────────────────────────────────

func TestRawEvents_FullMode_KeepsAll(t *testing.T) {
	smp := sampler.New(sampler.DefaultDevelopment()) // full
	agg := testAggregator(smp)

	agg.Ingest(Event{Category: "orm", Model: "res.partner", Method: "read", DurationMS: 1})
	agg.Ingest(Event{Category: "orm", Model: "res.partner", Method: "read", DurationMS: 2})

	batch := agg.flush()
	if len(batch.RawEvents) != 2 {
		t.Errorf("full mode: raw events = %d, want 2", len(batch.RawEvents))
	}
}

func TestRawEvents_AggregatedOnly_KeepsOnlyCritical(t *testing.T) {
	smp := sampler.New(sampler.DefaultProduction()) // aggregated_only
	agg := testAggregator(smp)

	// Normal ORM call — should be dropped from raw.
	agg.Ingest(Event{Category: "orm", Model: "res.partner", Method: "read", DurationMS: 5})
	// Error — should be kept.
	agg.Ingest(Event{Category: "error", Model: "sale.order", Method: "create", DurationMS: 10, IsError: true})
	// Slow query — should be kept (threshold = 200ms for production).
	agg.Ingest(Event{Category: "sql", Model: "res.partner", Method: "search", DurationMS: 300})

	batch := agg.flush()

	// Stats should count all 3 events.
	if batch.Summary.TotalQueries != 3 {
		t.Errorf("TotalQueries = %d, want 3", batch.Summary.TotalQueries)
	}

	// Raw events should only have the error + slow query.
	if len(batch.RawEvents) != 2 {
		t.Errorf("raw events = %d, want 2", len(batch.RawEvents))
	}
}

func TestRawEvents_MaxPerFlush(t *testing.T) {
	smp := sampler.New(sampler.DefaultDevelopment()) // full mode
	cfg := Config{
		FlushInterval:    50 * time.Millisecond,
		EnvID:            "test-env",
		EventChannelSize: 256,
		MaxRawPerFlush:   5,
	}
	agg := New(cfg, smp, zerolog.Nop())

	for i := 0; i < 20; i++ {
		agg.Ingest(Event{Category: "orm", Model: "res.partner", Method: "read", DurationMS: 1})
	}

	batch := agg.flush()
	if len(batch.RawEvents) != 5 {
		t.Errorf("raw events = %d, want max 5", len(batch.RawEvents))
	}
	// But stats should count all 20.
	if batch.Summary.TotalQueries != 20 {
		t.Errorf("TotalQueries = %d, want 20", batch.Summary.TotalQueries)
	}
}

// ── Run loop integration ────────────────────────────────────────────────────

func TestRun_FlushesOnInterval(t *testing.T) {
	smp := sampler.New(sampler.DefaultDevelopment())
	agg := testAggregator(smp)

	ctx, cancel := context.WithCancel(context.Background())
	go agg.Run(ctx)

	// Send events via channel.
	agg.EventCh <- Event{Category: "orm", Model: "res.partner", Method: "read", DurationMS: 10}
	agg.EventCh <- Event{Category: "orm", Model: "res.partner", Method: "read", DurationMS: 20}

	// Wait for at least one flush (50ms interval).
	select {
	case batch := <-agg.SendCh:
		if batch.Summary.TotalQueries < 1 {
			t.Errorf("expected at least 1 query in batch, got %d", batch.Summary.TotalQueries)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for batch")
	}

	cancel()
}

func TestRun_FinalFlushOnShutdown(t *testing.T) {
	smp := sampler.New(sampler.DefaultDevelopment())
	cfg := Config{
		FlushInterval:    10 * time.Second, // long interval — won't fire before cancel
		EnvID:            "test-env",
		EventChannelSize: 256,
		MaxRawPerFlush:   100,
	}
	agg := New(cfg, smp, zerolog.Nop())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		agg.Run(ctx)
		close(done)
	}()

	// Send an event and cancel immediately — the final flush should capture it.
	agg.EventCh <- Event{Category: "orm", Model: "res.partner", Method: "read", DurationMS: 42}
	time.Sleep(10 * time.Millisecond) // let the event be consumed
	cancel()

	select {
	case batch := <-agg.SendCh:
		if batch.Summary.TotalQueries != 1 {
			t.Errorf("final flush: TotalQueries = %d, want 1", batch.Summary.TotalQueries)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for final flush batch")
	}

	<-done
}

// ── Events without model/method ─────────────────────────────────────────────

func TestIngest_NoModelMethod_SkipsStats(t *testing.T) {
	agg := testAggregator(nil)

	// Pure error event with no model/method.
	agg.Ingest(Event{Category: "error", IsError: true, DurationMS: 0})

	batch := agg.flush()
	if batch == nil {
		t.Fatal("expected batch")
	}
	if len(batch.ORMStats) != 0 {
		t.Errorf("expected 0 ORM stats for events with no model, got %d", len(batch.ORMStats))
	}
	if batch.Summary.Errors != 1 {
		t.Errorf("Errors = %d, want 1", batch.Summary.Errors)
	}
}

// ── DurationMS in batch ─────────────────────────────────────────────────────

func TestFlush_BatchDurationMS(t *testing.T) {
	agg := testAggregator(nil)
	agg.windowStart = time.Now().UTC().Add(-5 * time.Second)

	agg.Ingest(Event{Model: "x", Method: "y", DurationMS: 1})

	batch := agg.flush()
	// Should be approximately 5000ms.
	if batch.DurationMS < 4500 || batch.DurationMS > 6000 {
		t.Errorf("DurationMS = %d, expected ~5000", batch.DurationMS)
	}
}
