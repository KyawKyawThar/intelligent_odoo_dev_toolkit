// Package main is the entry point for the Odoo agent.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"math/rand"

	"Intelligent_Dev_ToolKit_Odoo/internal/agent/aggregator"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/collector"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/creds"
	agenterrors "Intelligent_Dev_ToolKit_Odoo/internal/agent/errors"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/flags"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/hook"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/odoo"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/ringbuf"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/sampler"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/syncer"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/transport"
	"Intelligent_Dev_ToolKit_Odoo/internal/config"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// ORM collector source identifiers.
const (
	ormSourceLog       = "log"
	ormSourceIRLogging = "irlogging"
)

// systemConfigPath returns the platform-specific path for the installed
// agent config file written by the installer scripts:
//   - Linux / macOS : /etc/odoodevtools/agent.env
//   - Windows       : %PROGRAMDATA%\OdooDevTools\agent.env
//
// On Linux the file is sourced by systemd's EnvironmentFile= directive so
// values arrive as plain OS env vars.  On macOS and Windows the binary loads
// the file explicitly because there is no equivalent service mechanism.
func systemConfigPath() string {
	if runtime.GOOS == "windows" {
		base := os.Getenv("PROGRAMDATA")
		if base == "" {
			base = `C:\ProgramData`
		}
		return filepath.Join(base, "OdooDevTools", "agent.env")
	}
	return "/etc/odoodevtools/agent.env"
}

// loadSystemConfig parses KEY=VALUE pairs from the system-installed agent.env
// and sets each one as an OS environment variable (skipping keys that are
// already set, so explicit env overrides still work).  Must be called before
// LoadConfig so that viper.AutomaticEnv() picks up the values.
func loadSystemConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" || os.Getenv(k) != "" {
			continue // already set — explicit env overrides file
		}
		if err := os.Setenv(k, v); err != nil {
			return fmt.Errorf("setenv %s: %w", k, err)
		}
	}
	return nil
}

// initConfig loads agent configuration. When the system config file is present
// (installed binary on macOS/Linux without systemd), it loads that file as OS
// env vars so viper.AutomaticEnv() picks them up, then calls LoadConfig to
// avoid the .env.agent overlay overriding the system-installed AGENT_CLOUD_URL.
func initConfig() config.Config {
	sysConfig := systemConfigPath()
	var cfg config.Config
	var err error
	if _, statErr := os.Stat(sysConfig); statErr == nil {
		if loadErr := loadSystemConfig(sysConfig); loadErr != nil {
			fmt.Fprintf(os.Stderr, "WARN: could not read %s: %v\n", sysConfig, loadErr)
		}
		cfg, err = config.LoadConfig(".")
	} else {
		cfg, err = config.LoadAgentConfig(".")
	}
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}
	return cfg
}

// initLogger configures the global zerolog logger and sets the log level.
func initLogger(cfg *config.Config) {
	log.Logger = zerolog.New(zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: "15:04:05",
	}).With().Timestamp().Str("service", "odoo-agent").Logger()

	if cfg.Environment == config.EnvironmentDevelopment {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
}

// validateConfig terminates the process if required config fields are missing.
func validateConfig(cfg *config.Config) {
	if cfg.OdooURL == "" || cfg.OdooDB == "" || cfg.OdooUser == "" || cfg.OdooPassword == "" {
		log.Fatal().Msg("Odoo configuration is missing — check ODOO_URL, PG_ODOO_DB, ODOO_ADMIN_USER, ODOO_ADMIN_PASSWORD in /etc/odoodevtools/agent.env (installed) or .env.agent (local dev)")
	}
	if cfg.AgentCloudURL == "" {
		log.Fatal().Msg("AGENT_CLOUD_URL is required")
	}
}

