// Package config ...
package config

import (
	"Intelligent_Dev_ToolKit_Odoo/utils"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// EnvironmentDevelopment is a constant for the development environment.
const (
	EnvironmentDevelopment = "development"
	EnvironmentStaging     = "staging"
	EnvironmentProduction  = "production"
)

// StatusHealthy is a constant for the healthy status.
const (
	StatusHealthy   = "healthy"
	StatusUnhealthy = "unhealthy"
)

// Config is the configuration for the application.
type Config struct {
	// ── App ──────────────────────────────────────────────────────
	Environment string `mapstructure:"APP_ENV"` // development | staging | production
	AppVersion  string `mapstructure:"APP_VERSION"`
	LogLevel    string `mapstructure:"LOG_LEVEL"`  // debug | info | warn | error
	LogFormat   string `mapstructure:"LOG_FORMAT"` // pretty | json

	// ── Server ───────────────────────────────────────────────────
	ServerPort          string        `mapstructure:"SERVER_PORT"`
	BaseURL             string        `mapstructure:"BASE_URL"`
	ServerHost          string        `mapstructure:"SERVER_HOST"`
	ServerReadTimeout   time.Duration `mapstructure:"SERVER_READ_TIMEOUT"`
	ServerWriteTimeout  time.Duration `mapstructure:"SERVER_WRITE_TIMEOUT"`
	ServerIdleTimeout   time.Duration `mapstructure:"SERVER_IDLE_TIMEOUT"`
	WSMaxConnsPerServer int           `mapstructure:"WS_MAX_CONNECTIONS_PER_SERVER"`

	// ── PostgreSQL ───────────────────────────────────────────────
	DBSource          string `mapstructure:"DATABASE_URL"`
	DBPoolMaxConns    int    `mapstructure:"DB_POOL_MAX_CONNS"`
	DBPoolMinConns    int    `mapstructure:"DB_POOL_MIN_CONns"`
	DBPoolMaxConnLife string `mapstructure:"DB_POOL_MAX_CONN_LIFETIME"`
	DBPoolMaxConnIdle string `mapstructure:"DB_POOL_MAX_CONN_IDLE_TIME"`

	// ── Redis ────────────────────────────────────────────────────
	RedisURL           string `mapstructure:"REDIS_URL"`
	RedisPassword      string `mapstructure:"REDIS_PASSWORD"`
	RedisDB            int    `mapstructure:"REDIS_DB"`
	RedisMaxMemory     string `mapstructure:"REDIS_MAXMEMORY"`
	RedisStreamIngest  string `mapstructure:"REDIS_STREAM_INGEST"`
	RedisStreamAgg     string `mapstructure:"REDIS_STREAM_AGGREGATE"`
	RedisStreamAlert   string `mapstructure:"REDIS_STREAM_ALERT"`
	RedisStreamRetain  string `mapstructure:"REDIS_STREAM_RETENTION"`
	RedisConsumerGroup string `mapstructure:"REDIS_CONSUMER_GROUP"`

	// ── S3 / MinIO ───────────────────────────────────────────────
	S3Endpoint       string `mapstructure:"S3_ENDPOINT"`
	S3Bucket         string `mapstructure:"S3_BUCKET"`
	S3Region         string `mapstructure:"S3_REGION"`
	S3AccessKey      string `mapstructure:"S3_ACCESS_KEY"`
	S3SecretKey      string `mapstructure:"S3_SECRET_KEY"`
	S3ForcePathStyle bool   `mapstructure:"S3_FORCE_PATH_STYLE"`

	// ── SMTP ─────────────────────────────────────────────────────
	SMTPHost     string `mapstructure:"SMTP_HOST"`
	SMTPPort     int    `mapstructure:"SMTP_PORT"`
	SMTPUsername string `mapstructure:"SMTP_USERNAME"`
	SMTPPassword string `mapstructure:"SMTP_PASSWORD"`
	EmailFrom    string `mapstructure:"EMAIL_FROM"`
	ClientAppURL string `mapstructure:"CLIENT_APP_URL"`

	// ── JWT Auth ─────────────────────────────────────────────────
	TokenSymmetricKey    string        `mapstructure:"TOKEN_SYMMETRIC_KEY"`
	AccessTokenDuration  time.Duration `mapstructure:"JWT_ACCESS_TOKEN_TTL"`
	RefreshTokenDuration time.Duration `mapstructure:"JWT_REFRESH_TOKEN_TTL"`
	JWTIssuer            string        `mapstructure:"JWT_ISSUER"`

	// CORS
	AllowedOrigins []string `mapstructure:"ALLOWED_ORIGINS"`

	// ── API Keys ─────────────────────────────────────────────────
	APIKeyPrefix     string `mapstructure:"API_KEY_PREFIX"`
	APIKeyHashPepper string `mapstructure:"API_KEY_HASH_PEPPER"`

	// ── Workers ──────────────────────────────────────────────────
	WorkerReplicas    int `mapstructure:"WORKER_REPLICAS"`
	IngestWorkerCount int `mapstructure:"INGEST_WORKER_COUNT"`
	IngestBatchSize   int `mapstructure:"INGEST_BATCH_SIZE"`

	IngestMaxRetries        int    `mapstructure:"INGEST_MAX_RETRIES"`
	AggregatorFlushInterval string `mapstructure:"AGGREGATOR_FLUSH_INTERVAL"`
	AlertWorkerCount        int    `mapstructure:"ALERT_WORKER_COUNT"`
	RetentionCron           string `mapstructure:"RETENTION_CRON"`
	RetentionDryRun         bool   `mapstructure:"RETENTION_DRY_RUN"`

	// ── Rate Limiting ────────────────────────────────────────────
	RateLimitCloud                int   `mapstructure:"RATE_LIMIT_CLOUD"`
	RateLimitOnprem               int   `mapstructure:"RATE_LIMIT_ONPREM"`
	RateLimitEnterprise           int   `mapstructure:"RATE_LIMIT_ENTERPRISE"`
	AgentRateLimitCloudBytes      int64 `mapstructure:"AGENT_RATE_LIMIT_CLOUD_BYTES"`
	AgentRateLimitOnpremBytes     int64 `mapstructure:"AGENT_RATE_LIMIT_ONPREM_BYTES"`
	AgentRateLimitEnterpriseBytes int64 `mapstructure:"AGENT_RATE_LIMIT_ENTERPRISE_BYTES"`
	AgentRateLimitBatchesPerMin   int   `mapstructure:"AGENT_RATE_LIMIT_BATCHES_PER_MIN"`

	// ── Stripe ───────────────────────────────────────────────────
	StripeSecretKey       string `mapstructure:"STRIPE_SECRET_KEY"`
	StripeWebhookSecret   string `mapstructure:"STRIPE_WEBHOOK_SECRET"`
	StripePriceCloud      string `mapstructure:"STRIPE_PRICE_CLOUD"`
	StripePriceOnprem     string `mapstructure:"STRIPE_PRICE_ONPREM"`
	StripePriceEnterprise string `mapstructure:"STRIPE_PRICE_ENTERPRISE"`

	// ── CORS ─────────────────────────────────────────────────────
	CORSAllowedOrigins   string `mapstructure:"CORS_ALLOWED_ORIGINS"`
	CORSAllowCredentials bool   `mapstructure:"CORS_ALLOW_CREDENTIALS"`

	// ── Agent (local dev) ────────────────────────────────────────
	AgentID                    string `mapstructure:"AGENT_ID"`
	AgentCloudURL              string `mapstructure:"AGENT_CLOUD_URL"`
	AgentAPIKey                string `mapstructure:"AGENT_API_KEY"`
	AgentRegistrationToken     string `mapstructure:"AGENT_REGISTRATION_TOKEN"`
	AgentLogFile               string `mapstructure:"AGENT_LOG_FILE"`
	AgentSchemaInterval        string `mapstructure:"AGENT_SCHEMA_INTERVAL"`
	AgentErrorBufferSize       int    `mapstructure:"AGENT_ERROR_BUFFER_SIZE"`
	AgentProfilerBatchInterval string `mapstructure:"AGENT_PROFILER_BATCH_INTERVAL"`

	// ── Agent Sampler ───────────────────────────────────────────
	// AGENT_SAMPLER_MODE overrides the environment-based default.
	// Valid values: "full", "sampled", "aggregated_only".
	AgentSamplerMode     string  `mapstructure:"AGENT_SAMPLER_MODE"`
	AgentSamplerRate     float64 `mapstructure:"AGENT_SAMPLER_RATE"`
	AgentSlowThresholdMS int     `mapstructure:"AGENT_SLOW_THRESHOLD_MS"`

	// ── Agent Aggregator ────────────────────────────────────────
	// AGENT_AGGREGATOR_FLUSH_SEC overrides the 30s default flush interval.
	AgentAggregatorFlushSec int `mapstructure:"AGENT_AGGREGATOR_FLUSH_SEC"`
	AgentAggregatorMaxRaw   int `mapstructure:"AGENT_AGGREGATOR_MAX_RAW"`

	// ── Agent ORM Collector ──────────────────────────────────────
	// AGENT_ORM_COLLECTOR selects the ORM event source.
	// Valid values: "log" (default when AGENT_LOG_FILE is set),
	//               "irlogging" (poll ir.logging), "none" (disabled).
	AgentORMCollector   string `mapstructure:"AGENT_ORM_COLLECTOR"`
	AgentORMN1Threshold int    `mapstructure:"AGENT_ORM_N1_THRESHOLD"`
	AgentORMN1WindowSec int    `mapstructure:"AGENT_ORM_N1_WINDOW_SEC"`

	// ── Agent pg_stat_statements ─────────────────────────────
	// PG_ODOO_DSN is the direct PostgreSQL connection string for Odoo's database.
	// Required for the pg_stat_statements collector.
	// Example: postgres://odoo:odoo@localhost:5432/odoo?sslmode=disable
	PgOdooDSN           string `mapstructure:"PG_ODOO_DSN"`
	AgentPgStatInterval int    `mapstructure:"AGENT_PGSTAT_INTERVAL_SEC"`
	AgentPgStatEnabled  bool   `mapstructure:"AGENT_PGSTAT_ENABLED"`

	// ── Agent Compute Chain Collector ───────────────────────
	// AGENT_COMPUTE_COLLECTOR_ENABLED=true activates the ir.profile-based
	// compute chain collector (requires Odoo 15+ with profiling enabled).
	AgentComputeCollectorEnabled bool `mapstructure:"AGENT_COMPUTE_COLLECTOR_ENABLED"`
	// AGENT_COMPUTE_POLL_SEC is the polling interval in seconds (default: 30).
	AgentComputePollSec int `mapstructure:"AGENT_COMPUTE_POLL_SEC"`
	// AGENT_ODOO_ENABLE_PROFILING=true causes the agent to automatically set
	// base_setup.profiling_enabled_until in Odoo so ir.profile records are
	// created. Safe for development; avoid in production (profiling has overhead).
	AgentOdooEnableProfiling bool `mapstructure:"AGENT_ODOO_ENABLE_PROFILING"`

	// ── Agent Debug (temporary — DELETE before production) ────
	// AGENT_DEBUG_FEEDER=true enables a synthetic event generator that
	// pumps fake ORM events into the aggregator for local testing.
	AgentDebugFeeder bool `mapstructure:"AGENT_DEBUG_FEEDER"`

	// ── Odoo XML-RPC (agent → Odoo application) ─────────────────
	// ODOO_URL     : HTTP base URL, e.g. http://localhost:8069
	// ODOO_ADMIN_USER / ODOO_ADMIN_PASSWORD : Odoo application user credentials
	//                created when Odoo initializes the database (default: admin/admin).
	//                These are NOT the PostgreSQL credentials below.
	OdooURL      string `mapstructure:"ODOO_URL"`
	OdooDB       string `mapstructure:"PG_ODOO_DB"`
	OdooUser     string `mapstructure:"ODOO_ADMIN_USER"`
	OdooPassword string `mapstructure:"ODOO_ADMIN_PASSWORD"`
}

// LoadConfig loads the configuration from the given path.
func LoadConfig(path string) (config Config, err error) {
	viper.AddConfigPath(path)
	viper.SetConfigName(".env") // filename without extension
	viper.SetConfigType("env")  // treat as key=value env file

	viper.AutomaticEnv()

	// Set defaults
	viper.SetDefault("ENVIRONMENT", EnvironmentDevelopment)
	viper.SetDefault("SERVER_HOST", "0.0.0.0")
	viper.SetDefault("SERVER_PORT", "8080")
	viper.SetDefault("ACCESS_TOKEN_DURATION", "15m")
	viper.SetDefault("REFRESH_TOKEN_DURATION", "24h")
	viper.SetDefault("ALLOWED_ORIGINS", "*")
	viper.SetDefault("RATE_LIMIT_RPM", 100)
	viper.SetDefault("RATE_LIMIT_INGEST_PM", 1000)
	viper.SetDefault("REDIS_DB", 0)
	viper.SetDefault("TOKEN_SYMMETRIC_KEY", utils.RandomString(32))

	err = viper.ReadInConfig()
	if err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			return config, err
		}
	}

	err = viper.Unmarshal(&config)
	if err != nil {
		return config, err
	}

	// Makefile `include .env` exports values with trailing whitespace when the
	// .env line has an inline comment (e.g. APP_ENV=development  # hint).
	// Trim string fields used for equality comparisons so they always match.
	config.Environment = strings.TrimSpace(config.Environment)
	config.LogLevel = strings.TrimSpace(config.LogLevel)
	config.LogFormat = strings.TrimSpace(config.LogFormat)

	// Workaround for viper bug
	if config.IngestWorkerCount == 0 {
		ingestWorkerCount, err := strconv.Atoi(viper.GetString("INGEST_WORKER_COUNT"))
		if err == nil {
			config.IngestWorkerCount = ingestWorkerCount
		}
	}
	if config.IngestBatchSize == 0 {
		ingestBatchSize, err := strconv.Atoi(viper.GetString("INGEST_BATCH_SIZE"))
		if err == nil {
			config.IngestBatchSize = ingestBatchSize
		}
	}
	if config.AgentRateLimitCloudBytes == 0 {
		agentRateLimitCloudBytes, err := strconv.ParseInt(viper.GetString("AGENT_RATE_LIMIT_CLOUD_BYTES"), 10, 64)
		if err == nil {
			config.AgentRateLimitCloudBytes = agentRateLimitCloudBytes
		}
	}
	if config.AgentRateLimitOnpremBytes == 0 {
		agentRateLimitOnpremBytes, err := strconv.ParseInt(viper.GetString("AGENT_RATE_LIMIT_ONPREM_BYTES"), 10, 64)
		if err == nil {
			config.AgentRateLimitOnpremBytes = agentRateLimitOnpremBytes
		}
	}
	if config.AgentRateLimitEnterpriseBytes == 0 {
		agentRateLimitEnterpriseBytes, err := strconv.ParseInt(viper.GetString("AGENT_RATE_LIMIT_ENTERPRISE_BYTES"), 10, 64)
		if err == nil {
			config.AgentRateLimitEnterpriseBytes = agentRateLimitEnterpriseBytes
		}
	}

	// Parse ALLOWED_ORIGINS from comma-separated string
	originsStr := viper.GetString("ALLOWED_ORIGINS")
	if originsStr != "" {
		config.AllowedOrigins = parseOrigins(originsStr)
	}
	return config, nil
}

