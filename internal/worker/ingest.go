package worker

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"time"

	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/aggregator"
	"Intelligent_Dev_ToolKit_Odoo/internal/service"
	"Intelligent_Dev_ToolKit_Odoo/internal/storage"

	"bytes"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

// IngestWorker processes AggregatedBatch messages from the Redis "agent:ingest"
// stream. For each batch it:
//  1. Upserts error groups into PostgreSQL (metadata only)
//  2. Stores raw tracebacks in S3 (full traceback data)
//  3. Stores ORM stats in PostgreSQL
//  4. Computes module overhead and inserts budget samples
type IngestWorker struct {
	store     db.Store
	s3        *storage.S3Client
	rdb       *redis.Client
	budgetSvc service.BudgetServicer
	config    IngestConfig
	logger    zerolog.Logger
}

// IngestConfig holds worker-specific configuration.
type IngestConfig struct {
	Consumer ConsumerConfig
	// WorkerCount is how many parallel consumer goroutines to run (default: 2).
	WorkerCount int
	// AlertStream is the Redis stream key for publishing threshold breaches
	// (default: "agent:alert"). Empty string disables alert publishing.
	AlertStream string
}

// DefaultIngestConfig returns sensible defaults.
func DefaultIngestConfig(stream, group string) IngestConfig {
	return IngestConfig{
		Consumer:    DefaultConsumerConfig(stream, group),
		WorkerCount: 2,
	}
}

// NewIngestWorker creates a new IngestWorker.
func NewIngestWorker(
	store db.Store,
	s3Client *storage.S3Client,
	rdb *redis.Client,
	budgetSvc service.BudgetServicer,
	cfg IngestConfig,
	logger zerolog.Logger,
) *IngestWorker {
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = 2
	}
	return &IngestWorker{
		store:     store,
		s3:        s3Client,
		rdb:       rdb,
		budgetSvc: budgetSvc,
		config:    cfg,
		logger:    logger.With().Str("component", "ingest-worker").Logger(),
	}
}

// Run starts the ingest worker pool. It creates the consumer group and
// spawns WorkerCount goroutines. Blocks until ctx is canceled.
func (w *IngestWorker) Run(ctx context.Context) error {
	cfg := w.config.Consumer

	if err := EnsureConsumerGroup(ctx, w.rdb, cfg.Stream, cfg.Group); err != nil {
		return fmt.Errorf("ensure consumer group: %w", err)
	}

	w.logger.Info().
		Int("workers", w.config.WorkerCount).
		Str("stream", cfg.Stream).
		Str("group", cfg.Group).
		Msg("ingest worker pool starting")

	done := make(chan struct{})
	for i := range w.config.WorkerCount {
		consumerName := fmt.Sprintf("ingest-%d", i)
		go func() {
			RunConsumer(ctx, w.rdb, cfg, consumerName, w.processMessage, w.logger)
		}()
	}

	<-ctx.Done()
	close(done)
	w.logger.Info().Msg("ingest worker pool stopped")
	return nil
}

// processMessage handles a single batch message from the Redis stream.
func (w *IngestWorker) processMessage(ctx context.Context, tenantID, data string) error {
	tid, err := uuid.Parse(tenantID)
	if err != nil {
		return fmt.Errorf("invalid tenant_id %q: %w", tenantID, err)
	}

	var batch aggregator.AggregatedBatch
	if uerr := json.Unmarshal([]byte(data), &batch); uerr != nil {
		return fmt.Errorf("unmarshal batch: %w", uerr)
	}

	envID, err := uuid.Parse(batch.EnvID)
	if err != nil {
		return fmt.Errorf("invalid env_id %q: %w", batch.EnvID, err)
	}

	errCount := w.processErrorEvents(ctx, tid, envID, &batch)
	ormCount := w.processORMStats(ctx, envID, &batch)

	profilerEvents := extractProfilerEvents(&batch)
	profilerCount := w.processProfilerEvents(ctx, envID, &batch, profilerEvents)
	budgetSamples := w.processBudgetAndAlerts(ctx, tenantID, envID, &batch, profilerEvents)

	w.logger.Info().
		Str("env_id", batch.EnvID).
		Int("errors_processed", errCount).
		Int("orm_stats_stored", ormCount).
		Int("profiler_events", profilerCount).
		Int("budget_samples", budgetSamples).
		Int("total_queries", batch.Summary.TotalQueries).
		Msg("batch processed")

	return nil
}

func (w *IngestWorker) processErrorEvents(ctx context.Context, tid, envID uuid.UUID, batch *aggregator.AggregatedBatch) int {
	errCount := 0
	for i := range batch.RawEvents {
		ev := &batch.RawEvents[i]
		if !ev.IsError {
			continue
		}
		if err := w.processErrorEvent(ctx, tid, envID, ev); err != nil {
			w.logger.Error().Err(err).
				Str("env_id", batch.EnvID).
				Str("category", ev.Category).
				Msg("failed to process error event")
			continue // don't fail the whole batch for one event
		}
		errCount++
	}
	return errCount
}

