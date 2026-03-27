package collector

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"Intelligent_Dev_ToolKit_Odoo/internal/agent/aggregator"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/sampler"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

// pgStatRow represents a single row from pg_stat_statements.
type pgStatRow struct {
	UserID          int64
	QueryID         int64
	Query           string
	Calls           int64
	TotalExecTimeMS float64
	MeanExecTimeMS  float64
	MaxExecTimeMS   float64
	MinExecTimeMS   float64
	Rows            int64
	SharedBlksHit   int64
	SharedBlksRead  int64
}

// pgStatSnapshot holds the cumulative counters from the last poll.
// pg_stat_statements counters are cumulative since the last reset, so we
// compute deltas between consecutive snapshots to get per-interval values.
type pgStatSnapshot struct {
	Calls           int64
	TotalExecTimeMS float64
	MaxExecTimeMS   float64
	Rows            int64
}

// PgStatCollector polls pg_stat_statements on Odoo's PostgreSQL database
// and pushes delta-based events into the aggregator pipeline.
type PgStatCollector struct {
	pool     *pgxpool.Pool
	eventCh  chan<- aggregator.Event
	sampler  *sampler.Sampler
	logger   zerolog.Logger
	baseline map[int64]pgStatSnapshot // queryid → last snapshot
}

// NewPgStatCollector creates a new pg_stat_statements collector.
func NewPgStatCollector(
	pool *pgxpool.Pool,
	eventCh chan<- aggregator.Event,
	smp *sampler.Sampler,
	logger zerolog.Logger,
) *PgStatCollector {
	return &PgStatCollector{
		pool:     pool,
		eventCh:  eventCh,
		sampler:  smp,
		logger:   logger.With().Str("component", "pgstat-collector").Logger(),
		baseline: make(map[int64]pgStatSnapshot),
	}
}

// pgStatQuery is the SQL used to read pg_stat_statements.
// We filter out utility statements (SET, SHOW, RESET, DEALLOCATE, BEGIN,
// COMMIT, ROLLBACK) that aren't interesting for profiling.
const pgStatQuery = `
SELECT
    s.userid,
    s.queryid,
    s.query,
    s.calls,
    s.total_exec_time   AS total_exec_time_ms,
    s.mean_exec_time    AS mean_exec_time_ms,
    s.max_exec_time     AS max_exec_time_ms,
    s.min_exec_time     AS min_exec_time_ms,
    s.rows,
    s.shared_blks_hit,
    s.shared_blks_read
FROM pg_stat_statements s
WHERE s.dbid = (SELECT oid FROM pg_database WHERE datname = current_database())
  AND s.query NOT LIKE 'SET %'
  AND s.query NOT LIKE 'SHOW %'
  AND s.query NOT LIKE 'RESET %'
  AND s.query NOT LIKE 'DEALLOCATE %'
  AND s.query !~* '^(BEGIN|COMMIT|ROLLBACK)'
ORDER BY s.total_exec_time DESC
LIMIT 500
`

// Poll reads current pg_stat_statements, computes deltas against the
// baseline, and pushes events for queries that had new activity.
func (c *PgStatCollector) Poll(ctx context.Context) error {
	rows, err := c.pool.Query(ctx, pgStatQuery)
	if err != nil {
		return fmt.Errorf("pg_stat_statements query: %w", err)
	}
	defer rows.Close()

	now := time.Now().UTC()
	pushed := 0
	newBaseline := make(map[int64]pgStatSnapshot, len(c.baseline))

	for rows.Next() {
		var r pgStatRow
		if err := rows.Scan(
			&r.UserID,
			&r.QueryID,
			&r.Query,
			&r.Calls,
			&r.TotalExecTimeMS,
			&r.MeanExecTimeMS,
			&r.MaxExecTimeMS,
			&r.MinExecTimeMS,
			&r.Rows,
			&r.SharedBlksHit,
			&r.SharedBlksRead,
		); err != nil {
			c.logger.Warn().Err(err).Msg("scan pg_stat_statements row")
			continue
		}

		// Save current snapshot for next delta computation.
		newBaseline[r.QueryID] = pgStatSnapshot{
			Calls:           r.Calls,
			TotalExecTimeMS: r.TotalExecTimeMS,
			MaxExecTimeMS:   r.MaxExecTimeMS,
			Rows:            r.Rows,
		}

		// Compute delta against previous baseline.
		prev, seen := c.baseline[r.QueryID]
		if !seen {
			// First time seeing this query — record baseline, skip event.
			continue
		}

		deltaCalls := r.Calls - prev.Calls
		if deltaCalls <= 0 {
			// No new executions since last poll.
			continue
		}

		deltaTotalMS := r.TotalExecTimeMS - prev.TotalExecTimeMS
		avgMS := deltaTotalMS / float64(deltaCalls)

		ev := c.toEvent(r, deltaCalls, int(avgMS), int(r.MaxExecTimeMS), now)

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

	if err := rows.Err(); err != nil {
		return fmt.Errorf("pg_stat_statements rows iteration: %w", err)
	}

	c.baseline = newBaseline

	if pushed > 0 {
		c.logger.Info().
			Int("pushed", pushed).
			Int("tracked", len(newBaseline)).
			Msg("pg_stat_statements events captured")
	}

	return nil
}

// RunLoop polls on interval until ctx is canceled.
func (c *PgStatCollector) RunLoop(ctx context.Context, interval time.Duration) {
	runPollLoop(ctx, interval, c.logger, "pg_stat_statements", c.Poll)
}

// Regex to extract the table name from SQL.
var pgStatTableRe = regexp.MustCompile(`(?i)(?:FROM|INTO|UPDATE|JOIN)\s+"?(\w+)"?`)

// toEvent converts a pg_stat_statements delta into an aggregator.Event.
func (c *PgStatCollector) toEvent(r pgStatRow, deltaCalls int64, avgMS, maxMS int, ts time.Time) aggregator.Event {
	model := extractModel(r.Query)
	method := sqlToMethod(r.Query)

	// Use the average duration as the representative duration for this window.
	durationMS := avgMS
	if durationMS == 0 && maxMS > 0 {
		durationMS = maxMS
	}

	return aggregator.Event{
		Category:   "sql",
		Model:      model,
		Method:     method,
		DurationMS: durationMS,
		SQL:        truncateSQL(r.Query, 1024),
		Timestamp:  ts,
	}
}

// extractModel extracts the Odoo model name from a SQL query by finding the
// primary table and converting underscores to dots.
func extractModel(sql string) string {
	m := pgStatTableRe.FindStringSubmatch(sql)
	if m == nil {
		return ""
	}
	table := m[1]
	// Skip PostgreSQL system tables.
	if strings.HasPrefix(table, "pg_") || strings.HasPrefix(table, "information_") {
		return ""
	}
	return strings.ReplaceAll(table, "_", ".")
}

// sqlToMethod maps the SQL statement type to an Odoo-like method name.
func sqlToMethod(sql string) string {
	upper := strings.ToUpper(strings.TrimSpace(sql))
	switch {
	case strings.HasPrefix(upper, "SELECT"):
		return "read"
	case strings.HasPrefix(upper, "INSERT"):
		return "create"
	case strings.HasPrefix(upper, "UPDATE"):
		return "write"
	case strings.HasPrefix(upper, "DELETE"):
		return "unlink"
	default:
		return "unknown"
	}
}

// truncateSQL truncates a SQL string to maxLen bytes, appending "…" if truncated.
func truncateSQL(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
