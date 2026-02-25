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

func createTestTenant(t *testing.T) Tenant {
	t.Helper()
	slug := "tenant-" + utils.RandomString(8)
	tenant, err := testStore.CreateTenant(context.Background(), CreateTenantParams{
		Name:            "Test Tenant " + slug,
		Slug:            slug,
		Plan:            "pro",
		PlanStatus:      "active",
		Settings:        json.RawMessage(`{"timezone":"UTC"}`),
		RetentionConfig: json.RawMessage(`{"audit_logs_days":90,"error_groups_days":30}`),
	})
	require.NoError(t, err)
	require.NotZero(t, tenant.ID)
	return tenant
}
func createTestUser(t *testing.T, tenantID uuid.UUID) User {
	t.Helper()
	email := utils.RandomString(8) + "@example.com"
	fullName := "Test User"
	user, err := testStore.CreateUser(context.Background(), CreateUserParams{
		TenantID:     tenantID,
		Email:        email,
		PasswordHash: "$2a$10$" + utils.RandomString(50),
		FullName:     &fullName,
		Role:         "admin",
		IsActive:     true,
	})
	require.NoError(t, err)
	require.NotZero(t, user.ID)
	return user
}

// ═══════════════════════════════════════════════════════════════
//  CreateTenant
// ═══════════════════════════════════════════════════════════════

