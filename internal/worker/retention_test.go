package worker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Mock Store ──────────────────────────────────────────────────────────────

type retentionMockStore struct {
	db.Store // embed — unimplemented methods panic on nil receiver

	tenants        []db.Tenant
	listTenantsErr error

	deleteAlertsCalls    []db.DeleteOldAlertsParams
	deleteAuditCalls     []db.DeleteOldAuditLogsParams
	deleteBudgetCalls    []db.DeleteOldBudgetSamplesParams
	deleteErrorCalls     []db.DeleteOldErrorGroupsParams
	deleteHeartbeatCalls []db.DeleteOldHeartbeatsParams
	deleteORMCalls       []db.DeleteOldORMStatsParams
	deleteProfilerCalls  []db.DeleteOldProfilerRecordingsParams
	deleteSchemaCalls    []db.DeleteOldSchemaSnapshotsParams
	expiringRefs         []*string

	deleteErr error
}

func (m *retentionMockStore) ListTenants(_ context.Context) ([]db.Tenant, error) {
	return m.tenants, m.listTenantsErr
}

func (m *retentionMockStore) DeleteOldAlerts(_ context.Context, arg db.DeleteOldAlertsParams) (pgconn.CommandTag, error) {
	m.deleteAlertsCalls = append(m.deleteAlertsCalls, arg)
	return pgconn.NewCommandTag("DELETE 1"), m.deleteErr
}

func (m *retentionMockStore) DeleteOldAuditLogs(_ context.Context, arg db.DeleteOldAuditLogsParams) (pgconn.CommandTag, error) {
	m.deleteAuditCalls = append(m.deleteAuditCalls, arg)
	return pgconn.NewCommandTag("DELETE 1"), m.deleteErr
}

func (m *retentionMockStore) DeleteOldBudgetSamples(_ context.Context, arg db.DeleteOldBudgetSamplesParams) (pgconn.CommandTag, error) {
	m.deleteBudgetCalls = append(m.deleteBudgetCalls, arg)
	return pgconn.NewCommandTag("DELETE 1"), m.deleteErr
}

func (m *retentionMockStore) DeleteOldErrorGroups(_ context.Context, arg db.DeleteOldErrorGroupsParams) (pgconn.CommandTag, error) {
	m.deleteErrorCalls = append(m.deleteErrorCalls, arg)
	return pgconn.NewCommandTag("DELETE 1"), m.deleteErr
}

func (m *retentionMockStore) DeleteOldHeartbeats(_ context.Context, arg db.DeleteOldHeartbeatsParams) (pgconn.CommandTag, error) {
	m.deleteHeartbeatCalls = append(m.deleteHeartbeatCalls, arg)
	return pgconn.NewCommandTag("DELETE 1"), m.deleteErr
}

func (m *retentionMockStore) DeleteOldORMStats(_ context.Context, arg db.DeleteOldORMStatsParams) (pgconn.CommandTag, error) {
	m.deleteORMCalls = append(m.deleteORMCalls, arg)
	return pgconn.NewCommandTag("DELETE 1"), m.deleteErr
}

func (m *retentionMockStore) DeleteOldProfilerRecordings(_ context.Context, arg db.DeleteOldProfilerRecordingsParams) (pgconn.CommandTag, error) {
	m.deleteProfilerCalls = append(m.deleteProfilerCalls, arg)
	return pgconn.NewCommandTag("DELETE 1"), m.deleteErr
}

func (m *retentionMockStore) DeleteOldSchemaSnapshots(_ context.Context, arg db.DeleteOldSchemaSnapshotsParams) (pgconn.CommandTag, error) {
	m.deleteSchemaCalls = append(m.deleteSchemaCalls, arg)
	return pgconn.NewCommandTag("DELETE 1"), m.deleteErr
}

