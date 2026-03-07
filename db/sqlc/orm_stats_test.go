package db

import (
	"Intelligent_Dev_ToolKit_Odoo/utils"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func int32Ptr(v int32) *int32 { return &v }

//lint:ignore U1000 unused
func createTestORMStat(t *testing.T, envID uuid.UUID, model, method string, n1 bool) OrmStat {
	t.Helper()
	sampleSQL := "SELECT * FROM " + model
	avgMs := "12.50"
	stat, err := testStore.InsertORMStat(context.Background(), InsertORMStatParams{
		EnvID:      envID,
		Model:      model,
		Method:     method,
		CallCount:  100,
		TotalMs:    1200,
		AvgMs:      &avgMs,
		MaxMs:      int32Ptr(85),
		P95Ms:      int32Ptr(72),
		N1Detected: n1,
		SampleSql:  &sampleSQL,
		Period:     time.Now().UTC().Truncate(time.Hour),
	})
	require.NoError(t, err)
	require.NotZero(t, stat.ID)
	return stat
}

// ═══════════════════════════════════════════════════════════════
//  InsertORMStat
// ═══════════════════════════════════════════════════════════════

func TestInsertORMStat_AllFields(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	sampleSQL := "SELECT id, name FROM res_partner WHERE active = true"
	avgMs := "15.75"
	period := time.Now().UTC().Truncate(time.Hour)

	stat, err := testStore.InsertORMStat(context.Background(), InsertORMStatParams{
		EnvID:      env.ID,
		Model:      "res.partner",
		Method:     "search_read",
		CallCount:  250,
		TotalMs:    3900,
		AvgMs:      &avgMs,
		MaxMs:      int32Ptr(120),
		P95Ms:      int32Ptr(95),
		N1Detected: true,
		SampleSql:  &sampleSQL,
		Period:     period,
	})
	require.NoError(t, err)

	require.NotZero(t, stat.ID)
	require.Equal(t, env.ID, stat.EnvID)
	require.Equal(t, "res.partner", stat.Model)
	require.Equal(t, "search_read", stat.Method)
	require.Equal(t, int32(250), stat.CallCount)
	require.Equal(t, int32(3900), stat.TotalMs)
	require.NotNil(t, stat.AvgMs)
	require.NotNil(t, stat.MaxMs)
	require.Equal(t, int32(120), *stat.MaxMs)
	require.NotNil(t, stat.P95Ms)
	require.Equal(t, int32(95), *stat.P95Ms)
	require.True(t, stat.N1Detected)
	require.NotNil(t, stat.SampleSql)
	require.Equal(t, sampleSQL, *stat.SampleSql)
	require.NotZero(t, stat.CreatedAt)
}
func TestInsertORMStat_NilOptionals(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	stat, err := testStore.InsertORMStat(context.Background(), InsertORMStatParams{
		EnvID:      env.ID,
		Model:      "sale.order",
		Method:     "create",
		CallCount:  10,
		TotalMs:    50,
		N1Detected: false,
		Period:     time.Now().UTC().Truncate(time.Hour),
	})
	require.NoError(t, err)
	require.NotZero(t, stat.ID)
	require.Nil(t, stat.AvgMs)
	require.Nil(t, stat.MaxMs)
	require.Nil(t, stat.P95Ms)
	require.Nil(t, stat.SampleSql)
	require.False(t, stat.N1Detected)
}

func TestInsertORMStat_InvalidEnvID_Fails(t *testing.T) {
	_, err := testStore.InsertORMStat(context.Background(), InsertORMStatParams{
		EnvID:      uuid.New(),
		Model:      "res.partner",
		Method:     "read",
		CallCount:  1,
		TotalMs:    5,
		N1Detected: false,
		Period:     time.Now().UTC().Truncate(time.Hour),
	})
	require.Error(t, err)
}

// ═══════════════════════════════════════════════════════════════
//  ListTopORMStats
// ═══════════════════════════════════════════════════════════════

func TestListTopORMStats_OrderByTotalMsDesc(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	period := time.Now().UTC().Truncate(time.Hour)

	// Insert with different total_ms
	for _, ms := range []int32{100, 500, 300, 800, 200} {
		_, err := testStore.InsertORMStat(context.Background(), InsertORMStatParams{
			EnvID: env.ID, Model: "model_" + utils.RandomString(4), Method: "read",
			CallCount: 10, TotalMs: ms, N1Detected: false, Period: period,
		})
		require.NoError(t, err)
	}

	stats, err := testStore.ListTopORMStats(context.Background(), ListTopORMStatsParams{
		EnvID: env.ID, Period: period.Add(-1 * time.Minute), Limit: 5,
	})
	require.NoError(t, err)
	require.Len(t, stats, 5)

	// Must be DESC by total_ms
	for i := 1; i < len(stats); i++ {
		require.GreaterOrEqual(t, stats[i-1].TotalMs, stats[i].TotalMs)
	}
}
func TestListTopORMStats_RespectsLimit(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	period := time.Now().UTC().Truncate(time.Hour)

	for i := 0; i < 5; i++ {
		createTestORMStat(t, env.ID, "model_"+utils.RandomString(4), "read", false)
	}

	stats, err := testStore.ListTopORMStats(context.Background(), ListTopORMStatsParams{
		EnvID: env.ID, Period: period.Add(-1 * time.Minute), Limit: 3,
	})
	require.NoError(t, err)
	require.Len(t, stats, 3)
}
func TestListTopORMStats_FiltersByPeriod(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	now := time.Now().UTC().Truncate(time.Hour)
	old := now.Add(-48 * time.Hour)

	// Old stat
	_, err := testStore.InsertORMStat(context.Background(), InsertORMStatParams{
		EnvID: env.ID, Model: "old.model", Method: "read",
		CallCount: 10, TotalMs: 100, N1Detected: false, Period: old,
	})
	require.NoError(t, err)

	// Recent stat
	_, err = testStore.InsertORMStat(context.Background(), InsertORMStatParams{
		EnvID: env.ID, Model: "new.model", Method: "read",
		CallCount: 10, TotalMs: 200, N1Detected: false, Period: now,
	})
	require.NoError(t, err)

	stats, err := testStore.ListTopORMStats(context.Background(), ListTopORMStatsParams{
		EnvID: env.ID, Period: now.Add(-1 * time.Hour), Limit: 10,
	})
	require.NoError(t, err)
	require.Len(t, stats, 1)
	require.Equal(t, "new.model", stats[0].Model)
}
func TestListTopORMStats_EnvIsolation(t *testing.T) {
	reg := createRegisteredTenant(t)
	env1 := createTestEnvironment(t, reg.Tenant.ID, "production")
	env2 := createTestEnvironment(t, reg.Tenant.ID, "production")
	period := time.Now().UTC().Truncate(time.Hour)

	createTestORMStat(t, env1.ID, "env1.model", "read", false)
	createTestORMStat(t, env2.ID, "env2.model", "read", false)

	stats, err := testStore.ListTopORMStats(context.Background(), ListTopORMStatsParams{
		EnvID: env1.ID, Period: period.Add(-1 * time.Minute), Limit: 10,
	})
	require.NoError(t, err)
	for _, s := range stats {
		require.Equal(t, env1.ID, s.EnvID)
	}
}

func TestListTopORMStats_Empty(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	stats, err := testStore.ListTopORMStats(context.Background(), ListTopORMStatsParams{
		EnvID: env.ID, Period: time.Now().Add(-1 * time.Hour), Limit: 10,
	})
	require.NoError(t, err)
	require.Empty(t, stats)
}

// ═══════════════════════════════════════════════════════════════
//  ListORMStatsByPeriod
// ═══════════════════════════════════════════════════════════════

func TestListORMStatsByPeriod_DateRange(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	now := time.Now().UTC().Truncate(time.Hour)

	// Inside range
	_, err := testStore.InsertORMStat(context.Background(), InsertORMStatParams{
		EnvID: env.ID, Model: "in.range", Method: "read",
		CallCount: 10, TotalMs: 100, N1Detected: false, Period: now,
	})
	require.NoError(t, err)

	// Outside range
	_, err = testStore.InsertORMStat(context.Background(), InsertORMStatParams{
		EnvID: env.ID, Model: "out.range", Method: "read",
		CallCount: 10, TotalMs: 100, N1Detected: false, Period: now.Add(-48 * time.Hour),
	})
	require.NoError(t, err)

	stats, err := testStore.ListORMStatsByPeriod(context.Background(), ListORMStatsByPeriodParams{
		EnvID:    env.ID,
		Period:   now.Add(-1 * time.Hour),
		Period_2: now.Add(1 * time.Hour),
	})
	require.NoError(t, err)
	require.Len(t, stats, 1)
	require.Equal(t, "in.range", stats[0].Model)
}
func TestListORMStatsByPeriod_OrderByTotalMsDesc(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	period := time.Now().UTC().Truncate(time.Hour)

	for _, ms := range []int32{50, 200, 150} {
		_, err := testStore.InsertORMStat(context.Background(), InsertORMStatParams{
			EnvID: env.ID, Model: "m_" + utils.RandomString(4), Method: "read",
			CallCount: 10, TotalMs: ms, N1Detected: false, Period: period,
		})
		require.NoError(t, err)
	}

	stats, err := testStore.ListORMStatsByPeriod(context.Background(), ListORMStatsByPeriodParams{
		EnvID: env.ID, Period: period.Add(-1 * time.Minute), Period_2: period.Add(1 * time.Minute),
	})
	require.NoError(t, err)
	for i := 1; i < len(stats); i++ {
		require.GreaterOrEqual(t, stats[i-1].TotalMs, stats[i].TotalMs)
	}
}

// ═══════════════════════════════════════════════════════════════
//  ListN1Detections
// ═══════════════════════════════════════════════════════════════

func TestListN1Detections_OnlyN1(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	createTestORMStat(t, env.ID, "n1.model", "search", true)
	createTestORMStat(t, env.ID, "safe.model", "search", false)

	period := time.Now().UTC().Truncate(time.Hour).Add(-1 * time.Minute)
	n1s, err := testStore.ListN1Detections(context.Background(), ListN1DetectionsParams{
		EnvID: env.ID, Period: period, Limit: 10,
	})
	require.NoError(t, err)
	require.Len(t, n1s, 1)
	require.Equal(t, "n1.model", n1s[0].Model)
	require.True(t, n1s[0].N1Detected)
}
func TestListN1Detections_OrderByCallCountDesc(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	period := time.Now().UTC().Truncate(time.Hour)

	for _, cc := range []int32{50, 300, 150} {
		_, err := testStore.InsertORMStat(context.Background(), InsertORMStatParams{
			EnvID: env.ID, Model: "n1_" + utils.RandomString(4), Method: "search",
			CallCount: cc, TotalMs: 100, N1Detected: true, Period: period,
		})
		require.NoError(t, err)
	}

	n1s, err := testStore.ListN1Detections(context.Background(), ListN1DetectionsParams{
		EnvID: env.ID, Period: period.Add(-1 * time.Minute), Limit: 10,
	})
	require.NoError(t, err)
	for i := 1; i < len(n1s); i++ {
		require.GreaterOrEqual(t, n1s[i-1].CallCount, n1s[i].CallCount)
	}
}
func TestListN1Detections_RespectsLimit(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	for i := 0; i < 5; i++ {
		createTestORMStat(t, env.ID, "n1_"+utils.RandomString(4), "search", true)
	}

	period := time.Now().UTC().Truncate(time.Hour).Add(-1 * time.Minute)
	n1s, err := testStore.ListN1Detections(context.Background(), ListN1DetectionsParams{
		EnvID: env.ID, Period: period, Limit: 2,
	})
	require.NoError(t, err)
	require.Len(t, n1s, 2)
}
func TestListN1Detections_Empty(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	// Only non-N1 stats
	createTestORMStat(t, env.ID, "safe.model", "read", false)

	period := time.Now().UTC().Truncate(time.Hour).Add(-1 * time.Minute)
	n1s, err := testStore.ListN1Detections(context.Background(), ListN1DetectionsParams{
		EnvID: env.ID, Period: period, Limit: 10,
	})
	require.NoError(t, err)
	require.Empty(t, n1s)
}

// ═══════════════════════════════════════════════════════════════
//  GetORMStatsAggregated
// ═══════════════════════════════════════════════════════════════

func TestGetORMStatsAggregated_AggregatesCorrectly(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	period := time.Now().UTC().Truncate(time.Hour)

	// Two stats for same model+method
	for _, ms := range []int32{100, 200} {
		_, err := testStore.InsertORMStat(context.Background(), InsertORMStatParams{
			EnvID: env.ID, Model: "res.partner", Method: "search_read",
			CallCount: 50, TotalMs: ms, MaxMs: int32Ptr(ms),
			N1Detected: ms == 200, // second one has N1
			Period:     period,
		})
		require.NoError(t, err)
	}

	rows, err := testStore.GetORMStatsAggregated(context.Background(), GetORMStatsAggregatedParams{
		EnvID: env.ID, Period: period.Add(-1 * time.Minute), Limit: 10,
	})
	require.NoError(t, err)

	// Find our aggregated row
	var found bool
	for _, r := range rows {
		if r.Model == "res.partner" && r.Method == "search_read" {
			found = true
			require.Equal(t, int32(100), r.TotalCalls)      // 50+50
			require.Equal(t, int32(300), r.TotalDurationMs) // 100+200
			require.True(t, r.HasN1)                        // bool_or(true, false) = true
		}
	}
	require.True(t, found)
}
func TestGetORMStatsAggregated_Empty(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	rows, err := testStore.GetORMStatsAggregated(context.Background(), GetORMStatsAggregatedParams{
		EnvID: env.ID, Period: time.Now().Add(-1 * time.Hour), Limit: 10,
	})
	require.NoError(t, err)
	require.Empty(t, rows)
}

// ═══════════════════════════════════════════════════════════════
//  DeleteOldORMStats
// ═══════════════════════════════════════════════════════════════

func TestDeleteOldORMStats_DeletesOld(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	createTestORMStat(t, env.ID, "old.model", "read", false)

	// Future cutoff → deletes everything
	result, err := testStore.DeleteOldORMStats(context.Background(), DeleteOldORMStatsParams{
		TenantID: reg.Tenant.ID,
		Period:   time.Now().Add(1 * time.Hour),
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, result.RowsAffected(), int64(1))
}
func TestDeleteOldORMStats_CutoffInPast_KeepsAll(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	createTestORMStat(t, env.ID, "keep.model", "read", false)

	result, err := testStore.DeleteOldORMStats(context.Background(), DeleteOldORMStatsParams{
		TenantID: reg.Tenant.ID,
		Period:   time.Now().Add(-24 * time.Hour),
	})
	require.NoError(t, err)
	require.Equal(t, int64(0), result.RowsAffected())
}
func TestDeleteOldORMStats_TenantIsolation(t *testing.T) {
	reg1 := createRegisteredTenant(t)
	reg2 := createRegisteredTenant(t)
	env1 := createTestEnvironment(t, reg1.Tenant.ID, "production")
	env2 := createTestEnvironment(t, reg2.Tenant.ID, "production")

	createTestORMStat(t, env1.ID, "t1.model", "read", false)
	createTestORMStat(t, env2.ID, "t2.model", "read", false)

	// Delete tenant 1's stats
	_, err := testStore.DeleteOldORMStats(context.Background(), DeleteOldORMStatsParams{
		TenantID: reg1.Tenant.ID,
		Period:   time.Now().Add(1 * time.Hour),
	})
	require.NoError(t, err)

	// Tenant 2's stats must survive
	period := time.Now().UTC().Truncate(time.Hour).Add(-1 * time.Minute)
	stats2, err := testStore.ListTopORMStats(context.Background(), ListTopORMStatsParams{
		EnvID: env2.ID, Period: period, Limit: 10,
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(stats2), 1)
}