func TestCreateTenant_AllFields(t *testing.T) {
	trialEnd := time.Now().Add(14 * 24 * time.Hour).UTC().Truncate(time.Microsecond)
	slug := "full-" + utils.RandomString(8)

	tenant, err := testStore.CreateTenant(context.Background(), CreateTenantParams{
		Name:            "Full Tenant",
		Slug:            slug,
		Plan:            "enterprise",
		PlanStatus:      "trialing",
		TrialEndsAt:     &trialEnd,
		Settings:        json.RawMessage(`{"timezone":"Asia/Bangkok","notifications":true}`),
		RetentionConfig: json.RawMessage(`{"audit_logs_days":365}`),
	})
	require.NoError(t, err)

	require.NotZero(t, tenant.ID)
	require.Equal(t, "Full Tenant", tenant.Name)
	require.Equal(t, slug, tenant.Slug)
	require.Equal(t, "enterprise", tenant.Plan)
	require.Equal(t, "trialing", tenant.PlanStatus)
	require.NotNil(t, tenant.TrialEndsAt)
	require.NotZero(t, tenant.CreatedAt)
	require.NotZero(t, tenant.UpdatedAt)
}
func TestCreateTenant_DuplicateSlug_Fails(t *testing.T) {
	slug := "dup-" + utils.RandomString(8)

	_, err := testStore.CreateTenant(context.Background(), CreateTenantParams{
		Name: "First", Slug: slug, Plan: "free", PlanStatus: "active",
		Settings: json.RawMessage(`{}`), RetentionConfig: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	_, err = testStore.CreateTenant(context.Background(), CreateTenantParams{
		Name: "Second", Slug: slug, Plan: "free", PlanStatus: "active",
		Settings: json.RawMessage(`{}`), RetentionConfig: json.RawMessage(`{}`),
	})
	require.Error(t, err, "duplicate slug must be rejected")
}

// ═══════════════════════════════════════════════════════════════
//  GetTenantByID / GetTenantBySlug
// ═══════════════════════════════════════════════════════════════

func TestGetTenantByID_Found(t *testing.T) {
	created := createTestTenant(t)

	fetched, err := testStore.GetTenantByID(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, fetched.ID)
	require.Equal(t, created.Slug, fetched.Slug)
}
func TestGetTenantByID_NotFound(t *testing.T) {
	_, err := testStore.GetTenantByID(context.Background(), uuid.New())
	require.Error(t, err)
}
func TestGetTenantBySlug_Found(t *testing.T) {
	created := createTestTenant(t)

	fetched, err := testStore.GetTenantBySlug(context.Background(), created.Slug)
	require.NoError(t, err)
	require.Equal(t, created.ID, fetched.ID)
}
func TestGetTenantBySlug_NotFound(t *testing.T) {
	_, err := testStore.GetTenantBySlug(context.Background(), "nonexistent-slug-"+utils.RandomString(8))
	require.Error(t, err)
}

// ═══════════════════════════════════════════════════════════════
//  ListTenants
// ═══════════════════════════════════════════════════════════════

func TestListTenants_ContainsCreated(t *testing.T) {
	created := createTestTenant(t)

	tenants, err := testStore.ListTenants(context.Background())
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(tenants), 1)

	found := false
	for _, ten := range tenants {
		if ten.ID == created.ID {
			found = true
		}
	}
	require.True(t, found)
}

// ═══════════════════════════════════════════════════════════════
//  UpdateTenantPlan
// ═══════════════════════════════════════════════════════════════

func TestUpdateTenantPlan(t *testing.T) {
	tenant := createTestTenant(t)
	trialEnd := time.Now().Add(30 * 24 * time.Hour).UTC().Truncate(time.Microsecond)

	updated, err := testStore.UpdateTenantPlan(context.Background(), UpdateTenantPlanParams{
		ID:          tenant.ID,
		Plan:        "enterprise",
		PlanStatus:  "trialing",
		TrialEndsAt: &trialEnd,
	})
	require.NoError(t, err)

	require.Equal(t, "enterprise", updated.Plan)
	require.Equal(t, "trialing", updated.PlanStatus)
	require.NotNil(t, updated.TrialEndsAt)
	require.Equal(t, tenant.Name, updated.Name)
	require.Equal(t, tenant.Slug, updated.Slug)
}
func TestUpdateTenantPlan_UpdatedAtAdvances(t *testing.T) {
	tenant := createTestTenant(t)

	updated, err := testStore.UpdateTenantPlan(context.Background(), UpdateTenantPlanParams{
		ID: tenant.ID, Plan: "free", PlanStatus: "active",
	})
	require.NoError(t, err)
	require.True(t, updated.UpdatedAt.After(tenant.UpdatedAt) ||
		updated.UpdatedAt.Equal(tenant.UpdatedAt))
}
func TestUpdateTenantPlan_NonexistentID(t *testing.T) {
	_, err := testStore.UpdateTenantPlan(context.Background(), UpdateTenantPlanParams{
		ID: uuid.New(), Plan: "free", PlanStatus: "active",
	})
	require.Error(t, err)
}

// ═══════════════════════════════════════════════════════════════
//  UpdateTenantSettings
// ═══════════════════════════════════════════════════════════════

func TestUpdateTenantSettings(t *testing.T) {
	tenant := createTestTenant(t)
	newSettings := json.RawMessage(`{"timezone":"America/New_York","dark_mode":true}`)

	updated, err := testStore.UpdateTenantSettings(context.Background(), UpdateTenantSettingsParams{
		ID:       tenant.ID,
		Name:     "Renamed Tenant",
		Settings: newSettings,
	})
	require.NoError(t, err)

	require.Equal(t, "Renamed Tenant", updated.Name)

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(updated.Settings, &parsed))
	require.Equal(t, "America/New_York", parsed["timezone"])
	require.Equal(t, true, parsed["dark_mode"])

	require.Equal(t, tenant.Plan, updated.Plan)
}

// ═══════════════════════════════════════════════════════════════
//  UpdateTenantRetention
// ═══════════════════════════════════════════════════════════════

func TestUpdateTenantRetention(t *testing.T) {
	tenant := createTestTenant(t)
	newRetention := json.RawMessage(`{"audit_logs_days":180,"error_groups_days":60}`)

	updated, err := testStore.UpdateTenantRetention(context.Background(), UpdateTenantRetentionParams{
		ID:              tenant.ID,
		RetentionConfig: newRetention,
	})
	require.NoError(t, err)

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(updated.RetentionConfig, &parsed))
	require.Equal(t, float64(180), parsed["audit_logs_days"])
	require.Equal(t, float64(60), parsed["error_groups_days"])

	require.Equal(t, tenant.Name, updated.Name)
	require.Equal(t, tenant.Plan, updated.Plan)
}

// ═══════════════════════════════════════════════════════════════
//  DeleteTenant
// ═══════════════════════════════════════════════════════════════

func TestDeleteTenant(t *testing.T) {
	tenant := createTestTenant(t)

	err := testStore.DeleteTenant(context.Background(), tenant.ID)
	require.NoError(t, err)

	_, err = testStore.GetTenantByID(context.Background(), tenant.ID)
	require.Error(t, err)
}
func TestDeleteTenant_NonexistentID_NoError(t *testing.T) {
	err := testStore.DeleteTenant(context.Background(), uuid.New())
	require.NoError(t, err)
}

