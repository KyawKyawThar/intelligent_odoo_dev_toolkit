package db

import (
	"Intelligent_Dev_ToolKit_Odoo/utils"
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// createTestAnonProfile creates an anon_profile in "draft" status.
func createTestAnonProfile(t *testing.T, tenantID, createdBy uuid.UUID, sourceEnvID, targetEnvID *uuid.UUID) AnonProfile {
	t.Helper()
	profile, err := testStore.CreateAnonProfile(context.Background(), CreateAnonProfileParams{
		TenantID:   tenantID,
		CreatedBy:  &createdBy,
		Name:       "Test Anon Profile " + utils.RandomString(4),
		SourceEnv:  sourceEnvID,
		TargetEnv:  targetEnvID,
		FieldRules: json.RawMessage(`[{"model":"res.partner","field":"name","strategy":"FAKE"}]`),
		Status:     "draft",
	})
	require.NoError(t, err)
	require.NotZero(t, profile.ID)
	return profile
}

// buildRunAnonArg builds a valid RunAnonymizationTxParams.
func buildRunAnonArg(profileID, tenantID, userID uuid.UUID, status string) RunAnonymizationTxParams {
	return RunAnonymizationTxParams{
		ProfileID: profileID,
		TenantID:  tenantID,
		UserID:    userID,
		AuditRef:  "s3://odoodevtools-data/" + tenantID.String() + "/anonymizer/" + profileID.String() + "/audit_" + utils.RandomString(8) + ".json.gz",
		Status:    status,
	}
}

// getLatestAuditLog fetches the most recent audit log for a tenant + action.
func getLatestAuditLog(t *testing.T, tenantID uuid.UUID, action string) AuditLog {
	t.Helper()
	logs, err := testStore.GetAuditLogsByTenant(context.Background(), GetAuditLogsByTenantParams{
		TenantID: tenantID,
		Action:   action,
	})
	require.NoError(t, err)
	require.NotEmpty(t, logs, "expected audit log with action %q to exist", action)
	return logs[0]
}
func TestRunAnonymizationTx_Running_UpdatesStatusAndWritesAuditLog(t *testing.T) {
	reg := createRegisteredTenant(t)
	profile := createTestAnonProfile(t, reg.Tenant.ID, reg.User.ID, nil, nil)

	arg := buildRunAnonArg(profile.ID, reg.Tenant.ID, reg.User.ID, "running")

	err := testStore.RunAnonymizationTx(context.Background(), arg)
	require.NoError(t, err)

	// Profile status updated
	updated, err := testStore.GetAnonProfile(context.Background(), GetAnonProfileParams{
		ID:       profile.ID,
		TenantID: arg.TenantID,
	})
	require.NoError(t, err)
	require.Equal(t, "running", updated.Status)
	require.NotNil(t, updated.LastRunBy)
	require.Equal(t, reg.User.ID, *updated.LastRunBy)
	require.NotNil(t, updated.AuditRef)
	require.Equal(t, arg.AuditRef, *updated.AuditRef)

	// Audit log written
	log := getLatestAuditLog(t, reg.Tenant.ID, "anon.running")
	require.Equal(t, "anon.running", log.Action)
	require.Equal(t, "anon_profiles", *log.Resource)
	require.Equal(t, profile.ID.String(), *log.ResourceID)
	require.Equal(t, reg.User.ID, *log.UserID)
	require.Equal(t, reg.Tenant.ID, log.TenantID)
}
func TestRunAnonymizationTx_Completed_UpdatesStatusAndWritesAuditLog(t *testing.T) {
	reg := createRegisteredTenant(t)
	profile := createTestAnonProfile(t, reg.Tenant.ID, reg.User.ID, nil, nil)

	// draft → running → completed
	err := testStore.RunAnonymizationTx(context.Background(),
		buildRunAnonArg(profile.ID, reg.Tenant.ID, reg.User.ID, "running"))
	require.NoError(t, err)

	err = testStore.RunAnonymizationTx(context.Background(),
		buildRunAnonArg(profile.ID, reg.Tenant.ID, reg.User.ID, "completed"))
	require.NoError(t, err)

	updated, err := testStore.GetAnonProfile(context.Background(), GetAnonProfileParams{
		ID:       profile.ID,
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "completed", updated.Status)

	log := getLatestAuditLog(t, reg.Tenant.ID, "anon.completed")
	require.Equal(t, "anon.completed", log.Action)
	require.Equal(t, profile.ID.String(), *log.ResourceID)
}
func TestRunAnonymizationTx_Failed_UpdatesStatusAndWritesAuditLog(t *testing.T) {
	reg := createRegisteredTenant(t)
	profile := createTestAnonProfile(t, reg.Tenant.ID, reg.User.ID, nil, nil)

	// draft → running → failed
	err := testStore.RunAnonymizationTx(context.Background(),
		buildRunAnonArg(profile.ID, reg.Tenant.ID, reg.User.ID, "running"))
	require.NoError(t, err)

	err = testStore.RunAnonymizationTx(context.Background(),
		buildRunAnonArg(profile.ID, reg.Tenant.ID, reg.User.ID, "failed"))
	require.NoError(t, err)

	updated, err := testStore.GetAnonProfile(context.Background(), GetAnonProfileParams{
		ID:       profile.ID,
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "failed", updated.Status)

	log := getLatestAuditLog(t, reg.Tenant.ID, "anon.failed")
	require.Equal(t, "anon.failed", log.Action)
}

// ─────────────────────────────────────────────────────────────
//  RunAnonymizationTx — audit log metadata
// ─────────────────────────────────────────────────────────────

func TestRunAnonymizationTx_AuditLog_ContainsCorrectMetadata(t *testing.T) {
	reg := createRegisteredTenant(t)
	profile := createTestAnonProfile(t, reg.Tenant.ID, reg.User.ID, nil, nil)

	arg := buildRunAnonArg(profile.ID, reg.Tenant.ID, reg.User.ID, "completed")

	err := testStore.RunAnonymizationTx(context.Background(), arg)
	require.NoError(t, err)

	log := getLatestAuditLog(t, reg.Tenant.ID, "anon.completed")

	// Metadata must be valid JSON with all 3 fields
	var meta map[string]any
	require.NoError(t, json.Unmarshal(log.Metadata, &meta))
	require.Equal(t, profile.ID.String(), meta["profile_id"])
	require.Equal(t, "completed", meta["status"])
	require.Equal(t, arg.AuditRef, meta["audit_ref"])
}
func TestRunAnonymizationTx_AuditLog_ActionMatchesStatus(t *testing.T) {
	reg := createRegisteredTenant(t)

	for _, status := range []string{"running", "completed", "failed"} {
		t.Run(status, func(t *testing.T) {
			profile := createTestAnonProfile(t, reg.Tenant.ID, reg.User.ID, nil, nil)

			err := testStore.RunAnonymizationTx(context.Background(),
				buildRunAnonArg(profile.ID, reg.Tenant.ID, reg.User.ID, status))
			require.NoError(t, err)

			log := getLatestAuditLog(t, reg.Tenant.ID, "anon."+status)
			require.Equal(t, "anon."+status, log.Action)
		})
	}
}
func TestRunAnonymizationTx_SetsLastRunBy(t *testing.T) {
	reg := createRegisteredTenant(t)
	profile := createTestAnonProfile(t, reg.Tenant.ID, reg.User.ID, nil, nil)

	err := testStore.RunAnonymizationTx(context.Background(),
		buildRunAnonArg(profile.ID, reg.Tenant.ID, reg.User.ID, "running"))
	require.NoError(t, err)

	updated, err := testStore.GetAnonProfile(context.Background(), GetAnonProfileParams{
		ID:       profile.ID,
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, updated.LastRunBy)
	require.Equal(t, reg.User.ID, *updated.LastRunBy)
}
func TestRunAnonymizationTx_SetsAuditRef(t *testing.T) {
	reg := createRegisteredTenant(t)
	profile := createTestAnonProfile(t, reg.Tenant.ID, reg.User.ID, nil, nil)

	expectedRef := "s3://odoodevtools-data/" + reg.Tenant.ID.String() + "/audit_abc123.json.gz"

	err := testStore.RunAnonymizationTx(context.Background(), RunAnonymizationTxParams{
		ProfileID: profile.ID,
		TenantID:  reg.Tenant.ID,
		UserID:    reg.User.ID,
		AuditRef:  expectedRef,
		Status:    "completed",
	})
	require.NoError(t, err)

	updated, err := testStore.GetAnonProfile(context.Background(), GetAnonProfileParams{
		ID:       profile.ID,
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, updated.AuditRef)
	require.Equal(t, expectedRef, *updated.AuditRef)
}
func TestRunAnonymizationTx_OverwritesPreviousAuditRef(t *testing.T) {
	reg := createRegisteredTenant(t)
	profile := createTestAnonProfile(t, reg.Tenant.ID, reg.User.ID, nil, nil)

	// First run
	err := testStore.RunAnonymizationTx(context.Background(), RunAnonymizationTxParams{
		ProfileID: profile.ID,
		TenantID:  reg.Tenant.ID,
		UserID:    reg.User.ID,
		AuditRef:  "s3://bucket/first_run.json.gz",
		Status:    "completed",
	})
	require.NoError(t, err)

	// Second run with new audit ref
	newRef := "s3://bucket/second_run.json.gz"
	err = testStore.RunAnonymizationTx(context.Background(), RunAnonymizationTxParams{
		ProfileID: profile.ID,
		TenantID:  reg.Tenant.ID,
		UserID:    reg.User.ID,
		AuditRef:  newRef,
		Status:    "completed",
	})
	require.NoError(t, err)

	updated, err := testStore.GetAnonProfile(context.Background(), GetAnonProfileParams{
		ID:       profile.ID,
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)
	require.Equal(t, newRef, *updated.AuditRef)
}

// ─────────────────────────────────────────────────────────────
//  RunAnonymizationTx — atomicity / rollback
// ─────────────────────────────────────────────────────────────

func TestRunAnonymizationTx_InvalidProfileID_RollsBack(t *testing.T) {
	reg := createRegisteredTenant(t)

	arg := RunAnonymizationTxParams{
		ProfileID: uuid.New(), // does not exist → UpdateAnonProfileStatus fails
		TenantID:  reg.Tenant.ID,
		UserID:    reg.User.ID,
		AuditRef:  "s3://bucket/nonexistent.json.gz",
		Status:    "running",
	}

	err := testStore.RunAnonymizationTx(context.Background(), arg)
	require.Error(t, err)
	require.ErrorContains(t, err, "update anon profile status")

	// No audit log must be written — step 1 failed so tx rolled back
	logs, err := testStore.GetAuditLogsByTenant(context.Background(), GetAuditLogsByTenantParams{
		TenantID: reg.Tenant.ID,
		Action:   "anon.running",
	})
	require.NoError(t, err)
	require.Empty(t, logs, "audit log must not be written if profile update failed")
}
func TestRunAnonymizationTx_WrongTenantID_RollsBack(t *testing.T) {
	reg1 := createRegisteredTenant(t)
	reg2 := createRegisteredTenant(t)

	// Profile belongs to tenant 1
	profile := createTestAnonProfile(t, reg1.Tenant.ID, reg1.User.ID, nil, nil)

	// Tenant 2 tries to update it
	err := testStore.RunAnonymizationTx(context.Background(), RunAnonymizationTxParams{
		ProfileID: profile.ID,
		TenantID:  reg2.Tenant.ID, // wrong tenant
		UserID:    reg2.User.ID,
		AuditRef:  "s3://bucket/wrong_tenant.json.gz",
		Status:    "running",
	})
	require.Error(t, err, "wrong tenant must be rejected")

	// Profile must be unchanged
	unchanged, err := testStore.GetAnonProfile(context.Background(), GetAnonProfileParams{
		ID:       profile.ID,
		TenantID: reg1.Tenant.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "draft", unchanged.Status, "profile status must remain draft")
	require.Nil(t, unchanged.LastRunBy)
}

// ─────────────────────────────────────────────────────────────
//  RunAnonymizationTx — full persistence round-trip
// ─────────────────────────────────────────────────────────────

func TestRunAnonymizationTx_PersistsBothChanges(t *testing.T) {
	reg := createRegisteredTenant(t)
	profile := createTestAnonProfile(t, reg.Tenant.ID, reg.User.ID, nil, nil)

	arg := buildRunAnonArg(profile.ID, reg.Tenant.ID, reg.User.ID, "completed")

	err := testStore.RunAnonymizationTx(context.Background(), arg)
	require.NoError(t, err)

	// 1. Profile persisted correctly
	updatedProfile, err := testStore.GetAnonProfile(context.Background(), GetAnonProfileParams{
		ID:       profile.ID,
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "completed", updatedProfile.Status)
	require.Equal(t, arg.AuditRef, *updatedProfile.AuditRef)
	require.Equal(t, reg.User.ID, *updatedProfile.LastRunBy)

	// 2. Audit log persisted correctly
	log := getLatestAuditLog(t, reg.Tenant.ID, "anon.completed")
	require.Equal(t, "anon.completed", log.Action)
	require.Equal(t, "anon_profiles", *log.Resource)
	require.Equal(t, profile.ID.String(), *log.ResourceID)
	require.Equal(t, reg.User.ID, *log.UserID)
	require.NotNil(t, log.Metadata)

	// 3. Metadata JSON is valid and complete
	var meta map[string]any
	require.NoError(t, json.Unmarshal(log.Metadata, &meta))
	require.Equal(t, profile.ID.String(), meta["profile_id"])
	require.Equal(t, "completed", meta["status"])
	require.Equal(t, arg.AuditRef, meta["audit_ref"])
}

func TestRunAnonymizationTx_Completed_SetsLastRun(t *testing.T) {
	reg := createRegisteredTenant(t)
	profile := createTestAnonProfile(t, reg.Tenant.ID, reg.User.ID, nil, nil)

	// last_run must be nil initially
	require.Nil(t, profile.LastRun)

	err := testStore.RunAnonymizationTx(context.Background(),
		buildRunAnonArg(profile.ID, reg.Tenant.ID, reg.User.ID, "completed"))
	require.NoError(t, err)

	updated, err := testStore.GetAnonProfile(context.Background(), GetAnonProfileParams{
		ID:       profile.ID,
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)

	// SQL: last_run = CASE WHEN $2 = 'completed' THEN now() ELSE last_run END
	require.NotNil(t, updated.LastRun, "last_run must be set when status is completed")
}
func TestRunAnonymizationTx_Running_DoesNotSetLastRun(t *testing.T) {
	reg := createRegisteredTenant(t)
	profile := createTestAnonProfile(t, reg.Tenant.ID, reg.User.ID, nil, nil)

	err := testStore.RunAnonymizationTx(context.Background(),
		buildRunAnonArg(profile.ID, reg.Tenant.ID, reg.User.ID, "running"))
	require.NoError(t, err)

	updated, err := testStore.GetAnonProfile(context.Background(), GetAnonProfileParams{
		ID:       profile.ID,
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)

	// "running" must NOT update last_run (it was nil before)
	require.Nil(t, updated.LastRun, "last_run must remain nil for running status")
}

func TestRunAnonymizationTx_Failed_DoesNotSetLastRun(t *testing.T) {
	reg := createRegisteredTenant(t)
	profile := createTestAnonProfile(t, reg.Tenant.ID, reg.User.ID, nil, nil)

	err := testStore.RunAnonymizationTx(context.Background(),
		buildRunAnonArg(profile.ID, reg.Tenant.ID, reg.User.ID, "failed"))
	require.NoError(t, err)

	updated, err := testStore.GetAnonProfile(context.Background(), GetAnonProfileParams{
		ID:       profile.ID,
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)

	require.Nil(t, updated.LastRun, "last_run must remain nil for failed status")
}
func TestRunAnonymizationTx_CompletedThenFailed_LastRunPreserved(t *testing.T) {
	reg := createRegisteredTenant(t)
	profile := createTestAnonProfile(t, reg.Tenant.ID, reg.User.ID, nil, nil)

	// First: completed → sets last_run
	err := testStore.RunAnonymizationTx(context.Background(),
		buildRunAnonArg(profile.ID, reg.Tenant.ID, reg.User.ID, "completed"))
	require.NoError(t, err)

	afterCompleted, err := testStore.GetAnonProfile(context.Background(), GetAnonProfileParams{
		ID:       profile.ID,
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, afterCompleted.LastRun)
	savedLastRun := *afterCompleted.LastRun

	// Second: failed → last_run must NOT be overwritten (CASE WHEN preserves it)
	err = testStore.RunAnonymizationTx(context.Background(),
		buildRunAnonArg(profile.ID, reg.Tenant.ID, reg.User.ID, "failed"))
	require.NoError(t, err)

	afterFailed, err := testStore.GetAnonProfile(context.Background(), GetAnonProfileParams{
		ID:       profile.ID,
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, afterFailed.LastRun)
	require.Equal(t, savedLastRun, *afterFailed.LastRun,
		"last_run must be preserved when status is not completed")
}
func TestRunAnonymizationTx_UpdatedAtAdvances(t *testing.T) {
	reg := createRegisteredTenant(t)
	profile := createTestAnonProfile(t, reg.Tenant.ID, reg.User.ID, nil, nil)
	originalUpdatedAt := profile.UpdatedAt

	err := testStore.RunAnonymizationTx(context.Background(),
		buildRunAnonArg(profile.ID, reg.Tenant.ID, reg.User.ID, "running"))
	require.NoError(t, err)

	updated, err := testStore.GetAnonProfile(context.Background(), GetAnonProfileParams{
		ID:       profile.ID,
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)
	require.True(t, updated.UpdatedAt.After(originalUpdatedAt) || updated.UpdatedAt.Equal(originalUpdatedAt),
		"updated_at must advance after status change")
}

func TestRunAnonymizationTx_DoesNotMutateProfileFields(t *testing.T) {
	reg := createRegisteredTenant(t)
	profile := createTestAnonProfile(t, reg.Tenant.ID, reg.User.ID, nil, nil)

	err := testStore.RunAnonymizationTx(context.Background(),
		buildRunAnonArg(profile.ID, reg.Tenant.ID, reg.User.ID, "completed"))
	require.NoError(t, err)

	updated, err := testStore.GetAnonProfile(context.Background(), GetAnonProfileParams{
		ID:       profile.ID,
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)

	// These fields must be untouched by the transaction
	require.Equal(t, profile.Name, updated.Name)
	require.Equal(t, profile.TenantID, updated.TenantID)
	require.Equal(t, profile.CreatedBy, updated.CreatedBy)
	require.Equal(t, profile.SourceEnv, updated.SourceEnv)
	require.Equal(t, profile.TargetEnv, updated.TargetEnv)

	var originalRules, updatedRules any
	require.NoError(t, json.Unmarshal(profile.FieldRules, &originalRules))
	require.NoError(t, json.Unmarshal(updated.FieldRules, &updatedRules))
	require.Equal(t, originalRules, updatedRules, "field_rules must not be mutated")
}

func TestRunAnonymizationTx_WithSourceAndTargetEnv(t *testing.T) {
	reg := createRegisteredTenant(t)

	srcEnv, err := testStore.CreateEnvironment(context.Background(), CreateEnvironmentParams{
		TenantID:     reg.Tenant.ID,
		Name:         "Source Env",
		OdooUrl:      "https://source.example.com",
		DbName:       "odoo_source",
		EnvType:      "production",
		Status:       "connected",
		FeatureFlags: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	tgtEnv, err := testStore.CreateEnvironment(context.Background(), CreateEnvironmentParams{
		TenantID:     reg.Tenant.ID,
		Name:         "Target Env",
		OdooUrl:      "https://target.example.com",
		DbName:       "odoo_target",
		EnvType:      "staging",
		Status:       "connected",
		FeatureFlags: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	profile := createTestAnonProfile(t, reg.Tenant.ID, reg.User.ID, &srcEnv.ID, &tgtEnv.ID)

	err = testStore.RunAnonymizationTx(context.Background(),
		buildRunAnonArg(profile.ID, reg.Tenant.ID, reg.User.ID, "completed"))
	require.NoError(t, err)

	updated, err := testStore.GetAnonProfile(context.Background(), GetAnonProfileParams{
		ID:       profile.ID,
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "completed", updated.Status)
	require.NotNil(t, updated.SourceEnv)
	require.NotNil(t, updated.TargetEnv)
	require.Equal(t, srcEnv.ID, *updated.SourceEnv)
	require.Equal(t, tgtEnv.ID, *updated.TargetEnv)
}

func TestRunAnonymizationTx_FullLifecycle(t *testing.T) {
	reg := createRegisteredTenant(t)
	profile := createTestAnonProfile(t, reg.Tenant.ID, reg.User.ID, nil, nil)
	require.Equal(t, "draft", profile.Status)

	// Step 1: draft → running
	err := testStore.RunAnonymizationTx(context.Background(),
		buildRunAnonArg(profile.ID, reg.Tenant.ID, reg.User.ID, "running"))
	require.NoError(t, err)

	p1, err := testStore.GetAnonProfile(context.Background(), GetAnonProfileParams{
		ID: profile.ID, TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "running", p1.Status)
	require.Nil(t, p1.LastRun, "last_run must be nil while running")

	// Step 2: running → completed
	err = testStore.RunAnonymizationTx(context.Background(),
		buildRunAnonArg(profile.ID, reg.Tenant.ID, reg.User.ID, "completed"))
	require.NoError(t, err)

	p2, err := testStore.GetAnonProfile(context.Background(), GetAnonProfileParams{
		ID: profile.ID, TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "completed", p2.Status)
	require.NotNil(t, p2.LastRun, "last_run must be set after completed")

	// Verify both audit logs exist
	runningLog := getLatestAuditLog(t, reg.Tenant.ID, "anon.running")
	require.Equal(t, profile.ID.String(), *runningLog.ResourceID)

	completedLog := getLatestAuditLog(t, reg.Tenant.ID, "anon.completed")
	require.Equal(t, profile.ID.String(), *completedLog.ResourceID)
}
func TestRunAnonymizationTx_FullLifecycle_WithFailure(t *testing.T) {
	reg := createRegisteredTenant(t)
	profile := createTestAnonProfile(t, reg.Tenant.ID, reg.User.ID, nil, nil)

	// draft → running
	err := testStore.RunAnonymizationTx(context.Background(),
		buildRunAnonArg(profile.ID, reg.Tenant.ID, reg.User.ID, "running"))
	require.NoError(t, err)

	// running → failed
	err = testStore.RunAnonymizationTx(context.Background(),
		buildRunAnonArg(profile.ID, reg.Tenant.ID, reg.User.ID, "failed"))
	require.NoError(t, err)

	p, err := testStore.GetAnonProfile(context.Background(), GetAnonProfileParams{
		ID: profile.ID, TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "failed", p.Status)
	require.Nil(t, p.LastRun, "last_run must remain nil if never completed")

	// Retry: failed → running → completed
	err = testStore.RunAnonymizationTx(context.Background(),
		buildRunAnonArg(profile.ID, reg.Tenant.ID, reg.User.ID, "running"))
	require.NoError(t, err)

	err = testStore.RunAnonymizationTx(context.Background(),
		buildRunAnonArg(profile.ID, reg.Tenant.ID, reg.User.ID, "completed"))
	require.NoError(t, err)

	pFinal, err := testStore.GetAnonProfile(context.Background(), GetAnonProfileParams{
		ID: profile.ID, TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "completed", pFinal.Status)
	require.NotNil(t, pFinal.LastRun, "last_run must be set after eventual completion")
}
func TestRunAnonymizationTx_MultipleProfiles_Isolated(t *testing.T) {
	reg := createRegisteredTenant(t)
	profile1 := createTestAnonProfile(t, reg.Tenant.ID, reg.User.ID, nil, nil)
	profile2 := createTestAnonProfile(t, reg.Tenant.ID, reg.User.ID, nil, nil)

	// Only update profile1
	err := testStore.RunAnonymizationTx(context.Background(),
		buildRunAnonArg(profile1.ID, reg.Tenant.ID, reg.User.ID, "completed"))
	require.NoError(t, err)

	// profile1 must be completed
	p1, err := testStore.GetAnonProfile(context.Background(), GetAnonProfileParams{
		ID: profile1.ID, TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "completed", p1.Status)

	// profile2 must still be draft — untouched
	p2, err := testStore.GetAnonProfile(context.Background(), GetAnonProfileParams{
		ID: profile2.ID, TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "draft", p2.Status)
	require.Nil(t, p2.LastRunBy)
	require.Nil(t, p2.AuditRef)
}

func TestRunAnonymizationTx_AuditLog_TenantIsolation(t *testing.T) {
	reg1 := createRegisteredTenant(t)
	reg2 := createRegisteredTenant(t)

	profile1 := createTestAnonProfile(t, reg1.Tenant.ID, reg1.User.ID, nil, nil)

	err := testStore.RunAnonymizationTx(context.Background(),
		buildRunAnonArg(profile1.ID, reg1.Tenant.ID, reg1.User.ID, "completed"))
	require.NoError(t, err)

	// Tenant 1 must have the audit log
	logs1, err := testStore.GetAuditLogsByTenant(context.Background(), GetAuditLogsByTenantParams{
		TenantID: reg1.Tenant.ID,
		Action:   "anon.completed",
	})
	require.NoError(t, err)
	require.NotEmpty(t, logs1)

	// Tenant 2 must NOT see tenant 1's audit logs
	logs2, err := testStore.GetAuditLogsByTenant(context.Background(), GetAuditLogsByTenantParams{
		TenantID: reg2.Tenant.ID,
		Action:   "anon.completed",
	})
	require.NoError(t, err)
	require.Empty(t, logs2, "tenant 2 must not see tenant 1's audit logs")
}
func TestRunAnonymizationTx_ConcurrentRuns(t *testing.T) {
	reg := createRegisteredTenant(t)

	const count = 8
	profiles := make([]AnonProfile, count)
	for i := 0; i < count; i++ {
		profiles[i] = createTestAnonProfile(t, reg.Tenant.ID, reg.User.ID, nil, nil)
	}

	var wg sync.WaitGroup
	errs := make([]error, count)

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = testStore.RunAnonymizationTx(context.Background(),
				buildRunAnonArg(profiles[idx].ID, reg.Tenant.ID, reg.User.ID, "completed"))
		}(i)
	}
	wg.Wait()

	for i := range count {
		require.NoError(t, errs[i], "goroutine %d failed", i)

		p, err := testStore.GetAnonProfile(context.Background(), GetAnonProfileParams{
			ID: profiles[i].ID, TenantID: reg.Tenant.ID,
		})
		require.NoError(t, err)
		require.Equal(t, "completed", p.Status, "profile %d must be completed", i)
		require.NotNil(t, p.LastRun)
	}
}

func TestRunAnonymizationTx_DifferentUserUpdatesLastRunBy(t *testing.T) {
	reg := createRegisteredTenant(t)
	profile := createTestAnonProfile(t, reg.Tenant.ID, reg.User.ID, nil, nil)

	// First run by creator
	err := testStore.RunAnonymizationTx(context.Background(),
		buildRunAnonArg(profile.ID, reg.Tenant.ID, reg.User.ID, "completed"))
	require.NoError(t, err)

	p1, err := testStore.GetAnonProfile(context.Background(), GetAnonProfileParams{
		ID: profile.ID, TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)
	require.Equal(t, reg.User.ID, *p1.LastRunBy)

	// Second run by a different user
	// We must create a real user in the DB, otherwise the foreign key constraint on last_run_by fails.
	reg2 := createRegisteredTenant(t)
	// We'll use reg2.User as the "other user".
	// NOTE: In a real scenario, this user should probably belong to the SAME tenant (reg.Tenant).
	// But the foreign key only checks if the user exists in the users table, not if they belong to the tenant.
	// To be strictly correct for a multi-tenant app, we should probably add a second user to the existing tenant.
	// However, createRegisteredTenant creates a new tenant + user.
	// Let's just create a standalone user, or use reg2.User.
	// Since the FK is just `REFERENCES users(id)`, reg2.User.ID is valid.

	otherUser := reg2.User.ID
	err = testStore.RunAnonymizationTx(context.Background(), RunAnonymizationTxParams{
		ProfileID: profile.ID,
		TenantID:  reg.Tenant.ID,
		UserID:    otherUser,
		AuditRef:  "s3://bucket/other_user_run.json.gz",
		Status:    "completed",
	})
	require.NoError(t, err)

	p2, err := testStore.GetAnonProfile(context.Background(), GetAnonProfileParams{
		ID: profile.ID, TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)
	require.Equal(t, otherUser, *p2.LastRunBy, "last_run_by must reflect the latest runner")

	// Audit log must reference the other user
	log := getLatestAuditLog(t, reg.Tenant.ID, "anon.completed")
	require.Equal(t, otherUser, *log.UserID)
}

func TestRunAnonymizationTx_CreatedByUnchanged(t *testing.T) {
	reg := createRegisteredTenant(t)
	profile := createTestAnonProfile(t, reg.Tenant.ID, reg.User.ID, nil, nil)

	reg2 := createRegisteredTenant(t)
	otherUser := reg2.User.ID
	err := testStore.RunAnonymizationTx(context.Background(), RunAnonymizationTxParams{
		ProfileID: profile.ID,
		TenantID:  reg.Tenant.ID,
		UserID:    otherUser,
		AuditRef:  "s3://bucket/run.json.gz",
		Status:    "completed",
	})
	require.NoError(t, err)

	updated, err := testStore.GetAnonProfile(context.Background(), GetAnonProfileParams{
		ID: profile.ID, TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)

	// created_by must still be the original creator, not the runner
	require.Equal(t, reg.User.ID, *updated.CreatedBy,
		"created_by must remain the original creator")
	require.Equal(t, otherUser, *updated.LastRunBy,
		"last_run_by must be the runner")
}