func (w *IngestWorker) processORMStats(ctx context.Context, envID uuid.UUID, batch *aggregator.AggregatedBatch) int {
	ormCount := 0
	for i := range batch.ORMStats {
		stat := &batch.ORMStats[i]
		if err := w.storeORMStat(ctx, envID, batch, stat); err != nil {
			w.logger.Error().Err(err).
				Str("model", stat.Model).
				Str("method", stat.Method).
				Msg("failed to store ORM stat")
			continue
		}
		ormCount++
	}
	return ormCount
}

func (w *IngestWorker) processProfilerEvents(ctx context.Context, envID uuid.UUID, batch *aggregator.AggregatedBatch, profilerEvents []service.ProfilerEvent) int {
	if len(profilerEvents) == 0 {
		return 0
	}
	if err := w.processProfilerEventsFromSlice(ctx, envID, batch, profilerEvents); err != nil {
		w.logger.Error().Err(err).
			Str("env_id", batch.EnvID).
			Msg("failed to process profiler events")
		return 0
	}
	return len(profilerEvents)
}

func (w *IngestWorker) processBudgetAndAlerts(ctx context.Context, tenantID string, envID uuid.UUID, batch *aggregator.AggregatedBatch, profilerEvents []service.ProfilerEvent) int {
	if w.budgetSvc == nil || len(profilerEvents) == 0 {
		return 0
	}

	result, err := w.budgetSvc.CalculateOverhead(ctx, envID, profilerEvents, w.logger)
	if err != nil {
		w.logger.Error().Err(err).
			Str("env_id", batch.EnvID).
			Msg("failed to calculate overhead")
		return 0
	}

	w.publishThresholdBreaches(ctx, tenantID, batch.EnvID, result.Breaches)

	return result.SamplesInserted
}

func (w *IngestWorker) publishThresholdBreaches(ctx context.Context, tenantID, envID string, breaches []service.OverheadBreach) {
	if w.rdb == nil || w.config.AlertStream == "" || len(breaches) == 0 {
		return
	}

	for _, b := range breaches {
		breach := ThresholdBreach{
			TenantID:     tenantID,
			EnvID:        envID,
			BudgetID:     b.BudgetID.String(),
			Module:       b.Module,
			Endpoint:     b.Endpoint,
			OverheadPct:  b.OverheadPct,
			ThresholdPct: b.ThresholdPct,
			TotalMS:      b.TotalMS,
			ModuleMS:     b.ModuleMS,
			Breakdown:    b.Breakdown,
		}
		if err := PublishThresholdBreach(ctx, w.rdb, w.config.AlertStream, breach); err != nil {
			w.logger.Error().Err(err).
				Str("module", b.Module).
				Float64("overhead_pct", b.OverheadPct).
				Msg("failed to publish threshold breach")
		}
	}
}

// processErrorEvent upserts an error group in the DB and stores the raw
// traceback in S3.
func (w *IngestWorker) processErrorEvent(
	ctx context.Context,
	tenantID, envID uuid.UUID,
	ev *aggregator.Event,
) error {
	// Generate a simple signature from error type + module + model.
	signature := errorSignature(ev)

	// Store raw traceback in S3 if we have one.
	var traceRef *string
	if ev.Traceback != "" && w.s3 != nil {
		ts := ev.Timestamp.UTC().Format("20060102T150405Z")
		key := storage.TraceKey(tenantID.String(), envID.String(), signature, ts)

		compressed, err := gzipJSON(map[string]any{
			"traceback":   ev.Traceback,
			"message":     ev.Category,
			"module":      ev.Module,
			"model":       ev.Model,
			"sql":         ev.SQL,
			"user_id":     ev.UserID,
			"captured_at": ev.Timestamp,
		})
		if err != nil {
			return fmt.Errorf("compress trace: %w", err)
		}

		if err := w.s3.PutGzip(ctx, key, compressed); err != nil {
			return fmt.Errorf("s3 put trace: %w", err)
		}
		traceRef = &key
	}

	// Build optional fields.
	var module, model *string
	if ev.Module != "" {
		module = &ev.Module
	}
	if ev.Model != "" {
		model = &ev.Model
	}

	var affectedUIDs []int32
	if ev.UserID > 0 {
		affectedUIDs = []int32{int32(ev.UserID)} //nolint:gosec // UserID is a small positive Odoo UID, fits int32
	}

	// Build error message from traceback or category.
	message := ev.Traceback
	if message == "" {
		message = ev.Category
	}
	// Truncate message for DB storage (keep first 2000 chars).
	if len(message) > 2000 {
		message = message[:2000]
	}

	errorType := "Error"
	if ev.Category != "" {
		errorType = ev.Category
	}

	ts := ev.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	return w.store.IngestErrorBatchTx(ctx, db.IngestErrorBatchParams{
		EnvID:          envID,
		TenantID:       tenantID,
		Signature:      signature,
		ErrorType:      errorType,
		Message:        message,
		Module:         module,
		Model:          model,
		Timestamp:      ts,
		AffectedUIDs:   affectedUIDs,
		RawTraceRef:    traceRef,
		SpikeThreshold: 0, // TODO: make configurable
	})
}

