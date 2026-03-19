package errors

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// ────────────────────────────────────────────────────────────────────────────
// Signature Generator
// ────────────────────────────────────────────────────────────────────────────
//
// A signature is a stable 16-hex-char identifier that groups repeated
// occurrences of the same bug together while keeping distinct bugs apart.
//
// Grouping inputs (what goes into the hash):
//
//   1. Error type     — e.g. "ValidationError", "AccessError"
//   2. Origin frame   — the innermost traceback frame: file path + function name
//   3. Normalized msg — first line of the error message with dynamic values stripped
//
// This means:
//   - Same bug hit from the same code path → same signature
//     (even if the message mentions different record IDs)
//   - Same exception type from different code paths → different signatures
//   - No traceback → falls back to errorType + normalized first line

// OriginFrame is the innermost (most specific) frame from a Python traceback.
// This is the frame closest to where the exception was raised.
type OriginFrame struct {
	File     string // e.g. "/opt/odoo/addons/sale/models/sale_order.py"
	Line     string // e.g. "42"
	Function string // e.g. "action_confirm"
}

// frameRe matches Python traceback frame lines:
//
//	File "/path/to/file.py", line 42, in function_name
var frameRe = regexp.MustCompile(
	`^\s*File "([^"]+)", line (\d+), in (.+)$`,
)

// ExtractOriginFrame finds the innermost (last) traceback frame.
// Returns an empty OriginFrame if no frames are found (no traceback).
func ExtractOriginFrame(traceback string) OriginFrame {
	var last OriginFrame
	for _, line := range strings.Split(traceback, "\n") {
		m := frameRe.FindStringSubmatch(line)
		if m != nil {
			last = OriginFrame{
				File:     m[1],
				Line:     m[2],
				Function: m[3],
			}
		}
	}
	return last
}

// ShortFile returns just the filename from the full path.
// "/opt/odoo/addons/sale/models/sale_order.py" → "sale_order.py"
func (f OriginFrame) ShortFile() string {
	if f.File == "" {
		return ""
	}
	idx := strings.LastIndex(f.File, "/")
	if idx >= 0 {
		return f.File[idx+1:]
	}
	return f.File
}

// ModulePath returns the Odoo-relevant portion of the frame path.
// "/opt/odoo/addons/sale/models/sale_order.py" → "sale/models/sale_order.py"
// Falls back to ShortFile() if no addons path is found.
func (f OriginFrame) ModulePath() string {
	if f.File == "" {
		return ""
	}
	// Look for /addons/ marker (standard Odoo addon layout).
	if idx := strings.LastIndex(f.File, "/addons/"); idx >= 0 {
		return f.File[idx+len("/addons/"):]
	}
	// Try /odoo/ for core Odoo code — use last occurrence to skip prefix dirs.
	if idx := strings.LastIndex(f.File, "/odoo/"); idx >= 0 {
		return f.File[idx+len("/odoo/"):]
	}
	return f.ShortFile()
}

// ────────────────────────────────────────────────────────────────────────────
// Message normalization
// ────────────────────────────────────────────────────────────────────────────
//
// Dynamic values in error messages cause signature fragmentation.
// "Record 42 not found" and "Record 789 not found" are the same bug.
// We normalize these out before hashing.

var normalizers = []*regexp.Regexp{
	// UUIDs: 550e8400-e29b-41d4-a716-446655440000
	regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`),

	// Email addresses
	regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),

	// ISO timestamps: 2024-01-15T10:30:45 or 2024-01-15 10:30:45
	regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}[.,]?\d*`),

	// IP addresses (v4)
	regexp.MustCompile(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`),

	// Hex strings (8+ chars, likely hashes/tokens)
	regexp.MustCompile(`\b[0-9a-fA-F]{8,}\b`),

	// Integers (standalone numbers, not part of identifiers).
	// Negative lookbehind/ahead for word chars ensures we don't mangle
	// identifiers like "sale_order" or "v2".
	regexp.MustCompile(`(?:^|[^a-zA-Z_])(\d+)(?:[^a-zA-Z_]|$)`),
}

// NormalizeMessage strips dynamic values from an error message so that
// repeated occurrences of the same bug produce identical normalized strings.
func NormalizeMessage(msg string) string {
	first := firstNonEmpty(msg)
	normalized := first

	for _, re := range normalizers {
		normalized = re.ReplaceAllString(normalized, "<?>")
	}

	// Collapse consecutive placeholders.
	for strings.Contains(normalized, "<?><?>"+"") {
		normalized = strings.ReplaceAll(normalized, "<?>< ?>", "<?>")
	}
	// Collapse whitespace.
	normalized = strings.Join(strings.Fields(normalized), " ")

	return normalized
}

// ────────────────────────────────────────────────────────────────────────────
// Signature computation
// ────────────────────────────────────────────────────────────────────────────

// GenerateSignature produces a stable 16-hex-char signature for an error.
//
// When a traceback is available, it uses:
//
//	errorType + originFrame.ModulePath + originFrame.Function + normalizedMsg
//
// When no traceback is available (single-line errors), it uses:
//
//	errorType + normalizedMsg
//
// This replaces the naive ComputeSignature for new callers.
func GenerateSignature(errorType, message, traceback string) string {
	origin := ExtractOriginFrame(traceback)
	normalized := NormalizeMessage(message)

	var parts []string
	parts = append(parts, errorType)

	if origin.File != "" {
		parts = append(parts, origin.ModulePath(), origin.Function)
	}

	parts = append(parts, normalized)

	raw := strings.Join(parts, "\x00")
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:8]) // 16 hex chars
}

// ────────────────────────────────────────────────────────────────────────────
// Multi-exception support
// ────────────────────────────────────────────────────────────────────────────

// ParseExceptionLine extracts the exception class and message from the last
// line of a Python traceback.
//
//	"odoo.exceptions.ValidationError: Cannot confirm order"
//	→ type="odoo.exceptions.ValidationError", msg="Cannot confirm order"
//
//	"ValueError: invalid literal for int()"
//	→ type="ValueError", msg="invalid literal for int()"
//
// Falls back to ("UnknownError", fullLine) if the pattern doesn't match.
func ParseExceptionLine(traceback string) (errType, errMsg string) {
	lines := strings.Split(strings.TrimSpace(traceback), "\n")
	// Walk backward to find the exception line (skip blank lines).
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		// Skip "During handling of..." chained exception markers.
		if strings.HasPrefix(line, "During handling of") ||
			strings.HasPrefix(line, "The above exception") {
			continue
		}

		if idx := strings.Index(line, ": "); idx > 0 {
			candidate := line[:idx]
			if isIdentifier(candidate) {
				return candidate, line[idx+2:]
			}
		}

		// No colon — the whole line might be the exception class (e.g. "KeyboardInterrupt").
		if isIdentifier(line) {
			return line, ""
		}

		return unknownErrorType, line
	}
	return unknownErrorType, ""
}

// ShortExceptionType returns the short class name from a dotted path.
// "odoo.exceptions.ValidationError" → "ValidationError"
// "ValueError" → "ValueError"
func ShortExceptionType(fullType string) string {
	if idx := strings.LastIndex(fullType, "."); idx >= 0 {
		return fullType[idx+1:]
	}
	return fullType
}
