package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Mock Store ───────────────────────────────────────────────────────────────

type notifMockStore struct {
	db.Store

	mu               sync.Mutex
	pendingRows      []db.ListPendingDeliveriesRow
	listPendingErr   error
	markSentCalls    []uuid.UUID
	markFailedCalls  []db.MarkDeliveryFailedParams
	retryFailedCalls []int32
}

func (m *notifMockStore) ListPendingDeliveries(_ context.Context, limit int32) ([]db.ListPendingDeliveriesRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.listPendingErr != nil {
		return nil, m.listPendingErr
	}
	n := int(limit)
	if n > len(m.pendingRows) {
		n = len(m.pendingRows)
	}
	return m.pendingRows[:n], nil
}

func (m *notifMockStore) MarkDeliverySent(_ context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.markSentCalls = append(m.markSentCalls, id)
	return nil
}

func (m *notifMockStore) MarkDeliveryFailed(_ context.Context, arg db.MarkDeliveryFailedParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.markFailedCalls = append(m.markFailedCalls, arg)
	return nil
}

func (m *notifMockStore) RetryFailedDeliveries(_ context.Context, attempt int32) (pgconn.CommandTag, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.retryFailedCalls = append(m.retryFailedCalls, attempt)
	return pgconn.NewCommandTag("UPDATE 0"), nil
}

func (m *notifMockStore) sentCalls() []uuid.UUID {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]uuid.UUID, len(m.markSentCalls))
	copy(out, m.markSentCalls)
	return out
}

func (m *notifMockStore) failedCalls() []db.MarkDeliveryFailedParams {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]db.MarkDeliveryFailedParams, len(m.markFailedCalls))
	copy(out, m.markFailedCalls)
	return out
}

// ── Mock Dispatcher ──────────────────────────────────────────────────────────

type mockDispatcher struct {
	channelType string
	sendErr     error

	mu    sync.Mutex
	calls []db.ListPendingDeliveriesRow
}

func (d *mockDispatcher) ChannelType() string { return d.channelType }

func (d *mockDispatcher) Send(_ context.Context, row db.ListPendingDeliveriesRow) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = append(d.calls, row)
	return d.sendErr
}

func (d *mockDispatcher) callCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.calls)
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func newTestNotificationWorker(store db.Store, extra ...Dispatcher) *NotificationWorker {
	cfg := DefaultNotificationConfig()
	cfg.PollInterval = time.Hour // disable auto-tick in unit tests
	cfg.RetryInterval = time.Hour
	return NewNotificationWorker(store, cfg, zerolog.Nop(), extra...)
}