// storeORMStat persists a single ORM model stat into the orm_stats table.
func (w *IngestWorker) storeORMStat(
	ctx context.Context,
	envID uuid.UUID,
	batch *aggregator.AggregatedBatch,
	stat *aggregator.ORMModelStat,
) error {
	avgStr := fmt.Sprintf("%.2f", stat.AvgMS)

	var maxMs *int32
	if stat.MaxMS > 0 {
		v := int32(stat.MaxMS) //nolint:gosec // G115: ORM durations won't exceed int32 range
		maxMs = &v
	}

	var p95Ms *int32
	if stat.P95MS > 0 {
		v := int32(stat.P95MS) //nolint:gosec // G115: ORM durations won't exceed int32 range
		p95Ms = &v
	}

	var sampleSQL *string
	if stat.SampleSQL != "" {
		sampleSQL = &stat.SampleSQL
	}

	period := batch.Period
	if period.IsZero() {
		period = time.Now().UTC()
	}

	_, err := w.store.InsertORMStat(ctx, db.InsertORMStatParams{
		EnvID:      envID,
		Model:      stat.Model,
		Method:     stat.Method,
		CallCount:  int32(stat.CallCount), //nolint:gosec // G115: per-window count won't exceed int32 range
		TotalMs:    int32(stat.TotalMS),   //nolint:gosec // G115: per-window total won't exceed int32 range
		AvgMs:      &avgStr,
		MaxMs:      maxMs,
		P95Ms:      p95Ms,
		N1Detected: stat.N1Detected,
		SampleSql:  sampleSQL,
		Period:     period,
	})
	return err
}

// errorSignature generates a deterministic signature for grouping.
func errorSignature(ev *aggregator.Event) string {
	sig := ev.Category
	if ev.Module != "" {
		sig += ":" + ev.Module
	}
	if ev.Model != "" {
		sig += ":" + ev.Model
	}
	if ev.Method != "" {
		sig += ":" + ev.Method
	}
	return sig
}

// extractProfilerEvents pulls profiler/orm/sql events from the batch into
// service.ProfilerEvent slice for reuse by both waterfall building and
// overhead calculation.
func extractProfilerEvents(batch *aggregator.AggregatedBatch) []service.ProfilerEvent {
	var events []service.ProfilerEvent
	for i := range batch.RawEvents {
		ev := &batch.RawEvents[i]
		if ev.Category == "profiler" || ev.Category == "orm" || ev.Category == "sql" {
			events = append(events, service.ProfilerEvent{
				Category:     ev.Category,
				Model:        ev.Model,
				Method:       ev.Method,
				DurationMS:   ev.DurationMS,
				IsError:      ev.IsError,
				IsN1:         ev.IsN1,
				SQL:          ev.SQL,
				Module:       ev.Module,
				Traceback:    ev.Traceback,
				UserID:       ev.UserID,
				Timestamp:    ev.Timestamp,
				FieldName:    ev.FieldName,
				IsCompute:    ev.IsCompute,
				DependsOn:    ev.DependsOn,
				TriggerField: ev.TriggerField,
			})
		}
	}
	return events
}

// processProfilerEventsFromSlice builds a waterfall and stores a profiler
// recording from pre-extracted profiler events.
func (w *IngestWorker) processProfilerEventsFromSlice(
	ctx context.Context,
	envID uuid.UUID,
	batch *aggregator.AggregatedBatch,
	profilerEvents []service.ProfilerEvent,
) error {
	waterfallJSON, n1JSON, meta := service.BuildWaterfallFromEvents(profilerEvents)
	computeChainJSON := service.BuildComputeChainFromEvents(profilerEvents)

	totalMS := int32(meta.TotalMS) //nolint:gosec // profiler durations fit int32
	if totalMS == 0 {
		totalMS = int32(batch.Summary.TotalDurationMS) //nolint:gosec // batch durations fit int32
	}

	sqlCount := int32(meta.SQLCount) //nolint:gosec // count fits int32
	sqlMS := int32(meta.SQLMS)       //nolint:gosec // duration fits int32
	pythonMS := int32(meta.PythonMS) //nolint:gosec // duration fits int32

	name := fmt.Sprintf("batch-%s", batch.Period.UTC().Format("20060102T150405Z"))

	_, err := w.store.CreateProfilerRecording(ctx, db.CreateProfilerRecordingParams{
		EnvID:        envID,
		Name:         name,
		TotalMs:      totalMS,
		SqlCount:     &sqlCount,
		SqlMs:        &sqlMS,
		PythonMs:     &pythonMS,
		Waterfall:    waterfallJSON,
		ComputeChain: computeChainJSON,
		N1Patterns:   n1JSON,
	})
	if err != nil {
		return fmt.Errorf("create profiler recording: %w", err)
	}

	return nil
}

// gzipJSON marshals v to JSON and gzip-compresses it.
func gzipJSON(v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(data); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
