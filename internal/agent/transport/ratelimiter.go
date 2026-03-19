package transport

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// RateLimiterConfig controls the agent-side rate limiter.
// Both limits are enforced independently — exceeding either one causes
// Allow to return false.
type RateLimiterConfig struct {
	// MaxBytesPerMinute is the maximum compressed bytes the agent may send
	// per minute. 0 = unlimited.
	MaxBytesPerMinute int64

	// MaxBatchesPerMinute is the maximum number of batches the agent may
	// send per minute. 0 = unlimited.
	MaxBatchesPerMinute int
}

// RateLimiterStats is a snapshot of the limiter's current window.
type RateLimiterStats struct {
	BytesUsed   int64
	BatchesUsed int
	BytesLimit  int64
	BatchLimit  int
}

// RateLimiter enforces per-minute byte and batch limits on the agent's
// outbound traffic. It resets its counters every minute. It is safe for
// concurrent use and never blocks — if the limit is exceeded the batch
// is simply dropped.
type RateLimiter struct {
	maxBytes   int64 // 0 = unlimited
	maxBatches int32 // 0 = unlimited

	currentBytes   atomic.Int64
	currentBatches atomic.Int32

	mu      sync.Mutex // protects stopped
	stopped bool
	cancel  context.CancelFunc
}

// NewRateLimiter creates a RateLimiter and starts the internal reset ticker.
// Call Stop() when done to release the goroutine.
func NewRateLimiter(cfg RateLimiterConfig) *RateLimiter {
	ctx, cancel := context.WithCancel(context.Background())
	rl := &RateLimiter{
		maxBytes:   cfg.MaxBytesPerMinute,
		maxBatches: int32(cfg.MaxBatchesPerMinute), //nolint:gosec // G115: config value; overflow not realistic
		cancel:     cancel,
	}
	go rl.resetLoop(ctx)
	return rl
}

// Allow checks whether a batch of the given compressed size may be sent.
// If allowed, it records the bytes and batch count and returns true.
// If either limit would be exceeded, it returns false and the caller
// should drop the batch.
func (rl *RateLimiter) Allow(compressedBytes int64) bool {
	// Check bytes limit.
	maxB := atomic.LoadInt64(&rl.maxBytes)
	if maxB > 0 {
		newBytes := rl.currentBytes.Load() + compressedBytes
		if newBytes > maxB {
			return false
		}
	}

	// Check batch limit.
	maxBat := atomic.LoadInt32(&rl.maxBatches)
	if maxBat > 0 {
		if rl.currentBatches.Load() >= maxBat {
			return false
		}
	}

	// Record usage. There is a small race window between the checks above
	// and the adds below, but for a single-sender agent this is fine —
	// exact enforcement is not required, just approximate throttling.
	rl.currentBytes.Add(compressedBytes)
	rl.currentBatches.Add(1)
	return true
}

// Stats returns a snapshot of the current window's usage.
func (rl *RateLimiter) Stats() RateLimiterStats {
	return RateLimiterStats{
		BytesUsed:   rl.currentBytes.Load(),
		BatchesUsed: int(rl.currentBatches.Load()),
		BytesLimit:  atomic.LoadInt64(&rl.maxBytes),
		BatchLimit:  int(atomic.LoadInt32(&rl.maxBatches)),
	}
}

// Stop releases the reset goroutine. Safe to call multiple times.
func (rl *RateLimiter) Stop() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if !rl.stopped {
		rl.stopped = true
		rl.cancel()
	}
}

// UpdateLimits hot-reloads the rate limits without restarting the limiter.
// Counters are NOT reset — the new limits take effect immediately for
// subsequent Allow() calls within the current minute window.
func (rl *RateLimiter) UpdateLimits(maxBytesPerMinute int64, maxBatchesPerMinute int) {
	atomic.StoreInt64(&rl.maxBytes, maxBytesPerMinute)
	atomic.StoreInt32(&rl.maxBatches, int32(maxBatchesPerMinute)) //nolint:gosec // G115: config value; overflow not realistic
}

// Reset manually resets the counters. Useful for testing.
func (rl *RateLimiter) Reset() {
	rl.currentBytes.Store(0)
	rl.currentBatches.Store(0)
}

// resetLoop resets counters every minute.
func (rl *RateLimiter) resetLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.Reset()
		case <-ctx.Done():
			return
		}
	}
}
