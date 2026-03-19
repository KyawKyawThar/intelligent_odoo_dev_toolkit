package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"
	"Intelligent_Dev_ToolKit_Odoo/internal/middleware"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// agentConn wraps a WebSocket connection with a write mutex to prevent
// concurrent writes (gorilla/websocket panics on concurrent WriteMessage).
type agentConn struct {
	conn *websocket.Conn
	wmu  sync.Mutex
}

// WsHandler handles agent WebSocket connections and provides a connection
// registry for pushing feature flag updates to connected agents.
type WsHandler struct {
	*BaseHandler
	store db.Store

	// Connection registry: env_id -> active agent connection.
	mu    sync.RWMutex
	conns map[uuid.UUID]*agentConn
}

func NewWsHandler(base *BaseHandler, store db.Store) *WsHandler {
	return &WsHandler{
		BaseHandler: base,
		store:       store,
		conns:       make(map[uuid.UUID]*agentConn),
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// Connection registry
// ═══════════════════════════════════════════════════════════════════════════

func (h *WsHandler) register(envID uuid.UUID, conn *websocket.Conn) *agentConn {
	h.mu.Lock()
	defer h.mu.Unlock()

	// If there's already a connection for this env, close the old one.
	if old, ok := h.conns[envID]; ok {
		_ = old.conn.Close() //nolint:errcheck // best-effort close of replaced connection
		h.logger.Warn().Str("env_id", envID.String()).Msg("replaced existing agent connection")
	}

	ac := &agentConn{conn: conn}
	h.conns[envID] = ac

	h.logger.Info().Str("env_id", envID.String()).Int("total", len(h.conns)).Msg("agent connected")
	return ac
}

func (h *WsHandler) unregister(envID uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.conns, envID)
	h.logger.Info().Str("env_id", envID.String()).Int("total", len(h.conns)).Msg("agent disconnected")
}

// PushFlags sends a flags_update message to the connected agent for envID.
// Returns true if the message was sent successfully.
func (h *WsHandler) PushFlags(envID uuid.UUID, flags json.RawMessage) bool {
	h.mu.RLock()
	ac, ok := h.conns[envID]
	h.mu.RUnlock()

	if !ok {
		return false
	}

	return h.writeFlags(ac, flags)
}

// writeFlags sends a flags_update WebSocket message. Thread-safe.
func (h *WsHandler) writeFlags(ac *agentConn, flags json.RawMessage) bool {
	if flags == nil || string(flags) == "null" {
		flags = json.RawMessage(`{}`)
	}

	msg, err := json.Marshal(map[string]any{
		"type":  "flags_update",
		"flags": json.RawMessage(flags),
	})
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to marshal flags_update")
		return false
	}

	ac.wmu.Lock()
	err = ac.conn.WriteMessage(websocket.TextMessage, msg)
	ac.wmu.Unlock()

	if err != nil {
		h.logger.Error().Err(err).Msg("failed to send flags_update")
		return false
	}
	return true
}

// ═══════════════════════════════════════════════════════════════════════════
// WebSocket handler (agent connection)
// ═══════════════════════════════════════════════════════════════════════════

type HeartbeatPayload struct {
	AgentVersion string          `json:"agent_version"`
	Status       string          `json:"status"`
	Metadata     json.RawMessage `json:"metadata"`
}

// HandleWebSocket godoc
// @Summary WebSocket endpoint for agent communication
// @Description Upgrades the HTTP connection to a WebSocket connection and handles agent heartbeats.
// @Tags Agents
// @Accept json
// @Produce json
// @Param agent_id query string true "Agent ID"
// @Success 101 {string} string "Switching Protocols"
// @Failure 400 {object} api.Error "Bad Request: Missing or invalid agent_id"
// @Failure 401 {object} api.Error "Unauthorized: Invalid or missing API key"
// @Failure 403 {object} api.Error "Forbidden: Agent does not belong to the tenant"
// @Failure 404 {object} api.Error "Not Found: Environment not found for the given agent_id"
// @Failure 500 {object} api.Error "Internal Server Error: Failed to upgrade connection or database error"
// @Router /agent/ws [get]
// @Security ApiKeyAuth
func (h *WsHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	env, err := h.validateRequest(r)
	if err != nil {
		api.HandleError(w, r, err)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to upgrade connection")
		return
	}

	// Register in connection registry + unregister on disconnect.
	ac := h.register(env.ID, conn)
	defer func() {
		h.unregister(env.ID)
		_ = conn.Close() //nolint:errcheck // best-effort close on disconnect
	}()

	// Send current feature flags immediately on connect — but only if the
	// environment actually has flags stored. Pushing an empty object would
	// reset the agent's sampler/rate-limiter config to zero values.
	if len(env.FeatureFlags) > 0 && string(env.FeatureFlags) != "null" && string(env.FeatureFlags) != "{}" {
		h.writeFlags(ac, env.FeatureFlags)
	}

	h.handleMessages(r, conn, env)
}

