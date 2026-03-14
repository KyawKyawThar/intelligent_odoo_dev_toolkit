package errors

import (
	"context"
	"fmt"
	"strings"
	"time"

	"Intelligent_Dev_ToolKit_Odoo/internal/agent/collector"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/odoo"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/ringbuf"

	"github.com/rs/zerolog"
)

// irLoggingFields are the ir.logging columns we request from Odoo.
var irLoggingFields = []string{
	"id", "name", "level", "message", "path", "line", "func", "create_date",
}

// Collector polls Odoo's ir.logging model for ERROR/CRITICAL server-side log
// entries and pushes them into a ring buffer as ErrorEvents.
// It tracks the highest ir.logging ID seen so each entry is processed once.
type Collector struct {
	client    *odoo.Client
	buf       *ringbuf.RingBuffer[ErrorEvent]
	lastLogID int // high-water mark — only fetch records with id > lastLogID
	logger    zerolog.Logger
}

// NewCollector creates a Collector.
// buf is the shared ring buffer that the Flusher will drain.
func NewCollector(
	client *odoo.Client,
	buf *ringbuf.RingBuffer[ErrorEvent],
	logger zerolog.Logger,
) *Collector {
	return &Collector{
		client: client,
		buf:    buf,
		logger: logger.With().Str("component", "error-collector").Logger(),
	}
}

// Poll fetches new ir.logging records since the last call and pushes them into
// the ring buffer. Safe to call from a ticker goroutine.
func (c *Collector) Poll(ctx context.Context) error {
	domain := []any{
		[]any{"level", "in", []any{"ERROR", "CRITICAL"}},
		[]any{"type", "=", "server"},
		[]any{"id", ">", c.lastLogID},
	}

	records, err := collector.FetchRecordsWithDomain(
		ctx, c.client, "ir.logging", irLoggingFields, domain,
		map[string]any{"order": "id asc", "limit": 200},
	)
	if err != nil {
		return fmt.Errorf("ir.logging search_read: %w", err)
	}

	pushed := 0
	for _, r := range records {
		ev, ok := c.toEvent(r)
		if !ok {
			continue
		}
		c.buf.Push(ev)
		if ev.LogID > c.lastLogID {
			c.lastLogID = ev.LogID
		}
		pushed++
	}

	if pushed > 0 {
		c.logger.Info().
			Int("fetched", len(records)).
			Int("pushed", pushed).
			Int("buf_len", c.buf.Len()).
			Msg("error events captured")
	}

	return nil
}

// RunLoop polls on interval until ctx is canceled.
func (c *Collector) RunLoop(ctx context.Context, interval time.Duration) {
	c.logger.Info().Dur("interval", interval).Msg("starting error log collector")

	if err := c.Poll(ctx); err != nil {
		c.logger.Error().Err(err).Msg("initial error log poll failed")
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.logger.Info().Msg("error log collector stopped")
			return
		case <-ticker.C:
			if err := c.Poll(ctx); err != nil {
				c.logger.Error().Err(err).Msg("error log poll failed")
			}
		}
	}
}

// ─── private ────────────────────────────────────────────────────────────────

func (c *Collector) toEvent(r map[string]any) (ErrorEvent, bool) {
	id := intVal(r["id"])
	if id == 0 {
		return ErrorEvent{}, false
	}

	message := stringVal(r["message"])
	if message == "" {
		return ErrorEvent{}, false
	}

	errorType := ParseErrorType(message)
	traceback := ExtractTraceback(message)
	module, model := splitLoggerName(stringVal(r["name"]))
	capturedAt := parseOdooDate(stringVal(r["create_date"]))

	return ErrorEvent{
		Signature:  ComputeSignature(errorType, message),
		ErrorType:  errorType,
		Message:    message,
		Module:     module,
		Model:      model,
		Traceback:  traceback,
		LogID:      id,
		CapturedAt: capturedAt,
	}, true
}

func splitLoggerName(name string) (module, model string) {
	if name == "" {
		return "", ""
	}
	name = strings.TrimPrefix(name, "odoo.addons.")
	name = strings.TrimPrefix(name, "odoo.")
	parts := strings.SplitN(name, ".", 2)
	module = parts[0]
	if len(parts) == 2 {
		model = parts[1]
	}
	return module, model
}

func parseOdooDate(s string) time.Time {
	for _, layout := range []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		time.RFC3339,
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Now().UTC()
}

func intVal(v any) int {
	if v == nil {
		return 0
	}
	switch t := v.(type) {
	case int:
		return t
	case float64:
		return int(t)
	}
	return 0
}

func stringVal(v any) string {
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}
