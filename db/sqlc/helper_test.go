package db

import (
	"Intelligent_Dev_ToolKit_Odoo/utils"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// ─── Tenant ───

func createRandomTenant(t *testing.T) Tenant {

	arg := CreateTenantParams{
		Name:       utils.RandomString(10),
		Slug:       utils.RandomSlug(),
		Plan:       "cloud",
		PlanStatus: "active",
		Settings:   json.RawMessage(`{}`),
		RetentionConfig: json.RawMessage(`{
			"error_traces_days": 7,
			"profiler_recordings_days": 7,
			"budget_samples_days": 30,
			"schema_snapshots_keep": 10,
			"raw_logs_days": 3
		}`),
	}

	tenant, err := testStore.CreateTenant(context.Background(), arg)

	require.NoError(t, err)
	require.NotEmpty(t, tenant)
	require.Equal(t, arg.Name, tenant.Name)
	require.Equal(t, arg.Plan, tenant.Plan)
	return tenant

}

// ─── User ───
func createRandomUser(t *testing.T, tenantID uuid.UUID) User {

	fullName := utils.RandomOwner()
	hashPassword, err := utils.HashPassword("$2a$10$")
	require.NoError(t, err)

	arg := CreateUserParams{
		TenantID:      tenantID,
		Email:         utils.RandomEmail(),
		PasswordHash:  hashPassword,
		FullName:      &fullName,
		EmailVerified: true,
		IsActive:      true,
	}

	user, err := testStore.CreateUser(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, user)
	require.Equal(t, arg.Email, user.Email)
	require.Equal(t, tenantID, user.TenantID)
	return user
}

// ─── Environment ───

func createRandomEnviroment(t *testing.T, tenantID uuid.UUID) Environment {
	arg := CreateEnvironmentParams{
		TenantID:     tenantID,
		Name:         fmt.Sprintf("env-%s", utils.RandomString(7)),
		OdooUrl:      fmt.Sprintf("http://%s.odoo.com", utils.RandomString(7)),
		DbName:       fmt.Sprintf("db_%s", utils.RandomString(7)),
		EnvType:      "development",
		FeatureFlags: json.RawMessage(`{}`),
	}
	env, err := testStore.CreateEnvironment(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, env)
	require.Equal(t, tenantID, env.TenantID)

	return env
}

// ─── Notification Channel ───
func createRandomChannel(t *testing.T, tennantID uuid.UUID) NotificationChannel {
	arg := CreateNotificationChannelParams{
		TenantID: tennantID,
		Name:     utils.RandomOwner(),
		Type:     "slack",
		Config:   json.RawMessage(`{"url":"https://hooks.slack.com/test"}`),
	}

	ch, err := testStore.CreateNotificationChannel(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, ch)
	return ch

}

// ─── Anonymization Profile ───

func createRandomAnonPrfile(t *testing.T, tenantID, sourceEnv, targetEnv uuid.UUID) AnonProfile {

	arg := CreateAnonProfileParams{
		TenantID:  tenantID,
		Name:      utils.RandomOwner(),
		SourceEnv: &sourceEnv,
		TargetEnv: &targetEnv,
		FieldRules: json.RawMessage(`[
			{"model":"res.partner","field":"name","strategy":"FAKE"},
			{"model":"res.partner","field":"email","strategy":"MASK"}
		]`),
	}

	profile, err := testStore.CreateAnonProfile(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, profile)
	return profile
}
