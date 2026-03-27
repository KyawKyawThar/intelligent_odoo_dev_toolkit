package worker

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/aggregator"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Mock Store ──────────────────────────────────────────────────────────────

// mockStore implements only the db.Store methods that IngestWorker uses.
// All other methods (inherited from the embedded nil *db.SQLStore) will panic
// if called — which is correct: the test should fail loudly if unexpected
// methods are invoked.
type mockStore struct {
	db.Store // embed interface — unimplemented methods panic on nil receiver

	ingestCalls  []db.IngestErrorBatchParams
	ormStatCalls []db.InsertORMStatParams
	ingestErr    error
	insertORMErr error
}

func (m *mockStore) IngestErrorBatchTx(_ context.Context, arg db.IngestErrorBatchParams) error {
	m.ingestCalls = append(m.ingestCalls, arg)
	return m.ingestErr
}

func (m *mockStore) InsertORMStat(_ context.Context, arg db.InsertORMStatParams) (db.OrmStat, error) {
	m.ormStatCalls = append(m.ormStatCalls, arg)
	return db.OrmStat{}, m.insertORMErr
}

func (m *mockStore) CreateProfilerRecording(_ context.Context, _ db.CreateProfilerRecordingParams) (db.ProfilerRecording, error) {
	return db.ProfilerRecording{}, nil
}

// ── Helper ──────────────────────────────────────────────────────────────────

func newTestWorker(store db.Store) *IngestWorker {
	logger := zerolog.Nop()
	return &IngestWorker{
		store:  store,
		s3:     nil, // S3 disabled — traceback storage skipped
		logger: logger,
	}
}

// ── errorSignature tests ────────────────────────────────────────────────────

func TestErrorSignature_AllFields(t *testing.T) {
	ev := &aggregator.Event{
		Category: "error",
		Module:   "sale",
		Model:    "sale.order",
		Method:   "write",
	}
	got := errorSignature(ev)
	assert.Equal(t, "error:sale:sale.order:write", got)
}

func TestErrorSignature_CategoryOnly(t *testing.T) {
	ev := &aggregator.Event{Category: "error"}
	got := errorSignature(ev)
	assert.Equal(t, "error", got)
}

func TestErrorSignature_WithModule(t *testing.T) {
	ev := &aggregator.Event{Category: "error", Module: "stock"}
	got := errorSignature(ev)
	assert.Equal(t, "error:stock", got)
}

func TestErrorSignature_WithModuleAndModel(t *testing.T) {
	ev := &aggregator.Event{Category: "error", Module: "stock", Model: "stock.picking"}
	got := errorSignature(ev)
	assert.Equal(t, "error:stock:stock.picking", got)
}

// ── gzipJSON tests ──────────────────────────────────────────────────────────

func TestGzipJSON_RoundTrips(t *testing.T) {
	input := map[string]any{
		"traceback": "Traceback (most recent call last):\n  ValueError: boom",
		"module":    "sale",
		"model":     "sale.order",
		"user_id":   float64(42), // JSON numbers are float64
	}

	compressed, err := gzipJSON(input)
	require.NoError(t, err)
	require.NotEmpty(t, compressed)

	// Decompress and verify round-trip.
	reader, err := gzip.NewReader(io.NopCloser(
		io.Reader(bytesReader(compressed)),
	))
	require.NoError(t, err)
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(decompressed, &got))

	assert.Equal(t, input["traceback"], got["traceback"])
	assert.Equal(t, input["module"], got["module"])
	assert.Equal(t, input["model"], got["model"])
	assert.Equal(t, input["user_id"], got["user_id"])
}

func TestGzipJSON_SmallerThanOriginal(t *testing.T) {
	// Large repetitive data should compress well.
	input := map[string]string{
		"data": string(make([]byte, 10000)),
	}
	compressed, err := gzipJSON(input)
	require.NoError(t, err)

	original, _ := json.Marshal(input)
	assert.Less(t, len(compressed), len(original), "compressed should be smaller")
}

// bytesReader is a helper that wraps a byte slice as an io.Reader.
type bytesReader []byte

func (b bytesReader) Read(p []byte) (int, error) {
	n := copy(p, b)
	if n < len(b) {
		return n, nil
	}
	return n, io.EOF
}

// ── processMessage tests ────────────────────────────────────────────────────

