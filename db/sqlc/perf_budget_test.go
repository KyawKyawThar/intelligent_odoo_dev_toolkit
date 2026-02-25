package db

import (
	"Intelligent_Dev_ToolKit_Odoo/utils"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func createTestPerfBudget(t *testing.T, envID uuid.UUID) PerfBudget {
	t.Helper()
	pb, err := testStore.CreatePerfBudget(context.Background(), CreatePerfBudgetParams{
		EnvID:        envID,
		Module:       "sale_" + utils.RandomString(4),
		Endpoint:     "/api/" + utils.RandomString(6),
		ThresholdPct: 20,
	})
	require.NoError(t, err)
	require.NotZero(t, pb.ID)
	return pb
}
func createTestBudgetSample(t *testing.T, budgetID uuid.UUID, overheadPct string, totalMs, moduleMs int32) PerfBudgetSample {
	t.Helper()
	breakdown := json.RawMessage(`{"orm":45,"http":30,"other":25}`)
	sample, err := testStore.InsertBudgetSample(context.Background(), InsertBudgetSampleParams{
		BudgetID:    budgetID,
		OverheadPct: overheadPct,
		TotalMs:     int32Ptr(totalMs),
		ModuleMs:    int32Ptr(moduleMs),
		Breakdown:   &breakdown,
	})
	require.NoError(t, err)
	require.NotZero(t, sample.ID)
	return sample
}

// ═══════════════════════════════════════════════════════════════
//  CreatePerfBudget
// ═══════════════════════════════════════════════════════════════

func TestCreatePerfBudget_AllFields(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	pb, err := testStore.CreatePerfBudget(context.Background(), CreatePerfBudgetParams{
		EnvID:        env.ID,
		Module:       "sale",
		Endpoint:     "/api/sale/order",
		ThresholdPct: 15,
	})
	require.NoError(t, err)

	require.NotZero(t, pb.ID)
	require.Equal(t, env.ID, pb.EnvID)
	require.Equal(t, "sale", pb.Module)
	require.Equal(t, "/api/sale/order", pb.Endpoint)
	require.Equal(t, int32(15), pb.ThresholdPct)
	require.True(t, pb.IsActive, "new budgets must default to active")
	require.NotZero(t, pb.CreatedAt)
}
func TestCreatePerfBudget_InvalidEnvID_Fails(t *testing.T) {
	_, err := testStore.CreatePerfBudget(context.Background(), CreatePerfBudgetParams{
		EnvID:        uuid.New(),
		Module:       "sale",
		Endpoint:     "/api/sale",
		ThresholdPct: 10,
	})
	require.Error(t, err)
}

// ═══════════════════════════════════════════════════════════════
//  GetPerfBudget
// ═══════════════════════════════════════════════════════════════

func TestGetPerfBudget_Found(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	created := createTestPerfBudget(t, env.ID)

	fetched, err := testStore.GetPerfBudget(context.Background(), GetPerfBudgetParams{
		ID:    created.ID,
		EnvID: env.ID,
	})
	require.NoError(t, err)
	require.Equal(t, created.ID, fetched.ID)
	require.Equal(t, created.Module, fetched.Module)
	require.Equal(t, created.Endpoint, fetched.Endpoint)
	require.Equal(t, created.ThresholdPct, fetched.ThresholdPct)
}
func TestGetPerfBudget_WrongEnvID(t *testing.T) {
	reg := createRegisteredTenant(t)
	env1 := createTestEnvironment(t, reg.Tenant.ID, "production")
	env2 := createTestEnvironment(t, reg.Tenant.ID, "production")
	pb := createTestPerfBudget(t, env1.ID)

	_, err := testStore.GetPerfBudget(context.Background(), GetPerfBudgetParams{
		ID:    pb.ID,
		EnvID: env2.ID,
	})
	require.Error(t, err)
}
func TestGetPerfBudget_NotFound(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	_, err := testStore.GetPerfBudget(context.Background(), GetPerfBudgetParams{
		ID:    uuid.New(),
		EnvID: env.ID,
	})
	require.Error(t, err)
}

// ═══════════════════════════════════════════════════════════════
//  ListPerfBudgets (active only)
// ═══════════════════════════════════════════════════════════════

func TestListPerfBudgets_OnlyActive(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	active := createTestPerfBudget(t, env.ID)
	inactive := createTestPerfBudget(t, env.ID)

	// Deactivate one
	_, err := testStore.UpdatePerfBudget(context.Background(), UpdatePerfBudgetParams{
		ID:           inactive.ID,
		ThresholdPct: inactive.ThresholdPct,
		IsActive:     false,
		EnvID:        env.ID,
	})
	require.NoError(t, err)

	budgets, err := testStore.ListPerfBudgets(context.Background(), env.ID)
	require.NoError(t, err)

	ids := map[uuid.UUID]bool{}
	for _, b := range budgets {
		ids[b.ID] = true
		require.True(t, b.IsActive, "ListPerfBudgets must only return active budgets")
	}
	require.True(t, ids[active.ID])
	require.False(t, ids[inactive.ID])
}
func TestListPerfBudgets_Empty(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	budgets, err := testStore.ListPerfBudgets(context.Background(), env.ID)
	require.NoError(t, err)
	require.Empty(t, budgets)
}
func TestListPerfBudgets_EnvIsolation(t *testing.T) {
	reg := createRegisteredTenant(t)
	env1 := createTestEnvironment(t, reg.Tenant.ID, "production")
	env2 := createTestEnvironment(t, reg.Tenant.ID, "production")

	createTestPerfBudget(t, env1.ID)
	createTestPerfBudget(t, env2.ID)

	budgets1, err := testStore.ListPerfBudgets(context.Background(), env1.ID)
	require.NoError(t, err)
	for _, b := range budgets1 {
		require.Equal(t, env1.ID, b.EnvID)
	}
}

// ═══════════════════════════════════════════════════════════════
//  ListAllPerfBudgetsByEnv (active + inactive)
// ═══════════════════════════════════════════════════════════════

func TestListAllPerfBudgetsByEnv_IncludesInactive(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	active := createTestPerfBudget(t, env.ID)
	inactive := createTestPerfBudget(t, env.ID)

	_, err := testStore.UpdatePerfBudget(context.Background(), UpdatePerfBudgetParams{
		ID:           inactive.ID,
		ThresholdPct: inactive.ThresholdPct,
		IsActive:     false,
		EnvID:        env.ID,
	})
	require.NoError(t, err)

	all, err := testStore.ListAllPerfBudgetsByEnv(context.Background(), env.ID)
	require.NoError(t, err)

	ids := map[uuid.UUID]bool{}
	for _, b := range all {
		ids[b.ID] = true
	}
	require.True(t, ids[active.ID], "active budget must appear")
	require.True(t, ids[inactive.ID], "inactive budget must also appear")
}

// ═══════════════════════════════════════════════════════════════
//  UpdatePerfBudget
// ═══════════════════════════════════════════════════════════════

func TestUpdatePerfBudget_UpdatesThresholdAndActive(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	pb := createTestPerfBudget(t, env.ID)

	updated, err := testStore.UpdatePerfBudget(context.Background(), UpdatePerfBudgetParams{
		ID:           pb.ID,
		ThresholdPct: 50,
		IsActive:     false,
		EnvID:        env.ID,
	})
	require.NoError(t, err)
	require.Equal(t, int32(50), updated.ThresholdPct)
	require.False(t, updated.IsActive)
	// Module and endpoint must be unchanged
	require.Equal(t, pb.Module, updated.Module)
	require.Equal(t, pb.Endpoint, updated.Endpoint)
}
func TestUpdatePerfBudget_WrongEnvID_Fails(t *testing.T) {
	reg := createRegisteredTenant(t)
	env1 := createTestEnvironment(t, reg.Tenant.ID, "production")
	env2 := createTestEnvironment(t, reg.Tenant.ID, "production")
	pb := createTestPerfBudget(t, env1.ID)

	_, err := testStore.UpdatePerfBudget(context.Background(), UpdatePerfBudgetParams{
		ID:           pb.ID,
		ThresholdPct: 99,
		IsActive:     true,
		EnvID:        env2.ID,
	})
	require.Error(t, err)
}
func TestUpdatePerfBudget_Reactivate(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	pb := createTestPerfBudget(t, env.ID)

	// Deactivate
	_, err := testStore.UpdatePerfBudget(context.Background(), UpdatePerfBudgetParams{
		ID: pb.ID, ThresholdPct: pb.ThresholdPct, IsActive: false, EnvID: env.ID,
	})
	require.NoError(t, err)

	// Reactivate
	reactivated, err := testStore.UpdatePerfBudget(context.Background(), UpdatePerfBudgetParams{
		ID: pb.ID, ThresholdPct: 30, IsActive: true, EnvID: env.ID,
	})
	require.NoError(t, err)
	require.True(t, reactivated.IsActive)
	require.Equal(t, int32(30), reactivated.ThresholdPct)
}

// ═══════════════════════════════════════════════════════════════
//  DeletePerfBudget
// ═══════════════════════════════════════════════════════════════

func TestDeletePerfBudget(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	pb := createTestPerfBudget(t, env.ID)

	err := testStore.DeletePerfBudget(context.Background(), DeletePerfBudgetParams{
		ID:    pb.ID,
		EnvID: env.ID,
	})
	require.NoError(t, err)

	_, err = testStore.GetPerfBudget(context.Background(), GetPerfBudgetParams{
		ID:    pb.ID,
		EnvID: env.ID,
	})
	require.Error(t, err)
}
func TestDeletePerfBudget_WrongEnvID_NoOp(t *testing.T) {
	reg := createRegisteredTenant(t)
	env1 := createTestEnvironment(t, reg.Tenant.ID, "production")
	env2 := createTestEnvironment(t, reg.Tenant.ID, "production")
	pb := createTestPerfBudget(t, env1.ID)

	err := testStore.DeletePerfBudget(context.Background(), DeletePerfBudgetParams{
		ID:    pb.ID,
		EnvID: env2.ID,
	})
	require.NoError(t, err)

	// Must still exist
	_, err = testStore.GetPerfBudget(context.Background(), GetPerfBudgetParams{
		ID:    pb.ID,
		EnvID: env1.ID,
	})
	require.NoError(t, err)
}

// ═══════════════════════════════════════════════════════════════
//  InsertBudgetSample
// ═══════════════════════════════════════════════════════════════

func TestInsertBudgetSample_AllFields(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	pb := createTestPerfBudget(t, env.ID)
	breakdown := json.RawMessage(`{"orm":120,"http":80,"template":50}`)

	sample, err := testStore.InsertBudgetSample(context.Background(), InsertBudgetSampleParams{
		BudgetID:    pb.ID,
		OverheadPct: "18.75",
		TotalMs:     int32Ptr(250),
		ModuleMs:    int32Ptr(180),
		Breakdown:   &breakdown,
	})
	require.NoError(t, err)

	require.NotZero(t, sample.ID)
	require.Equal(t, pb.ID, sample.BudgetID)
	require.Equal(t, "18.75", sample.OverheadPct)
	require.NotNil(t, sample.TotalMs)
	require.Equal(t, int32(250), *sample.TotalMs)
	require.NotNil(t, sample.ModuleMs)
	require.Equal(t, int32(180), *sample.ModuleMs)
	require.NotNil(t, sample.Breakdown)
	require.NotZero(t, sample.SampledAt)

	// Breakdown round-trip
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(*sample.Breakdown, &parsed))
	require.Equal(t, float64(120), parsed["orm"])
}
func TestInsertBudgetSample_NilOptionals(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	pb := createTestPerfBudget(t, env.ID)

	sample, err := testStore.InsertBudgetSample(context.Background(), InsertBudgetSampleParams{
		BudgetID:    pb.ID,
		OverheadPct: "5.00",
	})
	require.NoError(t, err)
	require.Nil(t, sample.TotalMs)
	require.Nil(t, sample.ModuleMs)
	require.Nil(t, sample.Breakdown)
}