func main() {
	// ── 1. Config ─────────────────────────────────────────────────────────────
	cfg := initConfig()

	// ── 2. Logger ─────────────────────────────────────────────────────────────
	initLogger(&cfg)
	printBanner(cfg.Environment)

	// ── 3. Validate ───────────────────────────────────────────────────────────
	validateConfig(&cfg)

	// ── 3b. Self-registration or cached credentials ─────────────────────────
	credMgr, err := loadOrRegister(&cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("agent registration failed")
	}
	cfg.AgentID = credMgr.agentID
	cfg.AgentAPIKey = credMgr.apiKey
	envID := credMgr.envID

	log.Info().
		Str("agent_id", cfg.AgentID).
		Str("environment_id", envID).
		Str("key_prefix", cfg.AgentAPIKey[:12]).
		Msg("agent credentials ready")

	// ── 4. Shutdown context (bound to OS signals) ─────────────────────────────
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// ── 5. Connect + authenticate ────────────────────────────────────────────
	client, err := connectOdoo(ctx, &cfg)
	if err != nil {
		log.Error().Err(err).Msg("Odoo connection/authentication failed")
		return
	}

	// ── 6. Schema sync interval ───────────────────────────────────────────────
	interval := parseInterval(cfg.AgentSchemaInterval, 60*time.Minute)
	log.Info().Dur("interval", interval).Msg("schema sync interval configured")

	httpBase := wsToHTTPBase(cfg.AgentCloudURL)
	log.Info().Str("http_base", httpBase).Msg("resolved HTTP base URL")

	// ── 7. Wait for server to be ready ───────────────────────────────────────
	if errWait := waitForServer(ctx, httpBase); errWait != nil {
		log.Error().Err(errWait).Msg("server did not become ready")
		return
	}

	// ── 8. Sampler ───────────────────────────────────────────────────────────
	smp := initSampler(&cfg)

	// ── 9. Aggregator ────────────────────────────────────────────────────────
	agg, aggCfg := initAggregator(ctx, &cfg, envID, smp)

	// ── ORM Collector ─────────────────────────────────────────────────────
	startORMCollector(ctx, &cfg, agg, client, smp)

	// ── pg_stat_statements collector ──────────────────────────────────────
	startPgStatCollector(ctx, &cfg, agg, smp)

	// ── Compute chain collector (ir.profile-based) ────────────────────────
	startComputeChainCollector(ctx, &cfg, agg, client)

	// Debug feeder: explicit opt-in for local development.
	if cfg.AgentDebugFeeder {
		go runDebugFeeder(ctx, agg)
	}

	// Agent-side rate limiter.
	rateLimiter := initRateLimiter(&cfg)
	if rateLimiter != nil {
		defer rateLimiter.Stop()
	}

	// Batch transport: compress (gzip) + POST to cloud server.
	senderCfg := transport.DefaultSenderConfig(httpBase)
	sender := transport.NewSender(senderCfg, agg.SendCh, rateLimiter, credMgr, log.Logger)
	go sender.Run(ctx)

	log.Info().
		Dur("flush_interval", aggCfg.FlushInterval).
		Int("max_raw_per_flush", aggCfg.MaxRawPerFlush).
		Str("batch_endpoint", httpBase+senderCfg.Endpoint).
		Msg("aggregator + transport started")

	// ── 10. Error pipeline ────────────────────────────────────────────────────
	errBuf := startErrorPipeline(ctx, client, httpBase, credMgr, envID, &cfg, smp)

	// ── 10b. Log file tailer (optional — runs when AGENT_LOG_FILE is set) ────
	startLogTailer(ctx, &cfg, errBuf, smp)

	// ── 11. Feature flag receiver (WebSocket) ────────────────────────────────
	startFlagReceiver(ctx, httpBase, &cfg, smp, rateLimiter, credMgr)

	// ── 12. Fetch Odoo version and run periodic sync loop ─────────────────────
	odooVersion, err := client.Version(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("could not fetch Odoo version, using fallback")
		odooVersion = "unknown"
	} else {
		log.Info().Str("odoo_version", odooVersion).Msg("detected Odoo version")
	}

	s := syncer.New(client, httpBase, credMgr, envID, odooVersion, log.Logger)
	s.RunLoop(ctx, interval)

	log.Info().Msg("agent stopped")
}

