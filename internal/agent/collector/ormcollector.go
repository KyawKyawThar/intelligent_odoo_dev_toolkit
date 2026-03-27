package collector

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"Intelligent_Dev_ToolKit_Odoo/internal/agent/aggregator"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/odoo"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/sampler"

	"github.com/rs/zerolog"
)

// ormLogFields are the ir.logging columns we request for ORM query entries.
var ormLogFields = []string{
	"id", "name", "level", "message", "create_date",
}

// ORMLogCollector polls Odoo's ir.logging model for ORM query entries
// (loggers: odoo.models.query, odoo.sql_db) and pushes them as
// aggregator.Event into the aggregator pipeline.
//
// This is a fallback for environments where log file access is unavailable.
// Most Odoo deployments do NOT log ORM queries to ir.logging by default —
// the log file tailer (hook.ORMCollector) is the recommended approach.
type ORMLogCollector struct {
	client    *odoo.Client
	eventCh   chan<- aggregator.Event
	sampler   *sampler.Sampler
	lastLogID int // high-water mark
	logger    zerolog.Logger
}

// NewORMLogCollector creates a new ir.logging based ORM collector.
func NewORMLogCollector(
	client *odoo.Client,
	eventCh chan<- aggregator.Event,
	smp *sampler.Sampler,
	logger zerolog.Logger,
) *ORMLogCollector {
	return &ORMLogCollector{
		client:  client,
		eventCh: eventCh,
		sampler: smp,
		logger:  logger.With().Str("component", "orm-irlogging-collector").Logger(),
	}
}

// Poll fetches new ORM-related ir.logging records and pushes events.
func (c *ORMLogCollector) Poll(ctx context.Context) error {
	domain := []any{
		[]any{"name", "in", []any{"odoo.models.query", "odoo.sql_db"}},
		[]any{"id", ">", c.lastLogID},
	}

	records, err := FetchRecordsWithDomain(
		ctx, c.client, "ir.logging", ormLogFields, domain,
		map[string]any{"order": "id asc", "limit": 500},
	)
	if err != nil {
		return fmt.Errorf("ir.logging ORM search_read: %w", err)
	}

	pushed := 0
	for _, r := range records {
		id := intVal(r["id"])
		if id > c.lastLogID {
			c.lastLogID = id
		}

		ev, ok := c.toEvent(r)
		if !ok {
			continue
		}

		// Apply sampling.
		if c.sampler != nil && !c.sampler.Allow(sampler.EventInfo{
			Category:   ev.Category,
			DurationMS: ev.DurationMS,
			IsError:    ev.IsError,
			IsN1:       ev.IsN1,
		}) {
			continue
		}

		// Non-blocking send.
		select {
		case c.eventCh <- ev:
			pushed++
		default:
		}
	}

	if pushed > 0 {
		c.logger.Info().
			Int("fetched", len(records)).
			Int("pushed", pushed).
			Msg("ORM events captured from ir.logging")
	}

	return nil
}

// RunLoop polls on interval until ctx is canceled.
func (c *ORMLogCollector) RunLoop(ctx context.Context, interval time.Duration) {
	runPollLoop(ctx, interval, c.logger, "ORM ir.logging", c.Poll)
}

// Regex duplicates from hook/ormparser.go — inlined to avoid import cycle.
var (
	ormModelsQueryRe = regexp.MustCompile(`^(\d+(?:\.\d+)?)ms\s+(.+)$`)
	ormSQLDbQueryRe  = regexp.MustCompile(`^query:\s+(.+)$`)
	ormTableFromSQL  = regexp.MustCompile(`(?i)(?:FROM|INTO|UPDATE)\s+"(\w+)"`)
)

const (
	ormMethodRead    = "read"
	ormMethodCreate  = "create"
	ormMethodWrite   = "write"
	ormMethodUnlink  = "unlink"
	ormMethodUnknown = "unknown"
)

// toEvent converts an ir.logging record to an aggregator.Event.
func (c *ORMLogCollector) toEvent(r map[string]any) (aggregator.Event, bool) {
	name := stringVal(r["name"])
	message := stringVal(r["message"])
	if message == "" {
		return aggregator.Event{}, false
	}

	ts := parseDate(stringVal(r["create_date"]))

	var durationMS int
	var sql string

	switch name {
	case "odoo.models.query":
		m := ormModelsQueryRe.FindStringSubmatch(message)
		if m == nil {
			return aggregator.Event{}, false
		}
		if f, err := strconv.ParseFloat(m[1], 64); err == nil {
			durationMS = int(f)
		}
		sql = m[2]

	case "odoo.sql_db":
		m := ormSQLDbQueryRe.FindStringSubmatch(message)
		if m == nil {
			return aggregator.Event{}, false
		}
		sql = m[1]

	default:
		return aggregator.Event{}, false
	}

	model := ""
	if tm := ormTableFromSQL.FindStringSubmatch(sql); tm != nil {
		model = strings.ReplaceAll(tm[1], "_", ".")
	}

	method := ormMethodUnknown
	upper := strings.ToUpper(strings.TrimSpace(sql))
	switch {
	case strings.HasPrefix(upper, "SELECT"):
		method = ormMethodRead
	case strings.HasPrefix(upper, "INSERT"):
		method = ormMethodCreate
	case strings.HasPrefix(upper, "UPDATE"):
		method = ormMethodWrite
	case strings.HasPrefix(upper, "DELETE"):
		method = ormMethodUnlink
	}

	return aggregator.Event{
		Category:   "orm",
		Model:      model,
		Method:     method,
		DurationMS: durationMS,
		SQL:        sql,
		Timestamp:  ts,
	}, true
}

func parseDate(s string) time.Time {
	s = strings.TrimSpace(s)
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