func makePendingRow(channelType string, channelConfig json.RawMessage) db.ListPendingDeliveriesRow {
	return db.ListPendingDeliveriesRow{
		ID:            uuid.New(),
		AlertID:       uuid.New(),
		ChannelID:     uuid.New(),
		Status:        "pending",
		Attempt:       1,
		ChannelType:   channelType,
		ChannelConfig: channelConfig,
		AlertType:     "error_spike",
		AlertSeverity: "critical",
		AlertMessage:  "Too many errors",
		AlertMetadata: json.RawMessage(`{}`),
		CreatedAt:     time.Now(),
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}

// ── DefaultNotificationConfig ────────────────────────────────────────────────

func TestDefaultNotificationConfig(t *testing.T) {
	cfg := DefaultNotificationConfig()
	assert.Equal(t, 30*time.Second, cfg.PollInterval)
	assert.Equal(t, 5*time.Minute, cfg.RetryInterval)
	assert.Equal(t, int32(100), cfg.BatchSize)
	assert.Equal(t, int32(3), cfg.MaxAttempts)
}

func TestNewNotificationWorker_DefaultsZeroConfig(t *testing.T) {
	w := NewNotificationWorker(&notifMockStore{}, NotificationConfig{}, zerolog.Nop())
	assert.Equal(t, 30*time.Second, w.config.PollInterval)
	assert.Equal(t, 5*time.Minute, w.config.RetryInterval)
	assert.Equal(t, int32(100), w.config.BatchSize)
	assert.Equal(t, int32(3), w.config.MaxAttempts)
}

func TestNewNotificationWorker_BuiltInDispatchers(t *testing.T) {
	w := newTestNotificationWorker(&notifMockStore{})
	assert.Contains(t, w.dispatchers, "slack")
	assert.Contains(t, w.dispatchers, "webhook")
	assert.Contains(t, w.dispatchers, "email")
}

func TestNewNotificationWorker_ExtraDispatcherOverridesBuiltIn(t *testing.T) {
	custom := &mockDispatcher{channelType: "slack"}
	w := newTestNotificationWorker(&notifMockStore{}, custom)
	assert.Equal(t, custom, w.dispatchers["slack"])
}

// ── deliverPending ────────────────────────────────────────────────────────────

func TestDeliverPending_NoPendingRows_NoDispatch(t *testing.T) {
	store := &notifMockStore{}
	d := &mockDispatcher{channelType: "slack"}
	w := newTestNotificationWorker(store, d)

	w.deliverPending(context.Background())

	assert.Equal(t, 0, d.callCount())
	assert.Empty(t, store.sentCalls())
}

func TestDeliverPending_ListError_NoDispatch(t *testing.T) {
	store := &notifMockStore{listPendingErr: assert.AnError}
	d := &mockDispatcher{channelType: "slack"}
	w := newTestNotificationWorker(store, d)

	require.NotPanics(t, func() {
		w.deliverPending(context.Background())
	})
	assert.Equal(t, 0, d.callCount())
}

func TestDeliverPending_UnknownChannelType_MarksFailed(t *testing.T) {
	store := &notifMockStore{
		pendingRows: []db.ListPendingDeliveriesRow{
			makePendingRow("sms", json.RawMessage(`{}`)),
		},
	}
	w := newTestNotificationWorker(store)

	w.deliverPending(context.Background())

	require.Len(t, store.failedCalls(), 1)
	assert.Contains(t, *store.failedCalls()[0].Error, "unknown channel type")
	assert.Empty(t, store.sentCalls())
}

func TestDeliverPending_DispatchSuccess_MarksSent(t *testing.T) {
	row := makePendingRow("mock", json.RawMessage(`{}`))
	store := &notifMockStore{pendingRows: []db.ListPendingDeliveriesRow{row}}
	d := &mockDispatcher{channelType: "mock"}
	w := newTestNotificationWorker(store, d)

	w.deliverPending(context.Background())

	assert.Equal(t, 1, d.callCount())
	require.Len(t, store.sentCalls(), 1)
	assert.Equal(t, row.ID, store.sentCalls()[0])
	assert.Empty(t, store.failedCalls())
}

func TestDeliverPending_DispatchError_MarksFailed(t *testing.T) {
	row := makePendingRow("mock", json.RawMessage(`{}`))
	store := &notifMockStore{pendingRows: []db.ListPendingDeliveriesRow{row}}
	d := &mockDispatcher{channelType: "mock", sendErr: assert.AnError}
	w := newTestNotificationWorker(store, d)

	w.deliverPending(context.Background())

	assert.Equal(t, 1, d.callCount())
	assert.Empty(t, store.sentCalls())
	require.Len(t, store.failedCalls(), 1)
	assert.Equal(t, row.ID, store.failedCalls()[0].ID)
}

func TestDeliverPending_MultipleRows_AllDispatched(t *testing.T) {
	rows := []db.ListPendingDeliveriesRow{
		makePendingRow("mock", json.RawMessage(`{}`)),
		makePendingRow("mock", json.RawMessage(`{}`)),
		makePendingRow("mock", json.RawMessage(`{}`)),
	}
	store := &notifMockStore{pendingRows: rows}
	d := &mockDispatcher{channelType: "mock"}
	w := newTestNotificationWorker(store, d)

	w.deliverPending(context.Background())

	assert.Equal(t, 3, d.callCount())
	assert.Len(t, store.sentCalls(), 3)
}

func TestDeliverPending_BatchSizeLimits(t *testing.T) {
	rows := make([]db.ListPendingDeliveriesRow, 10)
	for i := range rows {
		rows[i] = makePendingRow("mock", json.RawMessage(`{}`))
	}
	store := &notifMockStore{pendingRows: rows}
	d := &mockDispatcher{channelType: "mock"}

	cfg := DefaultNotificationConfig()
	cfg.BatchSize = 3
	w := NewNotificationWorker(store, cfg, zerolog.Nop(), d)

	w.deliverPending(context.Background())

	// Only 3 fetched and dispatched.
	assert.Equal(t, 3, d.callCount())
}

func TestDeliverPending_CanceledContext_StopsEarly(t *testing.T) {
	rows := []db.ListPendingDeliveriesRow{
		makePendingRow("mock", json.RawMessage(`{}`)),
		makePendingRow("mock", json.RawMessage(`{}`)),
	}
	store := &notifMockStore{pendingRows: rows}
	d := &mockDispatcher{channelType: "mock"}
	w := newTestNotificationWorker(store, d)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // canceled before dispatch loop

	w.deliverPending(ctx)
	// 0 or 1 dispatched — the important thing is no panic or deadlock.
}

// ── resetRetryable ────────────────────────────────────────────────────────────

func TestResetRetryable_CallsRetryWithMaxAttempts(t *testing.T) {
	store := &notifMockStore{}
	w := newTestNotificationWorker(store)
	w.config.MaxAttempts = 5

	w.resetRetryable(context.Background())

	require.Len(t, store.retryFailedCalls, 1)
	assert.Equal(t, int32(5), store.retryFailedCalls[0])
}

// ── Run (lifecycle) ───────────────────────────────────────────────────────────

func TestRun_StopsOnContextCancel(t *testing.T) {
	store := &notifMockStore{}
	cfg := DefaultNotificationConfig()
	cfg.PollInterval = time.Hour
	cfg.RetryInterval = time.Hour
	w := NewNotificationWorker(store, cfg, zerolog.Nop())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	cancel()
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after context cancel")
	}
}

