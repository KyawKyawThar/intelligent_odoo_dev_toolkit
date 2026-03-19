package hook

import (
	"testing"
	"time"
)

func TestParseOdooTimestamp(t *testing.T) {
	ts := parseOdooTimestamp("2024-01-15 10:30:45,123")
	want := time.Date(2024, 1, 15, 10, 30, 45, 123_000_000, time.UTC)
	if !ts.Equal(want) {
		t.Errorf("got %v, want %v", ts, want)
	}
}

func TestParseOdooTimestamp_Invalid(t *testing.T) {
	before := time.Now().UTC()
	ts := parseOdooTimestamp("not-a-date")
	if ts.Before(before) {
		t.Error("expected fallback to ~now for invalid input")
	}
}

func TestSplitLogger(t *testing.T) {
	tests := []struct {
		logger     string
		wantModule string
		wantModel  string
	}{
		{"odoo.addons.sale.models.sale_order", "sale", "models.sale_order"},
		{"odoo.http", "http", ""},
		{"odoo.addons.stock.wizard.stock_picking", "stock", "wizard.stock_picking"},
		{"odoo.sql_db", "sql_db", ""},
		{"", "", ""},
	}

	for _, tt := range tests {
		mod, model := splitLogger(tt.logger)
		if mod != tt.wantModule || model != tt.wantModel {
			t.Errorf("splitLogger(%q) = (%q, %q), want (%q, %q)",
				tt.logger, mod, model, tt.wantModule, tt.wantModel)
		}
	}
}

func TestLogLineRe(t *testing.T) {
	line := "2024-01-15 10:30:45,123 12345 ERROR mydb odoo.addons.sale.models.sale_order: Something failed"
	m := logLineRe.FindStringSubmatch(line)
	if m == nil {
		t.Fatal("expected regex to match")
	}
	if m[1] != "2024-01-15 10:30:45,123" {
		t.Errorf("timestamp = %q", m[1])
	}
	if m[2] != "12345" {
		t.Errorf("pid = %q", m[2])
	}
	if m[3] != "ERROR" {
		t.Errorf("level = %q", m[3])
	}
	if m[4] != "mydb" {
		t.Errorf("dbname = %q", m[4])
	}
	if m[5] != "odoo.addons.sale.models.sale_order" {
		t.Errorf("logger = %q", m[5])
	}
	if m[6] != "Something failed" {
		t.Errorf("message = %q", m[6])
	}
}

func TestLogLineRe_NoMatch(t *testing.T) {
	// Continuation lines should not match.
	lines := []string{
		"  File \"/opt/odoo/addons/sale/models/sale_order.py\", line 42, in action_confirm",
		"Traceback (most recent call last):",
		"    self._action_confirm()",
		"odoo.exceptions.ValidationError: Cannot confirm order",
		"",
	}
	for _, line := range lines {
		if logLineRe.MatchString(line) {
			t.Errorf("expected no match for continuation line: %q", line)
		}
	}
}

func TestEntryAccumulator_SingleLine(t *testing.T) {
	acc := &EntryAccumulator{}

	// First header → no previous entry to emit.
	entry := acc.Feed("2024-01-15 10:30:45,123 1 ERROR db odoo.http: request failed")
	if entry != nil {
		t.Error("first Feed should return nil")
	}

	// Flush the buffered entry.
	entry = acc.Flush()
	if entry == nil {
		t.Fatal("Flush should return the buffered entry")
	}
	if entry.Level != "ERROR" {
		t.Errorf("level = %q", entry.Level)
	}
	if entry.Message != "request failed" {
		t.Errorf("message = %q", entry.Message)
	}
	if entry.Contiguous != "" {
		t.Errorf("contiguous = %q, want empty", entry.Contiguous)
	}
}

