package worker

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupRedis starts a miniredis server and returns a go-redis client + cleanup.
func setupRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	return rdb, mr
}

// ── DefaultConsumerConfig tests ─────────────────────────────────────────────

func TestDefaultConsumerConfig_Values(t *testing.T) {
	cfg := DefaultConsumerConfig("agent:ingest", "ingest-workers")
	assert.Equal(t, "agent:ingest", cfg.Stream)
	assert.Equal(t, "ingest-workers", cfg.Group)
	assert.Equal(t, int64(10), cfg.BatchSize)
	assert.Equal(t, 5*time.Second, cfg.BlockTime)
	assert.Equal(t, 3, cfg.MaxRetries)
	assert.Equal(t, 30*time.Second, cfg.ClaimTimeout)
}

// ── EnsureConsumerGroup tests ───────────────────────────────────────────────

func TestEnsureConsumerGroup_CreatesGroup(t *testing.T) {
	rdb, _ := setupRedis(t)
	ctx := context.Background()

	err := EnsureConsumerGroup(ctx, rdb, "test:stream", "test-group")
	require.NoError(t, err)

	// Verify the group was created by adding a message and reading via the group.
	rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "test:stream",
		Values: map[string]any{"key": "val"},
	})

	result, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    "test-group",
		Consumer: "c1",
		Streams:  []string{"test:stream", ">"},
		Count:    1,
		Block:    0,
	}).Result()
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Len(t, result[0].Messages, 1)
}

func TestEnsureConsumerGroup_Idempotent(t *testing.T) {
	rdb, _ := setupRedis(t)
	ctx := context.Background()

	err := EnsureConsumerGroup(ctx, rdb, "test:stream", "test-group")
	require.NoError(t, err)

	// Calling again should NOT return an error (BUSYGROUP is silenced).
	err = EnsureConsumerGroup(ctx, rdb, "test:stream", "test-group")
	require.NoError(t, err)
}

// ── processStreams tests ────────────────────────────────────────────────────

func TestProcessStreams_DispatchesAndACKs(t *testing.T) {
	rdb, _ := setupRedis(t)
	ctx := context.Background()
	logger := zerolog.Nop()

	stream := "test:stream"
	group := "test-group"
	cfg := DefaultConsumerConfig(stream, group)

	require.NoError(t, EnsureConsumerGroup(ctx, rdb, stream, group))

	// Add 2 messages.
	rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]any{"tenant_id": "tid-1", "data": "payload-1"},
	})
	rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]any{"tenant_id": "tid-2", "data": "payload-2"},
	})

	// Read messages via XReadGroup (simulating what RunConsumer does).
	streams, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: "c1",
		Streams:  []string{stream, ">"},
		Count:    10,
		Block:    0,
	}).Result()
	require.NoError(t, err)

	// Track handler calls.
	var calls []struct{ tenantID, data string }
	handler := func(_ context.Context, tenantID, data string) error {
		calls = append(calls, struct{ tenantID, data string }{tenantID, data})
		return nil
	}

	processStreams(ctx, rdb, cfg, streams, handler, logger)

	// Handler should have been called for both messages.
	require.Len(t, calls, 2)
	assert.Equal(t, "tid-1", calls[0].tenantID)
	assert.Equal(t, "payload-1", calls[0].data)
	assert.Equal(t, "tid-2", calls[1].tenantID)
	assert.Equal(t, "payload-2", calls[1].data)

	// Both messages should be ACKed (no pending messages).
	pending, err := rdb.XPending(ctx, stream, group).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), pending.Count)
}

func TestProcessStreams_HandlerError_DoesNotACK(t *testing.T) {
	rdb, _ := setupRedis(t)
	ctx := context.Background()
	logger := zerolog.Nop()

	stream := "test:stream"
	group := "test-group"
	cfg := DefaultConsumerConfig(stream, group)

	require.NoError(t, EnsureConsumerGroup(ctx, rdb, stream, group))

	rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]any{"tenant_id": "tid-1", "data": "payload"},
	})

	streams, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: "c1",
		Streams:  []string{stream, ">"},
		Count:    10,
		Block:    0,
	}).Result()
	require.NoError(t, err)

	// Handler returns an error.
	handler := func(_ context.Context, _, _ string) error {
		return fmt.Errorf("processing failed")
	}

	processStreams(ctx, rdb, cfg, streams, handler, logger)

	// Message should NOT be ACKed — it remains pending for retry.
	pending, err := rdb.XPending(ctx, stream, group).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), pending.Count)
}