func (h *WsHandler) validateRequest(r *http.Request) (db.Environment, error) {
	agentID := r.URL.Query().Get("agent_id")
	if agentID == "" {
		return db.Environment{}, api.NewError(api.ErrCodeValidation, "agent_id is required", http.StatusBadRequest)
	}

	tenantIDStr := middleware.GetTenantID(r.Context())
	if tenantIDStr == "" {
		return db.Environment{}, api.ErrUnauthorized("missing tenant id")
	}
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		return db.Environment{}, api.ErrInvalidUUID("tenant_id")
	}

	env, err := h.store.GetEnvironmentByAgentID(r.Context(), &agentID)
	if err != nil {
		return db.Environment{}, api.ErrNotFound("environment")
	}

	if env.TenantID != tenantID {
		return db.Environment{}, api.ErrForbidden("environment does not belong to tenant")
	}

	return env, nil
}

// wsEnvelope is the generic message envelope for dispatching WebSocket messages.
type wsEnvelope struct {
	Type string `json:"type"`
}

func (h *WsHandler) handleMessages(r *http.Request, conn *websocket.Conn, env db.Environment) {
	agentID := ""
	if env.AgentID != nil {
		agentID = *env.AgentID
	}
	for {
		messageType, p, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				h.logger.Error().Err(err).Msg("unexpected close error")
			}
			break
		}
		if messageType != websocket.TextMessage {
			continue
		}

		// Parse the message type to dispatch.
		var envelope wsEnvelope
		if err := json.Unmarshal(p, &envelope); err != nil {
			h.logger.Error().Err(err).Msg("failed to unmarshal ws message")
			continue
		}

		switch envelope.Type {
		case "heartbeat":
			h.handleHeartbeat(r.Context(), p, env.ID, agentID)
		case "pong":
			// Agent responded to our ping — nothing to do.
		default:
			h.logger.Debug().Str("type", envelope.Type).Msg("unhandled ws message type")
		}
	}
}

func (h *WsHandler) handleHeartbeat(ctx context.Context, raw []byte, envID uuid.UUID, agentID string) {
	var payload HeartbeatPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		h.logger.Error().Err(err).Msg("failed to unmarshal heartbeat")
		return
	}

	metadata := payload.Metadata
	if metadata == nil {
		metadata = json.RawMessage(`{}`)
	}

	arg := db.InsertHeartbeatParams{
		EnvID:        envID,
		AgentID:      agentID,
		AgentVersion: &payload.AgentVersion,
		Status:       payload.Status,
		Metadata:     metadata,
	}

	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := h.store.InsertHeartbeat(dbCtx, arg)
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to insert heartbeat")
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// Admin endpoint: PUT /api/v1/environments/{env_id}/flags
// ═══════════════════════════════════════════════════════════════════════════

// HandleUpdateFlags updates an environment's feature flags and pushes them
// to the connected agent in real-time via WebSocket.
//
//	@Summary      Update feature flags
//	@Description  Update feature flags for an environment and push to connected agent
//	@Tags         environments
//	@Accept       json
//	@Produce      json
//	@Param        env_id  path      string                   true  "Environment ID"
//	@Param        body    body      dto.UpdateFlagsRequest   true  "Feature flags"
//	@Success      200     {object}  dto.UpdateFlagsResponse
//	@Failure      400     {object}  api.Error
//	@Failure      401     {object}  api.Error
//	@Failure      404     {object}  api.Error
//	@Router       /environments/{env_id}/flags [put]
//	@Security     BearerAuth
func (h *WsHandler) HandleUpdateFlags(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.MustTenantID(w, r)
	if !ok {
		return
	}
	envID, ok := h.MustUUIDParam(w, r, "env_id")
	if !ok {
		return
	}

	var req dto.UpdateFlagsRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	// Persist to database.
	updated, err := h.store.UpdateFeatureFlags(r.Context(), db.UpdateFeatureFlagsParams{
		ID:           envID,
		FeatureFlags: req.Flags,
		TenantID:     tenantID,
	})
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	// Push to connected agent (if online).
	pushed := h.PushFlags(envID, req.Flags)

	h.mu.RLock()
	_, connected := h.conns[envID]
	h.mu.RUnlock()

	resp := dto.UpdateFlagsResponse{
		ID:             updated.ID,
		FeatureFlags:   updated.FeatureFlags,
		AgentConnected: connected,
		Pushed:         pushed,
	}

	h.logger.Info().
		Str("env_id", envID.String()).
		Bool("agent_connected", connected).
		Bool("pushed", pushed).
		Msg("feature flags updated")

	dto.WriteSuccess(w, r, resp)
}
