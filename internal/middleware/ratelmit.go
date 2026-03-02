package middleware

import (
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type RateLimitConfig struct {
	// Requests per window
	Limit int
	// Window duration
	Window time.Duration
	// Key function to identify the client (e.g., by IP, tenant, user)
	KeyFunc func(r *http.Request) string
	// Skip function to bypass rate limiting for certain requests
	SkipFunc func(r *http.Request) bool
	// Custom headers
	LimitHeader     string
	RemainingHeader string
	ResetHeader     string
}

func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		Limit:  100,
		Window: time.Minute,
		KeyFunc: func(r *http.Request) string {
			// Default: rate limit by IP
			return r.RemoteAddr
		},
		SkipFunc:        nil,
		LimitHeader:     "X-RateLimit-Limit",
		RemainingHeader: "X-RateLimit-Remaining",
		ResetHeader:     "X-RateLimit-Reset",
	}
}

// PerTenantRateLimitConfig creates config for per-tenant rate limiting
func PerTenantRateLimitConfig(limit int, window time.Duration) RateLimitConfig {
	return RateLimitConfig{
		Limit:  limit,
		Window: window,
		KeyFunc: func(r *http.Request) string {
			tenantID := GetTenantID(r.Context())
			if tenantID != "" {
				return "tenant:" + tenantID
			}
			return "ip:" + r.RemoteAddr
		},
		LimitHeader:     "X-RateLimit-Limit",
		RemainingHeader: "X-RateLimit-Remaining",
		ResetHeader:     "X-RateLimit-Reset",
	}
}

// =============================================================================
// In-Memory Rate Limiter
// =============================================================================

type rateLimitEntry struct {
	count     int
	resetTime time.Time
}

type InMemoryRateLimiter struct {
	entries map[string]*rateLimitEntry
	mu      sync.RWMutex
	config  RateLimitConfig
}

func (rl *InMemoryRateLimiter) cleanup() {
	ticker := time.NewTicker(rl.config.Window)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for key, entry := range rl.entries {
			if entry.resetTime.Before(now) {
				delete(rl.entries, key)
			}
		}
		rl.mu.Unlock()
	}
}

func NewInMemoryRateLimiter(config RateLimitConfig) *InMemoryRateLimiter {
	rl := &InMemoryRateLimiter{
		entries: make(map[string]*rateLimitEntry),
		config:  config,
	}

	// Start cleanup goroutine
	go rl.cleanup()

	return rl
}
func (rl *InMemoryRateLimiter) Allow(key string) (allowed bool, remaining int, resetTime time.Time) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	entry, exists := rl.entries[key]

	if !exists || entry.resetTime.Before(now) {
		// Create new entry
		entry = &rateLimitEntry{
			count:     1,
			resetTime: now.Add(rl.config.Window),
		}
		rl.entries[key] = entry
		return true, rl.config.Limit - 1, entry.resetTime
	}

	if entry.count >= rl.config.Limit {
		return false, 0, entry.resetTime
	}

	entry.count++
	return true, rl.config.Limit - entry.count, entry.resetTime
}

// =============================================================================
// Rate Limit Middleware
// =============================================================================

