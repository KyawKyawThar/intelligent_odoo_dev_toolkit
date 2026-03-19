package sampler

import (
	"testing"
)

// ── Mode: full ──────────────────────────────────────────────────────────────

func TestFullMode_AllowsEverything(t *testing.T) {
	s := New(DefaultDevelopment())

	events := []EventInfo{
		{Category: "orm", DurationMS: 5},
		{Category: "sql", DurationMS: 1},
		{Category: "error", IsError: true},
		{Category: "profiler", DurationMS: 200},
	}

	for _, ev := range events {
		if !s.Allow(ev) {
			t.Errorf("full mode should allow %+v", ev)
		}
	}
}

// ── Mode: sampled ───────────────────────────────────────────────────────────

func TestSampledMode_AlwaysCapturesErrors(t *testing.T) {
	s := New(Config{
		Mode:            ModeSampled,
		SampleRate:      0.0, // drop everything except critical
		AlwaysCapture:   []string{"error", "slow_query", "n1"},
		SlowThresholdMS: 100,
	})

	ev := EventInfo{Category: "error", IsError: true}
	for i := 0; i < 100; i++ {
		if !s.Allow(ev) {
			t.Fatal("sampled mode must always capture errors")
		}
	}
}

func TestSampledMode_AlwaysCapturesSlowQueries(t *testing.T) {
	s := New(Config{
		Mode:            ModeSampled,
		SampleRate:      0.0,
		SlowThresholdMS: 100,
	})

	ev := EventInfo{Category: "sql", DurationMS: 150}
	for i := 0; i < 100; i++ {
		if !s.Allow(ev) {
			t.Fatal("sampled mode must always capture slow queries")
		}
	}
}

func TestSampledMode_AlwaysCapturesN1(t *testing.T) {
	s := New(Config{
		Mode:       ModeSampled,
		SampleRate: 0.0,
	})

	ev := EventInfo{Category: "orm", IsN1: true}
	for i := 0; i < 100; i++ {
		if !s.Allow(ev) {
			t.Fatal("sampled mode must always capture N+1 patterns")
		}
	}
}

func TestSampledMode_DropsAtZeroRate(t *testing.T) {
	s := New(Config{
		Mode:       ModeSampled,
		SampleRate: 0.0,
	})

	ev := EventInfo{Category: "orm", DurationMS: 5}
	for i := 0; i < 200; i++ {
		if s.Allow(ev) {
			t.Fatal("sampled mode with rate=0.0 should drop non-critical events")
		}
	}
}

func TestSampledMode_KeepsAtFullRate(t *testing.T) {
	s := New(Config{
		Mode:       ModeSampled,
		SampleRate: 1.0,
	})

	ev := EventInfo{Category: "orm", DurationMS: 5}
	for i := 0; i < 200; i++ {
		if !s.Allow(ev) {
			t.Fatal("sampled mode with rate=1.0 should keep all events")
		}
	}
}

func TestSampledMode_RespectsSampleRate(t *testing.T) {
	s := New(Config{
		Mode:       ModeSampled,
		SampleRate: 0.5,
	})

	ev := EventInfo{Category: "orm", DurationMS: 5}
	kept := 0
	const trials = 10000
	for i := 0; i < trials; i++ {
		if s.Allow(ev) {
			kept++
		}
	}

	ratio := float64(kept) / float64(trials)
	// Allow generous margin for randomness.
	if ratio < 0.4 || ratio > 0.6 {
		t.Errorf("expected ~50%% kept, got %.1f%%", ratio*100)
	}
}

// ── Mode: aggregated_only ───────────────────────────────────────────────────

func TestAggregatedOnly_DropsNonCritical(t *testing.T) {
	s := New(DefaultProduction())

	ev := EventInfo{Category: "orm", DurationMS: 5}
	for i := 0; i < 200; i++ {
		if s.Allow(ev) {
			t.Fatal("aggregated_only should drop non-critical events")
		}
	}
}

func TestAggregatedOnly_KeepsErrors(t *testing.T) {
	s := New(DefaultProduction())

	ev := EventInfo{Category: "error", IsError: true}
	for i := 0; i < 100; i++ {
		if !s.Allow(ev) {
			t.Fatal("aggregated_only must keep errors")
		}
	}
}

