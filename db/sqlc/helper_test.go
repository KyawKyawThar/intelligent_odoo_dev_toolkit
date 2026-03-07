package db

import (
	"Intelligent_Dev_ToolKit_Odoo/internal/config"
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

// ─── Environment ───

func createRandomEnviroment(t *testing.T, tenantID uuid.UUID) Environment {
	arg := CreateEnvironmentParams{
		TenantID:     tenantID,
		Name:         fmt.Sprintf("env-%s", utils.RandomString(7)),
		OdooUrl:      fmt.Sprintf("http://%s.odoo.com", utils.RandomString(7)),
		DbName:       fmt.Sprintf("db_%s", utils.RandomString(7)),
		EnvType:      config.EnvironmentDevelopment,
		FeatureFlags: json.RawMessage(`{}`),
	}
	env, err := testStore.CreateEnvironment(context.Background(), arg)
	require.NoError(t, err)
	require.NotEmpty(t, env)
	require.Equal(t, tenantID, env.TenantID)

	return env
}
