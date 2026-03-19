package transport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"Intelligent_Dev_ToolKit_Odoo/internal/agent/aggregator"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/creds"

	"github.com/rs/zerolog"
)

// SenderConfig controls the batch sender's behavior.
type SenderConfig struct {
	// ServerURL is the HTTP base URL of the cloud server (e.g. http://localhost:8080).
	ServerURL string

	// Endpoint is the path to POST batches to (default: /api/v1/agent/batch).
	Endpoint string

	// Timeout is the HTTP request timeout (default: 30s).
	Timeout time.Duration

	// MaxRetries is how many times to retry a failed POST before dropping
	// the batch. Default: 2 (total 3 attempts).
	MaxRetries int

	// RetryDelay is the base delay between retries (doubles each attempt).
	// Default: 1s.
	RetryDelay time.Duration
}

// DefaultSenderConfig returns sensible defaults.
func DefaultSenderConfig(serverURL string) SenderConfig {
	return SenderConfig{
		ServerURL:  serverURL,
		Endpoint:   "/api/v1/agent/batch",
		Timeout:    30 * time.Second,
		MaxRetries: 2,
		RetryDelay: 1 * time.Second,
	}
}

// Sender reads AggregatedBatches from a channel, compresses them with gzip,
// and POSTs them to the cloud server. It runs as a goroutine and stops when
// ctx is canceled or the channel is closed.
type Sender struct {
	config      SenderConfig
	sendCh      <-chan *aggregator.AggregatedBatch
	rateLimiter *RateLimiter // nil = no rate limiting
	creds       creds.Provider
	client      *http.Client
	logger      zerolog.Logger
}

// NewSender creates a Sender that reads batches from sendCh.
// rateLimiter may be nil — when nil, no rate limiting is applied.
func NewSender(
	cfg SenderConfig,
	sendCh <-chan *aggregator.AggregatedBatch,
	rateLimiter *RateLimiter,
	cp creds.Provider,
	logger zerolog.Logger,
) *Sender {
	return &Sender{
		config:      cfg,
		sendCh:      sendCh,
		rateLimiter: rateLimiter,
		creds:       cp,
		client:      &http.Client{Timeout: cfg.Timeout},
		logger:      logger.With().Str("component", "transport").Logger(),
	}
}

// Run reads batches from the channel, compresses and sends them.
// It blocks until ctx is canceled or the channel is closed.
func (s *Sender) Run(ctx context.Context) {
	s.logger.Info().
		Str("endpoint", s.config.ServerURL+s.config.Endpoint).
		Msg("batch sender started")

	for {
		select {
		case batch, ok := <-s.sendCh:
			if !ok {
				s.logger.Info().Msg("send channel closed, sender stopping")
				return
			}
			s.sendBatch(ctx, batch)

		case <-ctx.Done():
			// Drain any remaining batches in the channel.
			for {
				select {
				case batch, ok := <-s.sendCh:
					if !ok {
						s.logger.Info().Msg("sender stopped")
						return
					}
					drainCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
					s.sendBatch(drainCtx, batch)
					cancel()
				default:
					s.logger.Info().Msg("sender stopped")
					return
				}
			}
		}
	}
}

// SendBatch compresses and sends a single batch. Exported for direct use
// (e.g. in tests or one-off sends). Returns the CompressResult on success.
func (s *Sender) SendBatch(ctx context.Context, batch *aggregator.AggregatedBatch) (CompressResult, error) {
	cr, err := CompressBatch(batch)
	if err != nil {
		return cr, fmt.Errorf("compress: %w", err)
	}

	if err := s.post(ctx, cr.Data); err != nil {
		return cr, err
	}
	return cr, nil
}

// ── internal ────────────────────────────────────────────────────────────────

func (s *Sender) sendBatch(ctx context.Context, batch *aggregator.AggregatedBatch) {
	cr, err := CompressBatch(batch)
	if err != nil {
		s.logger.Error().Err(err).Msg("failed to compress batch")
		return
	}

	// Rate limit check — drop the batch if over budget.
	if s.rateLimiter != nil && !s.rateLimiter.Allow(int64(cr.CompressedSize)) {
		stats := s.rateLimiter.Stats()
		s.logger.Warn().
			Int("compressed_bytes", cr.CompressedSize).
			Int64("bytes_used", stats.BytesUsed).
			Int64("bytes_limit", stats.BytesLimit).
			Int("batches_used", stats.BatchesUsed).
			Int("batch_limit", stats.BatchLimit).
			Msg("batch dropped (rate limit exceeded)")
		return
	}

	var lastErr error
	maxAttempts := 1 + s.config.MaxRetries
	delay := s.config.RetryDelay

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := s.post(ctx, cr.Data); err != nil {
			lastErr = err
			s.logger.Warn().
				Err(err).
				Int("attempt", attempt).
				Int("max", maxAttempts).
				Msg("batch send failed")

			if attempt < maxAttempts {
				select {
				case <-ctx.Done():
					s.logger.Error().Err(lastErr).Msg("batch dropped (context canceled)")
					return
				case <-time.After(delay):
					delay *= 2 // exponential backoff
				}
			}
			continue
		}

		// Success.
		s.logger.Info().
			Int("original_bytes", cr.OriginalSize).
			Int("compressed_bytes", cr.CompressedSize).
			Float64("ratio", cr.Ratio()).
			Int("orm_stats", len(batch.ORMStats)).
			Int("raw_events", len(batch.RawEvents)).
			Int("total_queries", batch.Summary.TotalQueries).
			Msg("batch sent")
		return
	}

	s.logger.Error().Err(lastErr).Msg("batch dropped after max retries")
}

func (s *Sender) post(ctx context.Context, compressedBody []byte) error {
	endpoint := s.config.ServerURL + s.config.Endpoint

	status, err := s.doPost(ctx, endpoint, compressedBody, s.creds.APIKey())
	if err != nil {
		return err
	}

	// Auto-refresh on 401 and retry once.
	if status == http.StatusUnauthorized {
		newKey, refreshErr := s.creds.RefreshOnUnauthorized()
		if refreshErr != nil {
			s.logger.Error().Err(refreshErr).Msg("credential refresh failed")
			return fmt.Errorf("server returned 401 and refresh failed: %w", refreshErr)
		}
		s.logger.Info().Msg("credentials refreshed, retrying batch post")
		status, err = s.doPost(ctx, endpoint, compressedBody, newKey)
		if err != nil {
			return err
		}
	}

	if status < 200 || status >= 300 {
		return fmt.Errorf("server returned %d", status)
	}
	return nil
}

func (s *Sender) doPost(ctx context.Context, endpoint string, compressedBody []byte, apiKey string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(compressedBody))
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Authorization", "ApiKey "+apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	// Drain body so the connection can be reused.
	_, _ = io.Copy(io.Discard, resp.Body) //nolint:errcheck // drain body for connection reuse

	return resp.StatusCode, nil
}
