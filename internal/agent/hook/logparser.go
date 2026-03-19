package hook

import (
	"bufio"
	"context"
	"io"
	"os"
	"time"

	agenterrors "Intelligent_Dev_ToolKit_Odoo/internal/agent/errors"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/ringbuf"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/sampler"

	"github.com/rs/zerolog"
)

// TailerConfig controls how the log tailer operates.
type TailerConfig struct {
	// Path to the Odoo server log file.
	Path string

	// PollInterval is how often we check for new data. Default: 1s.
	PollInterval time.Duration

	// MaxLineLength caps the length of a single line to prevent memory
	// exhaustion on malformed logs. Default: 1 MB.
	MaxLineLength int
}

// DefaultTailerConfig returns sensible defaults.
func DefaultTailerConfig(path string) TailerConfig {
	return TailerConfig{
		Path:          path,
		PollInterval:  1 * time.Second,
		MaxLineLength: 1 << 20, // 1 MB
	}
}

// LogTailer tails the Odoo server log file, parses multi-line entries, and
// pushes ERROR/CRITICAL events into the shared ring buffer.
//
// It handles:
//   - Starting from the end of the file (no replay of old entries)
//   - Log rotation via truncation detection (file shrinks → re-read from start)
//   - Log rotation via rename/recreate (file disappears → reopen)
type LogTailer struct {
	cfg     TailerConfig
	buf     *ringbuf.RingBuffer[agenterrors.ErrorEvent]
	sampler *sampler.Sampler // nil = keep all
	logger  zerolog.Logger

	// stats
	linesRead  int64
	errorsSent int64
}

// NewLogTailer creates a new tailer. sampler may be nil (keep all errors).
func NewLogTailer(
	cfg TailerConfig,
	buf *ringbuf.RingBuffer[agenterrors.ErrorEvent],
	smp *sampler.Sampler,
	logger zerolog.Logger,
) *LogTailer {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 1 * time.Second
	}
	if cfg.MaxLineLength <= 0 {
		cfg.MaxLineLength = 1 << 20
	}

	return &LogTailer{
		cfg:     cfg,
		buf:     buf,
		sampler: smp,
		logger:  logger.With().Str("component", "log-tailer").Logger(),
	}
}

// Run blocks until ctx is canceled. It opens the log file, seeks to the end,
// and continuously reads new lines.
func (t *LogTailer) Run(ctx context.Context) {
	t.logger.Info().
		Str("path", t.cfg.Path).
		Dur("poll_interval", t.cfg.PollInterval).
		Msg("starting log tailer")

	for {
		// Try to open the file. If it doesn't exist yet (Odoo hasn't started
		// logging), wait and retry.
		err := t.tailFile(ctx)
		if err != nil && ctx.Err() == nil {
			t.logger.Warn().Err(err).Msg("log tailer error, will retry")
		}

		select {
		case <-ctx.Done():
			t.logger.Info().
				Int64("lines_read", t.linesRead).
				Int64("errors_sent", t.errorsSent).
				Msg("log tailer stopped")
			return
		case <-time.After(t.cfg.PollInterval):
		}
	}
}

// tailFile opens the log, seeks to end, and reads until error or ctx cancel.
func (t *LogTailer) tailFile(ctx context.Context) error {
	f, err := os.Open(t.cfg.Path)
	if err != nil {
		return err
	}
	defer closeFile(f, &t.logger)

	// Seek to end — we only care about new entries.
	lastSize, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}

	t.logger.Debug().Int64("offset", lastSize).Msg("seeked to end of log file")

	acc := &EntryAccumulator{}
	reader := bufio.NewReaderSize(f, 64*1024)
	ticker := time.NewTicker(t.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.flushAcc(acc)
			return ctx.Err()

		case <-ticker.C:
			reopened, err := t.checkRotation(f, lastSize, acc)
			if err != nil {
				return err
			}
			if reopened {
				return nil
			}

			// Read all available complete lines.
			t.readLines(reader, acc)

			// Update tracking size for next rotation check.
			if pos, err := f.Seek(0, io.SeekCurrent); err == nil {
				lastSize = pos
			}
		}
	}
}

// flushAcc flushes the accumulator and handles any remaining entry.
func (t *LogTailer) flushAcc(acc *EntryAccumulator) {
	if entry := acc.Flush(); entry != nil {
		t.handleEntry(entry)
	}
}

// checkRotation detects truncation and rename/recreate rotation.
// Returns true if the file was rotated and the caller should reopen.
func (t *LogTailer) checkRotation(f *os.File, lastSize int64, acc *EntryAccumulator) (bool, error) {
	info, err := f.Stat()
	if err != nil {
		return false, err
	}
	currentSize := info.Size()
	if currentSize < lastSize {
		t.logger.Info().
			Int64("old_size", lastSize).
			Int64("new_size", currentSize).
			Msg("log file truncated (rotation detected), reopening")
		t.flushAcc(acc)
		return true, nil
	}

	if rotated, _ := t.fileRotated(f); rotated { //nolint:errcheck // best-effort rotation check; stat errors treated as no rotation
		t.logger.Info().Msg("log file replaced (rotation detected), reopening")
		t.flushAcc(acc)
		return true, nil
	}

	return false, nil
}

// readLines reads complete lines from the reader until EOF and processes them
// through the accumulator. Partial lines (no trailing newline) are left in
// the reader's internal buffer for the next poll.
func (t *LogTailer) readLines(reader *bufio.Reader, acc *EntryAccumulator) {
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			// Strip trailing newline/carriage return.
			line = stripNewline(line)
			if len(line) > t.cfg.MaxLineLength {
				line = line[:t.cfg.MaxLineLength]
			}
			t.linesRead++

			if entry := acc.Feed(line); entry != nil {
				t.handleEntry(entry)
			}
		}
		if err != nil {
			// EOF — no more data right now. We'll try again next tick.
			return
		}
	}
}

// stripNewline removes trailing \n and \r\n.
func stripNewline(s string) string {
	n := len(s)
	if n > 0 && s[n-1] == '\n' {
		n--
	}
	if n > 0 && s[n-1] == '\r' {
		n--
	}
	return s[:n]
}

// fileRotated checks whether the file at t.cfg.Path is a different file than
// the one we have open (inode changed). Returns false if we can't determine.
func (t *LogTailer) fileRotated(f *os.File) (bool, error) {
	fdInfo, err := f.Stat()
	if err != nil {
		return false, err
	}

	pathInfo, err := os.Stat(t.cfg.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil // file was removed
		}
		return false, err
	}

	return !os.SameFile(fdInfo, pathInfo), nil
}

// handleEntry processes a complete log entry: filter for errors, apply
// sampling, push to ring buffer.
func (t *LogTailer) handleEntry(entry *LogEntry) {
	if !entry.IsError() {
		return
	}

	// Apply sampling.
	if t.sampler != nil && !t.sampler.Allow(sampler.EventInfo{
		Category: "error",
		IsError:  true,
	}) {
		return
	}

	ev := ToErrorEvent(entry)
	t.buf.Push(ev)
	t.errorsSent++

	t.logger.Debug().
		Str("signature", ev.Signature).
		Str("error_type", ev.ErrorType).
		Str("module", ev.Module).
		Msg("error captured from log file")
}

func closeFile(f *os.File, logger *zerolog.Logger) {
	if err := f.Close(); err != nil {
		logger.Warn().Err(err).Msg("failed to close file")
	}
}
