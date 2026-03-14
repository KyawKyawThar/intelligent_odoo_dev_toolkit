package errors

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"Intelligent_Dev_ToolKit_Odoo/internal/agent/ringbuf"

	"github.com/rs/zerolog"
)

// eventPayload mirrors the server's ErrorEventPayload JSON structure.
type eventPayload struct {
	Signature   string    `json:"signature"`
	ErrorType   string    `json:"error_type"`
	Message     string    `json:"message"`
	Module      string    `json:"module,omitempty"`
	Model       string    `json:"model,omitempty"`
	Traceback   string    `json:"traceback,omitempty"`
	AffectedUID int       `json:"affected_uid,omitempty"`
	CapturedAt  time.Time `json:"captured_at"`
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
	apiKey         string
	envID          string
	spikeThreshold int // 0 = disabled
	logger         zerolog.Logger
}

// NewFlusher creates a Flusher.
// spikeThreshold controls when the server triggers a spike alert; 0 disables it.
func NewFlusher(
	buf *ringbuf.RingBuffer[ErrorEvent],
	serverURL, apiKey, envID string,
	spikeThreshold int,
	logger zerolog.Logger,
) *Flusher {
	return &Flusher{
		buf:            buf,
		httpClient:     &http.Client{Timeout: 15 * time.Second},
		serverURL:      serverURL,
		apiKey:         apiKey,
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
		payload.Events = append(payload.Events, eventPayload{
			Signature:   ev.Signature,
			ErrorType:   ev.ErrorType,
			Message:     ev.Message,
			Module:      ev.Module,
			Model:       ev.Model,
			Traceback:   ev.Traceback,
			AffectedUID: ev.UserID,
			CapturedAt:  ev.CapturedAt,
		})
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

// RunLoop flushes on interval until ctx is cancelled.
// A final flush is attempted on shutdown.
func (f *Flusher) RunLoop(ctx context.Context, interval time.Duration) {
	f.logger.Info().Dur("interval", interval).Msg("starting error batch flusher")

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Best-effort final flush.
			flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		f.serverURL+"/api/v1/agent/errors", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "ApiKey "+f.apiKey)

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return nil
}