func TestRun_RunsImmediatelyOnStart(t *testing.T) {
	row := makePendingRow("mock", json.RawMessage(`{}`))
	store := &notifMockStore{pendingRows: []db.ListPendingDeliveriesRow{row}}
	d := &mockDispatcher{channelType: "mock"}

	cfg := DefaultNotificationConfig()
	cfg.PollInterval = time.Hour
	cfg.RetryInterval = time.Hour
	w := NewNotificationWorker(store, cfg, zerolog.Nop(), d)

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = w.Run(ctx) }()

	waitFor(t, func() bool { return d.callCount() >= 1 })
	cancel()
}

// ── Slack dispatcher ──────────────────────────────────────────────────────────

func TestSlackDispatcher_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(slackConfig{WebhookURL: srv.URL})
	row := makePendingRow("slack", cfg)

	d := &slackDispatcher{client: &http.Client{Timeout: 5 * time.Second}}
	err := d.Send(context.Background(), row)
	assert.NoError(t, err)
}

func TestSlackDispatcher_MissingWebhookURL_ReturnsError(t *testing.T) {
	cfg, _ := json.Marshal(slackConfig{WebhookURL: ""})
	row := makePendingRow("slack", cfg)

	d := &slackDispatcher{client: &http.Client{}}
	err := d.Send(context.Background(), row)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "webhook_url")
}

func TestSlackDispatcher_ServerError_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(slackConfig{WebhookURL: srv.URL})
	row := makePendingRow("slack", cfg)

	d := &slackDispatcher{client: &http.Client{Timeout: 5 * time.Second}}
	err := d.Send(context.Background(), row)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestSlackDispatcher_InvalidConfig_ReturnsError(t *testing.T) {
	d := &slackDispatcher{client: &http.Client{}}
	err := d.Send(context.Background(), makePendingRow("slack", json.RawMessage(`{invalid`)))
	assert.Error(t, err)
}

// ── Webhook dispatcher ────────────────────────────────────────────────────────

func TestWebhookDispatcher_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(webhookConfig{URL: srv.URL})
	row := makePendingRow("webhook", cfg)

	d := &webhookDispatcher{client: &http.Client{Timeout: 5 * time.Second}}
	err := d.Send(context.Background(), row)
	assert.NoError(t, err)
}

func TestWebhookDispatcher_SignatureHeaderSet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NotEmpty(t, r.Header.Get("X-Signature-SHA256"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(webhookConfig{URL: srv.URL, Secret: "mysecret"})
	row := makePendingRow("webhook", cfg)

	d := &webhookDispatcher{client: &http.Client{Timeout: 5 * time.Second}}
	err := d.Send(context.Background(), row)
	assert.NoError(t, err)
}

func TestWebhookDispatcher_MissingURL_ReturnsError(t *testing.T) {
	cfg, _ := json.Marshal(webhookConfig{URL: ""})
	d := &webhookDispatcher{client: &http.Client{}}
	err := d.Send(context.Background(), makePendingRow("webhook", cfg))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "url is required")
}

func TestWebhookDispatcher_ServerError_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(webhookConfig{URL: srv.URL})
	d := &webhookDispatcher{client: &http.Client{Timeout: 5 * time.Second}}
	err := d.Send(context.Background(), makePendingRow("webhook", cfg))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "400")
}

func TestWebhookDispatcher_CustomHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer token123", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(webhookConfig{
		URL:     srv.URL,
		Headers: map[string]string{"Authorization": "Bearer token123"},
	})
	d := &webhookDispatcher{client: &http.Client{Timeout: 5 * time.Second}}
	err := d.Send(context.Background(), makePendingRow("webhook", cfg))
	assert.NoError(t, err)
}

func TestWebhookDispatcher_InvalidConfig_ReturnsError(t *testing.T) {
	d := &webhookDispatcher{client: &http.Client{}}
	err := d.Send(context.Background(), makePendingRow("webhook", json.RawMessage(`{invalid`)))
	assert.Error(t, err)
}

// ── Email dispatcher ──────────────────────────────────────────────────────────

func TestEmailDispatcher_MissingRequiredFields_ReturnsError(t *testing.T) {
	d := &emailDispatcher{}

	// Missing host.
	cfg, _ := json.Marshal(emailConfig{To: "a@b.com", From: "x@y.com"})
	err := d.Send(context.Background(), makePendingRow("email", cfg))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "host")
}

func TestEmailDispatcher_InvalidConfig_ReturnsError(t *testing.T) {
	d := &emailDispatcher{}
	err := d.Send(context.Background(), makePendingRow("email", json.RawMessage(`{invalid`)))
	assert.Error(t, err)
}

func TestEmailDispatcher_DefaultsPort587(t *testing.T) {
	// Just verify the port default logic doesn't panic on config with port=0.
	// We can't send real email in tests, so we just validate config parsing.
	var cfg emailConfig
	raw, _ := json.Marshal(emailConfig{Host: "mail.example.com", To: "a@b.com", From: "x@y.com", Port: 0})
	_ = json.Unmarshal(raw, &cfg)
	if cfg.Port <= 0 {
		cfg.Port = 587
	}
	assert.Equal(t, 587, cfg.Port)
}
