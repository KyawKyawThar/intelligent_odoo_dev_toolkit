package transport

import (
	"testing"
)

func newTestLimiter(maxBytes int64, maxBatches int) *RateLimiter {
	rl := NewRateLimiter(RateLimiterConfig{
		MaxBytesPerMinute:   maxBytes,
		MaxBatchesPerMinute: maxBatches,
	})
	// We don't need the background ticker for unit tests.
	return rl
}

// ── Bytes limit ─────────────────────────────────────────────────────────────

func TestAllow_BytesLimit_Allows(t *testing.T) {
	rl := newTestLimiter(1000, 0)
	defer rl.Stop()

	if !rl.Allow(500) {
		t.Fatal("should allow 500 bytes within 1000 limit")
	}
	if !rl.Allow(400) {
		t.Fatal("should allow 400 more bytes (total 900)")
	}
}

func TestAllow_BytesLimit_Denies(t *testing.T) {
	rl := newTestLimiter(1000, 0)
	defer rl.Stop()

	rl.Allow(800)
	if rl.Allow(300) {
		t.Fatal("should deny: 800 + 300 = 1100 > 1000")
	}
}

func TestAllow_BytesLimit_ExactBoundary(t *testing.T) {
	rl := newTestLimiter(1000, 0)
	defer rl.Stop()

	if !rl.Allow(1000) {
		t.Fatal("should allow exactly at limit")
	}
	if rl.Allow(1) {
		t.Fatal("should deny: 1000 + 1 > 1000")
	}
}

// ── Batches limit ───────────────────────────────────────────────────────────

func TestAllow_BatchesLimit_Allows(t *testing.T) {
	rl := newTestLimiter(0, 4)
	defer rl.Stop()

	for i := 0; i < 4; i++ {
		if !rl.Allow(100) {
			t.Fatalf("should allow batch %d within limit of 4", i+1)
		}
	}
}

func TestAllow_BatchesLimit_Denies(t *testing.T) {
	rl := newTestLimiter(0, 4)
	defer rl.Stop()

	for i := 0; i < 4; i++ {
		rl.Allow(100)
	}
	if rl.Allow(100) {
		t.Fatal("should deny 5th batch when limit is 4")
	}
}

// ── Both limits ─────────────────────────────────────────────────────────────

func TestAllow_BothLimits_BytesHitsFirst(t *testing.T) {
	rl := newTestLimiter(500, 10)
	defer rl.Stop()

	rl.Allow(400)
	if rl.Allow(200) {
		t.Fatal("should deny: bytes exceeded (600 > 500) even though batches ok")
	}
}

func TestAllow_BothLimits_BatchesHitsFirst(t *testing.T) {
	rl := newTestLimiter(1_000_000, 2)
	defer rl.Stop()

	rl.Allow(100)
	rl.Allow(100)
	if rl.Allow(100) {
		t.Fatal("should deny: batches exceeded (3 > 2) even though bytes ok")
	}
}

// ── Unlimited ───────────────────────────────────────────────────────────────

func TestAllow_Unlimited_AlwaysAllows(t *testing.T) {
	rl := newTestLimiter(0, 0)
	defer rl.Stop()

	for i := 0; i < 1000; i++ {
		if !rl.Allow(1_000_000) {
			t.Fatalf("unlimited limiter should always allow, denied at %d", i)
		}
	}
}

// ── Reset ───────────────────────────────────────────────────────────────────

func TestReset_ClearsCounters(t *testing.T) {
	rl := newTestLimiter(1000, 4)
	defer rl.Stop()

	rl.Allow(800)
	rl.Allow(100)
	rl.Allow(100)

	// Should be at 1000 bytes and 3 batches.
	stats := rl.Stats()
	if stats.BytesUsed != 1000 {
		t.Errorf("bytes = %d, want 1000", stats.BytesUsed)
	}
	if stats.BatchesUsed != 3 {
		t.Errorf("batches = %d, want 3", stats.BatchesUsed)
	}

	rl.Reset()

	stats = rl.Stats()
	if stats.BytesUsed != 0 {
		t.Errorf("after reset: bytes = %d, want 0", stats.BytesUsed)
	}
	if stats.BatchesUsed != 0 {
		t.Errorf("after reset: batches = %d, want 0", stats.BatchesUsed)
	}

	// Should be able to allow again.
	if !rl.Allow(800) {
		t.Fatal("should allow after reset")
	}
}

// ── Stats ───────────────────────────────────────────────────────────────────

func TestStats_ReportsLimits(t *testing.T) {
	rl := newTestLimiter(5000, 10)
	defer rl.Stop()

	stats := rl.Stats()
	if stats.BytesLimit != 5000 {
		t.Errorf("BytesLimit = %d, want 5000", stats.BytesLimit)
	}
	if stats.BatchLimit != 10 {
		t.Errorf("BatchLimit = %d, want 10", stats.BatchLimit)
	}
}

func TestStats_TracksUsage(t *testing.T) {
	rl := newTestLimiter(0, 0)
	defer rl.Stop()

	rl.Allow(42)
	rl.Allow(58)

	stats := rl.Stats()
	if stats.BytesUsed != 100 {
		t.Errorf("BytesUsed = %d, want 100", stats.BytesUsed)
	}
	if stats.BatchesUsed != 2 {
		t.Errorf("BatchesUsed = %d, want 2", stats.BatchesUsed)
	}
}

// ── Stop idempotent ─────────────────────────────────────────────────────────

func TestStop_Idempotent(t *testing.T) {
	rl := newTestLimiter(1000, 4)
	rl.Stop()
	rl.Stop() // should not panic
}

// ── Denied batch does not count ─────────────────────────────────────────────

func TestAllow_DeniedDoesNotCount(t *testing.T) {
	rl := newTestLimiter(100, 0)
	defer rl.Stop()

	rl.Allow(90)            // accepted: 90 bytes, 1 batch
	denied := rl.Allow(200) // denied: would be 290 > 100
	if denied {
		t.Fatal("should be denied")
	}

	stats := rl.Stats()
	if stats.BytesUsed != 90 {
		t.Errorf("BytesUsed = %d, want 90 (denied batch should not count)", stats.BytesUsed)
	}
	if stats.BatchesUsed != 1 {
		t.Errorf("BatchesUsed = %d, want 1", stats.BatchesUsed)
	}
}
