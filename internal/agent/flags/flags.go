// Package flags implements the agent-side feature flag receiver. It connects
// to the cloud server via WebSocket, receives flag updates pushed by the
// server, and applies them hot to the running agent components (sampler,
// rate limiter, aggregator) without requiring a restart.
//
// Protocol:
//
//	Agent → Server:  {"type":"heartbeat", ...}   (periodic keep-alive)
//	Server → Agent:  {"type":"flags_update", "flags":{...}}
//	Server → Agent:  {"type":"ping"}
//	Agent → Server:  {"type":"pong"}
package flags

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"Intelligent_Dev_ToolKit_Odoo/internal/agent/creds"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/sampler"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

// ────────────────────────────────────────────────────────────────────────────
// FeatureFlags — the full set of flags pushed by the cloud
// ────────────────────────────────────────────────────────────────────────────

// FeatureFlags is the complete set of per-environment configuration pushed
// from the cloud to the agent. Fields use JSON tags matching the server's
// flag_update payload.
type FeatureFlags struct {
	// Sampling
	SamplingMode    string  `json:"sampling_mode"`     // full | sampled | aggregated_only
	SampleRate      float64 `json:"sample_rate"`       // 0.0–1.0
	SlowThresholdMS int     `json:"slow_threshold_ms"` // capture above this

	// Collection toggles
	CollectORM      bool `json:"collect_orm"`
	CollectSQL      bool `json:"collect_sql"`
	CollectErrors   bool `json:"collect_errors"`
	CollectProfiler bool `json:"collect_profiler"`

	// Limits
	MaxEventsPerBatch int   `json:"max_events_per_batch"`
	MaxBytesPerMinute int64 `json:"max_bytes_per_minute"`
	FlushIntervalSec  int   `json:"flush_interval_sec"`

	// Privacy
	StripPII     bool     `json:"strip_pii"`
	RedactFields []string `json:"redact_fields"`
}

// ToSamplerConfig converts the relevant flag fields into a sampler.Config.
func (f *FeatureFlags) ToSamplerConfig() sampler.Config {
	var alwaysCapture []string
	// Errors and slow queries are always captured by default when not in full mode.
	if f.SamplingMode != "full" {
		alwaysCapture = []string{"error", "slow_query", "n1"}
	}

	return sampler.Config{
		Mode:            sampler.Mode(f.SamplingMode),
		SampleRate:      f.SampleRate,
		AlwaysCapture:   alwaysCapture,
		SlowThresholdMS: f.SlowThresholdMS,
	}
}

// ────────────────────────────────────────────────────────────────────────────
// WebSocket message types
// ────────────────────────────────────────────────────────────────────────────

// wsMessage is the envelope for all WebSocket messages between cloud and agent.
type wsMessage struct {
	Type  string          `json:"type"`
	Flags json.RawMessage `json:"flags,omitempty"`
}

// heartbeatPayload is what the agent sends periodically.
type heartbeatPayload struct {
	Type         string `json:"type"`
	AgentVersion string `json:"agent_version"`
	Status       string `json:"status"`
}

// ────────────────────────────────────────────────────────────────────────────
// FlagApplier — callback interface for applying flags to components
// ────────────────────────────────────────────────────────────────────────────

// FlagApplier is called whenever new flags arrive from the cloud.
// main.go implements this to wire updated flags to the sampler, rate limiter,
// and other components.
type FlagApplier func(flags FeatureFlags)

// ────────────────────────────────────────────────────────────────────────────
// ReceiverConfig
// ────────────────────────────────────────────────────────────────────────────

// ReceiverConfig configures the WebSocket flag receiver.
type ReceiverConfig struct {
	// WebSocket URL, e.g. ws://server:8080/api/v1/agent/ws?agent_id=xxx
	URL string

	// AgentID passed as query parameter.
	AgentID string

	// HeartbeatInterval controls how often the agent sends a heartbeat.
	// Default: 30s.
	HeartbeatInterval time.Duration

	// ReconnectDelay is the initial delay between reconnection attempts.
	// It doubles each retry up to ReconnectMaxDelay. Default: 2s.
	ReconnectDelay time.Duration

	// ReconnectMaxDelay caps the exponential backoff. Default: 60s.
	ReconnectMaxDelay time.Duration

	// AgentVersion is reported in heartbeats.
	AgentVersion string
}

