package hook

import (
	"bufio"
	"context"
	"io"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"Intelligent_Dev_ToolKit_Odoo/internal/agent/aggregator"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/sampler"

	"github.com/rs/zerolog"
)

// ────────────────────────────────────────────────────────────────────────────
// ORM Collector — tails Odoo log file for ORM query lines
// ────────────────────────────────────────────────────────────────────────────

// ORMCollectorConfig controls the ORM log collector.
type ORMCollectorConfig struct {
	// Path to the Odoo server log file.
	Path string

	// PollInterval is how often we check for new data. Default: 1s.
	PollInterval time.Duration

	// MaxLineLength caps the length of a single line. Default: 1 MB.
	MaxLineLength int

	// N1WindowSize is the sliding window for N+1 detection. Default: 2s.
	N1WindowSize time.Duration

	// N1Threshold is how many similar queries in the window triggers N+1. Default: 10.
	N1Threshold int
}

// DefaultORMCollectorConfig returns sensible defaults.
func DefaultORMCollectorConfig(path string) ORMCollectorConfig {
	return ORMCollectorConfig{
		Path:          path,
		PollInterval:  1 * time.Second,
		MaxLineLength: 1 << 20,
		N1WindowSize:  2 * time.Second,
		N1Threshold:   10,
	}
}

// ORMCollector tails the Odoo server log file and pushes ORM query events
// into the aggregator pipeline. It detects N+1 query patterns on the fly.
type ORMCollector struct {
	cfg     ORMCollectorConfig
	eventCh chan<- aggregator.Event
	sampler *sampler.Sampler
	logger  zerolog.Logger
	n1      *n1Tracker

	linesRead     int64
	eventsEmitted int64
}

// NewORMCollector creates a new ORM log collector. sampler may be nil.
func NewORMCollector(
	cfg ORMCollectorConfig,
	eventCh chan<- aggregator.Event,
	smp *sampler.Sampler,
	logger zerolog.Logger,
) *ORMCollector {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 1 * time.Second
	}
	if cfg.MaxLineLength <= 0 {
		cfg.MaxLineLength = 1 << 20
	}
	if cfg.N1WindowSize <= 0 {
		cfg.N1WindowSize = 2 * time.Second
	}
	if cfg.N1Threshold <= 0 {
		cfg.N1Threshold = 10
	}

	return &ORMCollector{
		cfg:     cfg,
		eventCh: eventCh,
		sampler: smp,
		logger:  logger.With().Str("component", "orm-collector").Logger(),
		n1:      newN1Tracker(cfg.N1WindowSize, cfg.N1Threshold),
	}
}

// Run blocks until ctx is canceled. Tails the log file and emits ORM events.
func (c *ORMCollector) Run(ctx context.Context) {
	c.logger.Info().
		Str("path", c.cfg.Path).
		Int("n1_threshold", c.cfg.N1Threshold).
		Dur("n1_window", c.cfg.N1WindowSize).
		Msg("starting ORM log collector")

	// "No data" warning timer.
	noDataTimer := time.NewTimer(5 * time.Minute)
	defer noDataTimer.Stop()

	go func() {
		select {
		case <-ctx.Done():
		case <-noDataTimer.C:
			if atomic.LoadInt64(&c.eventsEmitted) == 0 {
				c.logger.Warn().
					Str("path", c.cfg.Path).
					Msg("no ORM query lines detected in 5 minutes — " +
						"ensure Odoo is configured with --log-handler=odoo.models.query:INFO")
			}
		}
	}()

	retryBackoff := c.cfg.PollInterval
	const maxBackoff = 30 * time.Second

	for {
		err := c.tailFile(ctx)
		if err != nil && ctx.Err() == nil {
			c.logger.Warn().Err(err).Dur("retry_in", retryBackoff).Msg("ORM collector error, will retry")

			select {
			case <-ctx.Done():
			case <-time.After(retryBackoff):
			}
			retryBackoff = min(retryBackoff*2, maxBackoff)
			continue
		}

		// tailFile returned normally — reset backoff.
		retryBackoff = c.cfg.PollInterval

		select {
		case <-ctx.Done():
			c.logger.Info().
				Int64("lines_read", atomic.LoadInt64(&c.linesRead)).
				Int64("events_emitted", atomic.LoadInt64(&c.eventsEmitted)).
				Msg("ORM log collector stopped")
			return
		case <-time.After(c.cfg.PollInterval):
		}
	}
}

// tailFile opens the log, seeks to end, and reads until error or ctx cancel.
func (c *ORMCollector) tailFile(ctx context.Context) error {
	f, err := os.Open(c.cfg.Path)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // read-only file; close error is harmless

	lastSize, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}

	acc := &EntryAccumulator{}
	reader := bufio.NewReaderSize(f, 64*1024)
	ticker := time.NewTicker(c.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.flushAcc(acc)
			return ctx.Err()

		case <-ticker.C:
			reopened, err := c.checkRotation(f, lastSize, acc)
			if err != nil {
				return err
			}
			if reopened {
				return nil
			}

			// Read available lines.
			c.readLines(reader, acc)

			if pos, err := f.Seek(0, io.SeekCurrent); err == nil {
				lastSize = pos
			}
		}
	}
}