// ═══════════════════════════════════════════════════════════════
//  CreateUser
// ═══════════════════════════════════════════════════════════════

func TestCreateUser_AllFields(t *testing.T) {
	tenant := createTestTenant(t)
	fullName := "Nicholas Dev"
	email := utils.RandomString(8) + "@test.com"

	user, err := testStore.CreateUser(context.Background(), CreateUserParams{
		TenantID:     tenant.ID,
		Email:        email,
		PasswordHash: "$2a$10$hashedpassword",
		FullName:     &fullName,
		Role:         "admin",
		IsActive:     true,
	})
	require.NoError(t, err)

	require.NotZero(t, user.ID)
	require.Equal(t, tenant.ID, user.TenantID)
	require.Equal(t, email, user.Email)
	require.Equal(t, "$2a$10$hashedpassword", user.PasswordHash)
	require.Equal(t, "Nicholas Dev", *user.FullName)
	require.Equal(t, "admin", user.Role)
	require.True(t, user.IsActive)
	require.Nil(t, user.LastLoginAt)
	require.NotZero(t, user.CreatedAt)
}
func TestCreateUser_NilFullName(t *testing.T) {
	tenant := createTestTenant(t)

	user, err := testStore.CreateUser(context.Background(), CreateUserParams{
		TenantID:     tenant.ID,
		Email:        utils.RandomString(8) + "@test.com",
		PasswordHash: "$2a$10$hash",
		Role:         "viewer",
		IsActive:     true,
	})
	require.NoError(t, err)
	require.Nil(t, user.FullName)
}
func TestCreateUser_InactiveUser(t *testing.T) {
	tenant := createTestTenant(t)

	user, err := testStore.CreateUser(context.Background(), CreateUserParams{
		TenantID:     tenant.ID,
		Email:        utils.RandomString(8) + "@test.com",
		PasswordHash: "$2a$10$hash",
		Role:         "viewer",
		IsActive:     false,
	})
	require.NoError(t, err)
	require.False(t, user.IsActive)
}

// ═══════════════════════════════════════════════════════════════
//  GetUserByID
// ═══════════════════════════════════════════════════════════════

func TestGetUserByID_Found(t *testing.T) {
	tenant := createTestTenant(t)
	created := createTestUser(t, tenant.ID)

	fetched, err := testStore.GetUserByID(context.Background(), GetUserByIDParams{
		ID:       created.ID,
		TenantID: tenant.ID,
	})
	require.NoError(t, err)
	require.Equal(t, created.ID, fetched.ID)
	require.Equal(t, created.Email, fetched.Email)
}
func TestGetUserByID_WrongTenant(t *testing.T) {
	t1 := createTestTenant(t)
	t2 := createTestTenant(t)
	user := createTestUser(t, t1.ID)

	_, err := testStore.GetUserByID(context.Background(), GetUserByIDParams{
		ID:       user.ID,
		TenantID: t2.ID,
	})
	require.Error(t, err)
}

// ═══════════════════════════════════════════════════════════════
//  GetUserByEmail / GetUserByEmailGlobal
// ═══════════════════════════════════════════════════════════════