func TestProcessMessage_ValidBatch_StoresORMStats(t *testing.T) {
	store := &mockStore{}
	w := newTestWorker(store)

	tenantID := uuid.New()
	envID := uuid.New()

	batch := aggregator.AggregatedBatch{
		EnvID:  envID.String(),
		Period: time.Now().UTC(),
		ORMStats: []aggregator.ORMModelStat{
			{Model: "res.partner", Method: "read", CallCount: 50, TotalMS: 1200, AvgMS: 24.0, MaxMS: 85, P95MS: 72},
			{Model: "sale.order", Method: "write", CallCount: 10, TotalMS: 500, AvgMS: 50.0, MaxMS: 200, P95MS: 180, N1Detected: true, SampleSQL: "SELECT ..."},
		},
		Summary: aggregator.BatchSummary{TotalQueries: 60, TotalDurationMS: 1700},
	}

	data, err := json.Marshal(batch)
	require.NoError(t, err)

	err = w.processMessage(context.Background(), tenantID.String(), string(data))
	require.NoError(t, err)

	// Verify ORM stats were stored.
	require.Len(t, store.ormStatCalls, 2)

	assert.Equal(t, envID, store.ormStatCalls[0].EnvID)
	assert.Equal(t, "res.partner", store.ormStatCalls[0].Model)
	assert.Equal(t, "read", store.ormStatCalls[0].Method)
	assert.Equal(t, int32(50), store.ormStatCalls[0].CallCount)
	assert.Equal(t, int32(1200), store.ormStatCalls[0].TotalMs)

	assert.Equal(t, "sale.order", store.ormStatCalls[1].Model)
	assert.True(t, store.ormStatCalls[1].N1Detected)
	assert.NotNil(t, store.ormStatCalls[1].SampleSql)
}

func TestProcessMessage_ValidBatch_ProcessesErrorEvents(t *testing.T) {
	store := &mockStore{}
	w := newTestWorker(store)

	tenantID := uuid.New()
	envID := uuid.New()

	batch := aggregator.AggregatedBatch{
		EnvID:  envID.String(),
		Period: time.Now().UTC(),
		RawEvents: []aggregator.Event{
			{
				Category:  "error",
				Model:     "sale.order",
				Module:    "sale",
				Method:    "write",
				IsError:   true,
				Traceback: "Traceback:\n  ValueError: boom",
				UserID:    42,
				Timestamp: time.Now().UTC(),
			},
			{
				// Non-error event should be skipped.
				Category:   "orm",
				Model:      "res.partner",
				Method:     "read",
				DurationMS: 5,
				Timestamp:  time.Now().UTC(),
			},
		},
		Summary: aggregator.BatchSummary{Errors: 1},
	}

	data, err := json.Marshal(batch)
	require.NoError(t, err)

	err = w.processMessage(context.Background(), tenantID.String(), string(data))
	require.NoError(t, err)

	// Only the error event should have been ingested.
	require.Len(t, store.ingestCalls, 1)
	call := store.ingestCalls[0]
	assert.Equal(t, envID, call.EnvID)
	assert.Equal(t, tenantID, call.TenantID)
	assert.Equal(t, "error:sale:sale.order:write", call.Signature)
	assert.Equal(t, "error", call.ErrorType)
	assert.NotNil(t, call.Module)
	assert.Equal(t, "sale", *call.Module)
	assert.NotNil(t, call.Model)
	assert.Equal(t, "sale.order", *call.Model)
	assert.Equal(t, []int32{42}, call.AffectedUIDs)
	// S3 is nil so RawTraceRef should be nil.
	assert.Nil(t, call.RawTraceRef)
}

func TestProcessMessage_EmptyBatch_NoStoreInteractions(t *testing.T) {
	store := &mockStore{}
	w := newTestWorker(store)

	tenantID := uuid.New()
	envID := uuid.New()

	batch := aggregator.AggregatedBatch{
		EnvID:  envID.String(),
		Period: time.Now().UTC(),
	}

	data, err := json.Marshal(batch)
	require.NoError(t, err)

	err = w.processMessage(context.Background(), tenantID.String(), string(data))
	require.NoError(t, err)

	assert.Empty(t, store.ingestCalls)
	assert.Empty(t, store.ormStatCalls)
}

func TestProcessMessage_InvalidTenantID_ReturnsError(t *testing.T) {
	store := &mockStore{}
	w := newTestWorker(store)

	err := w.processMessage(context.Background(), "not-a-uuid", "{}")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid tenant_id")
}

func TestProcessMessage_InvalidJSON_ReturnsError(t *testing.T) {
	store := &mockStore{}
	w := newTestWorker(store)

	err := w.processMessage(context.Background(), uuid.New().String(), "{{bad json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal batch")
}