// flushAcc flushes the accumulator and handles any remaining entry.
func (c *ORMCollector) flushAcc(acc *EntryAccumulator) {
	if entry := acc.Flush(); entry != nil {
		c.handleEntry(entry)
	}
}

// checkRotation detects truncation and rename/recreate rotation.
// Returns true if the file was rotated and the caller should reopen.
func (c *ORMCollector) checkRotation(f *os.File, lastSize int64, acc *EntryAccumulator) (bool, error) {
	info, err := f.Stat()
	if err != nil {
		return false, err
	}
	if info.Size() < lastSize {
		c.flushAcc(acc)
		return true, nil
	}

	if rotated, _ := c.fileRotated(f); rotated { //nolint:errcheck // best-effort rotation check; stat errors treated as no rotation
		c.flushAcc(acc)
		return true, nil
	}

	return false, nil
}

func (c *ORMCollector) readLines(reader *bufio.Reader, acc *EntryAccumulator) {
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			line = stripNewline(line)
			if len(line) > c.cfg.MaxLineLength {
				line = line[:c.cfg.MaxLineLength]
			}
			atomic.AddInt64(&c.linesRead, 1)

			if entry := acc.Feed(line); entry != nil {
				c.handleEntry(entry)
			}
		}
		if err != nil {
			return
		}
	}
}

func (c *ORMCollector) handleEntry(entry *LogEntry) {
	if !IsORMQueryEntry(entry) {
		return
	}

	ev, ok := ToORMEvent(entry)
	if !ok {
		return
	}

	// N+1 detection.
	ev.IsN1 = c.n1.Check(ev.SQL, ev.Timestamp)

	// Apply sampling.
	if c.sampler != nil && !c.sampler.Allow(sampler.EventInfo{
		Category:   ev.Category,
		DurationMS: ev.DurationMS,
		IsError:    ev.IsError,
		IsN1:       ev.IsN1,
	}) {
		return
	}

	// Non-blocking send.
	select {
	case c.eventCh <- ev:
		atomic.AddInt64(&c.eventsEmitted, 1)
	default:
		// Channel full — drop event. Aggregated stats are approximate anyway.
	}
}

func (c *ORMCollector) fileRotated(f *os.File) (bool, error) {
	fdInfo, err := f.Stat()
	if err != nil {
		return false, err
	}
	pathInfo, err := os.Stat(c.cfg.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, err
	}
	return !os.SameFile(fdInfo, pathInfo), nil
}

// ────────────────────────────────────────────────────────────────────────────
// N+1 query pattern detector
// ────────────────────────────────────────────────────────────────────────────

// n1Tracker detects N+1 query patterns by counting repeated normalized SQL
// signatures within a sliding time window.
type n1Tracker struct {
	window    time.Duration
	threshold int
	recent    map[string][]time.Time
	checks    int
}

func newN1Tracker(window time.Duration, threshold int) *n1Tracker {
	return &n1Tracker{
		window:    window,
		threshold: threshold,
		recent:    make(map[string][]time.Time),
	}
}

// Check returns true if the query matches an N+1 pattern (same normalized SQL
// repeated >= threshold times within the window).
func (t *n1Tracker) Check(sql string, ts time.Time) bool {
	sig := normalizeSQL(sql)
	if sig == "" {
		return false
	}

	cutoff := ts.Add(-t.window)

	// Prune old entries for this signature.
	times := t.recent[sig]
	start := 0
	for start < len(times) && times[start].Before(cutoff) {
		start++
	}
	times = append(times[start:], ts)
	t.recent[sig] = times

	// Periodic full prune to prevent unbounded memory growth.
	t.checks++
	if t.checks%500 == 0 {
		t.pruneStale(cutoff)
	}

	return len(times) >= t.threshold
}

func (t *n1Tracker) pruneStale(cutoff time.Time) {
	for sig, times := range t.recent {
		start := 0
		for start < len(times) && times[start].Before(cutoff) {
			start++
		}
		if start >= len(times) {
			delete(t.recent, sig)
		} else {
			t.recent[sig] = times[start:]
		}
	}
}

// normalizeSQLRe replaces dynamic literal values with placeholders so that
// queries differing only in bound values share the same signature.
var normalizeSQLRe = regexp.MustCompile(
	`(?:` +
		`'[^']*'` + // single-quoted strings
		`|` +
		`\b\d+\b` + // integers
		`)`,
)

// normalizeSQL replaces literal values with '?' to produce a query signature.
func normalizeSQL(sql string) string {
	s := strings.TrimSpace(sql)
	if s == "" {
		return ""
	}
	return normalizeSQLRe.ReplaceAllString(s, "?")
}