func TestGetUserByEmail_Found(t *testing.T) {
	tenant := createTestTenant(t)
	created := createTestUser(t, tenant.ID)

	fetched, err := testStore.GetUserByEmail(context.Background(), GetUserByEmailParams{
		Email:    created.Email,
		TenantID: tenant.ID,
	})
	require.NoError(t, err)
	require.Equal(t, created.ID, fetched.ID)
}
func TestGetUserByEmail_WrongTenant(t *testing.T) {
	t1 := createTestTenant(t)
	t2 := createTestTenant(t)
	user := createTestUser(t, t1.ID)

	_, err := testStore.GetUserByEmail(context.Background(), GetUserByEmailParams{
		Email:    user.Email,
		TenantID: t2.ID,
	})
	require.Error(t, err)
}
func TestGetUserByEmailGlobal_Found(t *testing.T) {
	tenant := createTestTenant(t)
	created := createTestUser(t, tenant.ID)

	fetched, err := testStore.GetUserByEmailGlobal(context.Background(), created.Email)
	require.NoError(t, err)
	require.Equal(t, created.ID, fetched.ID)
	require.Equal(t, tenant.ID, fetched.TenantID)
	require.Equal(t, tenant.Slug, fetched.TenantSlug)
	require.Equal(t, tenant.Plan, fetched.TenantPlan)
	require.True(t, fetched.IsActive)
}
func TestGetUserByEmailGlobal_InactiveUser_NotFound(t *testing.T) {
	tenant := createTestTenant(t)
	email := utils.RandomString(8) + "@test.com"

	_, err := testStore.CreateUser(context.Background(), CreateUserParams{
		TenantID:     tenant.ID,
		Email:        email,
		PasswordHash: "$2a$10$hash",
		Role:         "viewer",
		IsActive:     false,
	})
	require.NoError(t, err)

	_, err = testStore.GetUserByEmailGlobal(context.Background(), email)
	require.Error(t, err, "inactive user must not be returned by global lookup")
}
func TestGetUserByEmailGlobal_NotFound(t *testing.T) {
	_, err := testStore.GetUserByEmailGlobal(context.Background(), "nobody_"+utils.RandomString(8)+"@example.com")
	require.Error(t, err)
}

// ═══════════════════════════════════════════════════════════════
//  ListUsersByTenant / CountUsersByTenant
// ═══════════════════════════════════════════════════════════════

func TestListUsersByTenant(t *testing.T) {
	tenant := createTestTenant(t)
	u1 := createTestUser(t, tenant.ID)
	u2 := createTestUser(t, tenant.ID)

	users, err := testStore.ListUsersByTenant(context.Background(), tenant.ID)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(users), 2)

	ids := map[uuid.UUID]bool{}
	for _, u := range users {
		ids[u.ID] = true
		require.Equal(t, tenant.ID, u.TenantID)
	}
	require.True(t, ids[u1.ID])
	require.True(t, ids[u2.ID])
}
func TestListUsersByTenant_TenantIsolation(t *testing.T) {
	t1 := createTestTenant(t)
	t2 := createTestTenant(t)
	_ = createTestUser(t, t1.ID)
	_ = createTestUser(t, t2.ID)

	users1, err := testStore.ListUsersByTenant(context.Background(), t1.ID)
	require.NoError(t, err)
	for _, u := range users1 {
		require.Equal(t, t1.ID, u.TenantID)
	}
}
func TestCountUsersByTenant(t *testing.T) {
	tenant := createTestTenant(t)

	countBefore, err := testStore.CountUsersByTenant(context.Background(), tenant.ID)
	require.NoError(t, err)

	createTestUser(t, tenant.ID)
	createTestUser(t, tenant.ID)

	countAfter, err := testStore.CountUsersByTenant(context.Background(), tenant.ID)
	require.NoError(t, err)
	require.Equal(t, countBefore+2, countAfter)
}

// ═══════════════════════════════════════════════════════════════
//  UpdateUser
// ═══════════════════════════════════════════════════════════════

