// Package hook provides log-file tailing and traceback parsing for Odoo's
// server log. It complements the ir.logging XML-RPC collector by reading
// errors directly from the log file — faster, no polling delay, and captures
// entries that may not reach ir.logging (e.g. worker crashes, OOM kills).
package hook

import (
	"regexp"
	"strings"
	"time"

	agenterrors "Intelligent_Dev_ToolKit_Odoo/internal/agent/errors"
)

// ────────────────────────────────────────────────────────────────────────────
// Odoo log format
// ────────────────────────────────────────────────────────────────────────────
//
// Standard Odoo log lines look like:
//
//   2024-01-15 10:30:45,123 12345 ERROR dbname odoo.addons.sale.models.sale: msg
//
// Structure: DATETIME PID LEVEL DBNAME LOGGER: MESSAGE
//
// Continuation lines (tracebacks, multi-line messages) do NOT start with a
// timestamp. We accumulate them into the preceding log entry.

// logLineRe matches the start of a standard Odoo log line.
// Groups: 1=datetime  2=pid  3=level  4=dbname  5=logger  6=first-line message
var logLineRe = regexp.MustCompile(
	`^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2},\d{3})\s+(\d+)\s+(DEBUG|INFO|WARNING|ERROR|CRITICAL)\s+(\S+)\s+(\S+?):\s(.*)$`,
)

// LogEntry is a parsed, possibly multi-line Odoo log entry.
type LogEntry struct {
	Timestamp  time.Time
	PID        string
	Level      string // "ERROR", "CRITICAL", etc.
	DBName     string
	Logger     string // e.g. "odoo.addons.sale.models.sale_order"
	Message    string // first line after the logger
	Contiguous string // all continuation lines joined with \n
}

// FullMessage returns the complete log body (first line + continuation).
func (e *LogEntry) FullMessage() string {
	if e.Contiguous == "" {
		return e.Message
	}
	return e.Message + "\n" + e.Contiguous
}

// IsError returns true for ERROR and CRITICAL levels.
func (e *LogEntry) IsError() bool {
	return e.Level == "ERROR" || e.Level == "CRITICAL"
}

// ────────────────────────────────────────────────────────────────────────────
// Line accumulator
// ────────────────────────────────────────────────────────────────────────────

// EntryAccumulator collects raw log lines and emits complete LogEntry values
// once the next log header (or EOF flush) is seen.
type EntryAccumulator struct {
	current *LogEntry
	contBuf strings.Builder
}

// Feed processes a single raw line. If it detects that the previous entry is
// complete (because a new header started), it returns that entry.
// Returns nil when the line is a continuation or the first header.
func (a *EntryAccumulator) Feed(line string) *LogEntry {
	m := logLineRe.FindStringSubmatch(line)

	if m == nil {
		// Continuation line — append to current entry.
		if a.current != nil {
			if a.contBuf.Len() > 0 {
				a.contBuf.WriteByte('\n')
			}
			a.contBuf.WriteString(line)
		}
		return nil
	}

	// New log header detected → emit the previous entry (if any).
	var prev *LogEntry
	if a.current != nil {
		a.current.Contiguous = a.contBuf.String()
		prev = a.current
	}

	// Start new entry.
	a.current = &LogEntry{
		Timestamp: parseOdooTimestamp(m[1]),
		PID:       m[2],
		Level:     m[3],
		DBName:    m[4],
		Logger:    m[5],
		Message:   m[6],
	}
	a.contBuf.Reset()

	return prev
}

// Flush returns the in-progress entry (e.g. at EOF or shutdown). Safe to
// call when there is nothing buffered — returns nil.
func (a *EntryAccumulator) Flush() *LogEntry {
	if a.current == nil {
		return nil
	}
	a.current.Contiguous = a.contBuf.String()
	entry := a.current
	a.current = nil
	a.contBuf.Reset()
	return entry
}

// ────────────────────────────────────────────────────────────────────────────
// Conversion to ErrorEvent
// ────────────────────────────────────────────────────────────────────────────

// ToErrorEvent converts a LogEntry into an ErrorEvent suitable for the shared
// ring buffer. Only call this for entries where IsError() is true.
func ToErrorEvent(entry *LogEntry) agenterrors.ErrorEvent {
	full := entry.FullMessage()
	errorType := agenterrors.ParseErrorType(full)
	traceback := agenterrors.ExtractTraceback(full)
	module, model := splitLogger(entry.Logger)

	return agenterrors.ErrorEvent{
		Signature:  agenterrors.GenerateSignature(errorType, full, traceback),
		ErrorType:  errorType,
		Message:    full,
		Module:     module,
		Model:      model,
		Traceback:  traceback,
		CapturedAt: entry.Timestamp,
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Helpers
// ────────────────────────────────────────────────────────────────────────────

// splitLogger extracts the Odoo module and model from the logger name.
// "odoo.addons.sale.models.sale_order" → ("sale", "models.sale_order")
// "odoo.http" → ("http", "")
func splitLogger(logger string) (module, model string) {
	if logger == "" {
		return "", ""
	}
	name := strings.TrimPrefix(logger, "odoo.addons.")
	name = strings.TrimPrefix(name, "odoo.")
	parts := strings.SplitN(name, ".", 2)
	module = parts[0]
	if len(parts) == 2 {
		model = parts[1]
	}
	return module, model
}

// parseOdooTimestamp parses "2024-01-15 10:30:45,123" → time.Time.
func parseOdooTimestamp(s string) time.Time {
	// Odoo uses comma for milliseconds; Go's time.Parse expects a dot.
	s = strings.Replace(s, ",", ".", 1)
	t, err := time.Parse("2006-01-02 15:04:05.000", s)
	if err != nil {
		return time.Now().UTC()
	}
	return t.UTC()
}
