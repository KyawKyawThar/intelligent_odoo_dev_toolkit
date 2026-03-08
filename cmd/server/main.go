// Package main is the entry point of the application.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	_ "Intelligent_Dev_ToolKit_Odoo/docs"
	"Intelligent_Dev_ToolKit_Odoo/internal/cache"
	"Intelligent_Dev_ToolKit_Odoo/internal/config"
	"Intelligent_Dev_ToolKit_Odoo/internal/server"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// @title OdooDevTools API
// @version 1.0.0
// @description A comprehensive development toolkit for Odoo environments with profiling, monitoring, and optimization features.
// @termsOfService http://swagger.io/terms/
// @contact.name API Support
// @contact.url http://example.com/support
// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html
// @host localhost:8080
// @basePath /api/v1
// @schemes http https
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and JWT token.

//go:generate swag init -g main.go -d ../../ --output ../../docs

// Version info (set via ldflags during build)
var (
	version   = "1.0.0"
	buildTime = "unknown"
	gitCommit = "unknown"
)

func main() {
	// ==========================================================================
	// 1. Load Configuration
	// ==========================================================================
	cfg, err := config.LoadConfig(".")
	if err != nil {
		log.Error().Err(err).Msg("Failed to load config")
		return
	}

	// ==========================================================================
	// 2. Setup Logger (BEFORE anything else)
	// ==========================================================================
	setupLogger(cfg.Environment)

	log.Info().
		Str("environment", cfg.Environment).
		Str("version", version).
		Str("build_time", buildTime).
		Str("git_commit", gitCommit).
		Msg("Starting OdooDevTools Server")

	// ==========================================================================
	// 3. Setup Database Connection Pool
	// ==========================================================================
	connPool, err := setupDatabase(cfg.DBSource)
	if err != nil {
		log.Error().Err(err).Msg("Failed to setup database")
		return
	}
	defer connPool.Close()

	// ==========================================================================
	// 4. Create Store, optional cache and Server
	// ==========================================================================
	store := db.NewStore(connPool)

	// initialize redis client if configuration present
	redisClient, err := setupRedis(&cfg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to setup Redis")
		return
	}
	if redisClient != nil {
		defer func() {
			if closeErr := redisClient.Close(); closeErr != nil {
				log.Warn().Err(closeErr).Msg("Error closing Redis client")
			}
		}()
	}

	srv, err := server.NewServer(store, redisClient, cfg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create server")
		return
	}

	// ==========================================================================
	// 5. Setup HTTP Server with Timeouts
	// ==========================================================================
	address := fmt.Sprintf("%s:%s", cfg.ServerHost, cfg.ServerPort)
	httpServer := &http.Server{
		Addr:              address,
		Handler:           srv.Router(),
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1MB
	}

	// ==========================================================================
	// 6. Start Server in Goroutine (non-blocking)
	// ==========================================================================
	serverErrors := make(chan error, 1)

	go func() {
		log.Info().
			Str("address", address).
			Str("environment", cfg.Environment).
			Msg("HTTP server starting")

		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrors <- err
		}
	}()

	// ==========================================================================
	// 7. Wait for Shutdown Signal
	// ==========================================================================
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	// Block until signal or error
	select {
	case err := <-serverErrors:
		log.Error().Err(err).Msg("Server error")
		return

	case sig := <-shutdown:
		log.Info().
			Str("signal", sig.String()).
			Msg("Shutdown signal received, starting graceful shutdown...")

		// Graceful shutdown with timeout
		gracefulShutdown(httpServer, connPool, 30*time.Second)
	}
}

// =============================================================================
// Setup Functions
// =============================================================================

func setupLogger(environment string) {
	if environment == config.EnvironmentDevelopment {
		// Pretty console output for development
		log.Logger = zerolog.New(zerolog.ConsoleWriter{
			Out:        os.Stderr,
			TimeFormat: time.RFC3339,
		}).With().Timestamp().Caller().Logger()

		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		// JSON output for production (better for log aggregation)
		log.Logger = zerolog.New(os.Stderr).
			With().
			Timestamp().
			Str("service", "odoodevtools-server").
			Logger()

		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
}

func setupDatabase(dbSource string) (*pgxpool.Pool, error) {
	// Parse config
	poolConfig, err := pgxpool.ParseConfig(dbSource)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database config: %w", err)
	}

	// Configure connection pool
	poolConfig.MaxConns = 25                       // Maximum connections
	poolConfig.MinConns = 5                        // Keep warm connections
	poolConfig.MaxConnLifetime = 1 * time.Hour     // Recycle connections
	poolConfig.MaxConnIdleTime = 30 * time.Minute  // Close idle connections
	poolConfig.HealthCheckPeriod = 1 * time.Minute // Health check interval

	// Create pool with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Info().
		Int32("max_conns", poolConfig.MaxConns).
		Int32("min_conns", poolConfig.MinConns).
		Msg("Database connection pool established")

	return pool, nil
}

func gracefulShutdown(httpServer *http.Server, connPool *pgxpool.Pool, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// 1. Stop accepting new requests
	log.Info().Msg("Shutting down HTTP server...")
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("HTTP server shutdown error, forcing close")
		if err := httpServer.Close(); err != nil {
			log.Error().Err(err).Msg("Failed to close HTTP server")
		}
	} else {
		log.Info().Msg("HTTP server stopped gracefully")
	}

	// 2. Close database connections
	log.Info().Msg("Closing database connections...")
	connPool.Close()
	log.Info().Msg("Database connections closed")

	// 3. Close Redis client if we opened one
	// (we simply rely on the variable being closed by deferred call in main)

	// 4. Any other cleanup (queues, background workers) would go here

	log.Info().Msg("Shutdown complete")
}

func setupRedis(cfg *config.Config) (*cache.RedisClient, error) {
	if cfg.RedisURL == "" {
		return nil, nil //nolint:nilnil // Redis is optional, so it's okay to return nil, nil.
	}

	rc := cache.RedisConfig{
		Address: cfg.RedisURL,
	}
	if cfg.RedisPassword != "" {
		rc.Password = cfg.RedisPassword
	}
	if cfg.RedisDB != 0 {
		rc.DB = cfg.RedisDB
	}

	redisClient, err := cache.NewRedisClient(rc)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}
	return redisClient, nil
}
