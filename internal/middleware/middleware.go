package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// =============================================================================
// Context Keys
// =============================================================================

type contextKey string

const (
	ContextKeyRequestID   contextKey = "request_id"
	ContextKeyRequestTime contextKey = "request_time"
	ContextKeyTenantID    contextKey = "tenant_id"
	ContextKeyUserID      contextKey = "user_id"
	ContextKeyAPIKeyID    contextKey = "api_key_id"
	ContextKeyEnvID       contextKey = "environment_id"
)

// =============================================================================
// Context Getters
// =============================================================================

func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(ContextKeyRequestID).(string); ok {
		return id
	}
	return ""
}
func GetRequestTime(ctx context.Context) time.Time {
	if t, ok := ctx.Value(ContextKeyRequestTime).(time.Time); ok {
		return t
	}
	return time.Time{}
}

func GetTenantID(ctx context.Context) string {
	if id, ok := ctx.Value(ContextKeyTenantID).(string); ok {
		return id
	}
	return ""
}

func GetUserID(ctx context.Context) string {
	if id, ok := ctx.Value(ContextKeyUserID).(string); ok {
		return id
	}
	return ""
}

func GetAPIKeyID(ctx context.Context) string {
	if id, ok := ctx.Value(ContextKeyAPIKeyID).(string); ok {
		return id
	}
	return ""
}

func GetEnvID(ctx context.Context) string {
	if id, ok := ctx.Value(ContextKeyEnvID).(string); ok {
		return id
	}
	return ""
}

// =============================================================================
// Context Setters
// =============================================================================

func SetRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, ContextKeyRequestID, requestID)
}

func SetTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, ContextKeyTenantID, tenantID)
}

func SetUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, ContextKeyUserID, userID)
}

func SetAPIKeyID(ctx context.Context, keyID string) context.Context {
	return context.WithValue(ctx, ContextKeyAPIKeyID, keyID)
}

func SetEnvID(ctx context.Context, envID string) context.Context {
	return context.WithValue(ctx, ContextKeyEnvID, envID)
}

// =============================================================================
// Request ID Middleware
// =============================================================================

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}

		w.Header().Set("X-Request-ID", requestID)

		ctx := context.WithValue(r.Context(), ContextKeyRequestID, requestID)
		ctx = context.WithValue(ctx, ContextKeyRequestTime, time.Now())

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// =============================================================================
// Recovery Middleware (Panic Handler)
// =============================================================================

func Recoverer(logger *zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					requestID := GetRequestID(r.Context())
					stack := string(debug.Stack())

					logger.Error().
						Interface("error", err).
						Str("request_id", requestID).
						Str("method", r.Method).
						Str("path", r.URL.Path).
						Str("stack", stack).
						Msg("panic recovered")

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					if err := json.NewEncoder(w).Encode(map[string]any{
						"error": map[string]any{
							"code":       "INTERNAL_ERROR",
							"message":    "An internal error occurred",
							"request_id": requestID,
						},
					}); err != nil {
						logger.Error().Err(err).Msg("failed to write panic response")
					}
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// =============================================================================
// Security Headers Middleware
// =============================================================================

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		next.ServeHTTP(w, r)
	})
}

// =============================================================================
// Content-Type JSON Middleware
// =============================================================================

func ContentTypeJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
			contentType := r.Header.Get("Content-Type")
			if contentType != "" && len(contentType) >= 16 && contentType[:16] != "application/json" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnsupportedMediaType)
				_ = json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
					"error": map[string]any{
						"code":    "UNSUPPORTED_MEDIA_TYPE",
						"message": "Content-Type must be application/json",
					},
				})
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// =============================================================================
// Timeout Middleware
// =============================================================================

func Timeout(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			done := make(chan struct{})

			go func() {
				next.ServeHTTP(w, r.WithContext(ctx))
				close(done)
			}()

			select {
			case <-done:
				// Request completed normally
			case <-ctx.Done():
				if ctx.Err() == context.DeadlineExceeded {
					requestID := GetRequestID(r.Context())
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusGatewayTimeout)
					_ = json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
						"error": map[string]any{
							"code":       "TIMEOUT",
							"message":    "Request timed out",
							"request_id": requestID,
						},
					})
				}
			}
		})
	}
}

// =============================================================================
// Real IP Middleware
// =============================================================================

func RealIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
			if idx := findChar(ip, ','); idx != -1 {
				ip = ip[:idx]
			}
			r.RemoteAddr = ip
		} else if ip := r.Header.Get("X-Real-IP"); ip != "" {
			r.RemoteAddr = ip
		} else if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
			r.RemoteAddr = ip
		}

		next.ServeHTTP(w, r)
	})
}

func findChar(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

// =============================================================================
// Max Body Size Middleware
// =============================================================================

func MaxBodySize(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
