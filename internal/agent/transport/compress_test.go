package transport

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"testing"
	"time"

	"Intelligent_Dev_ToolKit_Odoo/internal/agent/aggregator"
)

func sampleBatch() *aggregator.AggregatedBatch {
	return &aggregator.AggregatedBatch{
		EnvID:      "env-test-001",
		Period:     time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
		DurationMS: 30000,
		ORMStats: []aggregator.ORMModelStat{
			{
				Model: "res.partner", Method: "search_read",
				CallCount: 148, TotalMS: 890, AvgMS: 6.01, MaxMS: 42, P95MS: 12,
				N1Detected: true, SampleSQL: "SELECT id, name FROM res_partner WHERE active = true",
			},
			{
				Model: "product.product", Method: "write",
				CallCount: 23, TotalMS: 115, AvgMS: 5.0, MaxMS: 18, P95MS: 10,
			},
		},
		RawEvents: []aggregator.Event{
			{
				Category: "error", Model: "sale.order", Method: "create",
				IsError: true, Traceback: "Traceback (most recent call last):\n  File ...\nValidationError: ...",
				Timestamp: time.Date(2026, 1, 15, 10, 0, 15, 0, time.UTC),
			},
			{
				Category: "sql", Model: "res.partner", Method: "search_read",
				DurationMS: 350, SQL: "SELECT * FROM res_partner WHERE ...",
				Timestamp: time.Date(2026, 1, 15, 10, 0, 20, 0, time.UTC),
			},
		},
		Summary: aggregator.BatchSummary{
			TotalQueries: 171, TotalDurationMS: 1005,
			SlowQueries: 3, N1Patterns: 1, Errors: 1,
		},
	}
}

// ── CompressBatch ───────────────────────────────────────────────────────────

func TestCompressBatch_ProducesValidGzip(t *testing.T) {
	batch := sampleBatch()
	cr, err := CompressBatch(batch)
	if err != nil {
		t.Fatalf("CompressBatch: %v", err)
	}

	if cr.CompressedSize == 0 {
		t.Fatal("compressed size is 0")
	}
	if cr.OriginalSize == 0 {
		t.Fatal("original size is 0")
	}

	// Decompress and verify it round-trips back to valid JSON.
	gz, err := gzip.NewReader(bytes.NewReader(cr.Data))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	decompressed, err := io.ReadAll(gz)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}
	gz.Close()

	var got aggregator.AggregatedBatch
	if err := json.Unmarshal(decompressed, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.EnvID != batch.EnvID {
		t.Errorf("EnvID = %q, want %q", got.EnvID, batch.EnvID)
	}
	if len(got.ORMStats) != len(batch.ORMStats) {
		t.Errorf("ORMStats len = %d, want %d", len(got.ORMStats), len(batch.ORMStats))
	}
	if len(got.RawEvents) != len(batch.RawEvents) {
		t.Errorf("RawEvents len = %d, want %d", len(got.RawEvents), len(batch.RawEvents))
	}
	if got.Summary.TotalQueries != batch.Summary.TotalQueries {
		t.Errorf("TotalQueries = %d, want %d", got.Summary.TotalQueries, batch.Summary.TotalQueries)
	}
}

func TestCompressBatch_SmallerThanOriginal(t *testing.T) {
	batch := sampleBatch()
	cr, err := CompressBatch(batch)
	if err != nil {
		t.Fatalf("CompressBatch: %v", err)
	}

	if cr.CompressedSize >= cr.OriginalSize {
		t.Errorf("compressed (%d) should be smaller than original (%d)",
			cr.CompressedSize, cr.OriginalSize)
	}
	t.Logf("compression: %d → %d (ratio %.2f)", cr.OriginalSize, cr.CompressedSize, cr.Ratio())
}

func TestCompressBatch_Ratio(t *testing.T) {
	batch := sampleBatch()
	cr, err := CompressBatch(batch)
	if err != nil {
		t.Fatalf("CompressBatch: %v", err)
	}

	ratio := cr.Ratio()
	if ratio <= 0 || ratio >= 1.0 {
		t.Errorf("ratio = %.3f, expected between 0 and 1", ratio)
	}
}

