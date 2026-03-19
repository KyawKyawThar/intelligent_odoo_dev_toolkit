package hook

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"Intelligent_Dev_ToolKit_Odoo/internal/agent/aggregator"
)

// ────────────────────────────────────────────────────────────────────────────
// ORM query log line parsing
// ────────────────────────────────────────────────────────────────────────────
//
// Odoo logs ORM queries via two loggers:
//
//   odoo.models.query (Odoo 16+, INFO level):
//     2024-01-15 10:30:45,123 1234 INFO mydb odoo.models.query: 4.2ms SELECT "res_partner"."id" ...
//
//   odoo.sql_db (all versions, DEBUG level):
//     2024-01-15 10:30:45,456 1234 DEBUG mydb odoo.sql_db: query: SELECT "sale_order"."id" ...
//
// To enable, start Odoo with: --log-handler=odoo.models.query:INFO

// modelsQueryRe matches the message from odoo.models.query: "4.2ms SELECT ..."
// Group 1 = duration (float), Group 2 = SQL text.
var modelsQueryRe = regexp.MustCompile(`^(\d+(?:\.\d+)?)ms\s+(.+)$`)

// sqlDbQueryRe matches the message from odoo.sql_db: "query: SELECT ..."
// Group 1 = SQL text. No duration available.
var sqlDbQueryRe = regexp.MustCompile(`^query:\s+(.+)$`)

// tableFromSQLRe extracts the primary table name from SQL.
// Matches FROM "table", INTO "table", UPDATE "table".
var tableFromSQLRe = regexp.MustCompile(`(?i)(?:FROM|INTO|UPDATE)\s+"(\w+)"`)

// IsORMQueryEntry returns true if the log entry is an ORM query line.
func IsORMQueryEntry(entry *LogEntry) bool {
	switch entry.Logger {
	case "odoo.models.query", "odoo.sql_db":
		return true
	}
	return false
}

// ToORMEvent converts a LogEntry into an aggregator.Event if it's an ORM
// query line. Returns (event, false) if the entry is not parseable.
func ToORMEvent(entry *LogEntry) (aggregator.Event, bool) {
	var durationMS int
	var sql string

	switch entry.Logger {
	case "odoo.models.query":
		m := modelsQueryRe.FindStringSubmatch(entry.Message)
		if m == nil {
			return aggregator.Event{}, false
		}
		if f, err := strconv.ParseFloat(m[1], 64); err == nil {
			durationMS = int(f)
		}
		sql = m[2]

	case "odoo.sql_db":
		m := sqlDbQueryRe.FindStringSubmatch(entry.Message)
		if m == nil {
			return aggregator.Event{}, false
		}
		sql = m[1]
		// odoo.sql_db doesn't provide duration

	default:
		return aggregator.Event{}, false
	}

	// Include continuation lines (multi-line SQL).
	if entry.Contiguous != "" {
		sql = sql + " " + strings.ReplaceAll(entry.Contiguous, "\n", " ")
	}

	model := modelFromSQL(sql)
	method := sqlVerbToMethod(sql)

	ts := entry.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
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

// modelFromSQL extracts the primary table name from SQL and converts it to
// an Odoo model name. "res_partner" → "res.partner".
func modelFromSQL(sql string) string {
	m := tableFromSQLRe.FindStringSubmatch(sql)
	if m == nil {
		return ""
	}
	return tableToModel(m[1])
}

// tableToModel converts an underscored PostgreSQL table name to a dotted
// Odoo model name. Examples:
//
//	"res_partner"     → "res.partner"
//	"sale_order_line" → "sale.order.line"
//	"ir_model_fields" → "ir.model.fields"
func tableToModel(table string) string {
	return strings.ReplaceAll(table, "_", ".")
}

// sqlVerbToMethod maps the SQL verb to an Odoo ORM method name.
func sqlVerbToMethod(sql string) string {
	trimmed := strings.TrimSpace(sql)
	if len(trimmed) < 6 {
		return "unknown"
	}

	// Check first word (case-insensitive).
	switch {
	case strings.HasPrefix(strings.ToUpper(trimmed), "SELECT"):
		return "read"
	case strings.HasPrefix(strings.ToUpper(trimmed), "INSERT"):
		return "create"
	case strings.HasPrefix(strings.ToUpper(trimmed), "UPDATE"):
		return "write"
	case strings.HasPrefix(strings.ToUpper(trimmed), "DELETE"):
		return "unlink"
	default:
		return "unknown"
	}
}