// DefaultReceiverConfig returns sensible defaults.
func DefaultReceiverConfig(wsURL, agentID string) ReceiverConfig {
	return ReceiverConfig{
		URL:               wsURL,
		AgentID:           agentID,
		HeartbeatInterval: 30 * time.Second,
		ReconnectDelay:    2 * time.Second,
		ReconnectMaxDelay: 60 * time.Second,
		AgentVersion:      "0.1.0",
	}
}

// ────────────────────────────────────────────────────────────────────────────
// FlagReceiver
// ────────────────────────────────────────────────────────────────────────────

// FlagReceiver maintains a WebSocket connection to the cloud server and
// applies incoming feature flag updates. It handles:
//   - Automatic reconnection with exponential backoff
//   - Heartbeat keep-alive
//   - Ping/pong responses
//   - Thread-safe access to the latest flags
type FlagReceiver struct {
	cfg     ReceiverConfig
	applier FlagApplier
	creds   creds.Provider
	logger  zerolog.Logger

	mu      sync.RWMutex
	current FeatureFlags
}

// NewFlagReceiver creates a receiver. applier is called on every flag update.
func NewFlagReceiver(cfg ReceiverConfig, applier FlagApplier, cp creds.Provider, logger zerolog.Logger) *FlagReceiver {
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = 30 * time.Second
	}
	if cfg.ReconnectDelay <= 0 {
		cfg.ReconnectDelay = 2 * time.Second
	}
	if cfg.ReconnectMaxDelay <= 0 {
		cfg.ReconnectMaxDelay = 60 * time.Second
	}

	return &FlagReceiver{
		cfg:     cfg,
		applier: applier,
		creds:   cp,
		logger:  logger.With().Str("component", "flag-receiver").Logger(),
	}
}

// CurrentFlags returns a copy of the most recently applied flags.
func (r *FlagReceiver) CurrentFlags() FeatureFlags {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.current
}

// Run connects to the cloud and processes messages until ctx is canceled.
// It automatically reconnects on disconnection with exponential backoff.
func (r *FlagReceiver) Run(ctx context.Context) {
	r.logger.Info().
		Str("url", r.cfg.URL).
		Dur("heartbeat", r.cfg.HeartbeatInterval).
		Msg("starting flag receiver")

	delay := r.cfg.ReconnectDelay

	for {
		err := r.connectAndListen(ctx)
		if ctx.Err() != nil {
			r.logger.Info().Msg("flag receiver stopped")
			return
		}

		if err != nil {
			r.logger.Warn().Err(err).
				Dur("retry_in", delay).
				Msg("WebSocket disconnected, will reconnect")
		} else {
			// Connected successfully at some point — reset backoff.
			delay = r.cfg.ReconnectDelay
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}

		// Exponential backoff, capped.
		delay *= 2
		if delay > r.cfg.ReconnectMaxDelay {
			delay = r.cfg.ReconnectMaxDelay
		}
	}
}

// connectAndListen dials the WebSocket, sends heartbeats, and reads messages.
// On handshake failure it attempts to refresh credentials and retries once.
func (r *FlagReceiver) connectAndListen(ctx context.Context) error {
	wsURL := r.buildURL()

	conn, err := r.dialWS(ctx, wsURL, r.creds.APIKey())
	if err != nil {
		// Try refreshing credentials on handshake failure and retry once.
		newKey, refreshErr := r.creds.RefreshOnUnauthorized()
		if refreshErr != nil {
			return fmt.Errorf("dial: %w (credential refresh also failed: %w)", err, refreshErr)
		}
		r.logger.Info().Msg("credentials refreshed, retrying WebSocket dial")
		conn, err = r.dialWS(ctx, wsURL, newKey)
		if err != nil {
			return fmt.Errorf("dial after refresh: %w", err)
		}
	}
	defer closeConn(conn, &r.logger)

	r.logger.Info().Msg("WebSocket connected")

	// Close the connection when context is canceled so ReadMessage unblocks.
	go func() {
		<-ctx.Done()
		closeConn(conn, &r.logger)
	}()

	// Start heartbeat sender in background.
	heartCtx, heartCancel := context.WithCancel(ctx)
	defer heartCancel()
	go r.heartbeatLoop(heartCtx, conn)

	// Read loop.
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return nil // clean shutdown
			}
			return fmt.Errorf("read: %w", err)
		}

		r.handleMessage(conn, raw)
	}
}

