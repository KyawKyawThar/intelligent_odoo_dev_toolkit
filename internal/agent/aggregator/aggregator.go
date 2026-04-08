package aggregator

import (
	"context"
	"sort"
	"time"

	"Intelligent_Dev_ToolKit_Odoo/internal/agent/sampler"

	"github.com/rs/zerolog"
)

// Config controls the aggregator's behavior.
type Config struct {
	// FlushInterval is how often the aggregator flushes a batch (default 30s).
	FlushInterval time.Duration

	// EnvID is the environment identifier included in every batch.
	EnvID string

	// EventChannelSize is the buffer size for the inbound event channel.
	// A larger buffer reduces the chance of blocking collectors during a
	// flush. Default: 4096.
	EventChannelSize int

	// MaxRawPerFlush caps the number of raw events kept per window to
	// bound memory. 0 = unlimited.
	MaxRawPerFlush int
}

// DefaultConfig returns sensible defaults.
func DefaultConfig(envID string) Config {
	return Config{
		FlushInterval:    30 * time.Second,
		EnvID:            envID,
		EventChannelSize: 4096,
		MaxRawPerFlush:   500,
	}
}

// statAccum accumulates data for a single model:method key within one window.
type statAccum struct {
	model     string
	method    string
	count     int
	totalMS   int
	maxMS     int
	n1        bool
	sampleSQL string
	durations []int // kept for P95 computation on flush
}

// Aggregator receives raw events from collectors, aggregates them into
// per-model:method stats, and emits AggregatedBatch payloads every flush
// interval. Critical events (errors, slow, N+1) are preserved as raw entries.
type Aggregator struct {
	config  Config
	sampler *sampler.Sampler
	logger  zerolog.Logger

	// EventCh is the inbound channel — collectors push events here.
	EventCh chan Event

	// SendCh is the outbound channel — the transport reads batches here.
	SendCh chan *AggregatedBatch

	// Current window state (reset on each flush).
	stats       map[string]*statAccum // key = "model:method"
	rawBuf      []Event
	summary     BatchSummary
	windowStart time.Time
}

// New creates an Aggregator. The returned Aggregator exposes EventCh (write)
// and SendCh (read) for integration with collectors and the transport layer.
func New(cfg Config, smp *sampler.Sampler, logger zerolog.Logger) *Aggregator {
	chSize := cfg.EventChannelSize
	if chSize <= 0 {
		chSize = 4096
	}
	return &Aggregator{
		config:  cfg,
		sampler: smp,
		logger:  logger.With().Str("component", "aggregator").Logger(),
		EventCh: make(chan Event, chSize),
		SendCh:  make(chan *AggregatedBatch, 4),
		stats:   make(map[string]*statAccum),
	}
}

// Run processes incoming events and flushes on the configured interval.
// It blocks until ctx is canceled, then performs a final flush.
func (a *Aggregator) Run(ctx context.Context) {
	a.windowStart = time.Now().UTC()
	ticker := time.NewTicker(a.config.FlushInterval)
	defer ticker.Stop()

	a.logger.Info().
		Dur("flush_interval", a.config.FlushInterval).
		Str("env_id", a.config.EnvID).
		Msg("aggregator started")

	for {
		select {
		case ev := <-a.EventCh:
			a.ingest(ev)

		case <-ticker.C:
			a.flushAndSend()

		case <-ctx.Done():
			// Drain remaining events in the channel.
			a.drainChannel()
			// Final flush.
			a.flushAndSend()
			a.logger.Info().Msg("aggregator stopped")
			return
		}
	}
}

// Ingest adds an event to the current window. Exported for direct use in
// tests; normally events are sent via EventCh.
func (a *Aggregator) Ingest(ev Event) {
	a.ingest(ev)
}

// ── internal ────────────────────────────────────────────────────────────────

func (a *Aggregator) ingest(ev Event) {
	// Always count in aggregated stats regardless of sampling.
	a.updateStats(ev)
	a.updateSummary(ev)

	// Decide whether to keep the raw event.
	// Compute-chain events are always kept — they are rare and critical for chain visualization.
	alwaysKeep := ev.IsCompute || ev.IsError
	if alwaysKeep || a.shouldKeepRaw(ev) {
		if alwaysKeep || a.config.MaxRawPerFlush <= 0 || len(a.rawBuf) < a.config.MaxRawPerFlush {
			a.rawBuf = append(a.rawBuf, ev)
		}
	}
}