func TestInsertBudgetSample_InvalidBudgetID_Fails(t *testing.T) {
	_, err := testStore.InsertBudgetSample(context.Background(), InsertBudgetSampleParams{
		BudgetID:    uuid.New(),
		OverheadPct: "10.00",
	})
	require.Error(t, err)
}
func TestGetLatestBudgetSample_ReturnsNewest(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	pb := createTestPerfBudget(t, env.ID)

	_ = createTestBudgetSample(t, pb.ID, "10.00", 100, 80)
	_ = createTestBudgetSample(t, pb.ID, "12.00", 120, 90)
	latest := createTestBudgetSample(t, pb.ID, "15.00", 150, 110)

	fetched, err := testStore.GetLatestBudgetSample(context.Background(), pb.ID)
	require.NoError(t, err)
	require.Equal(t, latest.ID, fetched.ID)
	require.Equal(t, "15.00", fetched.OverheadPct)
}
func TestGetLatestBudgetSample_NoSamples(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	pb := createTestPerfBudget(t, env.ID)

	_, err := testStore.GetLatestBudgetSample(context.Background(), pb.ID)
	require.Error(t, err)
}

// ═══════════════════════════════════════════════════════════════
//  ListBudgetSamples
// ═══════════════════════════════════════════════════════════════

