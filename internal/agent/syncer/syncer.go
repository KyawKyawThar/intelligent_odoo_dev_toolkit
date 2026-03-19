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
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"Intelligent_Dev_ToolKit_Odoo/internal/agent/collector"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/creds"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/odoo"

	"github.com/rs/zerolog"
)

// schemaPayload mirrors the server's StoreSchemaRequest body.
// Kept internal — we marshal it ourselves so we control ordering for the hash.
type schemaPayload struct {
	EnvID      string `json:"env_id,omitempty"`
	Version    string `json:"version"`
	Models     any    `json:"models"`
	ModelCount int    `json:"model_count"`
	FieldCount int    `json:"field_count"`
}

// Syncer collects schema from Odoo on a configurable interval, computes a
// content hash, and only POSTs to the server when the schema has changed.
type Syncer struct {
	odooClient  *odoo.Client
	httpClient  *http.Client
	serverURL   string // base URL, e.g. "http://localhost:8080"
	creds       creds.Provider
	envID       string // UUID of the environment registered on the server
	odooVersion string // e.g. "17.0"

	mu       sync.Mutex
	lastHash string // hex-encoded SHA-256 of last pushed payload

	logger zerolog.Logger
}

// New creates a Syncer. serverURL must not have a trailing slash.
func New(odooClient *odoo.Client, serverURL string, cp creds.Provider, envID, odooVersion string, logger zerolog.Logger) *Syncer {
	return &Syncer{
		odooClient:  odooClient,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		serverURL:   serverURL,
		creds:       cp,
		envID:       envID,
		odooVersion: odooVersion,
		logger:      logger.With().Str("component", "syncer").Logger(),
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

	// ── 3. Merge accesses/rules into each model by model ID ───────────────
	mergeAccessesAndRules(models, aclRules, recordRules)

	// Build models map keyed by technical name, converting each model to
	// match the server's SchemaModel shape exactly.
	modelsMap := make(map[string]any, len(models))
	for _, m := range models {
		name, ok := m["model"].(string)
		if !ok {
			continue
		}
		modelsMap[name] = convertModel(m)
	}

	modelCount := len(models)

	// ── 4. Build payload + hash ────────────────────────────────────────────
	payload := schemaPayload{
		EnvID:      s.envID,
		Version:    s.odooVersion,
		Models:     modelsMap,
		ModelCount: modelCount,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	hash := computeHash(body)

	// ── 5. Delta check ─────────────────────────────────────────────────────
	s.mu.Lock()
	same := s.lastHash == hash
	s.mu.Unlock()

	if same {
		s.logger.Info().Msg("schema unchanged, skipping push")
		return nil
	}

	// ── 6. Push to server ──────────────────────────────────────────────────
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
// On 401 it attempts to refresh credentials and retries once.
func (s *Syncer) push(ctx context.Context, body []byte) error {
	endpoint := s.serverURL + "/api/v1/agent/schema"

	status, respBody, err := s.doPost(ctx, endpoint, body, s.creds.APIKey())
	if err != nil {
		return err
	}

	// Auto-refresh on 401 and retry once.
	if status == http.StatusUnauthorized {
		newKey, refreshErr := s.creds.RefreshOnUnauthorized()
		if refreshErr != nil {
			s.logger.Error().Err(refreshErr).Msg("credential refresh failed")
			return fmt.Errorf("server returned 401 and refresh failed: %w", refreshErr)
		}
		s.logger.Info().Msg("credentials refreshed, retrying schema push")
		status, respBody, err = s.doPost(ctx, endpoint, body, newKey)
		if err != nil {
			return err
		}
	}

	if status < 200 || status >= 300 {
		s.logger.Error().
			Int("status", status).
			Str("body", string(respBody)).
			Msg("schema push rejected by server")
		return fmt.Errorf("server returned %d: %s", status, string(respBody))
	}
	return nil
}

// doPost performs a single POST and returns the status code, response body, and any transport error.
func (s *Syncer) doPost(ctx context.Context, endpoint string, body []byte, apiKey string) (statusCode int, respBody []byte, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "ApiKey "+apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	respBody, _ = io.ReadAll(io.LimitReader(resp.Body, 1024)) //nolint:errcheck // best-effort read for logging
	return resp.StatusCode, respBody, nil
}

// convertModel transforms a raw Odoo model map into the shape the server's
// StoreSchemaRequest expects: fields as map[string]SchemaModelField, and only
// the keys the DTO defines (model, name, fields, accesses, rules).
func convertModel(m map[string]any) map[string]any {
	out := map[string]any{
		"model": m["model"],
		"name":  m["name"],
	}

	// Convert fields from []map[string]any to map[string]SchemaModelField.
	fieldsMap := make(map[string]any)
	if rawFields, ok := m["fields"].([]map[string]any); ok {
		for _, f := range rawFields {
			fname, _ := f["name"].(string) //nolint:errcheck // type assertion; zero value is fine
			if fname == "" {
				continue
			}
			fieldsMap[fname] = map[string]any{
				"type":     f["ttype"],
				"string":   f["name"],
				"required": f["required"],
			}
		}
	}
	out["fields"] = fieldsMap

	// Pass through accesses/rules (already converted by mergeAccessesAndRules).
	if v, ok := m["accesses"]; ok {
		out["accesses"] = v
	}
	if v, ok := m["rules"]; ok {
		out["rules"] = v
	}

	return out
}

// mergeAccessesAndRules attaches ACL rules and record rules to their
// respective model entries by matching on the model ID field.
func mergeAccessesAndRules(models []map[string]any, acls []odoo.IrModelAccess, rules []odoo.IrRule) {
	// Build index: model ID → index in models slice.
	idToIdx := make(map[int]int, len(models))
	for i, m := range models {
		if id, ok := m["id"].(int); ok {
			idToIdx[id] = i
		}
	}

	// Group ACL rules by model — convert to the shape the server expects.
	for _, a := range acls {
		idx, ok := idToIdx[a.ModelID]
		if !ok {
			continue
		}
		entry := map[string]any{
			"group_id":    strconv.Itoa(a.GroupID),
			"perm_read":   a.PermRead,
			"perm_write":  a.PermWrite,
			"perm_create": a.PermCreate,
			"perm_unlink": a.PermUnlink,
		}
		existing, _ := models[idx]["accesses"].([]any) //nolint:errcheck // type assertion; nil is fine for append
		models[idx]["accesses"] = append(existing, entry)
	}

	// Group record rules by model — convert to the shape the server expects.
	for _, r := range rules {
		idx, ok := idToIdx[r.ModelID]
		if !ok {
			continue
		}
		entry := map[string]any{
			"name":        r.Name,
			"domain":      r.Domain,
			"perm_read":   r.PermRead,
			"perm_write":  r.PermWrite,
			"perm_create": r.PermCreate,
			"perm_unlink": r.PermUnlink,
		}
		existing, _ := models[idx]["rules"].([]any) //nolint:errcheck // type assertion; nil is fine for append
		models[idx]["rules"] = append(existing, entry)
	}
}

// computeHash returns the hex-encoded SHA-256 of data.
func computeHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
