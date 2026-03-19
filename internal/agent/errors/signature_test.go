package errors

import (
	"testing"
)

// ── ExtractOriginFrame ──────────────────────────────────────────────────────

func TestExtractOriginFrame_Standard(t *testing.T) {
	tb := `Traceback (most recent call last):
  File "/opt/odoo/addons/sale/models/sale_order.py", line 123, in action_confirm
    self._action_confirm()
  File "/opt/odoo/addons/sale/models/sale_order.py", line 456, in _action_confirm
    raise ValidationError("Cannot confirm")
odoo.exceptions.ValidationError: Cannot confirm`

	frame := ExtractOriginFrame(tb)
	if frame.File != "/opt/odoo/addons/sale/models/sale_order.py" {
		t.Errorf("File = %q", frame.File)
	}
	if frame.Line != "456" {
		t.Errorf("Line = %q", frame.Line)
	}
	if frame.Function != "_action_confirm" {
		t.Errorf("Function = %q", frame.Function)
	}
}

func TestExtractOriginFrame_DeepStack(t *testing.T) {
	tb := `Traceback (most recent call last):
  File "/opt/odoo/odoo/http.py", line 1800, in _serve_db
    return service_model.retrying(self._serve_ir_http, self.env)
  File "/opt/odoo/odoo/service/model.py", line 128, in retrying
    result = func()
  File "/opt/odoo/addons/stock/wizard/stock_picking_return.py", line 95, in create_returns
    new_picking.action_confirm()
  File "/opt/odoo/addons/stock/models/stock_picking.py", line 750, in action_confirm
    self.mapped('move_lines')._action_confirm()
  File "/opt/odoo/addons/stock/models/stock_move.py", line 1200, in _action_confirm
    raise UserError(_("No qty available"))
odoo.exceptions.UserError: No qty available`

	frame := ExtractOriginFrame(tb)
	// The innermost frame is the last one (stock_move.py:1200).
	if frame.Function != "_action_confirm" {
		t.Errorf("Function = %q, want _action_confirm", frame.Function)
	}
	if frame.File != "/opt/odoo/addons/stock/models/stock_move.py" {
		t.Errorf("File = %q", frame.File)
	}
	if frame.Line != "1200" {
		t.Errorf("Line = %q", frame.Line)
	}
}

func TestExtractOriginFrame_NoTraceback(t *testing.T) {
	frame := ExtractOriginFrame("just an error message with no traceback")
	if frame.File != "" || frame.Function != "" {
		t.Errorf("expected empty OriginFrame, got %+v", frame)
	}
}

func TestExtractOriginFrame_Empty(t *testing.T) {
	frame := ExtractOriginFrame("")
	if frame.File != "" {
		t.Errorf("expected empty, got %+v", frame)
	}
}

// ── OriginFrame helpers ─────────────────────────────────────────────────────

func TestOriginFrame_ShortFile(t *testing.T) {
	tests := []struct {
		file string
		want string
	}{
		{"/opt/odoo/addons/sale/models/sale_order.py", "sale_order.py"},
		{"sale_order.py", "sale_order.py"},
		{"", ""},
	}
	for _, tt := range tests {
		f := OriginFrame{File: tt.file}
		if got := f.ShortFile(); got != tt.want {
			t.Errorf("ShortFile(%q) = %q, want %q", tt.file, got, tt.want)
		}
	}
}

func TestOriginFrame_ModulePath(t *testing.T) {
	tests := []struct {
		file string
		want string
	}{
		{"/opt/odoo/addons/sale/models/sale_order.py", "sale/models/sale_order.py"},
		{"/opt/odoo/odoo/http.py", "http.py"},
		{"/some/random/path/file.py", "file.py"},
		{"", ""},
	}
	for _, tt := range tests {
		f := OriginFrame{File: tt.file}
		if got := f.ModulePath(); got != tt.want {
			t.Errorf("ModulePath(%q) = %q, want %q", tt.file, got, tt.want)
		}
	}
}

// ── NormalizeMessage ────────────────────────────────────────────────────────

func TestNormalizeMessage_RecordIDs(t *testing.T) {
	a := NormalizeMessage("Record 42 not found in sale.order")
	b := NormalizeMessage("Record 789 not found in sale.order")
	if a != b {
		t.Errorf("expected same normalized message:\n  a = %q\n  b = %q", a, b)
	}
}

