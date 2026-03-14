package handler

import (
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"encoding/json"
	"net/http"

	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/middleware"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow all connections by default.
		// In production, you should implement a proper origin check.
		return true
	},
}

type WsHandler struct {
	*BaseHandler
	store db.Store
}

func NewWsHandler(base *BaseHandler, store db.Store) *WsHandler {
	return &WsHandler{BaseHandler: base, store: store}
}

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
// @Router /api/v1/agent/ws [get]
// @Security ApiKeyAuth
func (h *WsHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to upgrade connection")
		api.HandleError(w, r, api.ErrInternal(err))
		return
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil {
			h.logger.Error().Err(closeErr).Msg("failed to close websocket connection")
		}
	}()

	env, err := h.validateRequest(r)
	if err != nil {
		api.HandleError(w, r, err)
		return
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

func (h *WsHandler) handleMessages(r *http.Request, conn *websocket.Conn, env db.Environment) {
	agentID := ""
	if env.AgentID != nil {
		agentID = *env.AgentID
	}
	for {
		messageType, p, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				h.logger.Error().Err(err).Msg("unxpected close error")
			}
			break
		}
		if messageType == websocket.TextMessage {

			var payload HeartbeatPayload
			if jsonErr := json.Unmarshal(p, &payload); jsonErr != nil {
				h.logger.Error().Err(jsonErr).Msg("failed to unmarshal heartbeat")
				continue
			}

			arg := db.InsertHeartbeatParams{
				EnvID:        env.ID,
				AgentID:      agentID,
				AgentVersion: &payload.AgentVersion,
				Status:       payload.Status,
				Metadata:     payload.Metadata,
			}
			_, err = h.store.InsertHeartbeat(r.Context(), arg)
			if err != nil {
				h.logger.Error().Err(err).Msg("failed to insert heartbeat")
				continue
			}
		}
	}
}