// connectOdoo creates and authenticates an Odoo XML-RPC client.
func connectOdoo(ctx context.Context, cfg *config.Config) (*odoo.Client, error) {
	client, err := odoo.NewClient(cfg.OdooURL, cfg.OdooDB, cfg.OdooUser, cfg.OdooPassword)
	if err != nil {
		return nil, fmt.Errorf("failed to create Odoo client: %w", err)
	}

	log.Info().
		Str("url", cfg.OdooURL).
		Str("db", cfg.OdooDB).
		Str("user", cfg.OdooUser).
		Msg("connecting to Odoo")

	if authErr := authenticateWithRetry(ctx, client); authErr != nil {
		return nil, fmt.Errorf("odoo authentication failed: %w", authErr)
	}

	log.Info().Int("uid", client.UID).Msg("authenticated with Odoo")
	return client, nil
}

// initSampler builds the sampler from config, applying overrides.
func initSampler(cfg *config.Config) *sampler.Sampler {
	samplerCfg := sampler.ForEnvironment(cfg.Environment)
	if cfg.AgentSamplerMode != "" {
		samplerCfg.Mode = sampler.Mode(cfg.AgentSamplerMode)
	}
	if cfg.AgentSamplerRate > 0 {
		samplerCfg.SampleRate = cfg.AgentSamplerRate
	}
	if cfg.AgentSlowThresholdMS > 0 {
		samplerCfg.SlowThresholdMS = cfg.AgentSlowThresholdMS
	}
	smp := sampler.New(samplerCfg)

	log.Info().
		Str("mode", string(samplerCfg.Mode)).
		Float64("sample_rate", samplerCfg.SampleRate).
		Int("slow_threshold_ms", samplerCfg.SlowThresholdMS).
		Msg("sampler initialized")

	return smp
}

// initAggregator builds and starts the aggregator.
func initAggregator(ctx context.Context, cfg *config.Config, envID string, smp *sampler.Sampler) (*aggregator.Aggregator, aggregator.Config) {
	aggCfg := aggregator.DefaultConfig(envID)
	if cfg.AgentAggregatorFlushSec > 0 {
		aggCfg.FlushInterval = time.Duration(cfg.AgentAggregatorFlushSec) * time.Second
	}
	if cfg.AgentAggregatorMaxRaw > 0 {
		aggCfg.MaxRawPerFlush = cfg.AgentAggregatorMaxRaw
	}
	agg := aggregator.New(aggCfg, smp, log.Logger)
	go agg.Run(ctx)
	return agg, aggCfg
}

// startORMCollector selects and launches the appropriate ORM collector.
func startORMCollector(ctx context.Context, cfg *config.Config, agg *aggregator.Aggregator, client *odoo.Client, smp *sampler.Sampler) {
	ormSource := cfg.AgentORMCollector
	if ormSource == "" {
		if cfg.AgentLogFile != "" {
			ormSource = ormSourceLog
		} else {
			ormSource = ormSourceIRLogging
		}
	}

	if ormSource == ormSourceLog && cfg.AgentLogFile == "" {
		log.Warn().Msg("ORM collector set to 'log' but AGENT_LOG_FILE is empty, falling back to irlogging")
		ormSource = ormSourceIRLogging
	}

	switch ormSource {
	case ormSourceLog:
		ormCfg := hook.DefaultORMCollectorConfig(cfg.AgentLogFile)
		if cfg.AgentORMN1Threshold > 0 {
			ormCfg.N1Threshold = cfg.AgentORMN1Threshold
		}
		if cfg.AgentORMN1WindowSec > 0 {
			ormCfg.N1WindowSize = time.Duration(cfg.AgentORMN1WindowSec) * time.Second
		}
		ormCollector := hook.NewORMCollector(ormCfg, agg.EventCh, smp, log.Logger)
		go ormCollector.Run(ctx)

		log.Info().
			Str("path", cfg.AgentLogFile).
			Int("n1_threshold", ormCfg.N1Threshold).
			Msg("ORM log collector started")

	case ormSourceIRLogging:
		ormCollector := collector.NewORMLogCollector(client, agg.EventCh, smp, log.Logger)
		go ormCollector.RunLoop(ctx, 10*time.Second)
		log.Info().Msg("ORM ir.logging collector started")

	case "none":
		log.Info().Msg("ORM collector disabled")

	default:
		log.Warn().Str("value", ormSource).Msg("unknown AGENT_ORM_COLLECTOR value, ORM collection disabled")
	}
}

