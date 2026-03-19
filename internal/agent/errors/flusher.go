package errors

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"Intelligent_Dev_ToolKit_Odoo/internal/agent/creds"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/ringbuf"

	"github.com/rs/zerolog"
)

// eventContext holds request-scoped metadata captured at the time of the error.
type eventContext struct {
	UID        int    `json:"uid,omitempty"`
	RequestURL string `json:"request_url,omitempty"`
}

// eventPayload mirrors the server's ErrorEventPayload JSON structure.
type eventPayload struct {
	Signature string        `json:"signature"`
	Type      string        `json:"type"`
	Message   string        `json:"message"`
	Module    string        `json:"module,omitempty"`
	Model     string        `json:"model,omitempty"`
	Timestamp time.Time     `json:"timestamp"`
	Traceback string        `json:"traceback,omitempty"`
	Context   *eventContext `json:"context,omitempty"`
}

// batchPayload is the body sent to POST /api/v1/agent/errors.
type batchPayload struct {
	EnvID          string         `json:"env_id"`
	Events         []eventPayload `json:"events"`
	SpikeThreshold int            `json:"spike_threshold,omitempty"`
}

// Flusher drains the ring buffer on a configurable interval and POSTs
// batches of error events to the central server.
type Flusher struct {
	buf            *ringbuf.RingBuffer[ErrorEvent]
	httpClient     *http.Client
	serverURL      string
	creds          creds.Provider
	envID          string
	spikeThreshold int // 0 = disabled
	logger         zerolog.Logger
}

// NewFlusher creates a Flusher.
// spikeThreshold controls when the server triggers a spike alert; 0 disables it.
func NewFlusher(
	buf *ringbuf.RingBuffer[ErrorEvent],
	serverURL string,
	cp creds.Provider,
	envID string,
	spikeThreshold int,
	logger zerolog.Logger,
) *Flusher {
	return &Flusher{
		buf:            buf,
		httpClient:     &http.Client{Timeout: 15 * time.Second},
		serverURL:      serverURL,
		creds:          cp,
		envID:          envID,
		spikeThreshold: spikeThreshold,
		logger:         logger.With().Str("component", "error-flusher").Logger(),
	}
}

// Flush drains the ring buffer and sends the batch to the server.
// Returns nil (no error) when the buffer is empty.
func (f *Flusher) Flush(ctx context.Context) error {
	events := f.buf.DrainAll()
	if len(events) == 0 {
		return nil
	}

	payload := batchPayload{
		EnvID:          f.envID,
		SpikeThreshold: f.spikeThreshold,
		Events:         make([]eventPayload, 0, len(events)),
	}
	for _, ev := range events {
		ep := eventPayload{
			Signature: ev.Signature,
			Type:      ev.ErrorType,
			Message:   ev.Message,
			Module:    ev.Module,
			Model:     ev.Model,
			Timestamp: ev.CapturedAt,
			Traceback: ev.Traceback,
		}
		if ev.UserID != 0 || ev.RequestURL != "" {
			ep.Context = &eventContext{
				UID:        ev.UserID,
				RequestURL: ev.RequestURL,
			}
		}
		payload.Events = append(payload.Events, ep)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal error batch: %w", err)
	}

	if err := f.post(ctx, body); err != nil {
		// Re-push events back into the buffer so they are not lost.
		for _, ev := range events {
			f.buf.Push(ev)
		}
		return fmt.Errorf("post error batch: %w", err)
	}

	f.logger.Info().
		Int("count", len(events)).
		Msg("error batch flushed to server")

	return nil
}

// RunLoop flushes on interval until ctx is canceled.
// A final flush is attempted on shutdown.
func (f *Flusher) RunLoop(ctx context.Context, interval time.Duration) {
	f.logger.Info().Dur("interval", interval).Msg("starting error batch flusher")

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Best-effort final flush.
			flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			if err := f.Flush(flushCtx); err != nil {
				f.logger.Error().Err(err).Msg("final flush failed")
			}
			cancel()
			f.logger.Info().Msg("error flusher stopped")
			return
		case <-ticker.C:
			if err := f.Flush(ctx); err != nil {
				f.logger.Error().Err(err).Msg("error flush failed")
			}
		}
	}
}

func (f *Flusher) post(ctx context.Context, body []byte) error {
	endpoint := f.serverURL + "/api/v1/agent/errors"

	status, err := f.doPost(ctx, endpoint, body, f.creds.APIKey())
	if err != nil {
		return err
	}

	// Auto-refresh on 401 and retry once.
	if status == http.StatusUnauthorized {
		newKey, refreshErr := f.creds.RefreshOnUnauthorized()
		if refreshErr != nil {
			f.logger.Error().Err(refreshErr).Msg("credential refresh failed")
			return fmt.Errorf("server returned 401 and refresh failed: %w", refreshErr)
		}
		f.logger.Info().Msg("credentials refreshed, retrying error flush")
		status, err = f.doPost(ctx, endpoint, body, newKey)
		if err != nil {
			return err
		}
	}

	if status < 200 || status >= 300 {
		return fmt.Errorf("server returned %d", status)
	}
	return nil
}

func (f *Flusher) doPost(ctx context.Context, endpoint string, body []byte, apiKey string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "ApiKey "+apiKey)

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		f.logger.Warn().Err(err).Msg("failed to discard response body")
	}
	return resp.StatusCode, nil
}
