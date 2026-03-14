// Package syncer handles periodic schema collection from Odoo and pushing
// snapshots to the central server. It uses SHA-256 hashing to detect changes
// so unchanged schemas are never re-uploaded (delta detection).
package syncer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"Intelligent_Dev_ToolKit_Odoo/internal/agent/collector"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/odoo"

	"github.com/rs/zerolog"
)

// schemaPayload mirrors the server's StoreSchemaRequest body.
// Kept internal — we marshal it ourselves so we control ordering for the hash.
type schemaPayload struct {
	EnvID       string `json:"env_id"`
	Models      any    `json:"models"`
	ACLRules    any    `json:"acl_rules"`
	RecordRules any    `json:"record_rules"`
	ModelCount  int    `json:"model_count"`
	FieldCount  int    `json:"field_count"`
}

// Syncer collects schema from Odoo on a configurable interval, computes a
// content hash, and only POSTs to the server when the schema has changed.
type Syncer struct {
	odooClient *odoo.Client
	httpClient *http.Client
	serverURL  string // base URL, e.g. "http://localhost:8080"
	apiKey     string
	envID      string // UUID of the environment registered on the server

	mu       sync.Mutex
	lastHash string // hex-encoded SHA-256 of last pushed payload

	logger zerolog.Logger
}

// New creates a Syncer. serverURL must not have a trailing slash.
func New(odooClient *odoo.Client, serverURL, apiKey, envID string, logger zerolog.Logger) *Syncer {
	return &Syncer{
		odooClient: odooClient,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		serverURL:  serverURL,
		apiKey:     apiKey,
		envID:      envID,
		logger:     logger.With().Str("component", "syncer").Logger(),
	}
}

// RunOnce performs a single collect → diff → push cycle.
// Returns nil even when no changes are detected (that is not an error).
func (s *Syncer) RunOnce(ctx context.Context) error {
	s.logger.Info().Msg("collecting schema from Odoo")

	// ── 1. Collect models + fields ─────────────────────────────────────────
	models, err := collector.CollectModels(ctx, s.odooClient)
	if err != nil {
		return fmt.Errorf("collect models: %w", err)
	}

	// ── 2. Collect ACL + record rules ──────────────────────────────────────
	aclRules, recordRules, err := collector.CollectACLAndRules(ctx, s.odooClient)
	if err != nil {
		return fmt.Errorf("collect acl: %w", err)
	}

	// Count fields by summing field_ids lengths stored per model (best-effort).
	modelCount := len(models)

	// ── 3. Build payload + hash ────────────────────────────────────────────
	payload := schemaPayload{
		EnvID:       s.envID,
		Models:      models,
		ACLRules:    aclRules,
		RecordRules: recordRules,
		ModelCount:  modelCount,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	hash := computeHash(body)

	// ── 4. Delta check ─────────────────────────────────────────────────────
	s.mu.Lock()
	same := s.lastHash == hash
	s.mu.Unlock()

	if same {
		s.logger.Info().Msg("schema unchanged, skipping push")
		return nil
	}

	// ── 5. Push to server ──────────────────────────────────────────────────
	if err := s.push(ctx, body); err != nil {
		return fmt.Errorf("push schema: %w", err)
	}

	s.mu.Lock()
	s.lastHash = hash
	s.mu.Unlock()

	s.logger.Info().
		Int("models", modelCount).
		Msg("schema snapshot pushed successfully")

	return nil
}

// RunLoop runs RunOnce immediately then repeats every interval until ctx is
// canceled. Errors from individual cycles are logged but do not stop the loop.
func (s *Syncer) RunLoop(ctx context.Context, interval time.Duration) {
	s.logger.Info().Dur("interval", interval).Msg("starting periodic schema sync")

	// Initial run.
	if err := s.RunOnce(ctx); err != nil {
		s.logger.Error().Err(err).Msg("schema sync failed")
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info().Msg("schema sync loop stopped")
			return
		case <-ticker.C:
			if err := s.RunOnce(ctx); err != nil {
				s.logger.Error().Err(err).Msg("schema sync failed")
			}
		}
	}
}

// push sends the marshaled payload to POST /api/v1/agent/schema.
func (s *Syncer) push(ctx context.Context, body []byte) error {
	endpoint := s.serverURL + "/api/v1/agent/schema"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "ApiKey "+s.apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return nil
}

// computeHash returns the hex-encoded SHA-256 of data.
func computeHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