// startPgStatCollector connects to Odoo's PostgreSQL database and launches
// the pg_stat_statements collector if enabled.
func startPgStatCollector(ctx context.Context, cfg *config.Config, agg *aggregator.Aggregator, smp *sampler.Sampler) {
	if !cfg.AgentPgStatEnabled {
		log.Info().Msg("pg_stat_statements collector disabled (AGENT_PGSTAT_ENABLED not set)")
		return
	}
	if cfg.PgOdooDSN == "" {
		log.Warn().Msg("pg_stat_statements enabled but PG_ODOO_DSN is empty, skipping")
		return
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.PgOdooDSN)
	if err != nil {
		log.Error().Err(err).Msg("failed to parse PG_ODOO_DSN")
		return
	}
	poolCfg.MaxConns = 2
	poolCfg.MinConns = 1

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		log.Error().Err(err).Msg("failed to connect to Odoo PostgreSQL for pg_stat_statements")
		return
	}

	// Verify the extension is available.
	var extExists bool
	err = pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements')").Scan(&extExists)
	if err != nil || !extExists {
		log.Warn().Err(err).Msg("pg_stat_statements extension not available, skipping collector")
		pool.Close()
		return
	}

	interval := 30 * time.Second
	if cfg.AgentPgStatInterval > 0 {
		interval = time.Duration(cfg.AgentPgStatInterval) * time.Second
	}

	pgCollector := collector.NewPgStatCollector(pool, agg.EventCh, smp, log.Logger)
	go func() {
		pgCollector.RunLoop(ctx, interval)
		pool.Close()
	}()

	log.Info().
		Dur("interval", interval).
		Msg("pg_stat_statements collector started")
}

// startComputeChainCollector launches the ir.profile-based compute chain
// collector when AGENT_COMPUTE_COLLECTOR_ENABLED=true.
func startComputeChainCollector(ctx context.Context, cfg *config.Config, agg *aggregator.Aggregator, client *odoo.Client) {
	if !cfg.AgentComputeCollectorEnabled {
		log.Info().Msg("compute chain collector disabled (set AGENT_COMPUTE_COLLECTOR_ENABLED=true to enable)")
		return
	}

	interval := 30 * time.Second
	if cfg.AgentComputePollSec > 0 {
		interval = time.Duration(cfg.AgentComputePollSec) * time.Second
	}

	c := collector.NewComputeChainCollector(client, agg.EventCh, log.Logger, cfg.AgentOdooEnableProfiling)
	go c.RunLoop(ctx, interval)

	log.Info().
		Dur("interval", interval).
		Msg("compute chain collector started (polling ir.profile)")
}

// initRateLimiter creates a rate limiter if configured, or returns nil.
func initRateLimiter(cfg *config.Config) *transport.RateLimiter {
	rlCfg := transport.RateLimiterConfig{
		MaxBytesPerMinute:   cfg.AgentRateLimitCloudBytes,
		MaxBatchesPerMinute: cfg.AgentRateLimitBatchesPerMin,
	}
	if rlCfg.MaxBytesPerMinute <= 0 && rlCfg.MaxBatchesPerMinute <= 0 {
		return nil
	}
	rl := transport.NewRateLimiter(rlCfg)
	log.Info().
		Int64("max_bytes_per_min", rlCfg.MaxBytesPerMinute).
		Int("max_batches_per_min", rlCfg.MaxBatchesPerMinute).
		Msg("rate limiter enabled")
	return rl
}

