package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

// AlertWorker consumes threshold-breach messages from the Redis "agent:alert"
// stream. For each breach it:
//  1. Checks for a recent duplicate alert (dedup window, default 10 min)
//  2. Determines severity (warning vs critical)
//  3. Creates an alert + delivery records via CreateAlertWithDeliveryTx
type AlertWorker struct {
	store  db.Store
	rdb    *redis.Client
	config AlertConfig
	logger zerolog.Logger
}

// AlertConfig holds alert-worker-specific configuration.
type AlertConfig struct {
	Consumer ConsumerConfig
	// WorkerCount is how many parallel consumer goroutines to run (default: 1).
	WorkerCount int
	// DedupeWindowMin is how many minutes to look back for duplicate alerts
	// before creating a new one (default: 10).
	DedupeWindowMin int
}

// DefaultAlertConfig returns sensible defaults.
func DefaultAlertConfig(stream, group string) AlertConfig {
	return AlertConfig{
		Consumer:        DefaultConsumerConfig(stream, group),
		WorkerCount:     1,
		DedupeWindowMin: 10,
	}
}

// NewAlertWorker creates a new AlertWorker.
func NewAlertWorker(
	store db.Store,
	rdb *redis.Client,
	cfg AlertConfig,
	logger zerolog.Logger,
) *AlertWorker {
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = 1
	}
	if cfg.DedupeWindowMin <= 0 {
		cfg.DedupeWindowMin = 10
	}
	return &AlertWorker{
		store:  store,
		rdb:    rdb,
		config: cfg,
		logger: logger.With().Str("component", "alert-worker").Logger(),
	}
}

// Run starts the alert worker pool. Blocks until ctx is canceled.
func (w *AlertWorker) Run(ctx context.Context) error {
	cfg := w.config.Consumer

	if err := EnsureConsumerGroup(ctx, w.rdb, cfg.Stream, cfg.Group); err != nil {
		return fmt.Errorf("ensure consumer group: %w", err)
	}

	w.logger.Info().
		Int("workers", w.config.WorkerCount).
		Str("stream", cfg.Stream).
		Str("group", cfg.Group).
		Int("dedupe_window_min", w.config.DedupeWindowMin).
		Msg("alert worker pool starting")

	done := make(chan struct{})
	for i := range w.config.WorkerCount {
		consumerName := fmt.Sprintf("alert-%d", i)
		go func() {
			RunConsumer(ctx, w.rdb, cfg, consumerName, w.processMessage, w.logger)
		}()
	}

	<-ctx.Done()
	close(done)
	w.logger.Info().Msg("alert worker pool stopped")
	return nil
}

// ThresholdBreach is the payload published to the agent:alert Redis stream
// when an overhead calculation exceeds a budget threshold.
type ThresholdBreach struct {
	TenantID     string  `json:"tenant_id"`
	EnvID        string  `json:"env_id"`
	BudgetID     string  `json:"budget_id"`
	Module       string  `json:"module"`
	Endpoint     string  `json:"endpoint"`
	OverheadPct  float64 `json:"overhead_pct"`
	ThresholdPct int32   `json:"threshold_pct"`
	TotalMS      int     `json:"total_ms"`
	ModuleMS     int     `json:"module_ms"`
	Breakdown    any     `json:"breakdown,omitempty"`
}

// processMessage handles a single alert check from the Redis stream.
func (w *AlertWorker) processMessage(ctx context.Context, tenantID, data string) error {
	tid, err := uuid.Parse(tenantID)
	if err != nil {
		return fmt.Errorf("invalid tenant_id %q: %w", tenantID, err)
	}

	var breach ThresholdBreach
	err = json.Unmarshal([]byte(data), &breach)
	if err != nil {
		return fmt.Errorf("unmarshal breach: %w", err)
	}

	envID, err := uuid.Parse(breach.EnvID)
	if err != nil {
		return fmt.Errorf("invalid env_id %q: %w", breach.EnvID, err)
	}

	// Dedup: check if a similar alert was created recently.
	dedupeKey, err := json.Marshal(map[string]string{
		"module":    breach.Module,
		"budget_id": breach.BudgetID,
	})
	if err != nil {
		return fmt.Errorf("marshal dedup key: %w", err)
	}

	exists, err := w.store.HasRecentAlert(ctx, db.HasRecentAlertParams{
		EnvID:   envID,
		Type:    "budget_exceeded",
		Column3: dedupeKey,
		Column4: int32(w.config.DedupeWindowMin), //nolint:gosec // config value fits int32
	})
	if err != nil {
		w.logger.Error().Err(err).Msg("dedup check failed, proceeding with alert creation")
	}
	if exists {
		w.logger.Debug().
			Str("module", breach.Module).
			Str("budget_id", breach.BudgetID).
			Msg("duplicate alert suppressed")
		return nil
	}

	// Determine severity.
	severity := classifySeverity(breach.OverheadPct, float64(breach.ThresholdPct))

	// Build alert message.
	message := fmt.Sprintf(
		"Module '%s' exceeded performance budget: %.1f%% overhead (threshold: %d%%)",
		breach.Module, breach.OverheadPct, breach.ThresholdPct,
	)

	// Build metadata.
	metadata, err := json.Marshal(map[string]any{
		"module":        breach.Module,
		"endpoint":      breach.Endpoint,
		"budget_id":     breach.BudgetID,
		"overhead_pct":  breach.OverheadPct,
		"threshold_pct": breach.ThresholdPct,
		"exceeded_by":   breach.OverheadPct - float64(breach.ThresholdPct),
		"total_ms":      breach.TotalMS,
		"module_ms":     breach.ModuleMS,
		"breakdown":     breach.Breakdown,
	})
	if err != nil {
		return fmt.Errorf("marshal alert metadata: %w", err)
	}

	// Create alert + delivery records in a single transaction.
	alert, err := w.store.CreateAlertWithDeliveryTx(ctx, db.CreateAlertWithDeliveryParams{
		EnvID:    envID,
		TenantID: tid,
		Type:     "budget_exceeded",
		Severity: severity,
		Message:  message,
		Metadata: metadata,
	})
	if err != nil {
		return fmt.Errorf("create alert: %w", err)
	}

	w.logger.Info().
		Str("alert_id", alert.ID.String()).
		Str("module", breach.Module).
		Str("severity", severity).
		Float64("overhead_pct", breach.OverheadPct).
		Int32("threshold_pct", breach.ThresholdPct).
		Msg("alert created")

	return nil
}

// classifySeverity determines alert severity based on how much the overhead
// exceeds the threshold:
//   - critical: overhead >= 1.5× threshold (50% or more over)
//   - warning:  otherwise
func classifySeverity(overheadPct, thresholdPct float64) string {
	if thresholdPct > 0 && overheadPct >= thresholdPct*1.5 {
		return "critical"
	}
	return "warning"
}

// PublishThresholdBreach publishes a threshold breach message to the
// agent:alert Redis stream for the alert worker to consume.
func PublishThresholdBreach(
	ctx context.Context,
	rdb *redis.Client,
	stream string,
	breach ThresholdBreach,
) error {
	data, err := json.Marshal(breach)
	if err != nil {
		return fmt.Errorf("marshal breach: %w", err)
	}

	return rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		MaxLen: 10000,
		Approx: true,
		Values: map[string]interface{}{
			"tenant_id": breach.TenantID,
			"data":      string(data),
		},
	}).Err()
}

// ── Time helper (for testing) ───────────────────────────────────────────────

// nowFunc is overridable for testing. Defaults to time.Now.
var nowFunc = time.Now //nolint:unused // reserved for future test overrides