func TestProcessStreams_MixedSuccess_OnlyACKsSuccessful(t *testing.T) {
	rdb, _ := setupRedis(t)
	ctx := context.Background()
	logger := zerolog.Nop()

	stream := "test:stream"
	group := "test-group"
	cfg := DefaultConsumerConfig(stream, group)

	require.NoError(t, EnsureConsumerGroup(ctx, rdb, stream, group))

	// Add 3 messages.
	rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]any{"tenant_id": "ok-1", "data": "d1"},
	})
	rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]any{"tenant_id": "fail", "data": "d2"},
	})
	rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]any{"tenant_id": "ok-2", "data": "d3"},
	})

	streams, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: "c1",
		Streams:  []string{stream, ">"},
		Count:    10,
		Block:    0,
	}).Result()
	require.NoError(t, err)

	// Second message fails, others succeed.
	handler := func(_ context.Context, tenantID, _ string) error {
		if tenantID == "fail" {
			return fmt.Errorf("boom")
		}
		return nil
	}

	processStreams(ctx, rdb, cfg, streams, handler, logger)

	// Only the failed message should remain pending.
	pending, err := rdb.XPending(ctx, stream, group).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), pending.Count)
}

func TestProcessStreams_EmptyStreams_NoHandlerCalls(t *testing.T) {
	rdb, _ := setupRedis(t)
	ctx := context.Background()
	logger := zerolog.Nop()

	cfg := DefaultConsumerConfig("test:stream", "test-group")

	called := false
	handler := func(_ context.Context, _, _ string) error {
		called = true
		return nil
	}

	// Empty stream list — nothing to process.
	processStreams(ctx, rdb, cfg, []redis.XStream{}, handler, logger)
	assert.False(t, called)
}

func TestProcessStreams_MissingFields_HandlerGetsZeroValues(t *testing.T) {
	rdb, _ := setupRedis(t)
	ctx := context.Background()
	logger := zerolog.Nop()

	stream := "test:stream"
	group := "test-group"
	cfg := DefaultConsumerConfig(stream, group)

	require.NoError(t, EnsureConsumerGroup(ctx, rdb, stream, group))

	// Add message without tenant_id or data fields.
	rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]any{"some_other_field": "value"},
	})

	streams, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: "c1",
		Streams:  []string{stream, ">"},
		Count:    10,
		Block:    0,
	}).Result()
	require.NoError(t, err)

	var gotTenantID, gotData string
	handler := func(_ context.Context, tenantID, data string) error {
		gotTenantID = tenantID
		gotData = data
		return nil
	}

	processStreams(ctx, rdb, cfg, streams, handler, logger)

	// Type assertion on missing keys yields zero values.
	assert.Equal(t, "", gotTenantID)
	assert.Equal(t, "", gotData)
}

// ── RunConsumer tests ───────────────────────────────────────────────────────

func TestRunConsumer_ProcessesMessagesUntilCanceled(t *testing.T) {
	rdb, _ := setupRedis(t)
	ctx, cancel := context.WithCancel(context.Background())
	logger := zerolog.Nop()

	stream := "test:stream"
	group := "test-group"
	cfg := ConsumerConfig{
		Stream:    stream,
		Group:     group,
		BatchSize: 10,
		BlockTime: 50 * time.Millisecond, // short block for fast test
	}

	require.NoError(t, EnsureConsumerGroup(ctx, rdb, stream, group))

	// Add a message before starting the consumer.
	rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]any{"tenant_id": "t1", "data": "d1"},
	})

	var mu sync.Mutex
	var calls []string
	handler := func(_ context.Context, tenantID, _ string) error {
		mu.Lock()
		calls = append(calls, tenantID)
		mu.Unlock()
		return nil
	}

	// Run consumer in a goroutine.
	done := make(chan struct{})
	go func() {
		RunConsumer(ctx, rdb, cfg, "consumer-1", handler, logger)
		close(done)
	}()

	// Wait for the first message to be processed.
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(calls) >= 1
	}, 2*time.Second, 10*time.Millisecond)

	// Add another message while consumer is running.
	rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]any{"tenant_id": "t2", "data": "d2"},
	})

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(calls) >= 2
	}, 2*time.Second, 10*time.Millisecond)

	mu.Lock()
	assert.Equal(t, "t1", calls[0])
	assert.Equal(t, "t2", calls[1])
	mu.Unlock()

	// Cancel and verify consumer exits.
	cancel()
	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("RunConsumer did not exit after context cancellation")
	}
}

func TestRunConsumer_StopsOnContextCancel(t *testing.T) {
	rdb, _ := setupRedis(t)
	ctx, cancel := context.WithCancel(context.Background())
	logger := zerolog.Nop()

	stream := "test:stream"
	group := "test-group"
	cfg := ConsumerConfig{
		Stream:    stream,
		Group:     group,
		BatchSize: 10,
		BlockTime: 50 * time.Millisecond,
	}

	require.NoError(t, EnsureConsumerGroup(ctx, rdb, stream, group))

	handler := func(_ context.Context, _, _ string) error { return nil }

	done := make(chan struct{})
	go func() {
		RunConsumer(ctx, rdb, cfg, "consumer-1", handler, logger)
		close(done)
	}()

	// Cancel immediately.
	cancel()

	select {
	case <-done:
		// Consumer exited as expected.
	case <-time.After(2 * time.Second):
		t.Fatal("RunConsumer did not exit after context cancellation")
	}
}