// startErrorPipeline sets up the error ring buffer, collector, and flusher.
func startErrorPipeline(
	ctx context.Context,
	client *odoo.Client,
	httpBase string,
	credMgr creds.Provider,
	envID string,
	cfg *config.Config,
	smp *sampler.Sampler,
) *ringbuf.RingBuffer[agenterrors.ErrorEvent] {
	bufSize := cfg.AgentErrorBufferSize
	if bufSize <= 0 {
		bufSize = 2048
	}
	errBuf := ringbuf.New[agenterrors.ErrorEvent](bufSize)

	errCollector := agenterrors.NewCollector(client, errBuf, smp, log.Logger)
	errFlusher := agenterrors.NewFlusher(
		errBuf,
		httpBase,
		credMgr,
		envID,
		0, // spike threshold — disabled until configured
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

	return errBuf
}

// startLogTailer launches the log file tailer if AGENT_LOG_FILE is set.
func startLogTailer(ctx context.Context, cfg *config.Config, errBuf *ringbuf.RingBuffer[agenterrors.ErrorEvent], smp *sampler.Sampler) {
	if cfg.AgentLogFile != "" {
		tailerCfg := hook.DefaultTailerConfig(cfg.AgentLogFile)
		tailer := hook.NewLogTailer(tailerCfg, errBuf, smp, log.Logger)
		go tailer.Run(ctx)

		log.Info().
			Str("path", cfg.AgentLogFile).
			Msg("log file tailer started")
	} else {
		log.Info().Msg("log file tailer disabled (AGENT_LOG_FILE not set)")
	}
}

// startFlagReceiver launches the WebSocket feature flag receiver.
func startFlagReceiver(ctx context.Context, httpBase string, cfg *config.Config, smp *sampler.Sampler, rateLimiter *transport.RateLimiter, credMgr creds.Provider) {
	wsURL := httpToWSBase(httpBase) + "/api/v1/agent/ws"

	flagApplier := func(f flags.FeatureFlags) {
		smp.UpdateConfig(f.ToSamplerConfig())
		log.Info().
			Str("mode", f.SamplingMode).
			Float64("rate", f.SampleRate).
			Int("slow_threshold_ms", f.SlowThresholdMS).
			Msg("sampler updated from feature flags")

		if rateLimiter != nil && f.MaxBytesPerMinute > 0 {
			rateLimiter.UpdateLimits(f.MaxBytesPerMinute, 0)
			log.Info().
				Int64("max_bytes_per_min", f.MaxBytesPerMinute).
				Msg("rate limiter updated from feature flags")
		}
	}

	flagCfg := flags.DefaultReceiverConfig(wsURL, cfg.AgentID)
	flagReceiver := flags.NewFlagReceiver(flagCfg, flagApplier, credMgr, log.Logger)
	go flagReceiver.Run(ctx)

	log.Info().Str("ws_url", wsURL).Msg("feature flag receiver started")
}

// httpToWSBase converts an HTTP base URL to a WebSocket base URL.
// http://host:port → ws://host:port
// https://host:port → wss://host:port
func httpToWSBase(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	}
	return u.String()
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
// while Odoo is still initializing its database on first start.
func authenticateWithRetry(ctx context.Context, client *odoo.Client) error {
	const maxAttempts = 15
	const maxDelay = 30 * time.Second

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := client.Authenticate(ctx)
		if err == nil {
			return nil
		}

		// Permanent failures: wrong credentials, fault from Odoo app layer.
		// Do not retry — the user needs to fix the config.
		if isPermanentAuthError(err) {
			return err
		}

		if attempt == maxAttempts {
			return fmt.Errorf("odoo not ready after %d attempts: %w", maxAttempts, err)
		}

		// Exponential back-off: 1s, 2s, 4s, 8s … capped at 30s
		delay := min(time.Duration(1<<uint(attempt-1))*time.Second, maxDelay)

		log.Warn().
			Err(err).
			Int("attempt", attempt).
			Int("max", maxAttempts).
			Dur("retry_in", delay).
			Msg("Odoo not ready yet, will retry (Odoo may still be initializing)")

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

// ═══════════════════════════════════════════════════════════════════════════
// Agent self-registration + credential manager
// ═══════════════════════════════════════════════════════════════════════════

const credentialsFile = ".agent_credentials.json" //nolint:gosec // G101 false positive — this is a filename, not a credential

// agentCredentials is persisted to disk after a successful registration.
type agentCredentials struct {
	AgentID       string `json:"agent_id"`
	APIKey        string `json:"api_key"`
	EnvironmentID string `json:"environment_id,omitempty"`
}

// credentialManager holds the current API key behind a mutex and can
// automatically re-register with the cloud server when a 401 is received.
// It implements creds.Provider.
type credentialManager struct {
	mu        sync.Mutex
	apiKey    string
	agentID   string
	envID     string
	httpBase  string
	regToken  string
	credsPath string
	logger    zerolog.Logger
}

// Compile-time check that credentialManager implements creds.Provider.
var _ creds.Provider = (*credentialManager)(nil)

// APIKey returns the current API key.
func (cm *credentialManager) APIKey() string {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.apiKey
}

// RefreshOnUnauthorized deletes stale cached credentials, re-registers using
// the registration token, persists the new credentials, and returns the new key.
// Only one refresh runs at a time; concurrent callers block on the mutex.
func (cm *credentialManager) RefreshOnUnauthorized() (string, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.regToken == "" {
		return "", fmt.Errorf("no registration token available — " +
			"generate a new token via POST /environments/{env_id}/agent")
	}

	cm.logger.Warn().Msg("API key rejected (401), attempting re-registration")

	// Remove stale credentials file.
	if err := os.Remove(cm.credsPath); err != nil && !os.IsNotExist(err) {
		cm.logger.Warn().Err(err).Msg("failed to remove stale credentials file")
	}

	newCreds, err := selfRegister(cm.httpBase, cm.regToken)
	if err != nil {
		return "", fmt.Errorf("re-registration failed: %w", err)
	}

	cm.apiKey = newCreds.APIKey
	cm.agentID = newCreds.AgentID
	if newCreds.EnvironmentID != "" {
		cm.envID = newCreds.EnvironmentID
	}

	if err := saveCredentials(cm.credsPath, newCreds); err != nil {
		cm.logger.Warn().Err(err).Msg("failed to save refreshed credentials")
	} else {
		cm.logger.Info().Str("path", cm.credsPath).Msg("refreshed credentials saved")
	}

	cm.logger.Info().
		Str("agent_id", cm.agentID).
		Str("key_prefix", cm.apiKey[:12]).
		Msg("re-registration successful")

	return cm.apiKey, nil
}

// validateAPIKey makes a lightweight HEAD request against a protected endpoint
// to check whether the cached API key is still accepted by the server.
func validateAPIKey(httpBase, apiKey string) bool {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, httpBase+"/api/v1/agent/schema", http.NoBody)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "ApiKey "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		// Network error — assume key might be fine, let the real request decide.
		return true
	}
	defer resp.Body.Close()

	return resp.StatusCode != http.StatusUnauthorized
}

