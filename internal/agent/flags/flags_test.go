package flags

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"Intelligent_Dev_ToolKit_Odoo/internal/agent/creds"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/sampler"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

// ── FeatureFlags ────────────────────────────────────────────────────────────

func TestFeatureFlags_ToSamplerConfig(t *testing.T) {
	flags := FeatureFlags{
		SamplingMode:    "sampled",
		SampleRate:      0.25,
		SlowThresholdMS: 100,
	}

	cfg := flags.ToSamplerConfig()

	if cfg.Mode != sampler.ModeSampled {
		t.Errorf("Mode = %q, want sampled", cfg.Mode)
	}
	if cfg.SampleRate != 0.25 {
		t.Errorf("SampleRate = %f, want 0.25", cfg.SampleRate)
	}
	if cfg.SlowThresholdMS != 100 {
		t.Errorf("SlowThresholdMS = %d, want 100", cfg.SlowThresholdMS)
	}
	if len(cfg.AlwaysCapture) != 3 {
		t.Errorf("AlwaysCapture = %v, want [error slow_query n1]", cfg.AlwaysCapture)
	}
}

func TestFeatureFlags_ToSamplerConfig_FullMode(t *testing.T) {
	flags := FeatureFlags{
		SamplingMode:    "full",
		SampleRate:      1.0,
		SlowThresholdMS: 50,
	}

	cfg := flags.ToSamplerConfig()

	if cfg.Mode != sampler.ModeFull {
		t.Errorf("Mode = %q, want full", cfg.Mode)
	}
	// Full mode should not force always-capture categories.
	if len(cfg.AlwaysCapture) != 0 {
		t.Errorf("AlwaysCapture should be empty for full mode, got %v", cfg.AlwaysCapture)
	}
}

// ── wsMessage parsing ───────────────────────────────────────────────────────

func TestWsMessage_FlagsUpdate(t *testing.T) {
	raw := `{"type":"flags_update","flags":{"sampling_mode":"sampled","sample_rate":0.1,"slow_threshold_ms":200}}`

	var msg wsMessage
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatal(err)
	}

	if msg.Type != "flags_update" {
		t.Errorf("Type = %q", msg.Type)
	}

	var flags FeatureFlags
	if err := json.Unmarshal(msg.Flags, &flags); err != nil {
		t.Fatal(err)
	}

	if flags.SamplingMode != "sampled" {
		t.Errorf("SamplingMode = %q", flags.SamplingMode)
	}
	if flags.SampleRate != 0.1 {
		t.Errorf("SampleRate = %f", flags.SampleRate)
	}
}

func TestWsMessage_Ping(t *testing.T) {
	raw := `{"type":"ping"}`
	var msg wsMessage
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Type != "ping" {
		t.Errorf("Type = %q, want ping", msg.Type)
	}
}

// ── buildURL ────────────────────────────────────────────────────────────────

func TestBuildURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		agentID string
		want    string
	}{
		{
			"simple",
			"ws://server:8080/api/v1/agent/ws",
			"agent-01",
			"ws://server:8080/api/v1/agent/ws?agent_id=agent-01",
		},
		{
			"existing query",
			"ws://server:8080/api/v1/agent/ws?token=abc",
			"agent-02",
			"ws://server:8080/api/v1/agent/ws?token=abc&agent_id=agent-02",
		},
		{
			"empty agent ID",
			"ws://server:8080/api/v1/agent/ws",
			"",
			"ws://server:8080/api/v1/agent/ws",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &FlagReceiver{cfg: ReceiverConfig{URL: tt.url, AgentID: tt.agentID}}
			got := r.buildURL()
			if got != tt.want {
				t.Errorf("buildURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ── Integration: WebSocket flag push ────────────────────────────────────────

// testWSServer creates an httptest server that upgrades to WebSocket,
// sends a flags_update, and reads the heartbeat.
func testWSServer(t *testing.T, flagsJSON string) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("upgrade error: %v", err)
			return
		}
		defer conn.Close()

		// Send flag update.
		msg := `{"type":"flags_update","flags":` + flagsJSON + `}`
		if err := conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
			return
		}

		// Read heartbeat (agent sends one immediately on connect).
		_, _, _ = conn.ReadMessage()

		// Send ping.
		ping := `{"type":"ping"}`
		if err := conn.WriteMessage(websocket.TextMessage, []byte(ping)); err != nil {
			return
		}

		// Read pong.
		_, _, _ = conn.ReadMessage()

		// Keep connection open until test closes it.
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}))
}