func TestListBudgetSamples_LimitAndOrder(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	pb := createTestPerfBudget(t, env.ID)

	for i := 0; i < 5; i++ {
		createTestBudgetSample(t, pb.ID, "10.00", 100, 80)
	}

	samples, err := testStore.ListBudgetSamples(context.Background(), ListBudgetSamplesParams{
		BudgetID: pb.ID,
		Limit:    3,
	})
	require.NoError(t, err)
	require.Len(t, samples, 3)

	// DESC order by sampled_at
	for i := 1; i < len(samples); i++ {
		require.True(t, samples[i-1].SampledAt.After(samples[i].SampledAt) ||
			samples[i-1].SampledAt.Equal(samples[i].SampledAt))
	}
}
func TestListBudgetSamples_Empty(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	pb := createTestPerfBudget(t, env.ID)

	samples, err := testStore.ListBudgetSamples(context.Background(), ListBudgetSamplesParams{
		BudgetID: pb.ID,
		Limit:    10,
	})
	require.NoError(t, err)
	require.Empty(t, samples)
}
func TestListBudgetSamples_BudgetIsolation(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	pb1 := createTestPerfBudget(t, env.ID)
	pb2 := createTestPerfBudget(t, env.ID)

	createTestBudgetSample(t, pb1.ID, "10.00", 100, 80)
	createTestBudgetSample(t, pb2.ID, "20.00", 200, 160)

	samples1, err := testStore.ListBudgetSamples(context.Background(), ListBudgetSamplesParams{
		BudgetID: pb1.ID, Limit: 10,
	})
	require.NoError(t, err)
	require.Len(t, samples1, 1)
	require.Equal(t, pb1.ID, samples1[0].BudgetID)
}