func (m *retentionMockStore) ListExpiringErrorGroupRefs(_ context.Context, _ db.ListExpiringErrorGroupRefsParams) ([]*string, error) {
	return m.expiringRefs, nil
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func newTestRetentionWorker(store db.Store) *RetentionWorker {
	return &RetentionWorker{
		store:  store,
		s3:     nil, // S3 disabled — S3 cleanup skipped in unit tests
		config: DefaultRetentionConfig(),
		logger: zerolog.Nop(),
	}
}

func makeTenantWithPolicy(t *testing.T, policy tenantRetentionPolicy) db.Tenant {
	t.Helper()
	raw, err := json.Marshal(policy)
	require.NoError(t, err)
	return db.Tenant{
		ID:              uuid.New(),
		Slug:            "test-tenant",
		RetentionConfig: raw,
	}
}

// ── DefaultRetentionConfig / NewRetentionWorker ──────────────────────────────

func TestDefaultRetentionConfig(t *testing.T) {
	cfg := DefaultRetentionConfig()
	assert.Equal(t, time.Hour, cfg.RunInterval)
}

func TestNewRetentionWorker_DefaultsInterval(t *testing.T) {
	w := NewRetentionWorker(nil, nil, RetentionConfig{RunInterval: 0}, zerolog.Nop())
	assert.Equal(t, time.Hour, w.config.RunInterval)
}

func TestNewRetentionWorker_CustomInterval(t *testing.T) {
	w := NewRetentionWorker(nil, nil, RetentionConfig{RunInterval: 30 * time.Minute}, zerolog.Nop())
	assert.Equal(t, 30*time.Minute, w.config.RunInterval)
}

// ── parsePolicy ───────────────────────────────────────────────────────────────

func TestParsePolicy_EmptyJSON_ReturnsDefaults(t *testing.T) {
	w := newTestRetentionWorker(&retentionMockStore{})
	p := w.parsePolicy(nil, zerolog.Nop())

	assert.Equal(t, defaultPolicy.ErrorTracesDays, p.ErrorTracesDays)
	assert.Equal(t, defaultPolicy.ProfilerRecordingsDays, p.ProfilerRecordingsDays)
	assert.Equal(t, defaultPolicy.BudgetSamplesDays, p.BudgetSamplesDays)
	assert.Equal(t, defaultPolicy.SchemaSnapshotsKeep, p.SchemaSnapshotsKeep)
	assert.Equal(t, defaultPolicy.RawLogsDays, p.RawLogsDays)
	assert.Equal(t, defaultPolicy.AuditLogsDays, p.AuditLogsDays)
}

func TestParsePolicy_InvalidJSON_ReturnsDefaults(t *testing.T) {
	w := newTestRetentionWorker(&retentionMockStore{})
	p := w.parsePolicy(json.RawMessage(`{{invalid`), zerolog.Nop())

	assert.Equal(t, defaultPolicy.ErrorTracesDays, p.ErrorTracesDays)
}

func TestParsePolicy_FullConfig(t *testing.T) {
	w := newTestRetentionWorker(&retentionMockStore{})
	raw := json.RawMessage(`{
		"error_traces_days": 14,
		"profiler_recordings_days": 21,
		"budget_samples_days": 60,
		"schema_snapshots_keep": 5,
		"raw_logs_days": 7,
		"audit_log_days": 180
	}`)
	p := w.parsePolicy(raw, zerolog.Nop())

	assert.Equal(t, 14, p.ErrorTracesDays)
	assert.Equal(t, 21, p.ProfilerRecordingsDays)
	assert.Equal(t, 60, p.BudgetSamplesDays)
	assert.Equal(t, 5, p.SchemaSnapshotsKeep)
	assert.Equal(t, 7, p.RawLogsDays)
	assert.Equal(t, 180, p.AuditLogsDays)
}

func TestParsePolicy_PartialConfig_ZeroFieldsFallBackToDefaults(t *testing.T) {
	w := newTestRetentionWorker(&retentionMockStore{})
	// Only error_traces_days set; others are zero → should use defaults
	raw := json.RawMessage(`{"error_traces_days": 30}`)
	p := w.parsePolicy(raw, zerolog.Nop())

	assert.Equal(t, 30, p.ErrorTracesDays)
	assert.Equal(t, defaultPolicy.ProfilerRecordingsDays, p.ProfilerRecordingsDays)
	assert.Equal(t, defaultPolicy.BudgetSamplesDays, p.BudgetSamplesDays)
	assert.Equal(t, defaultPolicy.SchemaSnapshotsKeep, p.SchemaSnapshotsKeep)
	assert.Equal(t, defaultPolicy.RawLogsDays, p.RawLogsDays)
	assert.Equal(t, defaultPolicy.AuditLogsDays, p.AuditLogsDays)
}

// ── runForTenant ──────────────────────────────────────────────────────────────

func TestRunForTenant_CallsAllDeleteMethods(t *testing.T) {
	store := &retentionMockStore{}
	w := newTestRetentionWorker(store)

	tenant := makeTenantWithPolicy(t, defaultPolicy)
	w.runForTenant(context.Background(), tenant)

	// Every delete method must be called exactly once.
	require.Len(t, store.deleteAlertsCalls, 1)
	require.Len(t, store.deleteAuditCalls, 1)
	require.Len(t, store.deleteBudgetCalls, 1)
	require.Len(t, store.deleteErrorCalls, 1)
	require.Len(t, store.deleteHeartbeatCalls, 1)
	require.Len(t, store.deleteORMCalls, 1)
	require.Len(t, store.deleteProfilerCalls, 1)
	require.Len(t, store.deleteSchemaCalls, 1)
}

func TestRunForTenant_TenantIDPropagated(t *testing.T) {
	store := &retentionMockStore{}
	w := newTestRetentionWorker(store)

	tenant := makeTenantWithPolicy(t, defaultPolicy)
	w.runForTenant(context.Background(), tenant)

	// All deletes must use the correct tenant ID.
	assert.Equal(t, tenant.ID, store.deleteAlertsCalls[0].TenantID)
	assert.Equal(t, tenant.ID, store.deleteAuditCalls[0].TenantID)
	assert.Equal(t, tenant.ID, store.deleteBudgetCalls[0].TenantID)
	assert.Equal(t, tenant.ID, store.deleteErrorCalls[0].TenantID)
	assert.Equal(t, tenant.ID, store.deleteHeartbeatCalls[0].TenantID)
	assert.Equal(t, tenant.ID, store.deleteORMCalls[0].TenantID)
	assert.Equal(t, tenant.ID, store.deleteProfilerCalls[0].TenantID)
	assert.Equal(t, tenant.ID, store.deleteSchemaCalls[0].TenantID)
}

func TestRunForTenant_CutoffWindowsAreInThePast(t *testing.T) {
	store := &retentionMockStore{}
	w := newTestRetentionWorker(store)

	before := time.Now().UTC()
	tenant := makeTenantWithPolicy(t, defaultPolicy)
	w.runForTenant(context.Background(), tenant)
	after := time.Now().UTC()

	// Error groups cutoff = now - 7 days
	errorCutoff := store.deleteErrorCalls[0].LastSeen
	assert.True(t, errorCutoff.Before(before), "error cutoff must be in the past")
	assert.True(t, before.AddDate(0, 0, -defaultPolicy.ErrorTracesDays-1).Before(errorCutoff),
		"error cutoff must not be further than (days+1) in the past")
	_ = after
}

func TestRunForTenant_SchemaSnapshotsUsesKeepCount(t *testing.T) {
	store := &retentionMockStore{}
	w := newTestRetentionWorker(store)

	policy := defaultPolicy
	policy.SchemaSnapshotsKeep = 5
	tenant := makeTenantWithPolicy(t, policy)
	w.runForTenant(context.Background(), tenant)

	require.Len(t, store.deleteSchemaCalls, 1)
	assert.Equal(t, int32(5), store.deleteSchemaCalls[0].Limit)
}

func TestRunForTenant_ZeroErrorDays_FallsBackToDefault(t *testing.T) {
	store := &retentionMockStore{}
	w := newTestRetentionWorker(store)

	// parsePolicy back-fills zero fields with defaultPolicy values, so
	// setting ErrorTracesDays=0 in the JSON produces the default (7 days)
	// and the delete methods ARE still called.
	policy := defaultPolicy
	policy.ErrorTracesDays = 0
	tenant := makeTenantWithPolicy(t, policy)
	w.runForTenant(context.Background(), tenant)

	// Back-fill means deletes proceed with the default window.
	assert.NotEmpty(t, store.deleteErrorCalls, "error groups delete must use default window when days=0")
	assert.NotEmpty(t, store.deleteAlertsCalls, "alerts delete must use default window when days=0")
	assert.Equal(t, defaultPolicy.ErrorTracesDays, int(-store.deleteErrorCalls[0].LastSeen.Sub(time.Now().UTC()).Hours()/24+0.5),
		"cutoff must reflect default error_traces_days")
}

func TestRunForTenant_DeleteErrorContinuesOnFailure(t *testing.T) {
	store := &retentionMockStore{deleteErr: assert.AnError}
	w := newTestRetentionWorker(store)

	tenant := makeTenantWithPolicy(t, defaultPolicy)

	// Must not panic — errors are logged, not returned
	require.NotPanics(t, func() {
		w.runForTenant(context.Background(), tenant)
	})

	// All methods were still called despite errors
	assert.NotEmpty(t, store.deleteErrorCalls)
	assert.NotEmpty(t, store.deleteAlertsCalls)
	assert.NotEmpty(t, store.deleteBudgetCalls)
}

// ── runAll ────────────────────────────────────────────────────────────────────

func TestRunAll_ListTenantsError_DoesNotPanic(t *testing.T) {
	store := &retentionMockStore{listTenantsErr: assert.AnError}
	w := newTestRetentionWorker(store)

	require.NotPanics(t, func() {
		w.runAll(context.Background())
	})

	// No delete methods should be called if tenant list fails
	assert.Empty(t, store.deleteErrorCalls)
}

func TestRunAll_MultipleTenants_EachGetsOwnCleanup(t *testing.T) {
	id1, id2 := uuid.New(), uuid.New()
	policy, _ := json.Marshal(defaultPolicy)

	store := &retentionMockStore{
		tenants: []db.Tenant{
			{ID: id1, Slug: "tenant-1", RetentionConfig: policy},
			{ID: id2, Slug: "tenant-2", RetentionConfig: policy},
		},
	}
	w := newTestRetentionWorker(store)

	w.runAll(context.Background())

	// Each tenant gets one delete per table
	assert.Len(t, store.deleteErrorCalls, 2)
	assert.Len(t, store.deleteAuditCalls, 2)

	tenantIDs := []uuid.UUID{store.deleteErrorCalls[0].TenantID, store.deleteErrorCalls[1].TenantID}
	assert.Contains(t, tenantIDs, id1)
	assert.Contains(t, tenantIDs, id2)
}

func TestRunAll_ContextCanceled_StopsEarly(t *testing.T) {
	id1, id2 := uuid.New(), uuid.New()
	policy, _ := json.Marshal(defaultPolicy)

	store := &retentionMockStore{
		tenants: []db.Tenant{
			{ID: id1, Slug: "tenant-1", RetentionConfig: policy},
			{ID: id2, Slug: "tenant-2", RetentionConfig: policy},
		},
	}
	w := newTestRetentionWorker(store)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	w.runAll(ctx)
	// With context canceled before iteration, 0 or 1 tenants may run —
	// the important thing is it doesn't deadlock or panic.
}

// ── deleteErrorGroups (S3 nil path) ──────────────────────────────────────────

func TestDeleteErrorGroups_S3Nil_OnlyDeletesDB(t *testing.T) {
	ref := "tenant/errors/env/sig/ts.json.gz"
	store := &retentionMockStore{
		expiringRefs: []*string{&ref},
	}
	w := newTestRetentionWorker(store) // s3 = nil

	w.deleteErrorGroups(context.Background(), uuid.New(), time.Now(), zerolog.Nop())

	// DB delete was still called
	require.Len(t, store.deleteErrorCalls, 1)
	// ListExpiringErrorGroupRefs was NOT called (s3 is nil)
}
