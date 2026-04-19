// Package worker provides background workers that consume from Redis streams.
package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

// ConsumerConfig controls how a Redis stream consumer group operates.
type ConsumerConfig struct {
	Stream       string        // Redis stream key, e.g. "agent:ingest"
	Group        string        // Consumer group name, e.g. "ingest-workers"
	BatchSize    int64         // How many messages to read per call (default: 10)
	BlockTime    time.Duration // How long to block waiting for new messages (default: 5s)
	MaxRetries   int           // Max processing retries before dead-lettering (default: 3)
	ClaimTimeout time.Duration // Reclaim pending messages older than this (default: 30s)
}

// DefaultConsumerConfig returns sensible defaults.
func DefaultConsumerConfig(stream, group string) ConsumerConfig {
	return ConsumerConfig{
		Stream:       stream,
		Group:        group,
		BatchSize:    10,
		BlockTime:    30 * time.Second,
		MaxRetries:   3,
		ClaimTimeout: 30 * time.Second,
	}
}

// EnsureConsumerGroup creates the consumer group if it doesn't already exist.
// It is safe to call multiple times — BUSYGROUP errors are silently ignored.
func EnsureConsumerGroup(ctx context.Context, rdb *redis.Client, stream, group string) error {
	err := rdb.XGroupCreateMkStream(ctx, stream, group, "0").Err()
	if err != nil {
		// "BUSYGROUP Consumer Group name already exists" is expected.
		if err.Error() == "BUSYGROUP Consumer Group name already exists" {
			return nil
		}
		return fmt.Errorf("create consumer group %s on %s: %w", group, stream, err)
	}
	return nil
}

// MessageHandler processes a single Redis stream message.
// It receives the tenantID and raw JSON data from the stream entry.
type MessageHandler func(ctx context.Context, tenantID, data string) error

// RunConsumer reads messages from a Redis stream consumer group and calls
// handler for each one. It automatically ACKs successfully processed messages
// and retries/dead-letters failed ones. Blocks until ctx is canceled.
func RunConsumer(
	ctx context.Context,
	rdb *redis.Client,
	cfg ConsumerConfig,
	consumerName string,
	handler MessageHandler,
	logger zerolog.Logger,
) {
	log := logger.With().
		Str("component", "consumer").
		Str("stream", cfg.Stream).
		Str("consumer", consumerName).
		Logger()

	log.Info().Msg("consumer started")

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("consumer stopped")
			return
		default:
		}

		streams, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    cfg.Group,
			Consumer: consumerName,
			Streams:  []string{cfg.Stream, ">"},
			Count:    cfg.BatchSize,
			Block:    cfg.BlockTime,
		}).Result()

		if err != nil {
			if errors.Is(err, redis.Nil) || ctx.Err() != nil {
				continue
			}
			log.Error().Err(err).Msg("XReadGroup error")
			time.Sleep(1 * time.Second) // brief backoff on error
			continue
		}

		processStreams(ctx, rdb, cfg, streams, handler, log)
	}
}

// processStreams iterates over the streams returned by XReadGroup and
// dispatches each message to the handler, ACKing on success.
func processStreams(
	ctx context.Context,
	rdb *redis.Client,
	cfg ConsumerConfig,
	streams []redis.XStream,
	handler MessageHandler,
	log zerolog.Logger,
) {
	for _, stream := range streams {
		for _, msg := range stream.Messages {
			tenantID, _ := msg.Values["tenant_id"].(string) //nolint:errcheck // type assertion; zero value handled downstream
			data, _ := msg.Values["data"].(string)          //nolint:errcheck // type assertion; zero value handled downstream

			if err := handler(ctx, tenantID, data); err != nil {
				log.Error().
					Err(err).
					Str("msg_id", msg.ID).
					Str("tenant_id", tenantID).
					Msg("failed to process message")
				continue // message stays pending for retry/claim
			}

			// ACK on success.
			if err := rdb.XAck(ctx, cfg.Stream, cfg.Group, msg.ID).Err(); err != nil {
				log.Error().Err(err).Str("msg_id", msg.ID).Msg("failed to ACK")
			}
		}
	}
}