func (a *Aggregator) updateStats(ev Event) {
	if ev.Model == "" && ev.Method == "" {
		return // nothing to aggregate (e.g. pure error with no model)
	}

	key := ev.Model + ":" + ev.Method
	st, ok := a.stats[key]
	if !ok {
		st = &statAccum{
			model:  ev.Model,
			method: ev.Method,
		}
		a.stats[key] = st
	}

	st.count++
	st.totalMS += ev.DurationMS
	if ev.DurationMS > st.maxMS {
		st.maxMS = ev.DurationMS
	}
	if ev.IsN1 {
		st.n1 = true
	}
	if ev.SQL != "" && st.sampleSQL == "" {
		st.sampleSQL = ev.SQL
	}
	st.durations = append(st.durations, ev.DurationMS)
}

func (a *Aggregator) updateSummary(ev Event) {
	a.summary.TotalQueries++
	a.summary.TotalDurationMS += ev.DurationMS

	if ev.IsError {
		a.summary.Errors++
	}
	if ev.IsN1 {
		a.summary.N1Patterns++
	}

	threshold := 0
	if a.sampler != nil {
		threshold = a.sampler.CurrentConfig().SlowThresholdMS
	}
	if threshold > 0 && ev.DurationMS > threshold {
		a.summary.SlowQueries++
	}
}

func (a *Aggregator) shouldKeepRaw(ev Event) bool {
	// If no sampler is set, keep everything.
	if a.sampler == nil {
		return true
	}
	return a.sampler.Allow(sampler.EventInfo{
		Category:   ev.Category,
		DurationMS: ev.DurationMS,
		IsError:    ev.IsError,
		IsN1:       ev.IsN1,
	})
}

func (a *Aggregator) flushAndSend() {
	batch := a.flush()
	if batch == nil {
		return
	}

	// Non-blocking send — if the transport is slow, drop the batch rather
	// than blocking the aggregator loop.
	select {
	case a.SendCh <- batch:
		a.logger.Info().
			Int("orm_stats", len(batch.ORMStats)).
			Int("raw_events", len(batch.RawEvents)).
			Int("total_queries", batch.Summary.TotalQueries).
			Msg("batch flushed")
	default:
		a.logger.Warn().Msg("SendCh full, batch dropped")
	}
}

// flush builds the batch from the current window and resets state.
func (a *Aggregator) flush() *AggregatedBatch {
	if a.summary.TotalQueries == 0 && len(a.rawBuf) == 0 {
		// Nothing happened in this window — don't send an empty batch.
		a.resetWindow()
		return nil
	}

	now := time.Now().UTC()
	durationMS := int(now.Sub(a.windowStart).Milliseconds())

	ormStats := make([]ORMModelStat, 0, len(a.stats))
	for _, st := range a.stats {
		avg := 0.0
		if st.count > 0 {
			avg = float64(st.totalMS) / float64(st.count)
		}
		ormStats = append(ormStats, ORMModelStat{
			Model:      st.model,
			Method:     st.method,
			CallCount:  st.count,
			TotalMS:    st.totalMS,
			AvgMS:      avg,
			MaxMS:      st.maxMS,
			P95MS:      percentile(st.durations, 95),
			N1Detected: st.n1,
			SampleSQL:  st.sampleSQL,
		})
	}

	batch := &AggregatedBatch{
		EnvID:      a.config.EnvID,
		Period:     a.windowStart,
		DurationMS: durationMS,
		ORMStats:   ormStats,
		RawEvents:  a.rawBuf,
		Summary:    a.summary,
	}

	a.resetWindow()
	return batch
}

func (a *Aggregator) resetWindow() {
	a.stats = make(map[string]*statAccum)
	a.rawBuf = nil
	a.summary = BatchSummary{}
	a.windowStart = time.Now().UTC()
}

func (a *Aggregator) drainChannel() {
	for {
		select {
		case ev := <-a.EventCh:
			a.ingest(ev)
		default:
			return
		}
	}
}

// percentile computes the Pth percentile from a slice of ints.
// Returns 0 for empty slices.
func percentile(values []int, p int) int {
	n := len(values)
	if n == 0 {
		return 0
	}
	sorted := make([]int, n)
	copy(sorted, values)
	sort.Ints(sorted)

	// Nearest-rank method.
	rank := (p * n) / 100
	if rank >= n {
		rank = n - 1
	}
	return sorted[rank]
}
