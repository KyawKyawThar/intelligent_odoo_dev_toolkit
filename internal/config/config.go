package config

import (
	"Intelligent_Dev_ToolKit_Odoo/utils"
	"errors"
	"strings"
	"time"

	"github.com/spf13/viper"
)

const (
	EnvironmentDevelopment = "development"
	EnvironmentStaging     = "staging"
	EnvironmentProduction  = "production"
)

const (
	StatusHealthy   = "healthy"
	StatusUnhealthy = "unhealthy"
)

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
	DBPoolMinConns    int    `mapstructure:"DB_POOL_MIN_CONNS"`
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
	WorkerReplicas          int    `mapstructure:"WORKER_REPLICAS"`
	IngestWorkerCount       int    `mapstructure:"INGEST_WORKER_COUNT"`
	IngestBatchSize         int    `mapstructure:"INGEST_BATCH_SIZE"`
	IngestPollInterval      string `mapstructure:"INGEST_POLL_INTERVAL"`
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
	AgentLogFile               string `mapstructure:"AGENT_LOG_FILE"`
	AgentSchemaInterval        string `mapstructure:"AGENT_SCHEMA_INTERVAL"`
	AgentErrorBufferSize       int    `mapstructure:"AGENT_ERROR_BUFFER_SIZE"`
	AgentProfilerBatchInterval string `mapstructure:"AGENT_PROFILER_BATCH_INTERVAL"`
}

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
