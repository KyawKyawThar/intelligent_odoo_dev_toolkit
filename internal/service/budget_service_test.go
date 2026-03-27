package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

func TestToBudgetResponse(t *testing.T) {
	id := uuid.New()
	envID := uuid.New()
	now := time.Now().UTC()

	budget := db.PerfBudget{
		ID:           id,
		EnvID:        envID,
		Module:       "sale",
		Endpoint:     "/web/dataset/call_kw/sale.order/search_read",
		ThresholdPct: 15,
		IsActive:     true,
		CreatedAt:    now,
	}

	resp := toBudgetResponse(budget)

	if resp.ID != id {
		t.Errorf("ID = %v, want %v", resp.ID, id)
	}
	if resp.EnvID != envID {
		t.Errorf("EnvID = %v, want %v", resp.EnvID, envID)
	}
	if resp.Module != "sale" {
		t.Errorf("Module = %q, want sale", resp.Module)
	}
	if resp.Endpoint != "/web/dataset/call_kw/sale.order/search_read" {
		t.Errorf("Endpoint = %q, want /web/dataset/call_kw/sale.order/search_read", resp.Endpoint)
	}
	if resp.ThresholdPct != 15 {
		t.Errorf("ThresholdPct = %d, want 15", resp.ThresholdPct)
	}
	if !resp.IsActive {
		t.Error("expected IsActive = true")
	}
	if !resp.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", resp.CreatedAt, now)
	}
}

func TestToBudgetResponse_InactiveBudget(t *testing.T) {
	budget := db.PerfBudget{
		ID:           uuid.New(),
		EnvID:        uuid.New(),
		Module:       "stock",
		Endpoint:     "/api/stock/move",
		ThresholdPct: 25,
		IsActive:     false,
		CreatedAt:    time.Now(),
	}

	resp := toBudgetResponse(budget)

	if resp.IsActive {
		t.Error("expected IsActive = false for inactive budget")
	}
	if resp.ThresholdPct != 25 {
		t.Errorf("ThresholdPct = %d, want 25", resp.ThresholdPct)
	}
}

func TestToBudgetSampleResponse(t *testing.T) {
	id := uuid.New()
	budgetID := uuid.New()
	now := time.Now().UTC()
	totalMS := int32(1200)
	moduleMS := int32(450)
	breakdown := json.RawMessage(`{"sale.order._compute_amount_total": 200, "stock.move._compute_quantity": 150}`)

	sample := db.PerfBudgetSample{
		ID:          id,
		BudgetID:    budgetID,
		OverheadPct: "12.50",
		TotalMs:     &totalMS,
		ModuleMs:    &moduleMS,
		Breakdown:   &breakdown,
		SampledAt:   now,
	}

	resp := toBudgetSampleResponse(sample)

	if resp.ID != id {
		t.Errorf("ID = %v, want %v", resp.ID, id)
	}
	if resp.BudgetID != budgetID {
		t.Errorf("BudgetID = %v, want %v", resp.BudgetID, budgetID)
	}
	if resp.OverheadPct != "12.50" {
		t.Errorf("OverheadPct = %q, want 12.50", resp.OverheadPct)
	}
	if resp.TotalMS == nil || *resp.TotalMS != 1200 {
		t.Errorf("TotalMS = %v, want 1200", resp.TotalMS)
	}
	if resp.ModuleMS == nil || *resp.ModuleMS != 450 {
		t.Errorf("ModuleMS = %v, want 450", resp.ModuleMS)
	}
	if resp.Breakdown == nil {
		t.Fatal("expected non-nil breakdown")
	}
	if !resp.SampledAt.Equal(now) {
		t.Errorf("SampledAt = %v, want %v", resp.SampledAt, now)
	}
}

func TestToBudgetSampleResponse_NilOptionals(t *testing.T) {
	sample := db.PerfBudgetSample{
		ID:          uuid.New(),
		BudgetID:    uuid.New(),
		OverheadPct: "0.00",
		TotalMs:     nil,
		ModuleMs:    nil,
		Breakdown:   nil,
		SampledAt:   time.Now(),
	}

	resp := toBudgetSampleResponse(sample)

	if resp.TotalMS != nil {
		t.Errorf("expected nil TotalMS, got %v", resp.TotalMS)
	}
	if resp.ModuleMS != nil {
		t.Errorf("expected nil ModuleMS, got %v", resp.ModuleMS)
	}
	if resp.Breakdown != nil {
		t.Errorf("expected nil Breakdown, got %v", resp.Breakdown)
	}
}

func TestNewBudgetService(t *testing.T) {
	svc := NewBudgetService(nil)
	if svc == nil {
		t.Fatal("expected non-nil BudgetService")
	}
}

// ─── Mock store for CalculateOverhead tests ─────────────────────────────────

type overheadMockStore struct {
	db.Store // embed — unimplemented methods panic

	budgets       []db.PerfBudget
	listErr       error
	insertedCalls []db.InsertBudgetSampleParams
	insertErr     error
}

