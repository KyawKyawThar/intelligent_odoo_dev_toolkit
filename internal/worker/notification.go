package worker

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"

	"github.com/rs/zerolog"
)

// NotificationConfig holds tuning knobs for the notification delivery worker.
type NotificationConfig struct {
	// PollInterval controls how often pending deliveries are polled (default: 30s).
	PollInterval time.Duration
	// RetryInterval controls how often failed deliveries are reset to pending (default: 5m).
	RetryInterval time.Duration
	// BatchSize is the max deliveries fetched per poll cycle (default: 100).
	BatchSize int32
	// MaxAttempts is the attempt ceiling after which a delivery stays failed (default: 3).
	MaxAttempts int32
}

// DefaultNotificationConfig returns sensible defaults.
func DefaultNotificationConfig() NotificationConfig {
	return NotificationConfig{
		PollInterval:  30 * time.Second,
		RetryInterval: 5 * time.Minute,
		BatchSize:     100,
		MaxAttempts:   3,
	}
}

// Dispatcher sends a notification for a specific channel type.
type Dispatcher interface {
	// ChannelType returns the channel type string this dispatcher handles.
	ChannelType() string
	// Send delivers the notification and returns a non-nil error on failure.
	Send(ctx context.Context, delivery db.ListPendingDeliveriesRow) error
}

// NotificationWorker polls alert_deliveries for pending rows and dispatches them
// to the appropriate channel (email, Slack, webhook). Failed deliveries are
// reset to pending on RetryInterval until MaxAttempts is exhausted.
type NotificationWorker struct {
	store       db.Store
	dispatchers map[string]Dispatcher
	config      NotificationConfig
	logger      zerolog.Logger
	httpClient  *http.Client
}

// NewNotificationWorker creates a NotificationWorker with the built-in set of
// dispatchers (email, slack, webhook). Pass extra dispatchers to extend.
func NewNotificationWorker(
	store db.Store,
	cfg NotificationConfig,
	logger zerolog.Logger,
	extra ...Dispatcher,
) *NotificationWorker {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 30 * time.Second
	}
	if cfg.RetryInterval <= 0 {
		cfg.RetryInterval = 5 * time.Minute
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}

	w := &NotificationWorker{
		store:       store,
		dispatchers: make(map[string]Dispatcher),
		config:      cfg,
		logger:      logger.With().Str("component", "notification-worker").Logger(),
		httpClient:  httpClient,
	}

	// Register built-in dispatchers.
	for _, d := range []Dispatcher{
		&slackDispatcher{client: httpClient},
		&webhookDispatcher{client: httpClient},
		&emailDispatcher{},
	} {
		w.dispatchers[d.ChannelType()] = d
	}

	// Extra dispatchers override built-ins when types match.
	for _, d := range extra {
		w.dispatchers[d.ChannelType()] = d
	}

	return w
}

// Run starts the delivery and retry loops. Blocks until ctx is canceled.
func (w *NotificationWorker) Run(ctx context.Context) error {
	w.logger.Info().
		Dur("poll_interval", w.config.PollInterval).
		Dur("retry_interval", w.config.RetryInterval).
		Int32("batch_size", w.config.BatchSize).
		Int32("max_attempts", w.config.MaxAttempts).
		Msg("notification worker starting")

	// Run once immediately.
	w.deliverPending(ctx)
	w.resetRetryable(ctx)

	pollTicker := time.NewTicker(w.config.PollInterval)
	retryTicker := time.NewTicker(w.config.RetryInterval)
	defer pollTicker.Stop()
	defer retryTicker.Stop()

	for {
		select {
		case <-pollTicker.C:
			w.deliverPending(ctx)
		case <-retryTicker.C:
			w.resetRetryable(ctx)
		case <-ctx.Done():
			w.logger.Info().Msg("notification worker stopped")
			return nil
		}
	}
}

// deliverPending fetches pending deliveries and dispatches each one.
func (w *NotificationWorker) deliverPending(ctx context.Context) {
	rows, err := w.store.ListPendingDeliveries(ctx, w.config.BatchSize)
	if err != nil {
		w.logger.Error().Err(err).Msg("notification: failed to list pending deliveries")
		return
	}

	if len(rows) == 0 {
		return
	}

	w.logger.Info().Int("count", len(rows)).Msg("notification: dispatching deliveries")

	for _, row := range rows {
		if ctx.Err() != nil {
			return
		}
		w.dispatch(ctx, row)
	}
}

// dispatch sends a single delivery to the correct channel dispatcher.
func (w *NotificationWorker) dispatch(ctx context.Context, row db.ListPendingDeliveriesRow) {
	log := w.logger.With().
		Str("delivery_id", row.ID.String()).
		Str("channel_type", row.ChannelType).
		Str("alert_type", row.AlertType).
		Logger()

	d, ok := w.dispatchers[row.ChannelType]
	if !ok {
		errMsg := fmt.Sprintf("unknown channel type: %s", row.ChannelType)
		log.Warn().Msg(errMsg)
		w.markFailed(ctx, row, errMsg)
		return
	}

	if err := d.Send(ctx, row); err != nil {
		log.Warn().Err(err).Msg("notification: delivery failed")
		w.markFailed(ctx, row, err.Error())
		return
	}

	if err := w.store.MarkDeliverySent(ctx, row.ID); err != nil {
		log.Error().Err(err).Msg("notification: failed to mark delivery sent")
		return
	}

	log.Info().Msg("notification: delivery sent")
}