func TestEntryAccumulator_MultiLine(t *testing.T) {
	acc := &EntryAccumulator{}

	// First entry: ERROR with traceback.
	acc.Feed("2024-01-15 10:30:45,123 1 ERROR db odoo.addons.sale.models.sale: error occurred")
	acc.Feed("Traceback (most recent call last):")
	acc.Feed("  File \"/opt/odoo/sale.py\", line 42, in confirm")
	acc.Feed("    raise ValidationError(\"bad\")")
	acc.Feed("odoo.exceptions.ValidationError: bad")

	// Second entry starts → emits the first.
	entry := acc.Feed("2024-01-15 10:30:46,000 1 INFO db odoo.http: next request")
	if entry == nil {
		t.Fatal("expected first entry to be emitted")
	}

	if entry.Level != "ERROR" {
		t.Errorf("level = %q", entry.Level)
	}
	if !entry.IsError() {
		t.Error("expected IsError() = true")
	}

	full := entry.FullMessage()
	if full == "" {
		t.Error("full message should not be empty")
	}

	// The continuation should contain the traceback lines.
	if entry.Contiguous == "" {
		t.Error("expected continuation lines")
	}
}

func TestEntryAccumulator_FlushEmpty(t *testing.T) {
	acc := &EntryAccumulator{}
	if acc.Flush() != nil {
		t.Error("Flush on empty accumulator should return nil")
	}
}

func TestEntryAccumulator_MultiplEntries(t *testing.T) {
	acc := &EntryAccumulator{}

	lines := []string{
		"2024-01-15 10:00:00,000 1 INFO db odoo.http: GET /web",
		"2024-01-15 10:00:01,000 1 ERROR db odoo.addons.stock.models.picking: boom",
		"Traceback (most recent call last):",
		"  File \"picking.py\", line 10, in do_transfer",
		"    raise UserError(\"boom\")",
		"odoo.exceptions.UserError: boom",
		"2024-01-15 10:00:02,000 1 WARNING db odoo.http: slow request",
	}

	var entries []*LogEntry
	for _, line := range lines {
		if e := acc.Feed(line); e != nil {
			entries = append(entries, e)
		}
	}
	if e := acc.Flush(); e != nil {
		entries = append(entries, e)
	}

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Entry 0: INFO
	if entries[0].Level != "INFO" {
		t.Errorf("entry[0].Level = %q", entries[0].Level)
	}
	if entries[0].IsError() {
		t.Error("entry[0] should not be error")
	}

	// Entry 1: ERROR with traceback
	if entries[1].Level != "ERROR" {
		t.Errorf("entry[1].Level = %q", entries[1].Level)
	}
	if entries[1].Logger != "odoo.addons.stock.models.picking" {
		t.Errorf("entry[1].Logger = %q", entries[1].Logger)
	}
	if entries[1].Contiguous == "" {
		t.Error("entry[1] should have continuation lines")
	}

	// Entry 2: WARNING
	if entries[2].Level != "WARNING" {
		t.Errorf("entry[2].Level = %q", entries[2].Level)
	}
}

func TestToErrorEvent(t *testing.T) {
	entry := &LogEntry{
		Timestamp:  time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC),
		PID:        "1",
		Level:      "ERROR",
		DBName:     "testdb",
		Logger:     "odoo.addons.sale.models.sale_order",
		Message:    "something broke",
		Contiguous: "Traceback (most recent call last):\n  File \"sale.py\", line 1\nodoo.exceptions.ValidationError: bad input",
	}

	ev := ToErrorEvent(entry)

	if ev.Signature == "" {
		t.Error("expected non-empty signature")
	}
	if ev.Module != "sale" {
		t.Errorf("module = %q, want sale", ev.Module)
	}
	if ev.Model != "models.sale_order" {
		t.Errorf("model = %q", ev.Model)
	}
	if ev.CapturedAt != entry.Timestamp {
		t.Error("timestamp mismatch")
	}
	if ev.Traceback == "" {
		t.Error("expected traceback to be extracted")
	}
}
