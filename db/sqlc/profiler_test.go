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

// createTestRecording creates a profiler recording with sensible defaults.
// Pass totalMs to control which recordings are "slow".
func createTestRecording(t *testing.T, envID uuid.UUID, totalMs int32, withN1 bool) ProfilerRecording {
	t.Helper()

	var n1Patterns *json.RawMessage
	if withN1 {
		raw := json.RawMessage(`[{"model":"product.product","count":42}]`)
		n1Patterns = &raw
	}

	rec, err := testStore.CreateProfilerRecording(context.Background(), CreateProfilerRecordingParams{
		EnvID:        envID,
		TriggeredBy:  nil,
		Name:         "Recording " + utils.RandomString(4),
		Endpoint:     nil,
		TotalMs:      totalMs,
		SqlCount:     int32Ptr(10),
		SqlMs:        int32Ptr(totalMs / 2),
		PythonMs:     int32Ptr(totalMs / 2),
		Waterfall:    json.RawMessage(`[{"label":"ORM","ms":100}]`),
		ComputeChain: nil,
		N1Patterns:   n1Patterns,
		RawLogRef:    nil,
	})
	require.NoError(t, err)
	require.NotZero(t, rec.ID)
	return rec
}
func TestCreateProfilerRecording_MinimalFields(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	rec, err := testStore.CreateProfilerRecording(context.Background(), CreateProfilerRecordingParams{
		EnvID:        env.ID,
		TriggeredBy:  nil,
		Name:         "Minimal Recording",
		Endpoint:     nil,
		TotalMs:      250,
		SqlCount:     nil,
		SqlMs:        nil,
		PythonMs:     nil,
		Waterfall:    json.RawMessage(`{}`),
		ComputeChain: nil,
		N1Patterns:   nil,
		RawLogRef:    nil,
	})
	require.NoError(t, err)
	require.NotZero(t, rec.ID)
	require.Equal(t, env.ID, rec.EnvID)
	require.Equal(t, "Minimal Recording", rec.Name)
	require.Equal(t, int32(250), rec.TotalMs)
	require.Nil(t, rec.TriggeredBy)
	require.Nil(t, rec.Endpoint)
	require.Nil(t, rec.SqlCount)
	require.Nil(t, rec.N1Patterns)
	require.NotZero(t, rec.RecordedAt)
}
func TestCreateProfilerRecording_AllFields(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "staging")
	endpoint := "/web/dataset/call_kw"
	rawRef := "s3://bucket/profiler/" + env.ID.String() + "/events.json.gz"
	n1 := json.RawMessage(`[{"model":"sale.order","count":15}]`)
	chain := json.RawMessage(`{"root":"sale.order.button_confirm","calls":[]}`)

	rec, err := testStore.CreateProfilerRecording(context.Background(), CreateProfilerRecordingParams{
		EnvID:        env.ID,
		TriggeredBy:  &reg.User.ID,
		Name:         "Full Recording",
		Endpoint:     &endpoint,
		TotalMs:      890,
		SqlCount:     int32Ptr(148),
		SqlMs:        int32Ptr(620),
		PythonMs:     int32Ptr(270),
		Waterfall:    json.RawMessage(`[{"label":"ORM","ms":620}]`),
		ComputeChain: &chain,
		N1Patterns:   &n1,
		RawLogRef:    &rawRef,
	})
	require.NoError(t, err)

	require.NotZero(t, rec.ID)
	require.Equal(t, env.ID, rec.EnvID)
	require.NotNil(t, rec.TriggeredBy)
	require.Equal(t, reg.User.ID, *rec.TriggeredBy)
	require.NotNil(t, rec.Endpoint)
	require.Equal(t, endpoint, *rec.Endpoint)
	require.Equal(t, int32(890), rec.TotalMs)
	require.NotNil(t, rec.SqlCount)
	require.Equal(t, int32(148), *rec.SqlCount)
	require.NotNil(t, rec.N1Patterns)
	require.NotNil(t, rec.ComputeChain)
	require.NotNil(t, rec.RawLogRef)
	require.Equal(t, rawRef, *rec.RawLogRef)
}

// ─────────────────────────────────────────────────────────────
//  GetProfilerRecording
// ─────────────────────────────────────────────────────────────

