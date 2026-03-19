// Package main is the entry point for the background worker process.
// It runs the ingest worker that consumes from Redis streams and writes
// to PostgreSQL + S3.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/cache"
	"Intelligent_Dev_ToolKit_Odoo/internal/config"
	"Intelligent_Dev_ToolKit_Odoo/internal/storage"
	"Intelligent_Dev_ToolKit_Odoo/internal/worker"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	// ── 1. Load config ────────────────────────────────────────────────────────
	cfg, err := config.LoadConfig(".")
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}

	// ── 2. Logger ─────────────────────────────────────────────────────────────
	initLogger(cfg.Environment)

	printBanner(cfg.Environment)

	// ── 3. Database ───────────────────────────────────────────────────────────
	if cfg.DBSource == "" {
		log.Fatal().Msg("DATABASE_URL is required")
	}

	connPool, err := pgxpool.New(context.Background(), cfg.DBSource)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer connPool.Close()

	store := db.NewStore(connPool)
	log.Info().Msg("database connected")

	// ── 4. Redis ──────────────────────────────────────────────────────────────
	if cfg.RedisURL == "" {
		log.Error().Msg("REDIS_URL is required for worker")
		return
	}

	rc := cache.RedisConfig{
		Address:  cfg.RedisURL,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	}
	redisClient, err := cache.NewRedisClient(rc)
	if err != nil {
		log.Error().Err(err).Msg("failed to connect to Redis")
		return
	}
	defer func() {
		if cerr := redisClient.Close(); cerr != nil {
			log.Error().Err(cerr).Msg("failed to close redis client")
		}
	}()

	log.Info().Msg("redis connected")

	// ── 5. S3 / MinIO ────────────────────────────────────────────────────────
	s3Client := initS3(cfg)

	// ── 6. Ingest worker ─────────────────────────────────────────────────────
	streamName := cfg.RedisStreamIngest
	if streamName == "" {
		streamName = "agent:ingest"
	}
	groupName := cfg.RedisConsumerGroup
	if groupName == "" {
		groupName = "ingest-workers"
	}

	ingestCfg := worker.DefaultIngestConfig(streamName, groupName)
	if cfg.IngestWorkerCount > 0 {
		ingestCfg.WorkerCount = cfg.IngestWorkerCount
	}
	if cfg.IngestBatchSize > 0 {
		ingestCfg.Consumer.BatchSize = int64(cfg.IngestBatchSize)
	}

	iw := worker.NewIngestWorker(store, s3Client, redisClient.Client, ingestCfg, log.Logger)

	// ── 7. Shutdown context ──────────────────────────────────────────────────
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Info().
		Str("stream", streamName).
		Str("group", groupName).
		Int("workers", ingestCfg.WorkerCount).
		Msg("starting ingest worker")

	if err := iw.Run(ctx); err != nil {
		log.Error().Err(err).Msg("ingest worker error")
	}

	log.Info().Msg("worker stopped")
}

func initLogger(env string) {
	if env == config.EnvironmentDevelopment {
		log.Logger = zerolog.New(zerolog.ConsoleWriter{
			Out:        os.Stderr,
			TimeFormat: "15:04:05",
		}).With().Timestamp().Str("service", "odoo-worker").Logger()
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		log.Logger = zerolog.New(os.Stderr).With().
			Timestamp().
			Str("service", "odoo-worker").
			Logger()
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
}

func initS3(cfg config.Config) *storage.S3Client {
	if cfg.S3Endpoint == "" || cfg.S3Bucket == "" {
		log.Warn().Msg("S3 not configured — raw tracebacks will not be stored")
		return nil
	}
	s3Client, err := storage.NewS3Client(storage.S3Config{
		Endpoint:       cfg.S3Endpoint,
		Bucket:         cfg.S3Bucket,
		Region:         cfg.S3Region,
		AccessKey:      cfg.S3AccessKey,
		SecretKey:      cfg.S3SecretKey,
		ForcePathStyle: cfg.S3ForcePathStyle,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create S3 client")
	}
	log.Info().
		Str("endpoint", cfg.S3Endpoint).
		Str("bucket", cfg.S3Bucket).
		Msg("S3 client initialized")
	return s3Client
}

func printBanner(env string) {
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  ╔══════════════════════════════════════════╗\n")
	fmt.Fprintf(os.Stderr, "  ║         OdooDevTools Worker              ║\n")
	fmt.Fprintf(os.Stderr, "  ║  env: %-36s║\n", env)
	fmt.Fprintf(os.Stderr, "  ╚══════════════════════════════════════════╝\n")
	fmt.Fprintf(os.Stderr, "\n")
}