// parseOrigins parses a comma-separated string of origins.
func parseOrigins(s string) []string {
	if s == "*" {
		return []string{"*"}
	}

	origins := strings.Split(s, ",")
	result := make([]string, 0, len(origins))
	for _, origin := range origins {
		origin = strings.TrimSpace(origin)
		if origin != "" {
			result = append(result, origin)
		}
	}
	return result
}

// IsDevelopment returns true if running in development mode.
func (c *Config) IsDevelopment() bool {
	return c.Environment == EnvironmentDevelopment
}

// IsProduction returns true if running in production mode.
func (c *Config) IsProduction() bool {
	return c.Environment == EnvironmentProduction
}

// LoadAgentConfig loads the base config from .env then merges in .env.agent
// overrides. The overlay file is optional — if it does not exist the base
// config is returned unchanged. This lets the agent binary run locally with
// localhost URLs while the Docker stack still uses the service-name URLs in .env.
func LoadAgentConfig(path string) (Config, error) {
	cfg, err := LoadConfig(path)
	if err != nil {
		return cfg, err
	}

	v := viper.New()
	v.AddConfigPath(path)
	v.SetConfigName(".env.agent")
	v.SetConfigType("env")
	// Do NOT call v.AutomaticEnv() — the Makefile exports .env vars into the OS
	// environment before running the agent, and AutomaticEnv gives env vars higher
	// priority than the config file, which would make .env.agent overrides invisible.

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) {
			return cfg, nil // overlay is optional
		}
		return cfg, fmt.Errorf("read .env.agent: %w", err)
	}

	applyAgentStringOverrides(v, &cfg)
	applyAgentNumericOverrides(v, &cfg)

	// ── pg_stat_statements collector ──────
	if s := v.GetString("AGENT_PGSTAT_ENABLED"); s != "" {
		cfg.AgentPgStatEnabled = strings.EqualFold(s, "true") || s == "1"
	}

	// ── Compute chain collector ──────────
	if s := v.GetString("AGENT_COMPUTE_COLLECTOR_ENABLED"); s != "" {
		cfg.AgentComputeCollectorEnabled = strings.EqualFold(s, "true") || s == "1"
	}
	if s := v.GetString("AGENT_ODOO_ENABLE_PROFILING"); s != "" {
		cfg.AgentOdooEnableProfiling = strings.EqualFold(s, "true") || s == "1"
	}

	// ── Debug feeder (temporary — DELETE before production) ──────
	if s := v.GetString("AGENT_DEBUG_FEEDER"); s != "" {
		cfg.AgentDebugFeeder = strings.EqualFold(s, "true") || s == "1"
	}

	return cfg, nil
}