// loadOrRegister attempts to load saved credentials from disk. If the cached
// credentials fail validation (401), it automatically re-registers using the
// registration token. Returns a credentialManager that all components share.
func loadOrRegister(cfg *config.Config) (*credentialManager, error) {
	httpBase := wsToHTTPBase(cfg.AgentCloudURL)
	credsPath := filepath.Join(".", credentialsFile)

	cm := &credentialManager{
		httpBase:  httpBase,
		regToken:  cfg.AgentRegistrationToken,
		credsPath: credsPath,
		logger:    log.Logger,
	}

	// 1. If AGENT_API_KEY and AGENT_ID are explicitly set, use them directly.
	if cfg.AgentAPIKey != "" && cfg.AgentID != "" {
		log.Info().Msg("using AGENT_API_KEY and AGENT_ID from config")
		cm.apiKey = cfg.AgentAPIKey
		cm.agentID = cfg.AgentID
		return cm, nil
	}

	// 2. Try loading from credentials file.
	if cached, ok := tryLoadValidCredentials(credsPath, httpBase); ok {
		cm.apiKey = cached.APIKey
		cm.agentID = cached.AgentID
		cm.envID = cached.EnvironmentID
		return cm, nil
	}

	// 3. Self-register using the registration token (with retry).
	if cfg.AgentRegistrationToken == "" {
		return nil, fmt.Errorf("no credentials found and AGENT_REGISTRATION_TOKEN is not set — " +
			"generate a token via POST /environments/{env_id}/agent on the dashboard")
	}

	regCreds, err := registerWithRetry(httpBase, cfg.AgentRegistrationToken, 5)
	if err != nil {
		return nil, err
	}

	// 4. Save credentials to disk.
	if err := saveCredentials(credsPath, regCreds); err != nil {
		log.Warn().Err(err).Msg("failed to save credentials to disk — agent will re-register on next start")
	} else {
		log.Info().Str("path", credsPath).Msg("credentials saved to disk")
	}

	cm.apiKey = regCreds.APIKey
	cm.agentID = regCreds.AgentID
	cm.envID = regCreds.EnvironmentID
	return cm, nil
}