func TestFlagReceiver_ReceivesFlagsFromServer(t *testing.T) {
	flagsJSON := `{
		"sampling_mode":"aggregated_only",
		"sample_rate":0.05,
		"slow_threshold_ms":200,
		"collect_orm":true,
		"collect_sql":false,
		"collect_errors":true,
		"collect_profiler":false,
		"max_bytes_per_minute":524288,
		"strip_pii":true
	}`

	server := testWSServer(t, flagsJSON)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	var mu sync.Mutex
	var received *FeatureFlags

	applier := func(flags FeatureFlags) {
		mu.Lock()
		received = &flags
		mu.Unlock()
	}

	cfg := DefaultReceiverConfig(wsURL, "agent-01")
	cfg.HeartbeatInterval = 100 * time.Millisecond
	logger := zerolog.Nop()

	receiver := NewFlagReceiver(cfg, applier, &creds.Stub{Key: "test-key"}, logger)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		receiver.Run(ctx)
		close(done)
	}()

	// Wait for flags to be received and applied.
	deadline := time.After(3 * time.Second)
	for {
		mu.Lock()
		got := received
		mu.Unlock()
		if got != nil {
			break
		}
		select {
		case <-deadline:
			cancel()
			<-done
			t.Fatal("timed out waiting for flags")
		case <-time.After(50 * time.Millisecond):
		}
	}

	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()

	if received.SamplingMode != "aggregated_only" {
		t.Errorf("SamplingMode = %q", received.SamplingMode)
	}
	if received.SampleRate != 0.05 {
		t.Errorf("SampleRate = %f", received.SampleRate)
	}
	if received.SlowThresholdMS != 200 {
		t.Errorf("SlowThresholdMS = %d", received.SlowThresholdMS)
	}
	if !received.CollectORM {
		t.Error("CollectORM should be true")
	}
	if received.CollectSQL {
		t.Error("CollectSQL should be false")
	}
	if !received.StripPII {
		t.Error("StripPII should be true")
	}
	if received.MaxBytesPerMinute != 524288 {
		t.Errorf("MaxBytesPerMinute = %d", received.MaxBytesPerMinute)
	}

	// Also verify CurrentFlags() returns the same data.
	current := receiver.CurrentFlags()
	if current.SamplingMode != "aggregated_only" {
		t.Errorf("CurrentFlags().SamplingMode = %q", current.SamplingMode)
	}
}

func TestFlagReceiver_ApplierUpdatessampler(t *testing.T) {
	flagsJSON := `{"sampling_mode":"sampled","sample_rate":0.3,"slow_threshold_ms":150}`

	server := testWSServer(t, flagsJSON)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	smp := sampler.New(sampler.DefaultDevelopment())

	applier := func(flags FeatureFlags) {
		smp.UpdateConfig(flags.ToSamplerConfig())
	}

	cfg := DefaultReceiverConfig(wsURL, "agent-01")
	cfg.HeartbeatInterval = 100 * time.Millisecond
	logger := zerolog.Nop()

	receiver := NewFlagReceiver(cfg, applier, &creds.Stub{Key: "test-key"}, logger)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		receiver.Run(ctx)
		close(done)
	}()

	// Wait for the sampler to be updated.
	deadline := time.After(3 * time.Second)
	for {
		sc := smp.CurrentConfig()
		if sc.Mode == sampler.ModeSampled {
			break
		}
		select {
		case <-deadline:
			cancel()
			<-done
			t.Fatal("timed out waiting for sampler update")
		case <-time.After(50 * time.Millisecond):
		}
	}

	cancel()
	<-done

	sc := smp.CurrentConfig()
	if sc.Mode != sampler.ModeSampled {
		t.Errorf("sampler mode = %q, want sampled", sc.Mode)
	}
	if sc.SampleRate != 0.3 {
		t.Errorf("sampler rate = %f, want 0.3", sc.SampleRate)
	}
	if sc.SlowThresholdMS != 150 {
		t.Errorf("sampler threshold = %d, want 150", sc.SlowThresholdMS)
	}
}

func TestFlagReceiver_Reconnects(t *testing.T) {
	// Start a server that immediately closes the connection.
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	connectCount := 0
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		mu.Lock()
		connectCount++
		mu.Unlock()
		conn.Close() // immediately close
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	cfg := DefaultReceiverConfig(wsURL, "agent")
	cfg.ReconnectDelay = 50 * time.Millisecond
	cfg.ReconnectMaxDelay = 200 * time.Millisecond
	logger := zerolog.Nop()

	receiver := NewFlagReceiver(cfg, nil, &creds.Stub{Key: "test-key"}, logger)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		receiver.Run(ctx)
		close(done)
	}()

	// Let it reconnect a few times.
	time.Sleep(600 * time.Millisecond)
	cancel()
	<-done

	mu.Lock()
	count := connectCount
	mu.Unlock()

	if count < 2 {
		t.Errorf("expected at least 2 connection attempts, got %d", count)
	}
}