// ═══════════════════════════════════════════════════════════════
//  ListBudgetSamplesBetween
// ═══════════════════════════════════════════════════════════════

func TestListBudgetSamplesBetween_DateRange(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	pb := createTestPerfBudget(t, env.ID)

	// Create samples (all with sampled_at ≈ now)
	for i := 0; i < 3; i++ {
		createTestBudgetSample(t, pb.ID, "10.00", 100, 80)
	}

	now := time.Now()
	samples, err := testStore.ListBudgetSamplesBetween(context.Background(), ListBudgetSamplesBetweenParams{
		BudgetID:    pb.ID,
		SampledAt:   now.Add(-1 * time.Hour),
		SampledAt_2: now.Add(1 * time.Hour),
	})
	require.NoError(t, err)
	require.Len(t, samples, 3)
}

func TestListBudgetSamplesBetween_OutOfRange(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	pb := createTestPerfBudget(t, env.ID)

	createTestBudgetSample(t, pb.ID, "10.00", 100, 80)

	samples, err := testStore.ListBudgetSamplesBetween(context.Background(), ListBudgetSamplesBetweenParams{
		BudgetID:    pb.ID,
		SampledAt:   time.Now().Add(-48 * time.Hour),
		SampledAt_2: time.Now().Add(-24 * time.Hour),
	})
	require.NoError(t, err)
	require.Empty(t, samples)
}
func TestListBudgetSamplesBetween_OrderByASC(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	pb := createTestPerfBudget(t, env.ID)

	for i := 0; i < 3; i++ {
		createTestBudgetSample(t, pb.ID, "10.00", 100, 80)
	}

	now := time.Now()
	samples, err := testStore.ListBudgetSamplesBetween(context.Background(), ListBudgetSamplesBetweenParams{
		BudgetID:    pb.ID,
		SampledAt:   now.Add(-1 * time.Hour),
		SampledAt_2: now.Add(1 * time.Hour),
	})
	require.NoError(t, err)

	// ASC order
	for i := 1; i < len(samples); i++ {
		require.True(t, samples[i-1].SampledAt.Before(samples[i].SampledAt) ||
			samples[i-1].SampledAt.Equal(samples[i].SampledAt))
	}
}

