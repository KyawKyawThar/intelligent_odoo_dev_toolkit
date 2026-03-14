package middleware

import (
	"bufio"
	"net"
	"net/http"
	"time"

	"github.com/rs/zerolog"
)

// =============================================================================
// Response Writer Wrapper
// =============================================================================

type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	size        int
}

func wrapResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w, status: http.StatusOK}
}
func (rw *responseWriter) WriteHeader(code int) {
	if !rw.wroteHeader {
		rw.status = code
		rw.wroteHeader = true
		rw.ResponseWriter.WriteHeader(code)
	}
}
func (rw *responseWriter) Write(b []byte) (int, error) {
	rw.size += len(b)
	return rw.ResponseWriter.Write(b)
}

// Hijack implements http.Hijacker so WebSocket upgrades work through this wrapper.
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return rw.ResponseWriter.(http.Hijacker).Hijack()
}

// Status returns the HTTP status code
func (rw *responseWriter) Status() int {
	return rw.status
}

// Size returns the number of bytes written
func (rw *responseWriter) Size() int {
	return rw.size
}

// =============================================================================
// Request Logger Middleware
// =============================================================================

// RequestLogger logs HTTP requests using zerolog
func RequestLogger(logger *zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			wrapped := wrapResponseWriter(w)

			// Process request
			next.ServeHTTP(wrapped, r)

			// Calculate duration
			duration := time.Since(start)

			// Determine log level based on status
			var event *zerolog.Event
			switch {
			case wrapped.status >= 500:
				event = logger.Error()
			case wrapped.status >= 400:
				event = logger.Warn()
			default:
				event = logger.Info()
			}

			// Log the request
			event.
				Str("request_id", GetRequestID(r.Context())).
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Str("query", r.URL.RawQuery).
				Int("status", wrapped.status).
				Int("size", wrapped.size).
				Dur("duration", duration).
				Str("user_agent", r.UserAgent()).
				Str("remote_addr", r.RemoteAddr).
				Str("tenant_id", GetTenantID(r.Context())).
				Str("user_id", GetUserID(r.Context())).
				Msg("http request")
		})
	}
}

// =============================================================================
// Structured Request Logger (Alternative)
// =============================================================================

type LogEntry struct {
	RequestID  string        `json:"request_id"`
	Method     string        `json:"method"`
	Path       string        `json:"path"`
	Query      string        `json:"query,omitempty"`
	Status     int           `json:"status"`
	Size       int           `json:"size"`
	Duration   time.Duration `json:"duration_ns"`
	DurationMS float64       `json:"duration_ms"`
	UserAgent  string        `json:"user_agent,omitempty"`
	RemoteAddr string        `json:"remote_addr"`
	TenantID   string        `json:"tenant_id,omitempty"`
	UserID     string        `json:"user_id,omitempty"`
	Error      string        `json:"error,omitempty"`
}

// StructuredLogger logs requests with full structured data
func StructuredLogger(logger *zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			wrapped := wrapResponseWriter(w)

			// Process request
			next.ServeHTTP(wrapped, r)

			// Build log entry
			duration := time.Since(start)
			entry := LogEntry{
				RequestID:  GetRequestID(r.Context()),
				Method:     r.Method,
				Path:       r.URL.Path,
				Query:      r.URL.RawQuery,
				Status:     wrapped.status,
				Size:       wrapped.size,
				Duration:   duration,
				DurationMS: float64(duration.Nanoseconds()) / 1e6,
				UserAgent:  r.UserAgent(),
				RemoteAddr: r.RemoteAddr,
				TenantID:   GetTenantID(r.Context()),
				UserID:     GetUserID(r.Context()),
			}

			// Log based on status
			var event *zerolog.Event
			switch {
			case wrapped.status >= 500:
				event = logger.Error()
			case wrapped.status >= 400:
				event = logger.Warn()
			default:
				event = logger.Info()
			}

			event.
				Interface("entry", entry).
				Msg("request completed")
		})
	}
}

// =============================================================================
// Skip Paths (for health checks)
// =============================================================================

// SkipPaths returns a logger that skips certain paths
func SkipPaths(logger *zerolog.Logger, skipPaths ...string) func(http.Handler) http.Handler {
	skipMap := make(map[string]bool)
	for _, path := range skipPaths {
		skipMap[path] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip logging for certain paths
			if skipMap[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			// Use regular logger
			RequestLogger(logger)(next).ServeHTTP(w, r)
		})
	}
}