func TestProcessMessage_InvalidEnvID_ReturnsError(t *testing.T) {
	store := &mockStore{}
	w := newTestWorker(store)

	batch := aggregator.AggregatedBatch{
		EnvID: "not-a-uuid",
	}
	data, _ := json.Marshal(batch)

	err := w.processMessage(context.Background(), uuid.New().String(), string(data))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid env_id")
}

func TestProcessMessage_ORMStatStoreError_ContinuesProcessing(t *testing.T) {
	store := &mockStore{
		insertORMErr: assert.AnError,
	}
	w := newTestWorker(store)

	tenantID := uuid.New()
	envID := uuid.New()

	batch := aggregator.AggregatedBatch{
		EnvID:  envID.String(),
		Period: time.Now().UTC(),
		ORMStats: []aggregator.ORMModelStat{
			{Model: "res.partner", Method: "read", CallCount: 10, TotalMS: 100},
			{Model: "sale.order", Method: "write", CallCount: 5, TotalMS: 50},
		},
	}
	data, _ := json.Marshal(batch)

	// processMessage should NOT return error — it logs and continues.
	err := w.processMessage(context.Background(), tenantID.String(), string(data))
	require.NoError(t, err)

	// Both stats were attempted even though each one failed.
	assert.Len(t, store.ormStatCalls, 2)
}

func TestProcessMessage_ErrorIngestFails_ContinuesProcessing(t *testing.T) {
	store := &mockStore{
		ingestErr: assert.AnError,
	}
	w := newTestWorker(store)

	tenantID := uuid.New()
	envID := uuid.New()

	batch := aggregator.AggregatedBatch{
		EnvID:  envID.String(),
		Period: time.Now().UTC(),
		RawEvents: []aggregator.Event{
			{Category: "error", IsError: true, Timestamp: time.Now().UTC()},
			{Category: "error", IsError: true, Module: "sale", Timestamp: time.Now().UTC()},
		},
	}
	data, _ := json.Marshal(batch)

	// processMessage should NOT return error — it logs and continues.
	err := w.processMessage(context.Background(), tenantID.String(), string(data))
	require.NoError(t, err)

	// Both error events were attempted.
	assert.Len(t, store.ingestCalls, 2)
}

// ── processErrorEvent edge cases ────────────────────────────────────────────

func TestProcessErrorEvent_NoModuleNoModel(t *testing.T) {
	store := &mockStore{}
	w := newTestWorker(store)

	ev := &aggregator.Event{
		Category:  "error",
		IsError:   true,
		Traceback: "ValueError: something",
		Timestamp: time.Now().UTC(),
	}

	err := w.processErrorEvent(context.Background(), uuid.New(), uuid.New(), ev)
	require.NoError(t, err)

	require.Len(t, store.ingestCalls, 1)
	assert.Nil(t, store.ingestCalls[0].Module)
	assert.Nil(t, store.ingestCalls[0].Model)
	assert.Equal(t, "error", store.ingestCalls[0].ErrorType)
}

func TestProcessErrorEvent_ZeroTimestamp_UsesNow(t *testing.T) {
	store := &mockStore{}
	w := newTestWorker(store)

	ev := &aggregator.Event{
		Category: "error",
		IsError:  true,
		// Timestamp is zero value.
	}

	before := time.Now().UTC()
	err := w.processErrorEvent(context.Background(), uuid.New(), uuid.New(), ev)
	require.NoError(t, err)
	after := time.Now().UTC()

	require.Len(t, store.ingestCalls, 1)
	ts := store.ingestCalls[0].Timestamp
	assert.True(t, !ts.Before(before) && !ts.After(after),
		"expected timestamp between %v and %v, got %v", before, after, ts)
}

func TestProcessErrorEvent_LongTraceback_Truncated(t *testing.T) {
	store := &mockStore{}
	w := newTestWorker(store)

	longTrace := string(make([]byte, 5000))
	ev := &aggregator.Event{
		Category:  "error",
		IsError:   true,
		Traceback: longTrace,
		Timestamp: time.Now().UTC(),
	}

	err := w.processErrorEvent(context.Background(), uuid.New(), uuid.New(), ev)
	require.NoError(t, err)

	require.Len(t, store.ingestCalls, 1)
	assert.LessOrEqual(t, len(store.ingestCalls[0].Message), 2000)
}