// applyAgentStringOverrides applies plain string overrides from .env.agent.
func applyAgentStringOverrides(v *viper.Viper, cfg *Config) {
	overrideStr := func(key string, dest *string) {
		if s := v.GetString(key); s != "" {
			*dest = s
		}
	}

	overrideStr("AGENT_CLOUD_URL", &cfg.AgentCloudURL)
	overrideStr("AGENT_API_KEY", &cfg.AgentAPIKey)
	overrideStr("AGENT_ID", &cfg.AgentID)
	overrideStr("AGENT_REGISTRATION_TOKEN", &cfg.AgentRegistrationToken)
	overrideStr("AGENT_SCHEMA_INTERVAL", &cfg.AgentSchemaInterval)
	overrideStr("ODOO_URL", &cfg.OdooURL)
	overrideStr("ODOO_ADMIN_USER", &cfg.OdooUser)
	overrideStr("ODOO_ADMIN_PASSWORD", &cfg.OdooPassword)
	overrideStr("PG_ODOO_DB", &cfg.OdooDB)
	overrideStr("AGENT_SAMPLER_MODE", &cfg.AgentSamplerMode)
	overrideStr("AGENT_ORM_COLLECTOR", &cfg.AgentORMCollector)
	overrideStr("AGENT_LOG_FILE", &cfg.AgentLogFile)
	overrideStr("PG_ODOO_DSN", &cfg.PgOdooDSN)
}