// ═══════════════════════════════════════════════════════════════
//  GetBudgetAverage7d
// ═══════════════════════════════════════════════════════════════

func TestGetBudgetAverage7d_WithSamples(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	pb := createTestPerfBudget(t, env.ID)

	// Insert samples (all within last 7 days since sampled_at defaults to now)
	createTestBudgetSample(t, pb.ID, "10.00", 100, 80)
	createTestBudgetSample(t, pb.ID, "20.00", 200, 160)
	createTestBudgetSample(t, pb.ID, "30.00", 300, 240)

	avg, err := testStore.GetBudgetAverage7d(context.Background(), pb.ID)
	require.NoError(t, err)
	require.Equal(t, int64(3), avg.SampleCount)
	require.NotEmpty(t, avg.AvgOverhead)
	require.NotNil(t, avg.MaxOverhead)
}
func TestGetBudgetAverage7d_NoSamples(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	pb := createTestPerfBudget(t, env.ID)

	avg, err := testStore.GetBudgetAverage7d(context.Background(), pb.ID)
	require.NoError(t, err)
	require.Equal(t, int64(0), avg.SampleCount)
}

// ═══════════════════════════════════════════════════════════════
//  DeleteOldBudgetSamples
// ═══════════════════════════════════════════════════════════════

func TestDeleteOldBudgetSamples_DeletesOld(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	pb := createTestPerfBudget(t, env.ID)

	createTestBudgetSample(t, pb.ID, "10.00", 100, 80)
	createTestBudgetSample(t, pb.ID, "15.00", 150, 120)

	// Future cutoff → deletes everything
	result, err := testStore.DeleteOldBudgetSamples(context.Background(), DeleteOldBudgetSamplesParams{
		TenantID:  reg.Tenant.ID,
		SampledAt: time.Now().Add(1 * time.Hour),
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, result.RowsAffected(), int64(2))

	// No samples left
	samples, err := testStore.ListBudgetSamples(context.Background(), ListBudgetSamplesParams{
		BudgetID: pb.ID, Limit: 10,
	})
	require.NoError(t, err)
	require.Empty(t, samples)
}
func TestDeleteOldBudgetSamples_CutoffInPast_KeepsAll(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	pb := createTestPerfBudget(t, env.ID)

	createTestBudgetSample(t, pb.ID, "10.00", 100, 80)

	result, err := testStore.DeleteOldBudgetSamples(context.Background(), DeleteOldBudgetSamplesParams{
		TenantID:  reg.Tenant.ID,
		SampledAt: time.Now().Add(-24 * time.Hour),
	})
	require.NoError(t, err)
	require.Equal(t, int64(0), result.RowsAffected())
}
func TestDeleteOldBudgetSamples_TenantIsolation(t *testing.T) {
	reg1 := createRegisteredTenant(t)
	reg2 := createRegisteredTenant(t)
	env1 := createTestEnvironment(t, reg1.Tenant.ID, "production")
	env2 := createTestEnvironment(t, reg2.Tenant.ID, "production")
	pb1 := createTestPerfBudget(t, env1.ID)
	pb2 := createTestPerfBudget(t, env2.ID)

	createTestBudgetSample(t, pb1.ID, "10.00", 100, 80)
	createTestBudgetSample(t, pb2.ID, "20.00", 200, 160)

	// Delete tenant 1's samples
	_, err := testStore.DeleteOldBudgetSamples(context.Background(), DeleteOldBudgetSamplesParams{
		TenantID:  reg1.Tenant.ID,
		SampledAt: time.Now().Add(1 * time.Hour),
	})
	require.NoError(t, err)

	// Tenant 2's samples must survive
	samples2, err := testStore.ListBudgetSamples(context.Background(), ListBudgetSamplesParams{
		BudgetID: pb2.ID, Limit: 10,
	})
	require.NoError(t, err)
	require.Len(t, samples2, 1)
}

// ═══════════════════════════════════════════════════════════════
//  Full lifecycle: create budget → samples → query → delete
// ═══════════════════════════════════════════════════════════════

func TestPerfBudgetLifecycle(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	// 1. Create budget
	pb, err := testStore.CreatePerfBudget(context.Background(), CreatePerfBudgetParams{
		EnvID:        env.ID,
		Module:       "sale",
		Endpoint:     "/api/sale/confirm",
		ThresholdPct: 15,
	})
	require.NoError(t, err)
	require.True(t, pb.IsActive)

	// 2. Insert samples
	createTestBudgetSample(t, pb.ID, "12.50", 200, 150)
	createTestBudgetSample(t, pb.ID, "18.00", 250, 190)

	// 3. Get latest
	latest, err := testStore.GetLatestBudgetSample(context.Background(), pb.ID)
	require.NoError(t, err)
	require.Equal(t, "18.00", latest.OverheadPct)

	// 4. Get 7d average
	avg, err := testStore.GetBudgetAverage7d(context.Background(), pb.ID)
	require.NoError(t, err)
	require.Equal(t, int64(2), avg.SampleCount)

	// 5. Update threshold
	updated, err := testStore.UpdatePerfBudget(context.Background(), UpdatePerfBudgetParams{
		ID: pb.ID, ThresholdPct: 25, IsActive: true, EnvID: env.ID,
	})
	require.NoError(t, err)
	require.Equal(t, int32(25), updated.ThresholdPct)

	// 6. Deactivate
	deactivated, err := testStore.UpdatePerfBudget(context.Background(), UpdatePerfBudgetParams{
		ID: pb.ID, ThresholdPct: 25, IsActive: false, EnvID: env.ID,
	})
	require.NoError(t, err)
	require.False(t, deactivated.IsActive)

	// 7. Should not appear in active list
	activeBudgets, err := testStore.ListPerfBudgets(context.Background(), env.ID)
	require.NoError(t, err)
	for _, b := range activeBudgets {
		require.NotEqual(t, pb.ID, b.ID)
	}

	// 8. Should appear in all list
	allBudgets, err := testStore.ListAllPerfBudgetsByEnv(context.Background(), env.ID)
	require.NoError(t, err)
	found := false
	for _, b := range allBudgets {
		if b.ID == pb.ID {
			found = true
		}
	}
	require.True(t, found)

	// 9. Delete
	err = testStore.DeletePerfBudget(context.Background(), DeletePerfBudgetParams{
		ID: pb.ID, EnvID: env.ID,
	})
	require.NoError(t, err)
}