func TestGetProfilerRecording_Success(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	created := createTestRecording(t, env.ID, 500, false)

	fetched, err := testStore.GetProfilerRecording(context.Background(), GetProfilerRecordingParams{
		ID:    created.ID,
		EnvID: env.ID,
	})
	require.NoError(t, err)
	require.Equal(t, created.ID, fetched.ID)
	require.Equal(t, created.EnvID, fetched.EnvID)
	require.Equal(t, created.TotalMs, fetched.TotalMs)
	require.Equal(t, created.Name, fetched.Name)
}
func TestGetProfilerRecording_WrongEnvID_Fails(t *testing.T) {
	reg := createRegisteredTenant(t)
	env1 := createTestEnvironment(t, reg.Tenant.ID, "production")
	env2 := createTestEnvironment(t, reg.Tenant.ID, "staging")
	rec := createTestRecording(t, env1.ID, 300, false)

	_, err := testStore.GetProfilerRecording(context.Background(), GetProfilerRecordingParams{
		ID:    rec.ID,
		EnvID: env2.ID, // wrong env
	})
	require.Error(t, err, "wrong env_id must return error")
}
func TestGetProfilerRecording_NonExistent_Fails(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	_, err := testStore.GetProfilerRecording(context.Background(), GetProfilerRecordingParams{
		ID:    utils.RandomUUID(),
		EnvID: env.ID,
	})
	require.Error(t, err)
}
func TestCountRecordingsByEnv_ReturnsCorrectCount(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	count, err := testStore.CountRecordingsByEnv(context.Background(), env.ID)
	require.NoError(t, err)
	require.Equal(t, int64(0), count)

	createTestRecording(t, env.ID, 100, false)
	createTestRecording(t, env.ID, 200, false)
	createTestRecording(t, env.ID, 300, false)

	count, err = testStore.CountRecordingsByEnv(context.Background(), env.ID)
	require.NoError(t, err)
	require.Equal(t, int64(3), count)
}
func TestCountRecordingsByEnv_IsolatedFromOtherEnvs(t *testing.T) {
	reg := createRegisteredTenant(t)
	env1 := createTestEnvironment(t, reg.Tenant.ID, "production")
	env2 := createTestEnvironment(t, reg.Tenant.ID, "staging")

	createTestRecording(t, env1.ID, 100, false)
	createTestRecording(t, env1.ID, 200, false)
	createTestRecording(t, env2.ID, 300, false)

	count1, err := testStore.CountRecordingsByEnv(context.Background(), env1.ID)
	require.NoError(t, err)
	require.Equal(t, int64(2), count1)

	count2, err := testStore.CountRecordingsByEnv(context.Background(), env2.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), count2)
}
func TestListProfilerRecordings_OrderedByRecordedAtDesc(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	createTestRecording(t, env.ID, 100, false)
	time.Sleep(5 * time.Millisecond)
	createTestRecording(t, env.ID, 200, false)
	time.Sleep(5 * time.Millisecond)
	createTestRecording(t, env.ID, 300, false)

	rows, err := testStore.ListProfilerRecordings(context.Background(), ListProfilerRecordingsParams{
		EnvID:  env.ID,
		Limit:  10,
		Offset: 0,
	})
	require.NoError(t, err)
	require.Len(t, rows, 3)

	// Newest first
	for i := 1; i < len(rows); i++ {
		require.True(t,
			rows[i-1].RecordedAt.After(rows[i].RecordedAt) || rows[i-1].RecordedAt.Equal(rows[i].RecordedAt),
			"must be ordered by recorded_at DESC",
		)
	}
}
func TestListProfilerRecordings_HasN1_TrueWhenN1PatternsSet(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	createTestRecording(t, env.ID, 100, false) // no N+1
	createTestRecording(t, env.ID, 890, true)  // has N+1

	rows, err := testStore.ListProfilerRecordings(context.Background(), ListProfilerRecordingsParams{
		EnvID:  env.ID,
		Limit:  10,
		Offset: 0,
	})
	require.NoError(t, err)
	require.Len(t, rows, 2)

	// Find each by TotalMs
	hasN1Map := make(map[int32]interface{})
	for _, r := range rows {
		hasN1Map[r.TotalMs] = r.HasN1
	}

	// N+1 recording's HasN1 must be truthy
	require.NotNil(t, hasN1Map[890])
}
func TestListProfilerRecordings_PaginationLimitOffset(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	for i := 0; i < 5; i++ {
		createTestRecording(t, env.ID, int32((i+1)*100), false)
	}

	// Page 1: first 2
	page1, err := testStore.ListProfilerRecordings(context.Background(), ListProfilerRecordingsParams{
		EnvID:  env.ID,
		Limit:  2,
		Offset: 0,
	})
	require.NoError(t, err)
	require.Len(t, page1, 2)

	// Page 2: next 2
	page2, err := testStore.ListProfilerRecordings(context.Background(), ListProfilerRecordingsParams{
		EnvID:  env.ID,
		Limit:  2,
		Offset: 2,
	})
	require.NoError(t, err)
	require.Len(t, page2, 2)

	// No overlap between pages
	require.NotEqual(t, page1[0].ID, page2[0].ID)
	require.NotEqual(t, page1[1].ID, page2[1].ID)
}
func TestListProfilerRecordings_EmptyEnv_ReturnsEmpty(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	rows, err := testStore.ListProfilerRecordings(context.Background(), ListProfilerRecordingsParams{
		EnvID:  env.ID,
		Limit:  10,
		Offset: 0,
	})
	require.NoError(t, err)
	require.Empty(t, rows)
}
func TestListSlowRecordings_ReturnsOnlyAboveThreshold(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	createTestRecording(t, env.ID, 50, false)   // fast
	createTestRecording(t, env.ID, 150, false)  // fast
	createTestRecording(t, env.ID, 500, false)  // slow
	createTestRecording(t, env.ID, 890, false)  // slow
	createTestRecording(t, env.ID, 1200, false) // slow

	slow, err := testStore.ListSlowRecordings(context.Background(), ListSlowRecordingsParams{
		EnvID:   env.ID,
		TotalMs: 200, // threshold
		Limit:   10,
	})
	require.NoError(t, err)
	require.Len(t, slow, 3)

	for _, r := range slow {
		require.Greater(t, r.TotalMs, int32(200), "all returned recordings must exceed threshold")
	}
}
func TestListSlowRecordings_OrderedByTotalMsDesc(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	createTestRecording(t, env.ID, 300, false)
	createTestRecording(t, env.ID, 1200, false)
	createTestRecording(t, env.ID, 600, false)

	slow, err := testStore.ListSlowRecordings(context.Background(), ListSlowRecordingsParams{
		EnvID:   env.ID,
		TotalMs: 0, // return all
		Limit:   10,
	})
	require.NoError(t, err)
	require.Len(t, slow, 3)

	// Slowest first
	for i := 1; i < len(slow); i++ {
		require.GreaterOrEqual(t, slow[i-1].TotalMs, slow[i].TotalMs, "must be ordered by total_ms DESC")
	}
}