func tryLoadValidCredentials(credsPath, httpBase string) (*agentCredentials, bool) {
	data, err := os.ReadFile(credsPath)
	if err != nil {
		return nil, false
	}
	var cached agentCredentials
	if err := json.Unmarshal(data, &cached); err != nil || cached.AgentID == "" || cached.APIKey == "" {
		return nil, false
	}
	log.Info().Str("path", credsPath).Msg("loaded saved credentials, validating…")

	if validateAPIKey(httpBase, cached.APIKey) {
		log.Info().Msg("cached credentials are valid")
		return &cached, true
	}

	// Cached key is stale — fall through to re-registration.
	log.Warn().Msg("cached API key is no longer valid (401)")
	if err := os.Remove(credsPath); err != nil && !os.IsNotExist(err) {
		log.Warn().Err(err).Msg("failed to remove stale credentials file")
	}
	return nil, false
}

func registerWithRetry(httpBase, token string, maxRetries int) (*agentCredentials, error) {
	var (
		regCreds *agentCredentials
		err      error
	)
	backoff := 2 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		regCreds, err = selfRegister(httpBase, token)
		if err == nil {
			return regCreds, nil
		}

		if attempt == maxRetries {
			return nil, fmt.Errorf("self-register failed after %d attempts: %w", maxRetries, err)
		}

		log.Warn().
			Err(err).
			Int("attempt", attempt).
			Int("max_retries", maxRetries).
			Dur("retry_in", backoff).
			Msg("self-registration failed, retrying…")

		time.Sleep(backoff)
		backoff *= 2 // exponential backoff: 2s, 4s, 8s, 16s
	}
	return nil, err
}