func TestNormalizeMessage_UUIDs(t *testing.T) {
	a := NormalizeMessage("Token 550e8400-e29b-41d4-a716-446655440000 expired")
	b := NormalizeMessage("Token a1b2c3d4-e5f6-7890-abcd-ef1234567890 expired")
	if a != b {
		t.Errorf("expected same:\n  a = %q\n  b = %q", a, b)
	}
}

func TestNormalizeMessage_Emails(t *testing.T) {
	a := NormalizeMessage("Cannot send email to user@example.com")
	b := NormalizeMessage("Cannot send email to admin@company.org")
	if a != b {
		t.Errorf("expected same:\n  a = %q\n  b = %q", a, b)
	}
}

func TestNormalizeMessage_Timestamps(t *testing.T) {
	a := NormalizeMessage("Deadline 2024-01-15 10:30:45 passed")
	b := NormalizeMessage("Deadline 2025-12-31 23:59:59 passed")
	if a != b {
		t.Errorf("expected same:\n  a = %q\n  b = %q", a, b)
	}
}

func TestNormalizeMessage_IPs(t *testing.T) {
	a := NormalizeMessage("Connection refused from 192.168.1.100")
	b := NormalizeMessage("Connection refused from 10.0.0.1")
	if a != b {
		t.Errorf("expected same:\n  a = %q\n  b = %q", a, b)
	}
}

func TestNormalizeMessage_PreservesStructure(t *testing.T) {
	msg := "Field 'name' is required on model 'sale.order'"
	normalized := NormalizeMessage(msg)
	// The model name and field name should survive (they're not numbers/UUIDs).
	if normalized == "" {
		t.Error("normalized message should not be empty")
	}
	// Should still contain the structural parts.
	if !containsSubstring(normalized, "Field") || !containsSubstring(normalized, "required") {
		t.Errorf("normalized = %q, expected to preserve structural text", normalized)
	}
}

func TestNormalizeMessage_Empty(t *testing.T) {
	if NormalizeMessage("") != "" {
		t.Error("empty input should produce empty output")
	}
}

func TestNormalizeMessage_MultiLine(t *testing.T) {
	// Should only use the first non-empty line.
	msg := "Record 42 not found\nSome detail\nMore detail"
	normalized := NormalizeMessage(msg)
	if containsSubstring(normalized, "detail") {
		t.Errorf("should use only first line, got %q", normalized)
	}
}

// ── GenerateSignature ───────────────────────────────────────────────────────

func TestGenerateSignature_SameBugDifferentIDs(t *testing.T) {
	tb := `Traceback (most recent call last):
  File "/opt/odoo/addons/sale/models/sale_order.py", line 42, in action_confirm
    raise ValidationError("Cannot confirm order %d" % order_id)
odoo.exceptions.ValidationError: Cannot confirm order %d`

	sig1 := GenerateSignature("ValidationError", "Cannot confirm order 42", tb)
	sig2 := GenerateSignature("ValidationError", "Cannot confirm order 789", tb)
	if sig1 != sig2 {
		t.Errorf("same bug with different IDs should have same signature:\n  sig1 = %q\n  sig2 = %q", sig1, sig2)
	}
}

func TestGenerateSignature_DifferentCodePaths(t *testing.T) {
	tb1 := `Traceback (most recent call last):
  File "/opt/odoo/addons/sale/models/sale_order.py", line 42, in action_confirm
    raise ValidationError("bad")
odoo.exceptions.ValidationError: bad`

	tb2 := `Traceback (most recent call last):
  File "/opt/odoo/addons/purchase/models/purchase_order.py", line 99, in button_approve
    raise ValidationError("bad")
odoo.exceptions.ValidationError: bad`

	sig1 := GenerateSignature("ValidationError", "bad", tb1)
	sig2 := GenerateSignature("ValidationError", "bad", tb2)
	if sig1 == sig2 {
		t.Error("same error type from different code paths should have different signatures")
	}
}

func TestGenerateSignature_DifferentExceptionTypes(t *testing.T) {
	tb := `Traceback (most recent call last):
  File "/opt/odoo/addons/sale/models/sale_order.py", line 42, in action_confirm
    raise SomeError("bad")
`
	sig1 := GenerateSignature("ValidationError", "bad", tb)
	sig2 := GenerateSignature("AccessError", "bad", tb)
	if sig1 == sig2 {
		t.Error("different exception types should have different signatures")
	}
}

