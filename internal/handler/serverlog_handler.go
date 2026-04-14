package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/cache"
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"
	"Intelligent_Dev_ToolKit_Odoo/internal/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// ServerLogHandler exposes server log endpoints for the dashboard.
type ServerLogHandler struct {
	*BaseHandler
	cache *cache.RedisClient
}

// NewServerLogHandler creates a ServerLogHandler. Returns nil when cache is nil
// so callers can guard with a nil check before registering routes.
func NewServerLogHandler(base *BaseHandler, redisCache *cache.RedisClient) *ServerLogHandler {
	if redisCache == nil {
		return nil
	}
	return &ServerLogHandler{BaseHandler: base, cache: redisCache}
}

// HandleGetLogs returns recent server log lines for an environment.
//
//	@Summary     Get server logs
//	@Description Returns the last N Odoo server log lines streamed by the agent.
//	@Tags        environments
//	@Produce     json
//	@Param       env_id  path      string  true   "Environment ID"
//	@Param       limit   query     int     false  "Max lines to return (default 200, max 1000)"
//	@Param       level   query     string  false  "Filter by level: DEBUG|INFO|WARNING|ERROR|CRITICAL"
//	@Success     200     {object}  dto.ServerLogsResponse
//	@Failure     400     {object}  api.Error
//	@Failure     401     {object}  api.Error
//	@Failure     404     {object}  api.Error
//	@Router      /environments/{env_id}/server-logs [get]
//	@Security    BearerAuth
func (h *ServerLogHandler) HandleGetLogs(w http.ResponseWriter, r *http.Request) {
	envIDStr := chi.URLParam(r, "env_id")
	if _, err := uuid.Parse(envIDStr); err != nil {
		h.WriteErr(w, r, api.ErrBadRequest("invalid env_id"))
		return
	}

	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == "" {
		h.WriteErr(w, r, api.ErrUnauthorized("missing tenant"))
		return
	}

	limit := int64(200)
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
			limit = n
		}
	}

	levelFilter := r.URL.Query().Get("level")

	lines, err := h.cache.GetServerLogs(r.Context(), envIDStr, limit)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	// Apply optional level filter server-side.
	if levelFilter != "" {
		filtered := lines[:0]
		for _, l := range lines {
			if l.Level == levelFilter {
				filtered = append(filtered, l)
			}
		}
		lines = filtered
	}

	dto.WriteSuccess(w, r, dto.ServerLogsResponse{Lines: lines})
}

// HandleStreamLogs streams live server log lines via Server-Sent Events.
// The client receives historical lines first, then live events as they arrive.
//
//	@Summary     Stream server logs (SSE)
//	@Description Server-Sent Events stream of Odoo server log lines for an environment.
//	@Tags        environments
//	@Produce     text/event-stream
//	@Param       env_id  path  string  true  "Environment ID"
//	@Router      /environments/{env_id}/server-logs/stream [get]
//	@Security    BearerAuth
func (h *ServerLogHandler) HandleStreamLogs(w http.ResponseWriter, r *http.Request) {
	envIDStr := chi.URLParam(r, "env_id")
	if _, err := uuid.Parse(envIDStr); err != nil {
		http.Error(w, "invalid env_id", http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering

	// Send existing history first.
	history, err := h.cache.GetServerLogs(r.Context(), envIDStr, 200)
	if err == nil && len(history) > 0 {
		if data, jsonErr := json.Marshal(history); jsonErr == nil {
			fmt.Fprintf(w, "event: history\ndata: %s\n\n", data) //nolint:errcheck // SSE write; client disconnect detected on the next write
			flusher.Flush()
		}
	}

	// Subscribe to live updates via Redis Pub/Sub.
	sub := h.cache.SubscribeServerLogs(r.Context(), envIDStr)
	defer sub.Close() //nolint:errcheck // best-effort cleanup; connection is already being torn down

	ctx := r.Context()
	ch := sub.Channel()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: log_lines\ndata: %s\n\n", msg.Payload) //nolint:errcheck // SSE write; client disconnect detected on the next write
			flusher.Flush()
		}
	}
}