func TestCompressResult_Ratio_ZeroOriginal(t *testing.T) {
	cr := CompressResult{OriginalSize: 0, CompressedSize: 0}
	if cr.Ratio() != 1.0 {
		t.Errorf("ratio for zero original should be 1.0, got %f", cr.Ratio())
	}
}

// ── CompressBatchLevel ──────────────────────────────────────────────────────

func TestCompressBatchLevel_BestCompression(t *testing.T) {
	batch := sampleBatch()

	crDefault, err := CompressBatchLevel(batch, gzip.DefaultCompression)
	if err != nil {
		t.Fatal(err)
	}
	crBest, err := CompressBatchLevel(batch, gzip.BestCompression)
	if err != nil {
		t.Fatal(err)
	}

	// BestCompression should be <= DefaultCompression size.
	if crBest.CompressedSize > crDefault.CompressedSize {
		t.Errorf("BestCompression (%d) > DefaultCompression (%d)",
			crBest.CompressedSize, crDefault.CompressedSize)
	}
	t.Logf("default: %d, best: %d", crDefault.CompressedSize, crBest.CompressedSize)
}

func TestCompressBatchLevel_NoCompression(t *testing.T) {
	batch := sampleBatch()
	cr, err := CompressBatchLevel(batch, gzip.NoCompression)
	if err != nil {
		t.Fatal(err)
	}

	// gzip with no compression adds header overhead, so compressed can be
	// slightly larger than original. Just verify it's valid gzip.
	gz, err := gzip.NewReader(bytes.NewReader(cr.Data))
	if err != nil {
		t.Fatalf("not valid gzip: %v", err)
	}
	gz.Close()
}

// ── CompressJSON ────────────────────────────────────────────────────────────

func TestCompressJSON_RoundTrips(t *testing.T) {
	original := []byte(`{"hello":"world","numbers":[1,2,3,4,5]}`)
	compressed, err := CompressJSON(original)
	if err != nil {
		t.Fatalf("CompressJSON: %v", err)
	}

	gz, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(gz)
	if err != nil {
		t.Fatal(err)
	}
	gz.Close()

	if !bytes.Equal(got, original) {
		t.Errorf("round-trip mismatch: got %q", got)
	}
}

// ── Large batch compression ratio ───────────────────────────────────────────

func TestCompressBatch_LargeBatch_GoodRatio(t *testing.T) {
	// Build a batch with many repetitive ORM stats (simulates real workload).
	batch := &aggregator.AggregatedBatch{
		EnvID:      "env-prod-001",
		Period:     time.Now().UTC(),
		DurationMS: 30000,
	}

	for i := 0; i < 200; i++ {
		batch.ORMStats = append(batch.ORMStats, aggregator.ORMModelStat{
			Model:      "res.partner",
			Method:     "search_read",
			CallCount:  100 + i,
			TotalMS:    500 + i*3,
			AvgMS:      5.0 + float64(i)*0.01,
			MaxMS:      42,
			P95MS:      12,
			N1Detected: i%10 == 0,
			SampleSQL:  "SELECT id, name, email, phone FROM res_partner WHERE active = true AND company_id = 1",
		})
	}

	batch.Summary = aggregator.BatchSummary{
		TotalQueries: 20000, TotalDurationMS: 95000,
		SlowQueries: 45, N1Patterns: 20, Errors: 3,
	}

	cr, err := CompressBatch(batch)
	if err != nil {
		t.Fatal(err)
	}

	// With highly repetitive data, expect good compression.
	ratio := cr.Ratio()
	t.Logf("large batch: %d → %d (ratio %.3f, %.0f%% reduction)",
		cr.OriginalSize, cr.CompressedSize, ratio, (1-ratio)*100)

	if ratio > 0.5 {
		t.Errorf("expected ratio < 0.5 for repetitive data, got %.3f", ratio)
	}
}