// markFailed records a failed delivery attempt.
func (w *NotificationWorker) markFailed(ctx context.Context, row db.ListPendingDeliveriesRow, reason string) {
	if err := w.store.MarkDeliveryFailed(ctx, db.MarkDeliveryFailedParams{
		ID:    row.ID,
		Error: &reason,
	}); err != nil {
		w.logger.Error().Err(err).
			Str("delivery_id", row.ID.String()).
			Msg("notification: failed to mark delivery failed")
	}
}

// resetRetryable resets failed deliveries (below MaxAttempts) back to pending.
func (w *NotificationWorker) resetRetryable(ctx context.Context) {
	tag, err := w.store.RetryFailedDeliveries(ctx, w.config.MaxAttempts)
	if err != nil {
		w.logger.Error().Err(err).Msg("notification: failed to reset retryable deliveries")
		return
	}
	if tag.RowsAffected() > 0 {
		w.logger.Info().Int64("reset", tag.RowsAffected()).Msg("notification: reset failed deliveries to pending")
	}
}

// =============================================================================
// Channel dispatchers
// =============================================================================

// ── Slack ─────────────────────────────────────────────────────────────────────

type slackConfig struct {
	WebhookURL string `json:"webhook_url"`
}

type slackDispatcher struct {
	client *http.Client
}

func (d *slackDispatcher) ChannelType() string { return "slack" }

func (d *slackDispatcher) Send(ctx context.Context, row db.ListPendingDeliveriesRow) error {
	var cfg slackConfig
	if err := json.Unmarshal(row.ChannelConfig, &cfg); err != nil {
		return fmt.Errorf("invalid slack config: %w", err)
	}
	if cfg.WebhookURL == "" {
		return fmt.Errorf("slack: webhook_url is required")
	}

	text := fmt.Sprintf("[%s] *%s*: %s", strings.ToUpper(row.AlertSeverity), row.AlertType, row.AlertMessage)
	payload, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return fmt.Errorf("slack: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.WebhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("slack: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("slack: send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("slack: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// ── Webhook ───────────────────────────────────────────────────────────────────

type webhookConfig struct {
	URL     string            `json:"url"`
	Secret  string            `json:"secret"`  // HMAC-SHA256 signing secret (optional)
	Method  string            `json:"method"`  // POST by default
	Headers map[string]string `json:"headers"` // extra headers
}

type webhookDispatcher struct {
	client *http.Client
}

func (d *webhookDispatcher) ChannelType() string { return "webhook" }

func (d *webhookDispatcher) Send(ctx context.Context, row db.ListPendingDeliveriesRow) error {
	var cfg webhookConfig
	if err := json.Unmarshal(row.ChannelConfig, &cfg); err != nil {
		return fmt.Errorf("invalid webhook config: %w", err)
	}
	if cfg.URL == "" {
		return fmt.Errorf("webhook: url is required")
	}
	method := strings.ToUpper(cfg.Method)
	if method == "" {
		method = http.MethodPost
	}

	payload, err := json.Marshal(map[string]any{
		"delivery_id":    row.ID,
		"alert_type":     row.AlertType,
		"alert_severity": row.AlertSeverity,
		"alert_message":  row.AlertMessage,
		"alert_metadata": row.AlertMetadata,
		"sent_at":        time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("webhook: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, cfg.URL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("webhook: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// HMAC-SHA256 signature header when secret is configured.
	if cfg.Secret != "" {
		mac := hmac.New(sha256.New, []byte(cfg.Secret))
		mac.Write(payload)
		req.Header.Set("X-Signature-SHA256", hex.EncodeToString(mac.Sum(nil)))
	}

	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook: send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// ── Email ─────────────────────────────────────────────────────────────────────

type emailConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	To       string `json:"to"`
}

type emailDispatcher struct{}

func (d *emailDispatcher) ChannelType() string { return "email" }

func (d *emailDispatcher) Send(_ context.Context, row db.ListPendingDeliveriesRow) error {
	var cfg emailConfig
	if err := json.Unmarshal(row.ChannelConfig, &cfg); err != nil {
		return fmt.Errorf("invalid email config: %w", err)
	}
	if cfg.Host == "" || cfg.To == "" || cfg.From == "" {
		return fmt.Errorf("email: host, from and to are required")
	}
	if cfg.Port <= 0 {
		cfg.Port = 587
	}

	subject := fmt.Sprintf("[%s] OdooDevTools Alert: %s", strings.ToUpper(row.AlertSeverity), row.AlertType)
	body := fmt.Sprintf("Alert: %s\nSeverity: %s\nMessage: %s\n",
		row.AlertType, row.AlertSeverity, row.AlertMessage)

	msg := "From: " + cfg.From + "\r\n" +
		"To: " + cfg.To + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"\r\n" + body

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	var auth smtp.Auth
	if cfg.Username != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	}

	if err := smtp.SendMail(addr, auth, cfg.From, []string{cfg.To}, []byte(msg)); err != nil {
		return fmt.Errorf("email: send: %w", err)
	}
	return nil
}