func TestProcessErrorEvent_EmptyTraceback_UsesCategoryAsMessage(t *testing.T) {
	store := &mockStore{}
	w := newTestWorker(store)

	ev := &aggregator.Event{
		Category:  "error",
		IsError:   true,
		Timestamp: time.Now().UTC(),
	}

	err := w.processErrorEvent(context.Background(), uuid.New(), uuid.New(), ev)
	require.NoError(t, err)

	require.Len(t, store.ingestCalls, 1)
	assert.Equal(t, "error", store.ingestCalls[0].Message)
}

// ── storeORMStat edge cases ─────────────────────────────────────────────────

func TestStoreORMStat_OptionalFieldsNil_WhenZero(t *testing.T) {
	store := &mockStore{}
	w := newTestWorker(store)

	batch := &aggregator.AggregatedBatch{
		Period: time.Now().UTC(),
	}
	stat := &aggregator.ORMModelStat{
		Model:     "res.partner",
		Method:    "read",
		CallCount: 10,
		TotalMS:   100,
		AvgMS:     10.0,
		// MaxMS, P95MS, SampleSQL are zero/empty.
	}

	err := w.storeORMStat(context.Background(), uuid.New(), batch, stat)
	require.NoError(t, err)

	require.Len(t, store.ormStatCalls, 1)
	assert.Nil(t, store.ormStatCalls[0].MaxMs)
	assert.Nil(t, store.ormStatCalls[0].P95Ms)
	assert.Nil(t, store.ormStatCalls[0].SampleSql)
}

func TestStoreORMStat_ZeroPeriod_UsesNow(t *testing.T) {
	store := &mockStore{}
	w := newTestWorker(store)

	batch := &aggregator.AggregatedBatch{
		// Period is zero.
	}
	stat := &aggregator.ORMModelStat{
		Model:     "res.partner",
		Method:    "read",
		CallCount: 1,
		TotalMS:   10,
	}

	before := time.Now().UTC()
	err := w.storeORMStat(context.Background(), uuid.New(), batch, stat)
	require.NoError(t, err)
	after := time.Now().UTC()

	require.Len(t, store.ormStatCalls, 1)
	period := store.ormStatCalls[0].Period
	assert.True(t, !period.Before(before) && !period.After(after))
}

func TestStoreORMStat_AllOptionalFieldsPopulated(t *testing.T) {
	store := &mockStore{}
	w := newTestWorker(store)

	batch := &aggregator.AggregatedBatch{Period: time.Now().UTC()}
	stat := &aggregator.ORMModelStat{
		Model:      "sale.order",
		Method:     "search_read",
		CallCount:  100,
		TotalMS:    5000,
		AvgMS:      50.0,
		MaxMS:      300,
		P95MS:      250,
		N1Detected: true,
		SampleSQL:  "SELECT * FROM sale_order WHERE id IN (1,2,3)",
	}

	err := w.storeORMStat(context.Background(), uuid.New(), batch, stat)
	require.NoError(t, err)

	require.Len(t, store.ormStatCalls, 1)
	call := store.ormStatCalls[0]
	assert.NotNil(t, call.MaxMs)
	assert.Equal(t, int32(300), *call.MaxMs)
	assert.NotNil(t, call.P95Ms)
	assert.Equal(t, int32(250), *call.P95Ms)
	assert.NotNil(t, call.SampleSql)
	assert.Contains(t, *call.SampleSql, "sale_order")
	assert.True(t, call.N1Detected)
	assert.NotNil(t, call.AvgMs)
	assert.Equal(t, "50.00", *call.AvgMs)
}

// ── DefaultIngestConfig / NewIngestWorker tests ─────────────────────────────

func TestDefaultIngestConfig(t *testing.T) {
	cfg := DefaultIngestConfig("agent:ingest", "ingest-workers")
	assert.Equal(t, "agent:ingest", cfg.Consumer.Stream)
	assert.Equal(t, "ingest-workers", cfg.Consumer.Group)
	assert.Equal(t, 2, cfg.WorkerCount)
	assert.Equal(t, int64(10), cfg.Consumer.BatchSize)
}

func TestNewIngestWorker_DefaultsWorkerCount(t *testing.T) {
	store := &mockStore{}
	logger := zerolog.Nop()

	cfg := IngestConfig{WorkerCount: 0}
	w := NewIngestWorker(store, nil, nil, nil, cfg, logger)
	assert.Equal(t, 2, w.config.WorkerCount)

	cfg = IngestConfig{WorkerCount: -1}
	w = NewIngestWorker(store, nil, nil, nil, cfg, logger)
	assert.Equal(t, 2, w.config.WorkerCount)
}
