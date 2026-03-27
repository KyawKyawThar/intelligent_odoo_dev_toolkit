package worker

import (
	"context"
	"encoding/json"
	"testing"

	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Alert Mock Store ────────────────────────────────────────────────────────

type alertMockStore struct {
	db.Store // embed — unimplemented methods panic

	hasRecentAlertResult bool
	hasRecentAlertErr    error
	hasRecentAlertCalls  []db.HasRecentAlertParams

	createAlertCalls []db.CreateAlertWithDeliveryParams
	createAlertErr   error
	createdAlert     db.Alert
}

func (m *alertMockStore) HasRecentAlert(_ context.Context, arg db.HasRecentAlertParams) (bool, error) {
	m.hasRecentAlertCalls = append(m.hasRecentAlertCalls, arg)
	return m.hasRecentAlertResult, m.hasRecentAlertErr
}

func (m *alertMockStore) CreateAlertWithDeliveryTx(_ context.Context, arg db.CreateAlertWithDeliveryParams) (db.Alert, error) {
	m.createAlertCalls = append(m.createAlertCalls, arg)
	if m.createAlertErr != nil {
		return db.Alert{}, m.createAlertErr
	}
	alert := m.createdAlert
	if alert.ID == uuid.Nil {
		alert.ID = uuid.New()
	}
	alert.EnvID = arg.EnvID
	alert.Type = arg.Type
	alert.Severity = arg.Severity
	alert.Message = arg.Message
	alert.Metadata = arg.Metadata
	return alert, nil
}

// ── Helper ──────────────────────────────────────────────────────────────────

func newTestAlertWorker(store db.Store) *AlertWorker {
	logger := zerolog.Nop()
	return &AlertWorker{
		store:  store,
		config: AlertConfig{DedupeWindowMin: 10},
		logger: logger,
	}
}

// ── classifySeverity tests ──────────────────────────────────────────────────

func TestClassifySeverity_Warning(t *testing.T) {
	// 25% overhead with 20% threshold = 1.25x — warning
	assert.Equal(t, "warning", classifySeverity(25, 20))
}

func TestClassifySeverity_Critical(t *testing.T) {
	// 30% overhead with 20% threshold = 1.5x — critical
	assert.Equal(t, "critical", classifySeverity(30, 20))
}

func TestClassifySeverity_CriticalAbove(t *testing.T) {
	// 40% overhead with 20% threshold = 2.0x — critical
	assert.Equal(t, "critical", classifySeverity(40, 20))
}

func TestClassifySeverity_ExactThreshold(t *testing.T) {
	// At exactly 1.49x — still warning
	assert.Equal(t, "warning", classifySeverity(29.9, 20))
}

func TestClassifySeverity_ZeroThreshold(t *testing.T) {
	// Zero threshold — always warning (avoids division issues)
	assert.Equal(t, "warning", classifySeverity(50, 0))
}

// ── processMessage tests ────────────────────────────────────────────────────

func TestAlertProcessMessage_CreatesAlert(t *testing.T) {
	store := &alertMockStore{}
	w := newTestAlertWorker(store)

	tenantID := uuid.New()
	envID := uuid.New()
	budgetID := uuid.New()

	breach := ThresholdBreach{
		TenantID:     tenantID.String(),
		EnvID:        envID.String(),
		BudgetID:     budgetID.String(),
		Module:       "sale",
		Endpoint:     "/web/dataset/call_kw",
		OverheadPct:  45.5,
		ThresholdPct: 20,
		TotalMS:      1000,
		ModuleMS:     455,
	}
	data, err := json.Marshal(breach)
	require.NoError(t, err)

	err = w.processMessage(context.Background(), tenantID.String(), string(data))
	require.NoError(t, err)

	// Alert should have been created.
	require.Len(t, store.createAlertCalls, 1)
	call := store.createAlertCalls[0]
	assert.Equal(t, envID, call.EnvID)
	assert.Equal(t, tenantID, call.TenantID)
	assert.Equal(t, "budget_exceeded", call.Type)
	assert.Equal(t, "critical", call.Severity) // 45.5% exceeds critical threshold (30%)
	assert.Contains(t, call.Message, "sale")
	assert.Contains(t, call.Message, "45.5%")
	assert.Contains(t, call.Message, "20%")

	// Verify metadata.
	var meta map[string]any
	require.NoError(t, json.Unmarshal(call.Metadata, &meta))
	assert.Equal(t, "sale", meta["module"])
	assert.Equal(t, float64(45.5), meta["overhead_pct"])
	assert.Equal(t, float64(20), meta["threshold_pct"])
}

func TestAlertProcessMessage_WarningSeverity(t *testing.T) {
	store := &alertMockStore{}
	w := newTestAlertWorker(store)

	tenantID := uuid.New()
	envID := uuid.New()

	breach := ThresholdBreach{
		TenantID:     tenantID.String(),
		EnvID:        envID.String(),
		BudgetID:     uuid.New().String(),
		Module:       "stock",
		OverheadPct:  25.0, // 1.25x threshold
		ThresholdPct: 20,
	}
	data, _ := json.Marshal(breach)

	err := w.processMessage(context.Background(), tenantID.String(), string(data))
	require.NoError(t, err)

	require.Len(t, store.createAlertCalls, 1)
	assert.Equal(t, "warning", store.createAlertCalls[0].Severity)
}

func TestAlertProcessMessage_DuplicateSuppressed(t *testing.T) {
	store := &alertMockStore{
		hasRecentAlertResult: true, // duplicate exists
	}
	w := newTestAlertWorker(store)

	tenantID := uuid.New()
	breach := ThresholdBreach{
		TenantID:     tenantID.String(),
		EnvID:        uuid.New().String(),
		BudgetID:     uuid.New().String(),
		Module:       "sale",
		OverheadPct:  50.0,
		ThresholdPct: 20,
	}
	data, _ := json.Marshal(breach)

	err := w.processMessage(context.Background(), tenantID.String(), string(data))
	require.NoError(t, err)

	// Dedup check was called.
	require.Len(t, store.hasRecentAlertCalls, 1)
	assert.Equal(t, "budget_exceeded", store.hasRecentAlertCalls[0].Type)
	assert.Equal(t, int32(10), store.hasRecentAlertCalls[0].Column4) // 10 min window

	// No alert was created due to dedup.
	assert.Empty(t, store.createAlertCalls)
}

func TestAlertProcessMessage_DedupErrorStillCreatesAlert(t *testing.T) {
	store := &alertMockStore{
		hasRecentAlertErr: assert.AnError, // dedup check fails
	}
	w := newTestAlertWorker(store)

	tenantID := uuid.New()
	breach := ThresholdBreach{
		TenantID:     tenantID.String(),
		EnvID:        uuid.New().String(),
		BudgetID:     uuid.New().String(),
		Module:       "sale",
		OverheadPct:  50.0,
		ThresholdPct: 20,
	}
	data, _ := json.Marshal(breach)

	err := w.processMessage(context.Background(), tenantID.String(), string(data))
	require.NoError(t, err)

	// Alert should still be created even though dedup check failed.
	require.Len(t, store.createAlertCalls, 1)
}

func TestAlertProcessMessage_InvalidTenantID(t *testing.T) {
	store := &alertMockStore{}
	w := newTestAlertWorker(store)

	err := w.processMessage(context.Background(), "not-a-uuid", "{}")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid tenant_id")
}

func TestAlertProcessMessage_InvalidJSON(t *testing.T) {
	store := &alertMockStore{}
	w := newTestAlertWorker(store)

	err := w.processMessage(context.Background(), uuid.New().String(), "{{bad")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal breach")
}

func TestAlertProcessMessage_InvalidEnvID(t *testing.T) {
	store := &alertMockStore{}
	w := newTestAlertWorker(store)

	breach := ThresholdBreach{EnvID: "not-a-uuid"}
	data, _ := json.Marshal(breach)

	err := w.processMessage(context.Background(), uuid.New().String(), string(data))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid env_id")
}

func TestAlertProcessMessage_CreateAlertError(t *testing.T) {
	store := &alertMockStore{
		createAlertErr: assert.AnError,
	}
	w := newTestAlertWorker(store)

	tenantID := uuid.New()
	breach := ThresholdBreach{
		TenantID:     tenantID.String(),
		EnvID:        uuid.New().String(),
		BudgetID:     uuid.New().String(),
		Module:       "sale",
		OverheadPct:  50.0,
		ThresholdPct: 20,
	}
	data, _ := json.Marshal(breach)

	err := w.processMessage(context.Background(), tenantID.String(), string(data))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create alert")
}

func TestAlertProcessMessage_MetadataContainsBreakdown(t *testing.T) {
	store := &alertMockStore{}
	w := newTestAlertWorker(store)

	tenantID := uuid.New()
	breach := ThresholdBreach{
		TenantID:     tenantID.String(),
		EnvID:        uuid.New().String(),
		BudgetID:     uuid.New().String(),
		Module:       "sale",
		OverheadPct:  50.0,
		ThresholdPct: 20,
		TotalMS:      1000,
		ModuleMS:     500,
		Breakdown: map[string]any{
			"sql_ms": 200,
			"orm_ms": 300,
		},
	}
	data, _ := json.Marshal(breach)

	err := w.processMessage(context.Background(), tenantID.String(), string(data))
	require.NoError(t, err)

	require.Len(t, store.createAlertCalls, 1)

	var meta map[string]any
	require.NoError(t, json.Unmarshal(store.createAlertCalls[0].Metadata, &meta))
	assert.Equal(t, float64(1000), meta["total_ms"])
	assert.Equal(t, float64(500), meta["module_ms"])
	assert.Equal(t, float64(30), meta["exceeded_by"]) // 50 - 20

	bd, ok := meta["breakdown"].(map[string]any)
	require.True(t, ok, "breakdown should be a map")
	assert.Equal(t, float64(200), bd["sql_ms"])
	assert.Equal(t, float64(300), bd["orm_ms"])
}

// ── DefaultAlertConfig / NewAlertWorker tests ───────────────────────────────

func TestDefaultAlertConfig(t *testing.T) {
	cfg := DefaultAlertConfig("agent:alert", "alert-workers")
	assert.Equal(t, "agent:alert", cfg.Consumer.Stream)
	assert.Equal(t, "alert-workers", cfg.Consumer.Group)
	assert.Equal(t, 1, cfg.WorkerCount)
	assert.Equal(t, 10, cfg.DedupeWindowMin)
}

func TestNewAlertWorker_DefaultsWorkerCount(t *testing.T) {
	logger := zerolog.Nop()

	cfg := AlertConfig{WorkerCount: 0}
	w := NewAlertWorker(nil, nil, cfg, logger)
	assert.Equal(t, 1, w.config.WorkerCount)
	assert.Equal(t, 10, w.config.DedupeWindowMin)

	cfg = AlertConfig{WorkerCount: -1, DedupeWindowMin: -5}
	w = NewAlertWorker(nil, nil, cfg, logger)
	assert.Equal(t, 1, w.config.WorkerCount)
	assert.Equal(t, 10, w.config.DedupeWindowMin)
}

func TestNewAlertWorker_CustomConfig(t *testing.T) {
	logger := zerolog.Nop()

	cfg := AlertConfig{WorkerCount: 3, DedupeWindowMin: 15}
	w := NewAlertWorker(nil, nil, cfg, logger)
	assert.Equal(t, 3, w.config.WorkerCount)
	assert.Equal(t, 15, w.config.DedupeWindowMin)
}