// selfRegister calls POST /api/v1/agent/register with the one-time token.
func selfRegister(httpBase, registrationToken string) (*agentCredentials, error) {
	registerURL := httpBase + "/api/v1/agent/register"

	body, err := json.Marshal(map[string]string{
		"registration_token": registrationToken,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, registerURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST %s: %w", registerURL, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse the nested response: {"data": {...}}
	var envelope struct {
		Data agentCredentials `json:"data"`
	}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if envelope.Data.AgentID == "" || envelope.Data.APIKey == "" {
		return nil, fmt.Errorf("server returned empty credentials: %s", string(respBody))
	}

	log.Info().
		Str("agent_id", envelope.Data.AgentID).
		Msg("self-registration successful")

	return &envelope.Data, nil
}

// saveCredentials writes credentials to a JSON file.
func saveCredentials(path string, ac *agentCredentials) error {
	data, err := json.MarshalIndent(ac, "", "  ") //nolint:gosec // G117: intentional — writing credentials to a local file with 0600 perms
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// ═══════════════════════════════════════════════════════════════════════════
// TODO(DELETE-BEFORE-PROD): Debug feeder — synthetic event generator
// Pumps fake ORM events into the aggregator so you can test the full
// aggregator → transport → server pipeline without a real ORM collector.
// Enable with AGENT_DEBUG_FEEDER=true in .env.agent.
// ═══════════════════════════════════════════════════════════════════════════

var debugModels = []struct {
	model  string
	method string
}{
	{"res.partner", "search_read"},
	{"res.partner", "read"},
	{"res.partner", "write"},
	{"res.partner", "name_get"},
	{"sale.order", "search_read"},
	{"sale.order", "create"},
	{"sale.order.line", "read"},
	{"product.product", "search_read"},
	{"product.product", "name_get"},
	{"account.move", "search_read"},
	{"account.move", "write"},
	{"stock.picking", "read"},
	{"hr.employee", "search_read"},
	{"mail.message", "search_read"},
}

// debugRand wraps math/rand for the synthetic debug feeder.
// Crypto-strength randomness is not needed for fake test data.
func debugRand(n int) int { return rand.Intn(n) } //nolint:gosec // G404

func runDebugFeeder(ctx context.Context, agg *aggregator.Aggregator) {
	log.Warn().Msg("⚠ DEBUG FEEDER ACTIVE — synthetic events being injected (delete before production)")

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	var seq int
	for {
		select {
		case <-ctx.Done():
			log.Info().Int("events_sent", seq).Msg("debug feeder stopped")
			return
		case <-ticker.C:
			// Send 1–5 events per tick to simulate realistic ORM traffic.
			batch := 1 + debugRand(5)
			for range batch {
				ev := generateDebugEvent(seq)
				agg.EventCh <- ev
				seq++
			}
		}
	}
}

func generateDebugEvent(seq int) aggregator.Event {
	m := debugModels[debugRand(len(debugModels))]

	// Most queries are fast (1–30ms), some are medium (30–150ms), few are slow (150–500ms).
	duration := 1 + debugRand(30)
	roll := debugRand(100)
	if roll < 10 {
		duration = 30 + debugRand(120) // 10% medium
	}
	if roll < 3 {
		duration = 150 + debugRand(350) // 3% slow
	}

	ev := aggregator.Event{
		Category:   "orm",
		Model:      m.model,
		Method:     m.method,
		DurationMS: duration,
		Timestamp:  time.Now().UTC(),
	}

	// ~2% are errors
	if debugRand(100) < 2 {
		ev.IsError = true
		ev.Category = "error"
		ev.Traceback = fmt.Sprintf("Traceback (most recent call last):\n  File \"/odoo/models.py\", line %d\nValueError: debug synthetic error #%d", 100+debugRand(900), seq)
	}

	// ~5% simulate N+1 pattern: same model:method repeated many times with
	// short duration, typical of looping over records one by one.
	if debugRand(100) < 5 {
		ev.IsN1 = true
		ev.DurationMS = 1 + debugRand(5) // N+1 queries are individually fast
		ev.SQL = fmt.Sprintf("SELECT \"res_partner\".\"id\" FROM \"res_partner\" WHERE \"res_partner\".\"id\" = %d", debugRand(10000))
	}

	// ~15% carry a sample SQL
	if ev.SQL == "" && debugRand(100) < 15 {
		ev.SQL = fmt.Sprintf("SELECT * FROM %q WHERE id IN (%d, %d, %d)",
			strings.ReplaceAll(m.model, ".", "_"),
			debugRand(1000), debugRand(1000), debugRand(1000))
	}

	return ev
}