func (m *overheadMockStore) ListPerfBudgets(_ context.Context, _ uuid.UUID) ([]db.PerfBudget, error) {
	return m.budgets, m.listErr
}

func (m *overheadMockStore) InsertBudgetSample(_ context.Context, arg db.InsertBudgetSampleParams) (db.PerfBudgetSample, error) {
	m.insertedCalls = append(m.insertedCalls, arg)
	return db.PerfBudgetSample{}, m.insertErr
}

// ─── CalculateOverhead tests ────────────────────────────────────────────────

func TestCalculateOverhead_BasicCalculation(t *testing.T) {
	budgetID := uuid.New()
	envID := uuid.New()

	store := &overheadMockStore{
		budgets: []db.PerfBudget{
			{ID: budgetID, EnvID: envID, Module: "sale", ThresholdPct: 20},
		},
	}
	svc := NewBudgetService(store)
	logger := zerolog.Nop()

	events := []ProfilerEvent{
		{Category: "orm", Module: "sale", DurationMS: 300},
		{Category: "sql", Module: "sale", DurationMS: 200},
		{Category: "orm", Module: "stock", DurationMS: 400},
		{Category: "python", Module: "stock", DurationMS: 100},
	}

	result, err := svc.CalculateOverhead(context.Background(), envID, events, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SamplesInserted != 1 {
		t.Fatalf("expected 1 sample inserted, got %d", result.SamplesInserted)
	}

	if len(store.insertedCalls) != 1 {
		t.Fatalf("expected 1 InsertBudgetSample call, got %d", len(store.insertedCalls))
	}

	call := store.insertedCalls[0]
	if call.BudgetID != budgetID {
		t.Errorf("BudgetID = %v, want %v", call.BudgetID, budgetID)
	}

	// sale module: 300+200 = 500ms out of 1000ms total = 50.00%
	if call.OverheadPct != "50.00" {
		t.Errorf("OverheadPct = %q, want 50.00", call.OverheadPct)
	}
	if call.TotalMs == nil || *call.TotalMs != 1000 {
		t.Errorf("TotalMs = %v, want 1000", call.TotalMs)
	}
	if call.ModuleMs == nil || *call.ModuleMs != 500 {
		t.Errorf("ModuleMs = %v, want 500", call.ModuleMs)
	}

	// Verify breakdown JSONB.
	if call.Breakdown == nil {
		t.Fatal("expected non-nil Breakdown")
	}
	var bd map[string]any
	if err := json.Unmarshal(*call.Breakdown, &bd); err != nil {
		t.Fatalf("unmarshal breakdown: %v", err)
	}
	if bd["orm_ms"] != float64(300) {
		t.Errorf("breakdown orm_ms = %v, want 300", bd["orm_ms"])
	}
	if bd["sql_ms"] != float64(200) {
		t.Errorf("breakdown sql_ms = %v, want 200", bd["sql_ms"])
	}

	// Threshold is 20%, overhead is 50% — should be a breach.
	if len(result.Breaches) != 1 {
		t.Fatalf("expected 1 breach, got %d", len(result.Breaches))
	}
	if result.Breaches[0].Module != "sale" {
		t.Errorf("breach module = %q, want sale", result.Breaches[0].Module)
	}
}

func TestCalculateOverhead_NoEvents_ReturnsZero(t *testing.T) {
	store := &overheadMockStore{}
	svc := NewBudgetService(store)
	logger := zerolog.Nop()

	result, err := svc.CalculateOverhead(context.Background(), uuid.New(), nil, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SamplesInserted != 0 {
		t.Errorf("expected 0, got %d", result.SamplesInserted)
	}
}

func TestCalculateOverhead_NoBudgets_ReturnsZero(t *testing.T) {
	store := &overheadMockStore{budgets: nil}
	svc := NewBudgetService(store)
	logger := zerolog.Nop()

	events := []ProfilerEvent{
		{Category: "orm", Module: "sale", DurationMS: 100},
	}

	result, err := svc.CalculateOverhead(context.Background(), uuid.New(), events, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SamplesInserted != 0 {
		t.Errorf("expected 0, got %d", result.SamplesInserted)
	}
}

func TestCalculateOverhead_NoMatchingModule_SkipsBudget(t *testing.T) {
	store := &overheadMockStore{
		budgets: []db.PerfBudget{
			{ID: uuid.New(), Module: "purchase"},
		},
	}
	svc := NewBudgetService(store)
	logger := zerolog.Nop()

	events := []ProfilerEvent{
		{Category: "orm", Module: "sale", DurationMS: 100},
	}

	result, err := svc.CalculateOverhead(context.Background(), uuid.New(), events, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SamplesInserted != 0 {
		t.Errorf("expected 0 (no matching module), got %d", result.SamplesInserted)
	}
	if len(store.insertedCalls) != 0 {
		t.Errorf("expected 0 insert calls, got %d", len(store.insertedCalls))
	}
}

func TestCalculateOverhead_MultipleBudgets(t *testing.T) {
	envID := uuid.New()
	store := &overheadMockStore{
		budgets: []db.PerfBudget{
			{ID: uuid.New(), EnvID: envID, Module: "sale", ThresholdPct: 20},
			{ID: uuid.New(), EnvID: envID, Module: "stock", ThresholdPct: 30},
		},
	}
	svc := NewBudgetService(store)
	logger := zerolog.Nop()

	events := []ProfilerEvent{
		{Category: "orm", Module: "sale", DurationMS: 200},
		{Category: "sql", Module: "stock", DurationMS: 300},
		{Category: "python", Module: "base", DurationMS: 500},
	}

	result, err := svc.CalculateOverhead(context.Background(), envID, events, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SamplesInserted != 2 {
		t.Fatalf("expected 2 samples, got %d", result.SamplesInserted)
	}
	if len(store.insertedCalls) != 2 {
		t.Fatalf("expected 2 insert calls, got %d", len(store.insertedCalls))
	}

	// sale: 200/1000 = 20.00% — exactly at threshold, no breach
	if store.insertedCalls[0].OverheadPct != "20.00" {
		t.Errorf("sale overhead = %q, want 20.00", store.insertedCalls[0].OverheadPct)
	}
	// stock: 300/1000 = 30.00% — exactly at threshold, no breach
	if store.insertedCalls[1].OverheadPct != "30.00" {
		t.Errorf("stock overhead = %q, want 30.00", store.insertedCalls[1].OverheadPct)
	}
	// Neither exceeds threshold (== is not >), so no breaches.
	if len(result.Breaches) != 0 {
		t.Errorf("expected 0 breaches (at threshold, not over), got %d", len(result.Breaches))
	}
}

func TestCalculateOverhead_ListBudgetsError_ReturnsError(t *testing.T) {
	store := &overheadMockStore{
		listErr: errors.New("db connection failed"),
	}
	svc := NewBudgetService(store)
	logger := zerolog.Nop()

	events := []ProfilerEvent{
		{Category: "orm", Module: "sale", DurationMS: 100},
	}

	_, err := svc.CalculateOverhead(context.Background(), uuid.New(), events, logger)
	if err == nil {
		t.Fatal("expected error from ListPerfBudgets, got nil")
	}
}

func TestCalculateOverhead_InsertError_ContinuesOtherBudgets(t *testing.T) {
	envID := uuid.New()
	store := &overheadMockStore{
		budgets: []db.PerfBudget{
			{ID: uuid.New(), EnvID: envID, Module: "sale"},
			{ID: uuid.New(), EnvID: envID, Module: "stock"},
		},
		insertErr: errors.New("insert failed"),
	}
	svc := NewBudgetService(store)
	logger := zerolog.Nop()

	events := []ProfilerEvent{
		{Category: "orm", Module: "sale", DurationMS: 100},
		{Category: "orm", Module: "stock", DurationMS: 200},
	}

	result, err := svc.CalculateOverhead(context.Background(), envID, events, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Both inserts failed, so 0 samples inserted.
	if result.SamplesInserted != 0 {
		t.Errorf("expected 0 successful samples, got %d", result.SamplesInserted)
	}
	// But both were attempted.
	if len(store.insertedCalls) != 2 {
		t.Errorf("expected 2 insert attempts, got %d", len(store.insertedCalls))
	}
}

func TestCalculateOverhead_EventsWithNoModule_Ignored(t *testing.T) {
	envID := uuid.New()
	store := &overheadMockStore{
		budgets: []db.PerfBudget{
			{ID: uuid.New(), EnvID: envID, Module: "sale", ThresholdPct: 10},
		},
	}
	svc := NewBudgetService(store)
	logger := zerolog.Nop()

	events := []ProfilerEvent{
		{Category: "sql", Module: "", DurationMS: 500},     // no module — counted in total but not attributed
		{Category: "orm", Module: "sale", DurationMS: 100}, // sale module
	}

	result, err := svc.CalculateOverhead(context.Background(), envID, events, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SamplesInserted != 1 {
		t.Fatalf("expected 1 sample, got %d", result.SamplesInserted)
	}

	// sale: 100/600 = 16.67%
	if store.insertedCalls[0].OverheadPct != "16.67" {
		t.Errorf("overhead = %q, want 16.67", store.insertedCalls[0].OverheadPct)
	}

	// 16.67% > 10% threshold — should be a breach.
	if len(result.Breaches) != 1 {
		t.Fatalf("expected 1 breach, got %d", len(result.Breaches))
	}
}

func TestCalculateOverhead_ZeroDurationEvents_ReturnsZero(t *testing.T) {
	store := &overheadMockStore{
		budgets: []db.PerfBudget{
			{ID: uuid.New(), Module: "sale"},
		},
	}
	svc := NewBudgetService(store)
	logger := zerolog.Nop()

	events := []ProfilerEvent{
		{Category: "orm", Module: "sale", DurationMS: 0},
	}

	result, err := svc.CalculateOverhead(context.Background(), uuid.New(), events, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SamplesInserted != 0 {
		t.Errorf("expected 0 (total_ms=0), got %d", result.SamplesInserted)
	}
}
