package errors

// Package errors provides the agent-side error event pipeline:
// capture from Odoo ir.logging → ring buffer → batch flush to server.

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

// unknownErrorType is the fallback error type when classification fails.
const unknownErrorType = "UnknownError"

// ErrorEvent is a single error occurrence captured from Odoo server logs.
type ErrorEvent struct {
	// Signature is a stable 16-char hex identifier derived from ErrorType +
	// the first line of Message. The server uses it for upsert deduplication.
	Signature string

	// ErrorType is the Python exception class, e.g. "ValidationError".
	ErrorType string

	// Message is the full log message (may include a Python traceback).
	Message string

	// Module is the Odoo module that emitted the log, if parseable.
	Module string

	// Model is the Odoo model name extracted from the logger name, if present.
	Model string

	// Traceback is the raw Python traceback text (subset of Message).
	Traceback string

	// UserID is the Odoo UID associated with the request (0 = unknown).
	UserID int

	// RequestURL is the HTTP request URL that triggered the error, if available.
	RequestURL string

	// LogID is the ir.logging record ID — used by the collector to track
	// the high-water mark and avoid re-processing the same entry.
	LogID int

	// CapturedAt is when the event was recorded by Odoo (create_date).
	CapturedAt time.Time
}

// ComputeSignature returns a stable 16-hex-char identifier for an error event.
// It hashes errorType + ":" + the first non-empty line of message so that
// repeated occurrences of the same error map to the same signature.
func ComputeSignature(errorType, message string) string {
	firstLine := firstNonEmpty(message)
	raw := errorType + ":" + firstLine
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:8]) // 16 hex chars
}

// ParseErrorType extracts the Python exception class name from a log message.
// Python tracebacks end with "ExceptionClass: message text", so we look for
// the last line that contains ": " before any blank line.
// Falls back to "UnknownError" if nothing matches.
func ParseErrorType(message string) string {
	lines := strings.Split(strings.TrimSpace(message), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if idx := strings.Index(line, ": "); idx > 0 {
			candidate := line[:idx]
			// Python class names contain only word chars and dots.
			if isIdentifier(candidate) {
				return candidate
			}
		}
		// Last non-blank line with no ":" — treat whole line as type.
		if i == len(lines)-1 || isIdentifier(line) {
			return line
		}
		break
	}
	return unknownErrorType
}

// ExtractTraceback returns the portion of message starting from
// "Traceback (most recent call last):", or the whole message if not found.
func ExtractTraceback(message string) string {
	idx := strings.Index(message, "Traceback (most recent call last):")
	if idx >= 0 {
		return message[idx:]
	}
	return message
}

// ─── helpers ────────────────────────────────────────────────────────────────

func firstNonEmpty(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return s
}

func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') &&
			(c < '0' || c > '9') && c != '_' && c != '.' {
			return false
		}
	}
	return true
}