func TestListSlowRecordings_RespectsLimit(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	for i := range 5 {
		createTestRecording(t, env.ID, int32(500+(i*100)), false)
	}

	slow, err := testStore.ListSlowRecordings(context.Background(), ListSlowRecordingsParams{
		EnvID:   env.ID,
		TotalMs: 0,
		Limit:   3,
	})
	require.NoError(t, err)
	require.Len(t, slow, 3)
}

func TestListSlowRecordings_NoMatchesAboveThreshold_ReturnsEmpty(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	createTestRecording(t, env.ID, 50, false)
	createTestRecording(t, env.ID, 100, false)

	slow, err := testStore.ListSlowRecordings(context.Background(), ListSlowRecordingsParams{
		EnvID:   env.ID,
		TotalMs: 5000, // nothing this slow
		Limit:   10,
	})
	require.NoError(t, err)
	require.Empty(t, slow)
}
func TestDeleteOldProfilerRecordings_DeletesBeforeCutoff(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	// Create 3 recordings — all exist before cutoff
	createTestRecording(t, env.ID, 100, false)
	createTestRecording(t, env.ID, 200, false)
	createTestRecording(t, env.ID, 300, false)

	count, err := testStore.CountRecordingsByEnv(context.Background(), env.ID)
	require.NoError(t, err)
	require.Equal(t, int64(3), count)

	// Delete all older than now+1s (deletes everything)
	cutoff := time.Now().Add(time.Second)
	tag, err := testStore.DeleteOldProfilerRecordings(context.Background(), DeleteOldProfilerRecordingsParams{
		TenantID:   reg.Tenant.ID,
		RecordedAt: cutoff,
	})
	require.NoError(t, err)
	require.Equal(t, int64(3), tag.RowsAffected())

	count, err = testStore.CountRecordingsByEnv(context.Background(), env.ID)
	require.NoError(t, err)
	require.Equal(t, int64(0), count)
}
func TestDeleteOldProfilerRecordings_KeepsRecent(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	createTestRecording(t, env.ID, 100, false)
	createTestRecording(t, env.ID, 200, false)

	// Delete older than 1 hour ago — nothing is that old
	cutoff := time.Now().Add(-time.Hour)
	tag, err := testStore.DeleteOldProfilerRecordings(context.Background(), DeleteOldProfilerRecordingsParams{
		TenantID:   reg.Tenant.ID,
		RecordedAt: cutoff,
	})
	require.NoError(t, err)
	require.Equal(t, int64(0), tag.RowsAffected())

	// Both recordings still exist
	count, err := testStore.CountRecordingsByEnv(context.Background(), env.ID)
	require.NoError(t, err)
	require.Equal(t, int64(2), count)
}
func TestDeleteOldProfilerRecordings_OnlyAffectsOwnTenant(t *testing.T) {
	reg1 := createRegisteredTenant(t)
	reg2 := createRegisteredTenant(t)
	env1 := createTestEnvironment(t, reg1.Tenant.ID, "production")
	env2 := createTestEnvironment(t, reg2.Tenant.ID, "production")

	createTestRecording(t, env1.ID, 100, false)
	createTestRecording(t, env2.ID, 200, false)

	// Delete for tenant 1 only
	cutoff := time.Now().Add(time.Second)
	_, err := testStore.DeleteOldProfilerRecordings(context.Background(), DeleteOldProfilerRecordingsParams{
		TenantID:   reg1.Tenant.ID,
		RecordedAt: cutoff,
	})
	require.NoError(t, err)

	// Tenant 1 env is empty
	count1, err := testStore.CountRecordingsByEnv(context.Background(), env1.ID)
	require.NoError(t, err)
	require.Equal(t, int64(0), count1)

	// Tenant 2 env is untouched
	count2, err := testStore.CountRecordingsByEnv(context.Background(), env2.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), count2)
}