// applyAgentNumericOverrides applies int and float overrides from .env.agent.
func applyAgentNumericOverrides(v *viper.Viper, cfg *Config) {
	overrideInt := func(key string, dest *int) {
		if s := v.GetString(key); s != "" {
			if n, err := strconv.Atoi(s); err == nil {
				*dest = n
			}
		}
	}

	overrideFloat := func(key string, dest *float64) {
		if s := v.GetString(key); s != "" {
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				*dest = f
			}
		}
	}

	overrideInt("AGENT_ERROR_BUFFER_SIZE", &cfg.AgentErrorBufferSize)
	overrideFloat("AGENT_SAMPLER_RATE", &cfg.AgentSamplerRate)
	overrideInt("AGENT_SLOW_THRESHOLD_MS", &cfg.AgentSlowThresholdMS)
	overrideInt("AGENT_AGGREGATOR_FLUSH_SEC", &cfg.AgentAggregatorFlushSec)
	overrideInt("AGENT_AGGREGATOR_MAX_RAW", &cfg.AgentAggregatorMaxRaw)
	overrideInt("AGENT_ORM_N1_THRESHOLD", &cfg.AgentORMN1Threshold)
	overrideInt("AGENT_ORM_N1_WINDOW_SEC", &cfg.AgentORMN1WindowSec)
	overrideInt("AGENT_PGSTAT_INTERVAL_SEC", &cfg.AgentPgStatInterval)
	overrideInt("AGENT_COMPUTE_POLL_SEC", &cfg.AgentComputePollSec)
}