// RateLimit creates a rate limiting middleware using in-memory storage
func RateLimit(config RateLimitConfig) func(http.Handler) http.Handler {
	limiter := NewInMemoryRateLimiter(config)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if we should skip rate limiting
			if config.SkipFunc != nil && config.SkipFunc(r) {
				next.ServeHTTP(w, r)
				return
			}

			// Get rate limit key
			key := config.KeyFunc(r)

			// Check rate limit
			allowed, remaining, resetTime := limiter.Allow(key)

			// Set rate limit headers
			w.Header().Set(config.LimitHeader, fmt.Sprintf("%d", config.Limit))
			w.Header().Set(config.RemainingHeader, fmt.Sprintf("%d", remaining))
			w.Header().Set(config.ResetHeader, fmt.Sprintf("%d", resetTime.Unix()))

			if !allowed {
				retryAfter := int(time.Until(resetTime).Seconds())
				if retryAfter < 1 {
					retryAfter = 1
				}
				w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
				api.HandleError(w, r, api.ErrRateLimited(retryAfter))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// =============================================================================
// Redis Rate Limiter Interface
// =============================================================================

// RedisRateLimiter is an interface for Redis-based rate limiting
type RedisRateLimiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (allowed bool, remaining int, resetTime time.Time, err error)
}

// RateLimitWithRedis creates rate limiting middleware using Redis
func RateLimitWithRedis(limiter RedisRateLimiter, config RateLimitConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if we should skip rate limiting
			if config.SkipFunc != nil && config.SkipFunc(r) {
				next.ServeHTTP(w, r)
				return
			}

			// Get rate limit key
			key := config.KeyFunc(r)

			// Check rate limit
			allowed, remaining, resetTime, err := limiter.Allow(r.Context(), key, config.Limit, config.Window)
			if err != nil {
				// On error, allow the request but log the error
				next.ServeHTTP(w, r)
				return
			}

			// Set rate limit headers
			w.Header().Set(config.LimitHeader, fmt.Sprintf("%d", config.Limit))
			w.Header().Set(config.RemainingHeader, fmt.Sprintf("%d", remaining))
			w.Header().Set(config.ResetHeader, fmt.Sprintf("%d", resetTime.Unix()))

			if !allowed {
				retryAfter := int(time.Until(resetTime).Seconds())
				if retryAfter < 1 {
					retryAfter = 1
				}
				w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
				api.HandleError(w, r, api.ErrRateLimited(retryAfter))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// =============================================================================
// Tiered Rate Limits (by plan)
// =============================================================================

type PlanLimits struct {
	RequestsPerMinute int
	RequestsPerHour   int
	IngestPerMinute   int // For agent ingestion endpoints
}

var DefaultPlanLimits = map[string]PlanLimits{
	"free": {
		RequestsPerMinute: 60,
		RequestsPerHour:   1000,
		IngestPerMinute:   100,
	},
	"cloud": {
		RequestsPerMinute: 100,
		RequestsPerHour:   5000,
		IngestPerMinute:   500,
	},
	"onprem": {
		RequestsPerMinute: 200,
		RequestsPerHour:   10000,
		IngestPerMinute:   1000,
	},
	"enterprise": {
		RequestsPerMinute: 500,
		RequestsPerHour:   50000,
		IngestPerMinute:   5000,
	},
}

// TieredRateLimit applies different rate limits based on tenant plan
func TieredRateLimit(planLimits map[string]PlanLimits) func(http.Handler) http.Handler {
	// Create a limiter for each plan
	limiters := make(map[string]*InMemoryRateLimiter)
	for plan, limits := range planLimits {
		config := RateLimitConfig{
			Limit:  limits.RequestsPerMinute,
			Window: time.Minute,
			KeyFunc: func(r *http.Request) string {
				return GetTenantID(r.Context())
			},
			LimitHeader:     "X-RateLimit-Limit",
			RemainingHeader: "X-RateLimit-Remaining",
			ResetHeader:     "X-RateLimit-Reset",
		}
		limiters[plan] = NewInMemoryRateLimiter(config)
	}

	// Default limiter for unknown plans
	defaultConfig := RateLimitConfig{
		Limit:  60,
		Window: time.Minute,
		KeyFunc: func(r *http.Request) string {
			return GetTenantID(r.Context())
		},
	}
	defaultLimiter := NewInMemoryRateLimiter(defaultConfig)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenantID := GetTenantID(r.Context())
			if tenantID == "" {
				// No tenant, use IP-based rate limiting
				next.ServeHTTP(w, r)
				return
			}

			plan := GetTenantPlan(r.Context())
			limiter, exists := limiters[plan]
			if !exists {
				limiter = defaultLimiter
			}

			limits := planLimits[plan]
			if limits.RequestsPerMinute == 0 {
				limits = DefaultPlanLimits["free"]
			}

			// Check rate limit
			allowed, remaining, resetTime := limiter.Allow(tenantID)

			// Set headers
			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", limits.RequestsPerMinute))
			w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
			w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetTime.Unix()))

			if !allowed {
				retryAfter := int(time.Until(resetTime).Seconds())
				if retryAfter < 1 {
					retryAfter = 1
				}
				w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
				api.HandleError(w, r, api.ErrRateLimited(retryAfter))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
