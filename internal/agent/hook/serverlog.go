package hook

import (
	"bufio"
	"context"
	"io"
	"os"
	"time"

	"Intelligent_Dev_ToolKit_Odoo/internal/agent/flags"

	"github.com/rs/zerolog"
)

// ServerLogForwarder tails the Odoo server log file and forwards ALL log
// lines (every level) to the cloud via the WebSocket log_lines channel.
//
// Unlike LogTailer (which only handles errors), this forwarder is purpose-built
// for the live log-streaming feature so developers can see logs in the dashboard
// without SSH access.
//
// It reuses the same rotation-detection logic as LogTailer and the existing
// EntryAccumulator for multi-line entry assembly.
type ServerLogForwarder struct {
	cfg    TailerConfig
	ch     chan<- []flags.LogLine
	logger zerolog.Logger
}

// NewServerLogForwarder creates a forwarder. ch receives batches of log lines;
// it should be buffered so the forwarder is never blocked by a slow consumer.
func NewServerLogForwarder(cfg TailerConfig, ch chan<- []flags.LogLine, logger zerolog.Logger) *ServerLogForwarder {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 1 * time.Second
	}
	if cfg.MaxLineLength <= 0 {
		cfg.MaxLineLength = 1 << 20
	}
	return &ServerLogForwarder{
		cfg:    cfg,
		ch:     ch,
		logger: logger.With().Str("component", "server-log-forwarder").Logger(),
	}
}

// Run blocks until ctx is canceled. It opens the log file, seeks to the end,
// and continuously forwards new parsed entries.
func (f *ServerLogForwarder) Run(ctx context.Context) {
	f.logger.Info().
		Str("path", f.cfg.Path).
		Dur("poll_interval", f.cfg.PollInterval).
		Msg("server log forwarder started")

	backoff := f.cfg.PollInterval
	const maxBackoff = 30 * time.Second

	for {
		err := f.tailFile(ctx)
		if ctx.Err() != nil {
			f.logger.Info().Msg("server log forwarder stopped")
			return
		}
		if err != nil {
			f.logger.Warn().Err(err).Dur("retry_in", backoff).Msg("server log forwarder error, will retry")
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, maxBackoff)
			continue
		}
		backoff = f.cfg.PollInterval
		select {
		case <-ctx.Done():
			return
		case <-time.After(f.cfg.PollInterval):
		}
	}
}

func (f *ServerLogForwarder) tailFile(ctx context.Context) error {
	file, err := os.Open(f.cfg.Path)
	if err != nil {
		return err
	}
	defer closeFile(file, &f.logger)

	// Seek to end — only stream new entries, not the full history.
	lastSize, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}

	acc := &EntryAccumulator{}
	reader := bufio.NewReaderSize(file, 64*1024)
	ticker := time.NewTicker(f.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			f.flush(acc)
			return ctx.Err()

		case <-ticker.C:
			rotated, newSize, err := f.checkRotation(file, lastSize)
			if err != nil {
				return err
			}
			if rotated {
				f.flush(acc)
				return nil
			}
			lastSize = newSize

			f.collectAndSend(reader, acc)
		}
	}
}

// checkRotation detects truncation and rename/recreate rotation.
// Returns (true, _, nil) when the file was rotated and the caller should reopen.
// Returns the current file position as newSize for tracking on non-rotation ticks.
func (f *ServerLogForwarder) checkRotation(file *os.File, lastSize int64) (rotated bool, newSize int64, err error) {
	info, statErr := file.Stat()
	if statErr != nil {
		return false, lastSize, statErr
	}
	currentSize := info.Size()

	if currentSize < lastSize {
		f.logger.Info().Msg("log file truncated, reopening")
		return true, lastSize, nil
	}

	replaced, checkErr := fileRotatedCheck(file, f.cfg.Path)
	if checkErr != nil {
		// Treat a stat error as a transient issue — log and continue.
		f.logger.Warn().Err(checkErr).Msg("rotation check error, skipping")
	} else if replaced {
		f.logger.Info().Msg("log file replaced, reopening")
		return true, lastSize, nil
	}

	return false, currentSize, nil
}

// collectAndSend reads all available complete lines from reader, converts them
// to LogLines via the accumulator, and sends any resulting batch to the channel.
func (f *ServerLogForwarder) collectAndSend(reader *bufio.Reader, acc *EntryAccumulator) {
	batch := f.readNewLines(reader, acc)
	if len(batch) == 0 {
		return
	}
	select {
	case f.ch <- batch:
	default:
		// Channel full — drop batch rather than block the tailer.
		f.logger.Warn().Int("dropped", len(batch)).Msg("log line channel full, dropping batch")
	}
}

// readNewLines drains the reader of all complete lines and returns their
// parsed log lines. Partial lines (no trailing newline) remain buffered.
func (f *ServerLogForwarder) readNewLines(reader *bufio.Reader, acc *EntryAccumulator) []flags.LogLine {
	var batch []flags.LogLine
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			line = stripNewline(line)
			if len(line) > f.cfg.MaxLineLength {
				line = line[:f.cfg.MaxLineLength]
			}
			if entry := acc.Feed(line); entry != nil {
				batch = append(batch, entryToLogLine(entry))
			}
		}
		if err != nil {
			break // EOF — wait for next tick
		}
	}
	return batch
}

// flush sends any buffered partial entry before reopening/stopping.
func (f *ServerLogForwarder) flush(acc *EntryAccumulator) {
	if entry := acc.Flush(); entry != nil {
		select {
		case f.ch <- []flags.LogLine{entryToLogLine(entry)}:
		default:
		}
	}
}

func entryToLogLine(e *LogEntry) flags.LogLine {
	return flags.LogLine{
		Timestamp: e.Timestamp.Format("2006-01-02 15:04:05,000"),
		Level:     e.Level,
		Logger:    e.Logger,
		DB:        e.DBName,
		Message:   e.FullMessage(),
	}
}

// fileRotatedCheck mirrors LogTailer.fileRotated but as a standalone function
// so ServerLogForwarder doesn't need to embed LogTailer.
func fileRotatedCheck(f *os.File, path string) (bool, error) {
	fdInfo, err := f.Stat()
	if err != nil {
		return false, err
	}
	pathInfo, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, err
	}
	return !os.SameFile(fdInfo, pathInfo), nil
}
