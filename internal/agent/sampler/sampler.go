// Package sampler decides which pipeline events are kept or dropped based on
// the current sampling mode. It supports three modes that map to environment
// presets (development / staging / production) and can be hot-reloaded when
// the cloud pushes updated feature flags.
//
// Modes:
//   - full:            keep every event (development)
//   - sampled:         keep a percentage + always capture errors/slow/n1 (staging)
//   - aggregated_only: only keep errors/slow/n1 as raw events (production)
package sampler

import (
	"math/rand/v2"
	"strings"
	"sync"
)

// Mode controls the sampling strategy.
type Mode string

const (
	ModeFull           Mode = "full"
	ModeSampled        Mode = "sampled"
	ModeAggregatedOnly Mode = "aggregated_only"
)

// Config controls what the sampler keeps or drops.
// It is designed to be pushed from the cloud via feature flags.
type Config struct {
	// Mode selects the sampling strategy.
	Mode Mode `json:"mode"`

	// SampleRate is the probability (0.0–1.0) that a non-critical event is
	// kept in "sampled" mode. Ignored in "full" and "aggregated_only".
	SampleRate float64 `json:"sample_rate"`

	// AlwaysCapture lists event categories that bypass sampling entirely.
	// Common values: "error", "slow_query", "n1".
	AlwaysCapture []string `json:"always_capture"`

	// SlowThresholdMS marks events with DurationMS above this value as slow,
	// causing them to bypass sampling. 0 disables the threshold.
	SlowThresholdMS int `json:"slow_threshold_ms"`
}

// EventInfo describes a single pipeline event so the sampler can decide
// whether to keep or drop it. Callers construct this from their concrete
// event types (ErrorEvent, ORMEvent, etc.).
type EventInfo struct {
	// Category classifies the event: "error", "orm", "sql", "profiler".
	Category string

	// DurationMS is the operation duration in milliseconds (0 if N/A).
	DurationMS int

	// IsError is true for error/exception events.
	IsError bool

	// IsN1 is true when an N+1 query pattern was detected.
	IsN1 bool
}

// Sampler decides which events pass through the pipeline. It is safe for
// concurrent use and supports hot-reloading the config without restart.
type Sampler struct {
	mu     sync.RWMutex
	config Config

	// alwaysSet is a precomputed lookup from config.AlwaysCapture for O(1) checks.
	alwaysSet map[string]struct{}
}

// New creates a Sampler with the given initial configuration.
func New(cfg Config) *Sampler {
	s := &Sampler{}
	s.applyConfig(cfg)
	return s
}

// UpdateConfig hot-reloads the sampler with a new configuration.
// This is called when the cloud pushes updated feature flags.
func (s *Sampler) UpdateConfig(cfg Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.applyConfig(cfg)
}

// CurrentConfig returns a copy of the current configuration.
func (s *Sampler) CurrentConfig() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// Allow returns true if the event should be kept (sent to the server as a
// raw event). Dropped events are still counted in aggregated stats — this
// only controls raw event retention.
func (s *Sampler) Allow(info EventInfo) bool {
	s.mu.RLock()
	cfg := s.config
	always := s.alwaysSet
	s.mu.RUnlock()

	// Always-capture check: errors, slow queries, N+1 patterns, or any
	// category explicitly listed in AlwaysCapture.
	if isCritical(info, cfg.SlowThresholdMS, always) {
		return true
	}

	switch cfg.Mode {
	case ModeFull:
		return true

	case ModeSampled:
		return rand.Float64() < cfg.SampleRate //nolint:gosec // G404: sampling — crypto strength not needed

	case ModeAggregatedOnly:
		// Only critical events (handled above) pass through.
		return false

	default:
		// Unknown mode — safe default: keep everything.
		return true
	}
}

// ── Preset configs ──────────────────────────────────────────────────────────

// DefaultDevelopment returns the default config for development environments.
func DefaultDevelopment() Config {
	return Config{
		Mode:            ModeFull,
		SampleRate:      1.0,
		SlowThresholdMS: 50,
	}
}

// DefaultStaging returns the default config for staging environments.
func DefaultStaging() Config {
	return Config{
		Mode:            ModeSampled,
		SampleRate:      0.25,
		AlwaysCapture:   []string{"error", "slow_query", "n1"},
		SlowThresholdMS: 100,
	}
}

// DefaultProduction returns the default config for production environments.
func DefaultProduction() Config {
	return Config{
		Mode:            ModeAggregatedOnly,
		SampleRate:      0.05,
		AlwaysCapture:   []string{"error", "slow_query", "n1"},
		SlowThresholdMS: 200,
	}
}

// ForEnvironment returns the default sampler config for the given environment
// name ("development", "staging", "production"). Unknown names get the
// development preset.
func ForEnvironment(env string) Config {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "staging":
		return DefaultStaging()
	case "production":
		return DefaultProduction()
	default:
		return DefaultDevelopment()
	}
}

// ── internal ────────────────────────────────────────────────────────────────

// applyConfig sets the config and rebuilds the always-capture set.
// Caller must hold s.mu.
func (s *Sampler) applyConfig(cfg Config) {
	s.config = cfg
	s.alwaysSet = make(map[string]struct{}, len(cfg.AlwaysCapture))
	for _, cat := range cfg.AlwaysCapture {
		s.alwaysSet[strings.ToLower(strings.TrimSpace(cat))] = struct{}{}
	}
}

// isCritical returns true if the event matches any always-capture criteria.
func isCritical(info EventInfo, slowThreshold int, alwaysSet map[string]struct{}) bool {
	if info.IsError {
		return true
	}
	if info.IsN1 {
		return true
	}
	if slowThreshold > 0 && info.DurationMS > slowThreshold {
		return true
	}
	if _, ok := alwaysSet[strings.ToLower(info.Category)]; ok {
		return true
	}
	return false
}
