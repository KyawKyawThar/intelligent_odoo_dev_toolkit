package middleware

import (
	"net/http"
	"strings"
)

type CORSConfig struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	ExposedHeaders   []string
	AllowCredentials bool
	MaxAge           int // in seconds
}

// DefaultCORSConfig returns sensible defaults for OdooDevTools
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{
			"GET",
			"POST",
			"PUT",
			"PATCH",
			"DELETE",
			"OPTIONS",
		},
		AllowedHeaders: []string{
			"Accept",
			"Authorization",
			"Content-Type",
			"X-API-Key",
			"X-Request-ID",
			"X-Tenant-ID",
		},
		ExposedHeaders: []string{
			"X-Request-ID",
			"X-RateLimit-Limit",
			"X-RateLimit-Remaining",
			"X-RateLimit-Reset",
		},
		AllowCredentials: false,
		MaxAge:           86400, // 24 hours
	}
}

// ProductionCORSConfig returns a more restrictive config for production
func ProductionCORSConfig(allowedOrigins []string) CORSConfig {
	return CORSConfig{
		AllowedOrigins: allowedOrigins,
		AllowedMethods: []string{
			"GET",
			"POST",
			"PUT",
			"PATCH",
			"DELETE",
			"OPTIONS",
		},
		AllowedHeaders: []string{
			"Accept",
			"Authorization",
			"Content-Type",
			"X-API-Key",
			"X-Request-ID",
			"X-Tenant-ID",
		},
		ExposedHeaders: []string{
			"X-Request-ID",
			"X-RateLimit-Limit",
			"X-RateLimit-Remaining",
			"X-RateLimit-Reset",
		},
		AllowCredentials: true,
		MaxAge:           86400,
	}
}

// isOriginAllowed checks if the origin is in the allowed list
func isOriginAllowed(origin string, allowedOrigins []string) bool {
	if origin == "" {
		return false
	}

	for _, allowed := range allowedOrigins {
		// Wildcard allows all origins
		if allowed == "*" {
			return true
		}

		// Exact match
		if allowed == origin {
			return true
		}

		// Wildcard subdomain match (e.g., "*.example.com")
		if strings.HasPrefix(allowed, "*.") {
			suffix := allowed[1:] // ".example.com"
			if strings.HasSuffix(origin, suffix) {
				// Ensure there's something before the suffix (not just ".example.com")
				prefix := origin[:len(origin)-len(suffix)]
				if len(prefix) > 0 && strings.Contains(prefix, "://") {
					return true
				}
			}
		}
	}

	return false
}

// itoa converts int to string without importing strconv
func itoa(i int) string {
	if i == 0 {
		return "0"
	}

	var buf [20]byte
	n := len(buf)
	negative := i < 0
	if negative {
		i = -i
	}

	for i > 0 {
		n--
		buf[n] = byte(i%10) + '0'
		i /= 10
	}

	if negative {
		n--
		buf[n] = '-'
	}

	return string(buf[n:])
}

// =============================================================================
// CORS Middleware
// =============================================================================

// CORS handles Cross-Origin Resource Sharing
func CORS(config CORSConfig) func(http.Handler) http.Handler {
	// Pre-compute joined strings for headers
	allowedMethods := strings.Join(config.AllowedMethods, ", ")
	allowedHeaders := strings.Join(config.AllowedHeaders, ", ")
	exposedHeaders := strings.Join(config.ExposedHeaders, ", ")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Check if origin is allowed
			allowed := isOriginAllowed(origin, config.AllowedOrigins)

			if allowed && origin != "" {
				// Set CORS headers
				w.Header().Set("Access-Control-Allow-Origin", origin)

				if config.AllowCredentials {
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}

				if len(config.ExposedHeaders) > 0 {
					w.Header().Set("Access-Control-Expose-Headers", exposedHeaders)
				}
			}

			// Handle preflight request
			if r.Method == http.MethodOptions {
				if allowed {
					w.Header().Set("Access-Control-Allow-Methods", allowedMethods)
					w.Header().Set("Access-Control-Allow-Headers", allowedHeaders)
					w.Header().Set("Access-Control-Max-Age", itoa(config.MaxAge))
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// =============================================================================
// Simple CORS (for development)
// =============================================================================

// SimpleCORS allows all origins (use only in development!)
func SimpleCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-API-Key, X-Request-ID")
		w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