func TestGenerateSignature_NoTraceback(t *testing.T) {
	sig1 := GenerateSignature("ConnectionError", "Connection refused", "")
	sig2 := GenerateSignature("ConnectionError", "Connection refused", "no traceback here")
	// Both have no frames, so they fall back to errorType + normalized message.
	if sig1 != sig2 {
		t.Errorf("no-traceback signatures should match: %q vs %q", sig1, sig2)
	}
}

func TestGenerateSignature_Length(t *testing.T) {
	sig := GenerateSignature("ValidationError", "test", "")
	if len(sig) != 16 {
		t.Errorf("signature length = %d, want 16", len(sig))
	}
}

func TestGenerateSignature_Deterministic(t *testing.T) {
	tb := `Traceback (most recent call last):
  File "/opt/odoo/addons/sale/models/sale_order.py", line 42, in action_confirm
    raise ValidationError("bad")`

	sig1 := GenerateSignature("ValidationError", "bad", tb)
	sig2 := GenerateSignature("ValidationError", "bad", tb)
	if sig1 != sig2 {
		t.Errorf("signatures should be deterministic: %q vs %q", sig1, sig2)
	}
}

// ── ParseExceptionLine ──────────────────────────────────────────────────────

func TestParseExceptionLine_Standard(t *testing.T) {
	tb := `Traceback (most recent call last):
  File "sale.py", line 42, in confirm
    raise ValidationError("bad input")
odoo.exceptions.ValidationError: bad input`

	errType, errMsg := ParseExceptionLine(tb)
	if errType != "odoo.exceptions.ValidationError" {
		t.Errorf("errType = %q", errType)
	}
	if errMsg != "bad input" {
		t.Errorf("errMsg = %q", errMsg)
	}
}

func TestParseExceptionLine_NoColon(t *testing.T) {
	tb := `Traceback (most recent call last):
  File "test.py", line 1, in <module>
KeyboardInterrupt`

	errType, errMsg := ParseExceptionLine(tb)
	if errType != "KeyboardInterrupt" {
		t.Errorf("errType = %q", errType)
	}
	if errMsg != "" {
		t.Errorf("errMsg = %q, want empty", errMsg)
	}
}

func TestParseExceptionLine_ChainedException(t *testing.T) {
	tb := `Traceback (most recent call last):
  File "a.py", line 1, in foo
    bar()
ValueError: original error

During handling of the above exception, another exception occurred:

Traceback (most recent call last):
  File "b.py", line 5, in handler
    cleanup()
RuntimeError: cleanup failed`

	errType, errMsg := ParseExceptionLine(tb)
	if errType != "RuntimeError" {
		t.Errorf("errType = %q, want RuntimeError", errType)
	}
	if errMsg != "cleanup failed" {
		t.Errorf("errMsg = %q", errMsg)
	}
}

func TestParseExceptionLine_Empty(t *testing.T) {
	errType, _ := ParseExceptionLine("")
	if errType != "UnknownError" {
		t.Errorf("errType = %q, want UnknownError", errType)
	}
}

// ── ShortExceptionType ──────────────────────────────────────────────────────

func TestShortExceptionType(t *testing.T) {
	tests := []struct {
		full string
		want string
	}{
		{"odoo.exceptions.ValidationError", "ValidationError"},
		{"odoo.exceptions.AccessError", "AccessError"},
		{"ValueError", "ValueError"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := ShortExceptionType(tt.full); got != tt.want {
			t.Errorf("ShortExceptionType(%q) = %q, want %q", tt.full, got, tt.want)
		}
	}
}

// ── Regression: same frame, different normalized messages ───────────────────

func TestGenerateSignature_DifferentMessages(t *testing.T) {
	tb := `Traceback (most recent call last):
  File "/opt/odoo/addons/sale/models/sale_order.py", line 42, in action_confirm
    raise ValidationError(msg)`

	sig1 := GenerateSignature("ValidationError", "Cannot confirm: missing product", tb)
	sig2 := GenerateSignature("ValidationError", "Cannot confirm: missing warehouse", tb)
	// These are structurally different messages from the same code path.
	// They SHOULD have different signatures (different bugs).
	if sig1 == sig2 {
		t.Error("different error messages from same frame should produce different signatures")
	}
}

// ── helpers ─────────────────────────────────────────────────────────────────

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || sub == "" ||
		(s != "" && sub != "" && stringContains(s, sub)))
}

func stringContains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
