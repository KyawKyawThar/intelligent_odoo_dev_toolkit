package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	agenterrors "Intelligent_Dev_ToolKit_Odoo/internal/agent/errors"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/odoo"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/ringbuf"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/syncer"
	"Intelligent_Dev_ToolKit_Odoo/internal/config"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	// ── 1. Load config ────────────────────────────────────────────────────────
	cfg, err := config.LoadAgentConfig(".")
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}

	// ── 2. Logger ─────────────────────────────────────────────────────────────
	log.Logger = zerolog.New(zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: "15:04:05",
	}).With().Timestamp().Str("service", "odoo-agent").Logger()

	if cfg.Environment == config.EnvironmentDevelopment {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	printBanner(cfg.Environment)

	// ── 3. Validate config ────────────────────────────────────────────────────
	if cfg.OdooURL == "" || cfg.OdooDB == "" || cfg.OdooUser == "" || cfg.OdooPassword == "" {
		log.Fatal().Msg("Odoo configuration is missing — check ODOO_URL, PG_ODOO_DB, ODOO_ADMIN_USER, ODOO_ADMIN_PASSWORD in .env")
	}
	if cfg.AgentCloudURL == "" || cfg.AgentAPIKey == "" || cfg.AgentID == "" {
		log.Fatal().Msg("agent configuration is missing — check AGENT_CLOUD_URL, AGENT_API_KEY, AGENT_ID in .env")
	}

	// ── 4. Shutdown context (bound to OS signals) ─────────────────────────────
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// ── 5. Connect + authenticate (with retry for slow Odoo startup) ──────────
	client, err := odoo.NewClient(cfg.OdooURL, cfg.OdooDB, cfg.OdooUser, cfg.OdooPassword)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create Odoo client")
	}

	log.Info().
		Str("url", cfg.OdooURL).
		Str("db", cfg.OdooDB).
		Str("user", cfg.OdooUser).
		Msg("connecting to Odoo")

	if err := authenticateWithRetry(ctx, client); err != nil {
		log.Fatal().Err(err).Msg("Odoo authentication failed")
	}

	log.Info().Int("uid", client.UID).Msg("authenticated with Odoo")

	// ── 6. Schema sync interval ───────────────────────────────────────────────
	interval := parseInterval(cfg.AgentSchemaInterval, 60*time.Minute)
	log.Info().Dur("interval", interval).Msg("schema sync interval configured")

	// Derive the HTTP base URL from AGENT_CLOUD_URL.
	// AGENT_CLOUD_URL may be the WebSocket endpoint (ws://host/agent/ws);
	// the syncer and flusher need the plain HTTP base (http://host).
	httpBase := wsToHTTPBase(cfg.AgentCloudURL)
	log.Info().Str("http_base", httpBase).Msg("resolved HTTP base URL")

	// ── 7. Wait for server to be ready ───────────────────────────────────────
	if err := waitForServer(ctx, httpBase); err != nil {
		log.Fatal().Err(err).Msg("server did not become ready")
	}

	// ── 8. Error pipeline ─────────────────────────────────────────────────────
	bufSize := cfg.AgentErrorBufferSize
	if bufSize <= 0 {
		bufSize = 2048
	}
	errBuf := ringbuf.New[agenterrors.ErrorEvent](bufSize)

	errCollector := agenterrors.NewCollector(client, errBuf, log.Logger)
	errFlusher := agenterrors.NewFlusher(
		errBuf,
		httpBase,
		cfg.AgentAPIKey,
		cfg.AgentID, // env_id == agent ID for now
		0,           // spike threshold — disabled until configured
		log.Logger,
	)

	pollInterval := parseInterval("", 30*time.Second)
	flushInterval := parseInterval("", 60*time.Second)

	go errCollector.RunLoop(ctx, pollInterval)
	go errFlusher.RunLoop(ctx, flushInterval)

	log.Info().
		Int("buf_size", bufSize).
		Dur("poll_interval", pollInterval).
		Dur("flush_interval", flushInterval).
		Msg("error pipeline started")

	// ── 8. Run periodic sync loop (blocks until shutdown) ─────────────────────
	s := syncer.New(client, httpBase, cfg.AgentAPIKey, cfg.AgentID, log.Logger)
	s.RunLoop(ctx, interval)

	log.Info().Msg("agent stopped")
}

// wsToHTTPBase converts a WebSocket URL to an HTTP base URL and strips the path.
// ws://host:port/any/path  → http://host:port
// wss://host:port/any/path → https://host:port
// http(s):// URLs are returned with only scheme+host (path stripped).
func wsToHTTPBase(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	switch u.Scheme {
	case "ws":
		u.Scheme = "http"
	case "wss":
		u.Scheme = "https"
	}
	u.Path = ""
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

// parseInterval parses a duration string (e.g. "30m", "1h"). Falls back to
// defaultVal on empty or unparseable input.
func parseInterval(s string, defaultVal time.Duration) time.Duration {
	if s == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		log.Warn().Str("value", s).Msg("invalid AGENT_SCHEMA_INTERVAL, using default 1h")
		return defaultVal
	}
	return d
}

// authenticateWithRetry retries Odoo authentication with exponential backoff.
// It stops immediately on credential failures (HTTP 401 / wrong password),
// and retries on server errors (HTTP 500 / connection refused) which happen
// while Odoo is still initialising its database on first start.
func authenticateWithRetry(ctx context.Context, client *odoo.Client) error {
	const maxAttempts = 15
	const maxDelay = 30 * time.Second

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := client.Authenticate()
		if err == nil {
			return nil
		}

		// Permanent failures: wrong credentials, fault from Odoo app layer.
		// Do not retry — the user needs to fix the config.
		if isPermanentAuthError(err) {
			return err
		}

		if attempt == maxAttempts {
			return fmt.Errorf("Odoo not ready after %d attempts: %w", maxAttempts, err)
		}

		// Exponential back-off: 1s, 2s, 4s, 8s … capped at 30s
		delay := min(time.Duration(1<<uint(attempt-1))*time.Second, maxDelay)

		log.Warn().
			Err(err).
			Int("attempt", attempt).
			Int("max", maxAttempts).
			Dur("retry_in", delay).
			Msg("Odoo not ready yet, will retry (Odoo may still be initialising)")

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return nil
}

// waitForServer polls GET {base}/api/v1/health until the server process
// responds with any HTTP status (meaning the listener is up).
// Connection-refused errors are retried every 2s; ctx cancellation stops the loop.
func waitForServer(ctx context.Context, base string) error {
	healthURL := base + "/api/v1/health"
	client := &http.Client{Timeout: 3 * time.Second}

	log.Info().Str("url", healthURL).Msg("waiting for server to be ready")

	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, http.NoBody)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			log.Info().Int("status", resp.StatusCode).Msg("server is ready")
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// isPermanentAuthError returns true for errors that should not be retried
// (wrong credentials, Odoo-level fault).
func isPermanentAuthError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "invalid credentials") ||
		strings.Contains(msg, "odoo fault:")
}

func printBanner(env string) {
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  ╔══════════════════════════════════════════╗\n")
	fmt.Fprintf(os.Stderr, "  ║         OdooDevTools Agent               ║\n")
	fmt.Fprintf(os.Stderr, "  ║  env: %-36s║\n", env)
	fmt.Fprintf(os.Stderr, "  ╚══════════════════════════════════════════╝\n")
	fmt.Fprintf(os.Stderr, "\n")
}