// handleMessage dispatches a received WebSocket message.
func (r *FlagReceiver) handleMessage(conn *websocket.Conn, raw []byte) {
	var msg wsMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		r.logger.Error().Err(err).Msg("failed to unmarshal WebSocket message")
		return
	}

	switch msg.Type {
	case "flags_update":
		r.handleFlagUpdate(msg.Flags)

	case "ping":
		pong := wsMessage{Type: "pong"}
		data, err := json.Marshal(pong)
		if err != nil {
			r.logger.Error().Err(err).Msg("failed to marshal pong")
			return
		}
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			r.logger.Error().Err(err).Msg("failed to send pong")
		}

	default:
		r.logger.Debug().Str("type", msg.Type).Msg("unhandled message type")
	}
}

// handleFlagUpdate parses and applies a flags_update message.
func (r *FlagReceiver) handleFlagUpdate(raw json.RawMessage) {
	if raw == nil {
		r.logger.Warn().Msg("flags_update with nil payload, ignoring")
		return
	}

	var flags FeatureFlags
	if err := json.Unmarshal(raw, &flags); err != nil {
		r.logger.Error().Err(err).Msg("failed to unmarshal feature flags")
		return
	}

	// Ignore empty/zero-value flag pushes — these happen when the
	// environment has no flags stored in the DB yet. Applying zeroes
	// would wipe the agent's current sampler/rate-limiter config.
	if flags.SamplingMode == "" && flags.SampleRate == 0 && flags.SlowThresholdMS == 0 {
		r.logger.Debug().Msg("ignoring empty flags_update (no meaningful values)")
		return
	}

	r.mu.Lock()
	r.current = flags
	r.mu.Unlock()

	r.logger.Info().
		Str("mode", flags.SamplingMode).
		Float64("rate", flags.SampleRate).
		Int("slow_threshold_ms", flags.SlowThresholdMS).
		Bool("collect_orm", flags.CollectORM).
		Bool("collect_errors", flags.CollectErrors).
		Bool("strip_pii", flags.StripPII).
		Int64("max_bytes_per_min", flags.MaxBytesPerMinute).
		Msg("feature flags updated")

	if r.applier != nil {
		r.applier(flags)
	}
}

// heartbeatLoop sends periodic heartbeats to keep the connection alive.
func (r *FlagReceiver) heartbeatLoop(ctx context.Context, conn *websocket.Conn) {
	ticker := time.NewTicker(r.cfg.HeartbeatInterval)
	defer ticker.Stop()

	// Send an initial heartbeat immediately on connect.
	r.sendHeartbeat(conn)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.sendHeartbeat(conn)
		}
	}
}

// sendHeartbeat writes a single heartbeat message.
func (r *FlagReceiver) sendHeartbeat(conn *websocket.Conn) {
	msg := heartbeatPayload{
		Type:         "heartbeat",
		AgentVersion: r.cfg.AgentVersion,
		Status:       "healthy",
	}
	data, err := json.Marshal(msg)
	if err != nil {
		r.logger.Error().Err(err).Msg("failed to marshal heartbeat")
		return
	}
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		r.logger.Error().Err(err).Msg("failed to send heartbeat")
	}
}

// dialWS attempts a single WebSocket dial with the given API key.
func (r *FlagReceiver) dialWS(ctx context.Context, wsURL, apiKey string) (*websocket.Conn, error) {
	header := http.Header{}
	header.Set("Authorization", "ApiKey "+apiKey)

	r.logger.Debug().Str("url", wsURL).Msg("connecting to cloud WebSocket")

	conn, resp, err := websocket.DefaultDialer.DialContext(ctx, wsURL, header)
	if resp != nil {
		defer resp.Body.Close()
	}
	return conn, err
}

// buildURL constructs the full WebSocket URL with agent_id query parameter.
func (r *FlagReceiver) buildURL() string {
	u := r.cfg.URL
	if r.cfg.AgentID == "" {
		return u
	}
	sep := "?"
	if u != "" {
		for _, c := range u {
			if c == '?' {
				sep = "&"
				break
			}
		}
	}
	return u + sep + "agent_id=" + r.cfg.AgentID
}

func closeConn(conn *websocket.Conn, logger *zerolog.Logger) {
	if err := conn.Close(); err != nil {
		logger.Warn().Err(err).Msg("failed to close WebSocket connection")
	}
}
