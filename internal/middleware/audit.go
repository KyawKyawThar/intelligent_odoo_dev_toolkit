package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"time"

	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// AuditLog is a middleware that records every state-changing HTTP request
// (POST, PUT, PATCH, DELETE) to the audit_logs table after the handler runs.
// The write is fire-and-forget so it never blocks the response.
//
// It must be placed AFTER JWTAuth and TenantResolver so that tenant_id and
// user_id are already set in the request context.
func AuditLog(store db.Store, logger zerolog.Logger) func(http.Handler) http.Handler { //nolint:gocognit // middleware fan-out: skip, wrap writer, parse tenant/user, fire goroutine — linear but many branches
	log := logger.With().Str("component", "audit-middleware").Logger()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip read-only and pre-flight requests — nothing changes.
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}

			// Wrap the writer to capture the response status code.
			rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rw, r)

			// Only audit if we have a tenant (i.e. authenticated, protected route).
			tenantIDStr := GetTenantID(r.Context())
			if tenantIDStr == "" {
				return
			}
			tenantID, err := uuid.Parse(tenantIDStr)
			if err != nil {
				return
			}

			// Derive the action from method + Chi route pattern (e.g. "PATCH /api/v1/environments/{env_id}").
			routePattern := r.URL.Path
			if rc := chi.RouteContext(r.Context()); rc != nil {
				if rp := rc.RoutePattern(); rp != "" {
					routePattern = rp
				}
			}
			action := r.Method + " " + routePattern

			// Optional user ID — may be absent for API-key-authenticated requests.
			var userID *uuid.UUID
			if raw := GetUserID(r.Context()); raw != "" {
				if uid, parseErr := uuid.Parse(raw); parseErr == nil {
					userID = &uid
				}
			}

			// Client IP.
			ipAddr := parseClientIP(r.RemoteAddr)

			// Metadata: status code + user-agent captured for diagnostics.
			metadata, _ := json.Marshal(map[string]any{ //nolint:errcheck // static structure, always marshals
				"status_code": rw.status,
				"user_agent":  r.Header.Get("User-Agent"),
				"path":        r.URL.Path,
			})

			// Fire-and-forget — use a background context so the write outlives
			// the request context cancellation.
			go func() { //nolint:gosec,contextcheck // intentional: background context needed after request ends
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				if _, writeErr := store.CreateAuditLog(ctx, db.CreateAuditLogParams{
					TenantID:  tenantID,
					UserID:    userID,
					IpAddress: ipAddr,
					Action:    action,
					Metadata:  metadata,
				}); writeErr != nil {
					log.Warn().Err(writeErr).
						Str("action", action).
						Str("tenant_id", tenantIDStr).
						Msg("audit log write failed")
				}
			}()
		})
	}
}

// =============================================================================
// statusRecorder — captures the HTTP status code written by the handler.
// =============================================================================

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// =============================================================================
// parseClientIP — converts a remote address string to *netip.Addr.
// Returns nil if the address cannot be parsed (non-fatal).
// =============================================================================

func parseClientIP(remoteAddr string) *netip.Addr {
	host := remoteAddr
	// "host:port" → strip port
	if strings.ContainsRune(host, ':') {
		if h, _, err := splitHostPort(host); err == nil {
			host = h
		}
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return nil
	}
	return &addr
}

// splitHostPort mimics net.SplitHostPort without importing net (avoids import cycle risk).
func splitHostPort(hostport string) (host, port string, err error) {
	// IPv6 literal: [::1]:8080
	if hostport != "" && hostport[0] == '[' {
		end := strings.LastIndex(hostport, "]")
		if end < 0 {
			return "", "", fmt.Errorf("missing ']' in address %q", hostport)
		}
		host = hostport[1:end]
		if end+1 < len(hostport) && hostport[end+1] == ':' {
			port = hostport[end+2:]
		}
		return host, port, nil
	}
	// Regular host:port
	last := strings.LastIndex(hostport, ":")
	if last < 0 {
		return hostport, "", nil
	}
	return hostport[:last], hostport[last+1:], nil
}
