package transport

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"Intelligent_Dev_ToolKit_Odoo/internal/agent/aggregator"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/creds"

	"github.com/rs/zerolog"
)

// ── Sender.SendBatch ────────────────────────────────────────────────────────

func TestSenderSendBatch_PostsGzipJSON(t *testing.T) {
	var received aggregator.AggregatedBatch
	var gotEncoding string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotEncoding = r.Header.Get("Content-Encoding")

		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			t.Errorf("gzip reader: %v", err)
			http.Error(w, "bad gzip", 400)
			return
		}
		defer gz.Close()
		body, _ := io.ReadAll(gz)
		json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := DefaultSenderConfig(srv.URL)
	sendCh := make(chan *aggregator.AggregatedBatch, 1)
	sender := NewSender(cfg, sendCh, nil, &creds.Stub{Key: "test-key"}, zerolog.Nop())

	batch := sampleBatch()
	cr, err := sender.SendBatch(context.Background(), batch)
	if err != nil {
		t.Fatalf("SendBatch: %v", err)
	}

	if gotEncoding != "gzip" {
		t.Errorf("Content-Encoding = %q, want gzip", gotEncoding)
	}
	if received.EnvID != batch.EnvID {
		t.Errorf("received EnvID = %q, want %q", received.EnvID, batch.EnvID)
	}
	if received.Summary.TotalQueries != batch.Summary.TotalQueries {
		t.Errorf("TotalQueries = %d, want %d", received.Summary.TotalQueries, batch.Summary.TotalQueries)
	}
	if cr.CompressedSize == 0 {
		t.Error("CompressResult.CompressedSize should not be 0")
	}
}

func TestSenderSendBatch_SetsAuthHeader(t *testing.T) {
	var gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := DefaultSenderConfig(srv.URL)
	sendCh := make(chan *aggregator.AggregatedBatch, 1)
	sender := NewSender(cfg, sendCh, nil, &creds.Stub{Key: "odt_ak_test123"}, zerolog.Nop())

	sender.SendBatch(context.Background(), sampleBatch())

	if gotAuth != "ApiKey odt_ak_test123" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "ApiKey odt_ak_test123")
	}
}

func TestSenderSendBatch_ServerError_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cfg := DefaultSenderConfig(srv.URL)
	sendCh := make(chan *aggregator.AggregatedBatch, 1)
	sender := NewSender(cfg, sendCh, nil, &creds.Stub{Key: "test-key"}, zerolog.Nop())

	_, err := sender.SendBatch(context.Background(), sampleBatch())
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

// ── Sender.Run ──────────────────────────────────────────────────────────────

func TestSenderRun_ConsumesFromChannel(t *testing.T) {
	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sendCh := make(chan *aggregator.AggregatedBatch, 4)
	cfg := DefaultSenderConfig(srv.URL)
	cfg.MaxRetries = 0
	sender := NewSender(cfg, sendCh, nil, &creds.Stub{Key: "test-key"}, zerolog.Nop())

	ctx, cancel := context.WithCancel(context.Background())
	go sender.Run(ctx)

	// Send 3 batches.
	for i := 0; i < 3; i++ {
		sendCh <- sampleBatch()
	}

	// Give sender time to process.
	time.Sleep(200 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)

	got := callCount.Load()
	if got != 3 {
		t.Errorf("server received %d batches, want 3", got)
	}
}

func TestSenderRun_RetriesOnFailure(t *testing.T) {
	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sendCh := make(chan *aggregator.AggregatedBatch, 1)
	cfg := DefaultSenderConfig(srv.URL)
	cfg.MaxRetries = 3
	cfg.RetryDelay = 10 * time.Millisecond // fast for tests
	sender := NewSender(cfg, sendCh, nil, &creds.Stub{Key: "test-key"}, zerolog.Nop())

	ctx, cancel := context.WithCancel(context.Background())
	go sender.Run(ctx)

	sendCh <- sampleBatch()

	// Wait for retries.
	time.Sleep(500 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)

	got := callCount.Load()
	if got < 3 {
		t.Errorf("expected at least 3 attempts (2 failures + 1 success), got %d", got)
	}
}

func TestSenderRun_DrainOnShutdown(t *testing.T) {
	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sendCh := make(chan *aggregator.AggregatedBatch, 4)
	cfg := DefaultSenderConfig(srv.URL)
	cfg.MaxRetries = 0
	sender := NewSender(cfg, sendCh, nil, &creds.Stub{Key: "test-key"}, zerolog.Nop())

	// Pre-fill channel then cancel — the sender should drain remaining.
	sendCh <- sampleBatch()
	sendCh <- sampleBatch()

	ctx, cancel := context.WithCancel(context.Background())
	go sender.Run(ctx)

	time.Sleep(100 * time.Millisecond)
	cancel()
	time.Sleep(200 * time.Millisecond)

	got := callCount.Load()
	if got != 2 {
		t.Errorf("expected 2 batches drained on shutdown, got %d", got)
	}
}

// ── Rate limiting integration ───────────────────────────────────────────────

func TestSenderRun_RateLimitDropsBatches(t *testing.T) {
	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Allow only 1 batch per minute.
	rl := NewRateLimiter(RateLimiterConfig{
		MaxBytesPerMinute:   0, // unlimited bytes
		MaxBatchesPerMinute: 1,
	})
	defer rl.Stop()

	sendCh := make(chan *aggregator.AggregatedBatch, 8)
	cfg := DefaultSenderConfig(srv.URL)
	cfg.MaxRetries = 0
	sender := NewSender(cfg, sendCh, rl, &creds.Stub{Key: "test-key"}, zerolog.Nop())

	ctx, cancel := context.WithCancel(context.Background())
	go sender.Run(ctx)

	// Send 5 batches — only the first should go through.
	for i := 0; i < 5; i++ {
		sendCh <- sampleBatch()
	}

	time.Sleep(300 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)

	got := callCount.Load()
	if got != 1 {
		t.Errorf("server received %d batches, want 1 (rate limited)", got)
	}
}

func TestSenderRun_RateLimitByBytes(t *testing.T) {
	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// sampleBatch compresses to ~474 bytes. Allow 600 bytes/min — only 1 batch fits.
	rl := NewRateLimiter(RateLimiterConfig{
		MaxBytesPerMinute:   600,
		MaxBatchesPerMinute: 0, // unlimited batches
	})
	defer rl.Stop()

	sendCh := make(chan *aggregator.AggregatedBatch, 4)
	cfg := DefaultSenderConfig(srv.URL)
	cfg.MaxRetries = 0
	sender := NewSender(cfg, sendCh, rl, &creds.Stub{Key: "test-key"}, zerolog.Nop())

	ctx, cancel := context.WithCancel(context.Background())
	go sender.Run(ctx)

	sendCh <- sampleBatch()
	sendCh <- sampleBatch()
	sendCh <- sampleBatch()

	time.Sleep(300 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)

	got := callCount.Load()
	if got != 1 {
		t.Errorf("server received %d batches, want 1 (bytes rate limited)", got)
	}
}
