package hook

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	agenterrors "Intelligent_Dev_ToolKit_Odoo/internal/agent/errors"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/ringbuf"

	"github.com/rs/zerolog"
)

// writeLines is a test helper that appends lines to a file.
func writeLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, line := range lines {
		if _, err := f.WriteString(line + "\n"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLogTailer_CapturesErrors(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "odoo.log")

	// Write some initial content that should be skipped (tailer starts at end).
	writeLines(t, logPath,
		"2024-01-15 10:00:00,000 1 INFO db odoo.http: old request",
		"2024-01-15 10:00:01,000 1 ERROR db odoo.addons.sale.models.sale: old error",
	)

	buf := ringbuf.New[agenterrors.ErrorEvent](100)
	cfg := DefaultTailerConfig(logPath)
	cfg.PollInterval = 100 * time.Millisecond
	logger := zerolog.Nop()

	tailer := NewLogTailer(cfg, buf, nil, logger)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		tailer.Run(ctx)
		close(done)
	}()

	// Give the tailer time to open and seek.
	time.Sleep(200 * time.Millisecond)

	// Now write new error lines.
	writeLines(t, logPath,
		"2024-01-15 10:01:00,000 1 ERROR db odoo.addons.stock.models.picking: transfer failed",
		"Traceback (most recent call last):",
		"  File \"picking.py\", line 10, in do_transfer",
		"    raise UserError(\"no qty\")",
		"odoo.exceptions.UserError: no qty",
		// A non-error entry to flush the previous ERROR.
		"2024-01-15 10:01:01,000 1 INFO db odoo.http: next request",
	)

	// Wait for the tailer to process.
	time.Sleep(500 * time.Millisecond)

	cancel()
	<-done

	events := buf.DrainAll()
	if len(events) == 0 {
		t.Fatal("expected at least one error event from log tailer")
	}

	ev := events[0]
	if ev.Module != "stock" {
		t.Errorf("module = %q, want stock", ev.Module)
	}
	if ev.Signature == "" {
		t.Error("expected non-empty signature")
	}
	if ev.Traceback == "" {
		t.Error("expected traceback content")
	}
}

func TestLogTailer_SkipsNonErrors(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "odoo.log")

	// Create empty file.
	writeLines(t, logPath)

	buf := ringbuf.New[agenterrors.ErrorEvent](100)
	cfg := DefaultTailerConfig(logPath)
	cfg.PollInterval = 100 * time.Millisecond
	logger := zerolog.Nop()

	tailer := NewLogTailer(cfg, buf, nil, logger)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		tailer.Run(ctx)
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)

	// Write only INFO/WARNING lines.
	writeLines(t, logPath,
		"2024-01-15 10:01:00,000 1 INFO db odoo.http: GET /web",
		"2024-01-15 10:01:01,000 1 WARNING db odoo.http: slow request",
		"2024-01-15 10:01:02,000 1 DEBUG db odoo.sql_db: query done",
	)

	time.Sleep(500 * time.Millisecond)

	cancel()
	<-done

	events := buf.DrainAll()
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

func TestLogTailer_DetectsTruncation(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "odoo.log")

	// Write initial large content.
	writeLines(t, logPath,
		"2024-01-15 10:00:00,000 1 INFO db odoo.http: line 1",
		"2024-01-15 10:00:01,000 1 INFO db odoo.http: line 2",
		"2024-01-15 10:00:02,000 1 INFO db odoo.http: line 3",
	)

	buf := ringbuf.New[agenterrors.ErrorEvent](100)
	cfg := DefaultTailerConfig(logPath)
	cfg.PollInterval = 100 * time.Millisecond
	logger := zerolog.Nop()

	tailer := NewLogTailer(cfg, buf, nil, logger)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		tailer.Run(ctx)
		close(done)
	}()

	time.Sleep(300 * time.Millisecond)

	// Truncate the file (simulate logrotate copytruncate).
	if err := os.Truncate(logPath, 0); err != nil {
		t.Fatal(err)
	}

	time.Sleep(300 * time.Millisecond)

	// Write a new error after truncation.
	writeLines(t, logPath,
		"2024-01-15 11:00:00,000 1 ERROR db odoo.addons.sale.models.order: post-rotate error",
		"2024-01-15 11:00:01,000 1 INFO db odoo.http: flusher",
	)

	time.Sleep(500 * time.Millisecond)

	cancel()
	<-done

	events := buf.DrainAll()
	if len(events) == 0 {
		t.Fatal("expected error event after log rotation")
	}
}

func TestLogTailer_MissingFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "nonexistent.log")

	buf := ringbuf.New[agenterrors.ErrorEvent](100)
	cfg := DefaultTailerConfig(logPath)
	cfg.PollInterval = 100 * time.Millisecond
	logger := zerolog.Nop()

	tailer := NewLogTailer(cfg, buf, nil, logger)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		tailer.Run(ctx)
		close(done)
	}()

	// Let it retry a couple times with the file missing.
	time.Sleep(400 * time.Millisecond)

	// Create the empty file first so the tailer opens it and seeks to offset 0.
	writeLines(t, logPath)

	// Give the tailer time to open the file and reach its poll loop.
	time.Sleep(300 * time.Millisecond)

	// Now append an error after the tailer is already watching.
	writeLines(t, logPath,
		"2024-01-15 12:00:00,000 1 ERROR db odoo.addons.mrp.models.production: boom",
		"2024-01-15 12:00:01,000 1 INFO db odoo.http: flush",
	)

	time.Sleep(500 * time.Millisecond)

	cancel()
	<-done

	events := buf.DrainAll()
	if len(events) == 0 {
		t.Fatal("expected error event after file was created")
	}
}
