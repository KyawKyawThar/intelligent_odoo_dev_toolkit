package db

import (
	"Intelligent_Dev_ToolKit_Odoo/utils"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// buildRegisterArg builds a valid RegisterTenantParams for any plan.

func buildRegisterArg(t *testing.T, plan string) RegisterTenantParams {

	t.Helper()
	hashed_password, err := utils.HashPassword(utils.RandomString(8))

	require.NoError(t, err)

	return RegisterTenantParams{
		TenantName:   utils.RandomOwner() + "Corp",
		Slug:         utils.RandomSlug(),
		Plan:         plan,
		OwnerEmail:   utils.RandomEmail(),
		PasswordHash: hashed_password,
		FullName:     utils.RandomOwner(),
	}
}

// createRegisteredTenant is a test helper that registers a tenant and asserts
// basic success. Use it as a pre-condition for other tests (e.g. DeleteEnvironmentTx).

func createRegisteredTenant(t *testing.T) RegisterTenantResult {
	t.Helper()

	arg := buildRegisterArg(t, "cloud")
	result, err := testStore.RegisterTenantTx(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, result.Tenant)
	return result
}

func TestRegisterTenantTx_Cloud(t *testing.T) {
	arg := buildRegisterArg(t, "cloud")

	result, err := testStore.RegisterTenantTx(context.Background(), arg)
	require.NoError(t, err)

	// Tenant
	require.NotEmpty(t, result.Tenant)
	require.NotZero(t, result.Tenant.ID)
	require.Equal(t, arg.TenantName, result.Tenant.Name)
	require.Equal(t, arg.Slug, result.Tenant.Slug)
	require.Equal(t, "cloud", result.Tenant.Plan)
	require.Equal(t, "trialing", result.Tenant.PlanStatus)
	require.NotZero(t, result.Tenant.CreatedAt)
	require.NotNil(t, result.Tenant.Settings)
	require.NotNil(t, result.Tenant.RetentionConfig)

	// Owner user
	require.NotEmpty(t, result.User)
	require.NotZero(t, result.User.ID)
	require.Equal(t, arg.OwnerEmail, result.User.Email)
	require.Equal(t, result.Tenant.ID, result.User.TenantID)
	require.True(t, result.User.IsActive)
	require.NotNil(t, result.User.FullName)
	require.Equal(t, arg.FullName, *result.User.FullName)
	require.NotZero(t, result.User.CreatedAt)

	// Subscription
	require.NotEmpty(t, result.Subscription)
	require.NotZero(t, result.Subscription.ID)
	require.Equal(t, result.Tenant.ID, result.Subscription.TenantID)
	require.Equal(t, "cloud", result.Subscription.Plan)
	require.Equal(t, "trialing", result.Subscription.Status)
}
func TestRegisterTenantTx_OnPrem(t *testing.T) {
	arg := buildRegisterArg(t, "onprem")

	result, err := testStore.RegisterTenantTx(context.Background(), arg)
	require.NoError(t, err)

	require.Equal(t, "onprem", result.Tenant.Plan)
	require.Equal(t, "trialing", result.Tenant.PlanStatus)
	require.Equal(t, "onprem", result.Subscription.Plan)
	require.Equal(t, "trialing", result.Subscription.Status)
}

func TestRegisterTenantTx_Enterprise(t *testing.T) {
	arg := buildRegisterArg(t, "enterprise")

	result, err := testStore.RegisterTenantTx(context.Background(), arg)
	require.NoError(t, err)

	require.Equal(t, "enterprise", result.Tenant.Plan)
	require.Equal(t, "trialing", result.Tenant.PlanStatus)
	require.Equal(t, "enterprise", result.Subscription.Plan)
	require.Equal(t, "trialing", result.Subscription.Status)
}

func TestRegisterTenantTx_EmptyFullName_StoredAsNull(t *testing.T) {
	hashedPassword, err := utils.HashPassword(utils.RandomString(8))
	require.NoError(t, err)

	arg := RegisterTenantParams{
		TenantName:   utils.RandomOwner() + " Corp",
		Slug:         utils.RandomSlug(),
		Plan:         "cloud",
		OwnerEmail:   utils.RandomEmail(),
		PasswordHash: hashedPassword,
		FullName:     "", // intentionally empty → must be NULL in DB
	}

	result, err := testStore.RegisterTenantTx(context.Background(), arg)
	require.NoError(t, err)

	require.Nil(t, result.User.FullName, "empty FullName must be stored as NULL, not empty string")
}
func TestRegisterTenantTx_PersistsToDatabase(t *testing.T) {
	arg := buildRegisterArg(t, "cloud")

	result, err := testStore.RegisterTenantTx(context.Background(), arg)
	require.NoError(t, err)

	// Fetch tenant
	fetchedTenant, err := testStore.GetTenantByID(context.Background(), result.Tenant.ID)
	require.NoError(t, err)
	require.Equal(t, result.Tenant.ID, fetchedTenant.ID)
	require.Equal(t, arg.TenantName, fetchedTenant.Name)
	require.Equal(t, arg.Slug, fetchedTenant.Slug)

	// Fetch user
	fetchedUser, err := testStore.GetUserByEmail(context.Background(), GetUserByEmailParams{
		Email:    arg.OwnerEmail,
		TenantID: result.Tenant.ID,
	})
	require.NoError(t, err)
	require.Equal(t, result.User.ID, fetchedUser.ID)
	require.Equal(t, result.Tenant.ID, fetchedUser.TenantID)

	// Fetch subscription
	fetchedSub, err := testStore.GetSubscriptionByTenant(context.Background(), result.Tenant.ID)
	require.NoError(t, err)
	require.Equal(t, result.Subscription.ID, fetchedSub.ID)
	require.Equal(t, result.Tenant.ID, fetchedSub.TenantID)
}
func TestRegisterTenantTx_DuplicateSlug_Fails(t *testing.T) {
	arg1 := buildRegisterArg(t, "cloud")
	_, err := testStore.RegisterTenantTx(context.Background(), arg1)
	require.NoError(t, err)

	// Same slug, different email → must fail on tenants.slug UNIQUE
	arg2 := buildRegisterArg(t, "cloud")
	arg2.Slug = arg1.Slug // force collision

	_, err = testStore.RegisterTenantTx(context.Background(), arg2)
	require.Error(t, err, "duplicate slug must be rejected by DB unique constraint")
}
func TestRegisterTenantTx_DuplicateEmail_SameTenant_Fails(t *testing.T) {
	arg1 := buildRegisterArg(t, "cloud")

	result, err := testStore.RegisterTenantTx(context.Background(), arg1)
	require.NoError(t, err)

	// Directly try to insert a second user with the same email on the same tenant
	hashedPw, err := utils.HashPassword(utils.RandomString(8))
	require.NoError(t, err)

	_, err = testStore.CreateUser(context.Background(), CreateUserParams{
		TenantID:      result.Tenant.ID,
		Email:         arg1.OwnerEmail, // same email, same tenant
		PasswordHash:  hashedPw,
		FullName:      optionalStringPtr("Duplicate User"),
		EmailVerified: true,
		IsActive:      true,
	})
	require.Error(t, err, "duplicate (tenant_id, email) must be rejected")
}
func TestRegisterTenantTx_SameEmail_DifferentTenants_Succeeds(t *testing.T) {
	// Same email is allowed across different tenants
	sharedEmail := utils.RandomEmail()

	hashedPw1, err := utils.HashPassword(utils.RandomString(8))
	require.NoError(t, err)
	hashedPw2, err := utils.HashPassword(utils.RandomString(8))
	require.NoError(t, err)

	arg1 := RegisterTenantParams{
		TenantName:   "Tenant A",
		Slug:         utils.RandomSlug(),
		Plan:         "cloud",
		OwnerEmail:   sharedEmail,
		PasswordHash: hashedPw1,
		FullName:     "Owner A",
	}
	arg2 := RegisterTenantParams{
		TenantName:   "Tenant B",
		Slug:         utils.RandomSlug(),
		Plan:         "cloud",
		OwnerEmail:   sharedEmail, // same email, different tenant
		PasswordHash: hashedPw2,
		FullName:     "Owner B",
	}

	result1, err := testStore.RegisterTenantTx(context.Background(), arg1)
	require.NoError(t, err)

	result2, err := testStore.RegisterTenantTx(context.Background(), arg2)
	require.NoError(t, err)

	// Both tenants must exist and be separate
	require.NotEqual(t, result1.Tenant.ID, result2.Tenant.ID)
	require.NotEqual(t, result1.User.ID, result2.User.ID)
	require.Equal(t, sharedEmail, result1.User.Email)
	require.Equal(t, sharedEmail, result2.User.Email)
}

func TestRegisterTenantTx_Atomicity_NoPartialData(t *testing.T) {
	// Register successfully once
	arg := buildRegisterArg(t, "cloud")
	_, err := testStore.RegisterTenantTx(context.Background(), arg)
	require.NoError(t, err)

	// Second call with same slug → fails at step 1 (CreateTenant)
	// Entire transaction must rollback — no user or subscription should be written
	arg2 := buildRegisterArg(t, "cloud")
	arg2.Slug = arg.Slug // trigger unique violation

	_, err = testStore.RegisterTenantTx(context.Background(), arg2)
	require.Error(t, err)

	// The user from the failed tx must NOT exist in the DB
	// (we can't look up by TenantID since tenant creation failed,
	//  but the email is unique enough to verify absence)
	_, err = testStore.GetUserByEmail(context.Background(), GetUserByEmailParams{
		Email: arg2.OwnerEmail,
		// TenantID intentionally zero — we expect no row found
	})
	require.Error(t, err, "rolled-back user must not exist in DB")
}

// ─────────────────────────────────────────────────────────────
//  defaultRetentionConfig — pure unit tests (no DB)
// ─────────────────────────────────────────────────────────────

func TestDefaultRetentionConfig_Cloud(t *testing.T) {
	cfg := defaultRetentionConfig("cloud")
	require.NotEmpty(t, cfg)
	require.Contains(t, string(cfg), `"error_traces_days": 7`)
	require.Contains(t, string(cfg), `"profiler_recordings_days": 7`)
	require.Contains(t, string(cfg), `"budget_samples_days": 30`)
	require.Contains(t, string(cfg), `"schema_snapshots_keep": 10`)
	require.Contains(t, string(cfg), `"raw_logs_days": 3`)
}

func TestDefaultRetentionConfig_OnPrem(t *testing.T) {
	cfg := defaultRetentionConfig("onprem")
	require.NotEmpty(t, cfg)
	require.Contains(t, string(cfg), `"error_traces_days": 30`)
	require.Contains(t, string(cfg), `"raw_logs_days": 14`)
}

func TestDefaultRetentionConfig_Enterprise(t *testing.T) {
	cfg := defaultRetentionConfig("enterprise")
	require.NotEmpty(t, cfg)
	require.Contains(t, string(cfg), `"error_traces_days": -1`)
	require.Contains(t, string(cfg), `"raw_logs_days": 90`)
}

func TestDefaultRetentionConfig_UnknownPlan_FallsBackToCloud(t *testing.T) {
	cfg := defaultRetentionConfig("nonexistent_plan")
	require.NotEmpty(t, cfg)
	// Must fall back to cloud defaults
	require.Contains(t, string(cfg), `"error_traces_days": 7`)
	require.Contains(t, string(cfg), `"raw_logs_days": 3`)
}
func TestDeleteEnvironmentTx_Success(t *testing.T) {
	reg := createRegisteredTenant(t)

	env, err := testStore.CreateEnvironment(context.Background(), CreateEnvironmentParams{
		TenantID:     reg.Tenant.ID,
		Name:         "Staging Env",
		OdooUrl:      "https://staging.example.com",
		DbName:       "odoo_staging",
		EnvType:      "staging",
		Status:       "disconnected",
		FeatureFlags: json.RawMessage(`{}`),
	})
	require.NoError(t, err)
	require.NotZero(t, env.ID)

	err = testStore.DeleteEnvironmentTx(context.Background(), DeleteEnvironmentTxParams{
		EnvID:    env.ID,
		TenantID: reg.Tenant.ID,
		UserID:   reg.User.ID,
	})
	require.NoError(t, err)

	// Environment must be gone
	_, err = testStore.GetEnvironmentByID(context.Background(), GetEnvironmentByIDParams{
		ID:       env.ID,
		TenantID: reg.Tenant.ID,
	})
	require.Error(t, err, "deleted environment must not be fetchable")
}
func TestDeleteEnvironmentTx_AuditLogWritten(t *testing.T) {
	reg := createRegisteredTenant(t)

	env, err := testStore.CreateEnvironment(context.Background(), CreateEnvironmentParams{
		TenantID:     reg.Tenant.ID,
		Name:         "Prod Env",
		OdooUrl:      "https://prod.example.com",
		DbName:       "odoo_prod",
		EnvType:      "production",
		Status:       "disconnected",
		FeatureFlags: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	err = testStore.DeleteEnvironmentTx(context.Background(), DeleteEnvironmentTxParams{
		EnvID:    env.ID,
		TenantID: reg.Tenant.ID,
		UserID:   reg.User.ID,
	})
	require.NoError(t, err)

	// Audit log must exist
	logs, err := testStore.GetAuditLogsByTenant(context.Background(), GetAuditLogsByTenantParams{
		TenantID: reg.Tenant.ID,
		Action:   "env.delete",
	})
	require.NoError(t, err)
	require.NotEmpty(t, logs)

	log := logs[0]
	require.Equal(t, "env.delete", log.Action)
	require.Equal(t, "environments", *log.Resource)
	require.Equal(t, env.ID.String(), *log.ResourceID)
	require.Equal(t, reg.User.ID, *log.UserID)
	require.Equal(t, reg.Tenant.ID, log.TenantID)
}

// ─────────────────────────────────────────────────────────────
//
//	DeleteEnvironmentTx — security / edge cases
//
// ─────────────────────────────────────────────────────────────
func TestDeleteEnvironmentTx_WrongTenant_Fails(t *testing.T) {
	reg1 := createRegisteredTenant(t)
	reg2 := createRegisteredTenant(t)

	// Create env under tenant 1
	env, err := testStore.CreateEnvironment(context.Background(), CreateEnvironmentParams{
		TenantID:     reg1.Tenant.ID,
		Name:         "Tenant1 Env",
		OdooUrl:      "https://t1.example.com",
		DbName:       "odoo_t1",
		EnvType:      "production",
		Status:       "disconnected",
		FeatureFlags: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	// Tenant 2 tries to delete tenant 1's env — must fail
	err = testStore.DeleteEnvironmentTx(context.Background(), DeleteEnvironmentTxParams{
		EnvID:    env.ID,
		TenantID: reg2.Tenant.ID, // wrong tenant
		UserID:   reg2.User.ID,
	})
	require.Error(t, err, "cross-tenant delete must be rejected")

	// Env must still exist under tenant 1
	fetched, err := testStore.GetEnvironmentByID(context.Background(), GetEnvironmentByIDParams{
		ID:       env.ID,
		TenantID: reg1.Tenant.ID,
	})
	require.NoError(t, err)
	require.Equal(t, env.ID, fetched.ID)
}

func TestDeleteEnvironmentTx_NonExistentEnv_Fails(t *testing.T) {
	reg := createRegisteredTenant(t)

	err := testStore.DeleteEnvironmentTx(context.Background(), DeleteEnvironmentTxParams{
		EnvID:    utils.RandomUUID(),
		TenantID: reg.Tenant.ID,
		UserID:   reg.User.ID,
	})
	require.Error(t, err, "deleting non-existent environment must return an error")
}

func TestDeleteEnvironmentTx_CascadesChildData(t *testing.T) {
	// If your schema has ON DELETE CASCADE on schema_snapshots, error_groups, etc.
	// this test verifies those records are also removed when the env is deleted.
	reg := createRegisteredTenant(t)

	env, err := testStore.CreateEnvironment(context.Background(), CreateEnvironmentParams{
		TenantID:     reg.Tenant.ID,
		Name:         "Cascade Test Env",
		OdooUrl:      "https://cascade.example.com",
		DbName:       "odoo_cascade",
		EnvType:      "staging",
		Status:       "connected",
		FeatureFlags: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	// Create a schema snapshot under this env
	snapshot, err := testStore.CreateSchemaSnapshot(context.Background(), CreateSchemaSnapshotParams{
		EnvID:      env.ID,
		Version:    nil,
		Models:     []byte(`{}`),
		ModelCount: nil,
		FieldCount: nil,
	})
	require.NoError(t, err)
	require.NotZero(t, snapshot.ID)

	// Delete the environment
	err = testStore.DeleteEnvironmentTx(context.Background(), DeleteEnvironmentTxParams{
		EnvID:    env.ID,
		TenantID: reg.Tenant.ID,
		UserID:   reg.User.ID,
	})
	require.NoError(t, err)

	// Snapshot must also be gone (ON DELETE CASCADE)
	_, err = testStore.GetSchemaSnapshotByID(context.Background(), snapshot.ID)
	require.Error(t, err, "cascaded schema_snapshot must not exist after env deletion")
}