func TestAggregatedOnly_KeepsSlowQueries(t *testing.T) {
	s := New(Config{
		Mode:            ModeAggregatedOnly,
		SlowThresholdMS: 200,
	})

	ev := EventInfo{Category: "sql", DurationMS: 300}
	if !s.Allow(ev) {
		t.Fatal("aggregated_only must keep slow queries")
	}
}

func TestAggregatedOnly_KeepsN1(t *testing.T) {
	s := New(DefaultProduction())

	ev := EventInfo{Category: "orm", IsN1: true}
	if !s.Allow(ev) {
		t.Fatal("aggregated_only must keep N+1 patterns")
	}
}

// ── AlwaysCapture by category ───────────────────────────────────────────────

func TestAlwaysCapture_MatchesByCategory(t *testing.T) {
	s := New(Config{
		Mode:          ModeAggregatedOnly,
		AlwaysCapture: []string{"profiler"},
	})

	ev := EventInfo{Category: "profiler", DurationMS: 10}
	if !s.Allow(ev) {
		t.Fatal("category in AlwaysCapture list should always be kept")
	}
}

func TestAlwaysCapture_CaseInsensitive(t *testing.T) {
	s := New(Config{
		Mode:          ModeAggregatedOnly,
		AlwaysCapture: []string{"Error"},
	})

	ev := EventInfo{Category: "error"}
	if !s.Allow(ev) {
		t.Fatal("AlwaysCapture should be case-insensitive")
	}
}

// ── Hot-reload ──────────────────────────────────────────────────────────────

func TestUpdateConfig_ChangesMode(t *testing.T) {
	s := New(DefaultDevelopment()) // full mode

	ev := EventInfo{Category: "orm", DurationMS: 5}
	if !s.Allow(ev) {
		t.Fatal("full mode should allow")
	}

	// Switch to aggregated_only — non-critical events should be dropped.
	s.UpdateConfig(DefaultProduction())

	for i := 0; i < 100; i++ {
		if s.Allow(ev) {
			t.Fatal("after switching to aggregated_only, non-critical should be dropped")
		}
	}
}

func TestCurrentConfig_ReturnsSnapshot(t *testing.T) {
	cfg := DefaultStaging()
	s := New(cfg)

	got := s.CurrentConfig()
	if got.Mode != ModeSampled {
		t.Errorf("expected ModeSampled, got %s", got.Mode)
	}
	if got.SampleRate != 0.25 {
		t.Errorf("expected 0.25, got %f", got.SampleRate)
	}
}

// ── ForEnvironment ──────────────────────────────────────────────────────────

func TestForEnvironment(t *testing.T) {
	tests := []struct {
		env  string
		want Mode
	}{
		{"development", ModeFull},
		{"staging", ModeSampled},
		{"production", ModeAggregatedOnly},
		{"PRODUCTION", ModeAggregatedOnly},
		{"  staging  ", ModeSampled},
		{"unknown", ModeFull},
		{"", ModeFull},
	}

	for _, tt := range tests {
		cfg := ForEnvironment(tt.env)
		if cfg.Mode != tt.want {
			t.Errorf("ForEnvironment(%q) = %s, want %s", tt.env, cfg.Mode, tt.want)
		}
	}
}

// ── SlowThreshold edge cases ────────────────────────────────────────────────

func TestSlowThreshold_ExactBoundary(t *testing.T) {
	s := New(Config{
		Mode:            ModeAggregatedOnly,
		SlowThresholdMS: 100,
	})

	// Exactly at threshold — NOT slow (must exceed, not equal).
	ev := EventInfo{Category: "sql", DurationMS: 100}
	if s.Allow(ev) {
		t.Fatal("event at exact threshold should not be considered slow")
	}

	// One over — slow.
	ev.DurationMS = 101
	if !s.Allow(ev) {
		t.Fatal("event above threshold should be considered slow")
	}
}

func TestSlowThreshold_ZeroDisables(t *testing.T) {
	s := New(Config{
		Mode:            ModeAggregatedOnly,
		SlowThresholdMS: 0, // disabled
	})

	ev := EventInfo{Category: "sql", DurationMS: 99999}
	if s.Allow(ev) {
		t.Fatal("slow threshold=0 should disable slow-query capture")
	}
}