func TestUpdateUser_AllFields(t *testing.T) {
	tenant := createTestTenant(t)
	user := createTestUser(t, tenant.ID)
	newName := "Updated Name"

	updated, err := testStore.UpdateUser(context.Background(), UpdateUserParams{
		ID:       user.ID,
		FullName: &newName,
		Role:     "viewer",
		IsActive: false,
		TenantID: tenant.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "Updated Name", *updated.FullName)
	require.Equal(t, "viewer", updated.Role)
	require.False(t, updated.IsActive)
	require.Equal(t, user.Email, updated.Email)
}
func TestUpdateUser_WrongTenant(t *testing.T) {
	t1 := createTestTenant(t)
	t2 := createTestTenant(t)
	user := createTestUser(t, t1.ID)

	_, err := testStore.UpdateUser(context.Background(), UpdateUserParams{
		ID:       user.ID,
		Role:     "admin",
		IsActive: true,
		TenantID: t2.ID,
	})
	require.Error(t, err)
}
func TestUpdateUser_UpdatedAtAdvances(t *testing.T) {
	tenant := createTestTenant(t)
	user := createTestUser(t, tenant.ID)

	updated, err := testStore.UpdateUser(context.Background(), UpdateUserParams{
		ID: user.ID, Role: "viewer", IsActive: true, TenantID: tenant.ID,
	})
	require.NoError(t, err)
	require.True(t, updated.UpdatedAt.After(user.UpdatedAt) ||
		updated.UpdatedAt.Equal(user.UpdatedAt))
}

// ═══════════════════════════════════════════════════════════════
//  UpdateUserPassword
// ═══════════════════════════════════════════════════════════════

func TestUpdateUserPassword(t *testing.T) {
	tenant := createTestTenant(t)
	user := createTestUser(t, tenant.ID)
	newHash := "$2a$10$newpasswordhash" + utils.RandomString(20)

	err := testStore.UpdateUserPassword(context.Background(), UpdateUserPasswordParams{
		ID:           user.ID,
		PasswordHash: newHash,
		TenantID:     tenant.ID,
	})
	require.NoError(t, err)

	fetched, err := testStore.GetUserByID(context.Background(), GetUserByIDParams{
		ID: user.ID, TenantID: tenant.ID,
	})
	require.NoError(t, err)
	require.Equal(t, newHash, fetched.PasswordHash)
}
func TestUpdateUserPassword_WrongTenant_NoOp(t *testing.T) {
	t1 := createTestTenant(t)
	t2 := createTestTenant(t)
	user := createTestUser(t, t1.ID)
	originalHash := user.PasswordHash

	err := testStore.UpdateUserPassword(context.Background(), UpdateUserPasswordParams{
		ID:           user.ID,
		PasswordHash: "$2a$10$hackedpassword",
		TenantID:     t2.ID,
	})
	require.NoError(t, err)

	fetched, err := testStore.GetUserByID(context.Background(), GetUserByIDParams{
		ID: user.ID, TenantID: t1.ID,
	})
	require.NoError(t, err)
	require.Equal(t, originalHash, fetched.PasswordHash)
}

// ═══════════════════════════════════════════════════════════════
//  UpdateUserLastLogin
// ═══════════════════════════════════════════════════════════════

func TestUpdateUserLastLogin(t *testing.T) {
	tenant := createTestTenant(t)
	user := createTestUser(t, tenant.ID)
	require.Nil(t, user.LastLoginAt)

	err := testStore.UpdateUserLastLogin(context.Background(), user.ID)
	require.NoError(t, err)

	fetched, err := testStore.GetUserByID(context.Background(), GetUserByIDParams{
		ID: user.ID, TenantID: tenant.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, fetched.LastLoginAt)
}
func TestUpdateUserLastLogin_UpdatesUpdatedAt(t *testing.T) {
	tenant := createTestTenant(t)
	user := createTestUser(t, tenant.ID)

	err := testStore.UpdateUserLastLogin(context.Background(), user.ID)
	require.NoError(t, err)

	fetched, err := testStore.GetUserByID(context.Background(), GetUserByIDParams{
		ID: user.ID, TenantID: tenant.ID,
	})
	require.NoError(t, err)
	require.True(t, fetched.UpdatedAt.After(user.UpdatedAt) ||
		fetched.UpdatedAt.Equal(user.UpdatedAt))
}

// ═══════════════════════════════════════════════════════════════
//  DeleteUser
// ═══════════════════════════════════════════════════════════════

func TestDeleteUser(t *testing.T) {
	tenant := createTestTenant(t)
	user := createTestUser(t, tenant.ID)

	err := testStore.DeleteUser(context.Background(), DeleteUserParams{
		ID:       user.ID,
		TenantID: tenant.ID,
	})
	require.NoError(t, err)

	_, err = testStore.GetUserByID(context.Background(), GetUserByIDParams{
		ID: user.ID, TenantID: tenant.ID,
	})
	require.Error(t, err)
}
func TestDeleteUser_WrongTenant_NoOp(t *testing.T) {
	t1 := createTestTenant(t)
	t2 := createTestTenant(t)
	user := createTestUser(t, t1.ID)

	err := testStore.DeleteUser(context.Background(), DeleteUserParams{
		ID:       user.ID,
		TenantID: t2.ID,
	})
	require.NoError(t, err)

	_, err = testStore.GetUserByID(context.Background(), GetUserByIDParams{
		ID: user.ID, TenantID: t1.ID,
	})
	require.NoError(t, err)
}
