package db

import (
	"Intelligent_Dev_ToolKit_Odoo/internal/config"
	"Intelligent_Dev_ToolKit_Odoo/utils"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
)

func TestCheckEnvironmentNameExists_SameNameDifferentID(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	exists, err := testStore.CheckEnvironmentNameExists(context.Background(), CheckEnvironmentNameExistsParams{
		TenantID: reg.Tenant.ID,
		Name:     env.Name,
		ID:       uuid.New(), // not the env's own ID
	})
	require.NoError(t, err)
	require.True(t, exists)
}

func createTestEnvironment(t *testing.T, tenantID uuid.UUID, envType string) Environment {
	t.Helper()
	env, err := testStore.CreateEnvironment(context.Background(), CreateEnvironmentParams{
		TenantID:     tenantID,
		Name:         "Env " + utils.RandomString(6),
		OdooUrl:      "https://" + utils.RandomString(8) + ".example.com",
		DbName:       "odoo_" + utils.RandomString(6),
		EnvType:      envType,
		Status:       "connected",
		FeatureFlags: json.RawMessage(`{"n1_detection":true}`),
	})
	require.NoError(t, err)
	require.NotZero(t, env.ID)
	return env
}

func TestCreateEnvironment_AllFields(t *testing.T) {
	reg := createRegisteredTenant(t)
	version := "17.0"

	env, err := testStore.CreateEnvironment(context.Background(), CreateEnvironmentParams{
		TenantID:     reg.Tenant.ID,
		Name:         "Production",
		OdooUrl:      "https://prod.odoo.example.com",
		DbName:       "odoo_prod_all",
		OdooVersion:  &version,
		EnvType:      config.EnvironmentProduction,
		Status:       "connected",
		FeatureFlags: json.RawMessage(`{"n1_detection":true,"budget_alerts":false}`),
	})
	require.NoError(t, err)

	require.NotZero(t, env.ID)
	require.Equal(t, reg.Tenant.ID, env.TenantID)
	require.Equal(t, "Production", env.Name)
	require.Equal(t, "https://prod.odoo.example.com", env.OdooUrl)
	require.Equal(t, "odoo_prod_all", env.DbName)
	require.NotNil(t, env.OdooVersion)
	require.Equal(t, "17.0", *env.OdooVersion)
	require.Equal(t, "production", env.EnvType)
	require.Equal(t, "connected", env.Status)
	require.Nil(t, env.AgentID)
	require.Nil(t, env.LastSync)
	require.NotZero(t, env.CreatedAt)
	require.NotZero(t, env.UpdatedAt)
}
func TestCreateEnvironment_MinimalFields(t *testing.T) {
	reg := createRegisteredTenant(t)

	env, err := testStore.CreateEnvironment(context.Background(), CreateEnvironmentParams{
		TenantID:     reg.Tenant.ID,
		Name:         "Staging",
		OdooUrl:      "https://staging.example.com",
		DbName:       "odoo_staging_min",
		EnvType:      config.EnvironmentStaging,
		Status:       "disconnected",
		FeatureFlags: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	require.NotZero(t, env.ID)
	require.Nil(t, env.OdooVersion)
	require.Nil(t, env.AgentID)
}
func TestGetEnvironmentByID_Found(t *testing.T) {
	reg := createRegisteredTenant(t)
	created := createTestEnvironment(t, reg.Tenant.ID, "production")

	fetched, err := testStore.GetEnvironmentByID(context.Background(), GetEnvironmentByIDParams{
		ID:       created.ID,
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)
	require.Equal(t, created.ID, fetched.ID)
	require.Equal(t, created.Name, fetched.Name)
	require.Equal(t, created.OdooUrl, fetched.OdooUrl)
}
func TestGetEnvironmentByID_WrongTenant(t *testing.T) {
	reg1 := createRegisteredTenant(t)
	reg2 := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg1.Tenant.ID, "production")

	_, err := testStore.GetEnvironmentByID(context.Background(), GetEnvironmentByIDParams{
		ID:       env.ID,
		TenantID: reg2.Tenant.ID,
	})
	require.Error(t, err, "wrong tenant must not access environment")
}
func TestGetEnvironmentByID_NotFound(t *testing.T) {
	reg := createRegisteredTenant(t)

	_, err := testStore.GetEnvironmentByID(context.Background(), GetEnvironmentByIDParams{
		ID:       uuid.New(),
		TenantID: reg.Tenant.ID,
	})
	require.Error(t, err)
}
func TestListEnvironmentsByTenant_Multiple(t *testing.T) {
	reg := createRegisteredTenant(t)

	env1 := createTestEnvironment(t, reg.Tenant.ID, "production")
	env2 := createTestEnvironment(t, reg.Tenant.ID, "production")

	envs, err := testStore.ListEnvironmentsByTenant(context.Background(), reg.Tenant.ID)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(envs), 2)

	ids := map[uuid.UUID]bool{}
	for _, e := range envs {
		ids[e.ID] = true
		require.Equal(t, reg.Tenant.ID, e.TenantID)
	}
	require.True(t, ids[env1.ID])
	require.True(t, ids[env2.ID])
}
func TestListEnvironmentsByTenant_TenantIsolation(t *testing.T) {
	reg1 := createRegisteredTenant(t)
	reg2 := createRegisteredTenant(t)

	createTestEnvironment(t, reg1.Tenant.ID, "production")
	env2 := createTestEnvironment(t, reg2.Tenant.ID, "production")

	envs, err := testStore.ListEnvironmentsByTenant(context.Background(), reg2.Tenant.ID)
	require.NoError(t, err)

	for _, e := range envs {
		require.Equal(t, reg2.Tenant.ID, e.TenantID)
	}
	found := false
	for _, e := range envs {
		if e.ID == env2.ID {
			found = true
		}
	}
	require.True(t, found)
}
func TestCountEnvironmentsByTenant(t *testing.T) {
	reg := createRegisteredTenant(t)

	countBefore, err := testStore.CountEnvironmentsByTenant(context.Background(), reg.Tenant.ID)
	require.NoError(t, err)

	createTestEnvironment(t, reg.Tenant.ID, "production")
	createTestEnvironment(t, reg.Tenant.ID, "production")

	countAfter, err := testStore.CountEnvironmentsByTenant(context.Background(), reg.Tenant.ID)
	require.NoError(t, err)
	require.Equal(t, countBefore+2, countAfter)
}
func TestUpdateEnvironment_AllFields(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	newVersion := "16.0"
	newName := "Updated Name"
	newOdooUrl := "https://updated.example.com"
	newDbName := "odoo_updated"
	newEnvType := "staging"

	updated, err := testStore.UpdateEnvironment(context.Background(), UpdateEnvironmentParams{
		ID:          env.ID,
		Name:        &newName,
		OdooUrl:     &newOdooUrl,
		DbName:      &newDbName,
		OdooVersion: &newVersion,
		EnvType:     &newEnvType,
		TenantID:    reg.Tenant.ID,
	})
	require.NoError(t, err)

	require.Equal(t, "Updated Name", updated.Name)
	require.Equal(t, "https://updated.example.com", updated.OdooUrl)
	require.Equal(t, "odoo_updated", updated.DbName)
	require.Equal(t, "16.0", *updated.OdooVersion)
	require.Equal(t, "staging", updated.EnvType)
	require.True(t, updated.UpdatedAt.After(env.UpdatedAt) || updated.UpdatedAt.Equal(env.UpdatedAt))
}
func TestUpdateEnvironment_WrongTenant_Fails(t *testing.T) {
	reg1 := createRegisteredTenant(t)
	reg2 := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg1.Tenant.ID, "production")
	hackedName := "Hacked"
	hackedOdooUrl := "https://hacked.com"
	hackedDbName := "hacked"
	hackedEnvType := "production"

	_, err := testStore.UpdateEnvironment(context.Background(), UpdateEnvironmentParams{
		ID:       env.ID,
		Name:     &hackedName,
		OdooUrl:  &hackedOdooUrl,
		DbName:   &hackedDbName,
		EnvType:  &hackedEnvType,
		TenantID: reg2.Tenant.ID,
	})
	require.Error(t, err)
}
func TestUpdateEnvironment_PreservesStatusAndAgent(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	newName := "New Name"

	updated, err := testStore.UpdateEnvironment(context.Background(), UpdateEnvironmentParams{
		ID:       env.ID,
		Name:     &newName,
		OdooUrl:  &env.OdooUrl,
		DbName:   &env.DbName,
		EnvType:  &env.EnvType,
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)
	require.Equal(t, env.Status, updated.Status, "status must not change via UpdateEnvironment")
	require.Equal(t, env.AgentID, updated.AgentID, "agent_id must not change via UpdateEnvironment")
}
func TestUpdateEnvironmentStatus(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	err := testStore.UpdateEnvironmentStatus(context.Background(), UpdateEnvironmentStatusParams{
		ID:     env.ID,
		Status: "disconnected",
	})
	require.NoError(t, err)

	fetched, err := testStore.GetEnvironmentByID(context.Background(), GetEnvironmentByIDParams{
		ID:       env.ID,
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "disconnected", fetched.Status)
	require.NotNil(t, fetched.LastSync, "last_sync must be set on status update")
}
func TestDeleteEnvironment(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	_, err := testStore.DeleteEnvironment(context.Background(), DeleteEnvironmentParams{
		ID:       env.ID,
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)

	_, err = testStore.GetEnvironmentByID(context.Background(), GetEnvironmentByIDParams{
		ID:       env.ID,
		TenantID: reg.Tenant.ID,
	})
	require.Error(t, err)
}
func TestDeleteEnvironment_WrongTenant_NoOp(t *testing.T) {
	reg1 := createRegisteredTenant(t)
	reg2 := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg1.Tenant.ID, "production")

	// Wrong tenant delete — should not error but also not delete
	_, err := testStore.DeleteEnvironment(context.Background(), DeleteEnvironmentParams{
		ID:       env.ID,
		TenantID: reg2.Tenant.ID,
	})
	require.NoError(t, err)

	// Must still exist
	fetched, err := testStore.GetEnvironmentByID(context.Background(), GetEnvironmentByIDParams{
		ID:       env.ID,
		TenantID: reg1.Tenant.ID,
	})
	require.NoError(t, err)
	require.Equal(t, env.ID, fetched.ID)
}

// ═══════════════════════════════════════════════════════════════
//  RegisterAgent / GetEnvironmentByAgentID / DisconnectAgent
// ═══════════════════════════════════════════════════════════════

func TestRegisterAgent_SetsAgentAndStatus(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	agentID := "agent_" + utils.RandomString(8)

	registered, err := testStore.RegisterAgent(context.Background(), RegisterAgentParams{
		ID:       env.ID,
		AgentID:  &agentID,
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)

	require.Equal(t, "connected", registered.Status)
	require.NotNil(t, registered.AgentID)
	require.Equal(t, agentID, *registered.AgentID)
	require.NotNil(t, registered.LastSync)
}

func TestGetEnvironmentByAgentID_Found(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	agentID := "agent_" + utils.RandomString(8)

	_, err := testStore.RegisterAgent(context.Background(), RegisterAgentParams{
		ID:       env.ID,
		AgentID:  &agentID,
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)

	fetched, err := testStore.GetEnvironmentByAgentID(context.Background(), &agentID)
	require.NoError(t, err)
	require.Equal(t, env.ID, fetched.ID)
}
func TestGetEnvironmentByAgentID_NotFound(t *testing.T) {
	nonexistent := "agent_nonexistent_" + utils.RandomString(8)
	_, err := testStore.GetEnvironmentByAgentID(context.Background(), &nonexistent)
	require.Error(t, err)
}
func TestDisconnectAgent(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	agentID := "agent_" + utils.RandomString(8)

	_, err := testStore.RegisterAgent(context.Background(), RegisterAgentParams{
		ID:       env.ID,
		AgentID:  &agentID,
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)

	err = testStore.DisconnectAgent(context.Background(), &agentID)
	require.NoError(t, err)

	fetched, err := testStore.GetEnvironmentByID(context.Background(), GetEnvironmentByIDParams{
		ID:       env.ID,
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "disconnected", fetched.Status)
}
func TestGetFeatureFlags(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	flags, err := testStore.GetFeatureFlags(context.Background(), env.ID)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(flags, &parsed))
	require.Equal(t, true, parsed["n1_detection"])
}

func TestUpdateFeatureFlags(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	newFlags := json.RawMessage(`{"n1_detection":false,"budget_alerts":true,"new_flag":"test"}`)

	updated, err := testStore.UpdateFeatureFlags(context.Background(), UpdateFeatureFlagsParams{
		ID:           env.ID,
		FeatureFlags: newFlags,
		TenantID:     reg.Tenant.ID,
	})
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(updated.FeatureFlags, &parsed))
	require.Equal(t, false, parsed["n1_detection"])
	require.Equal(t, true, parsed["budget_alerts"])
	require.Equal(t, "test", parsed["new_flag"])
}
func TestUpdateFeatureFlags_WrongTenant(t *testing.T) {
	reg1 := createRegisteredTenant(t)
	reg2 := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg1.Tenant.ID, "production")

	_, err := testStore.UpdateFeatureFlags(context.Background(), UpdateFeatureFlagsParams{
		ID:           env.ID,
		FeatureFlags: json.RawMessage(`{"hacked":true}`),
		TenantID:     reg2.Tenant.ID,
	})
	require.Error(t, err)
}
func TestInsertHeartbeat_AllFields(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	hb, err := testStore.InsertHeartbeat(context.Background(), InsertHeartbeatParams{
		EnvID:        env.ID,
		AgentID:      "agent_hb_test",
		AgentVersion: strPtr("1.2.3"),
		Status:       config.StatusHealthy,
		Metadata:     json.RawMessage(`{"cpu":45.2,"memory":78.1}`),
	})
	require.NoError(t, err)

	require.NotZero(t, hb.ID)
	require.Equal(t, env.ID, hb.EnvID)
	require.Equal(t, "agent_hb_test", hb.AgentID)
	require.Equal(t, "1.2.3", *hb.AgentVersion)
	require.Equal(t, "healthy", hb.Status)
	require.NotZero(t, hb.ReceivedAt)
}
func TestGetLatestHeartbeat(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	// Insert 3 heartbeats
	for range 3 {
		_, err := testStore.InsertHeartbeat(context.Background(), InsertHeartbeatParams{
			EnvID:    env.ID,
			AgentID:  "agent_latest",
			Status:   "healthy",
			Metadata: json.RawMessage(`{}`),
		})
		require.NoError(t, err)
	}

	latest, err := testStore.GetLatestHeartbeat(context.Background(), env.ID)
	require.NoError(t, err)
	require.Equal(t, env.ID, latest.EnvID)
}
func TestListHeartbeats_LimitAndOrder(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	for i := 0; i < 5; i++ {
		_, err := testStore.InsertHeartbeat(context.Background(), InsertHeartbeatParams{
			EnvID:    env.ID,
			AgentID:  "agent_list",
			Status:   "healthy",
			Metadata: json.RawMessage(`{}`),
		})
		require.NoError(t, err)
	}

	hbs, err := testStore.ListHeartbeats(context.Background(), ListHeartbeatsParams{
		EnvID: env.ID,
		Limit: 3,
	})
	require.NoError(t, err)
	require.Len(t, hbs, 3)

	// DESC order
	for i := 1; i < len(hbs); i++ {
		require.True(t, hbs[i-1].ReceivedAt.After(hbs[i].ReceivedAt) ||
			hbs[i-1].ReceivedAt.Equal(hbs[i].ReceivedAt))
	}
}
func TestDeleteOldHeartbeats(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	_, err := testStore.InsertHeartbeat(context.Background(), InsertHeartbeatParams{
		EnvID:    env.ID,
		AgentID:  "agent_old",
		Status:   "healthy",
		Metadata: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	// Delete with future cutoff → removes everything
	result, err := testStore.DeleteOldHeartbeats(context.Background(), DeleteOldHeartbeatsParams{
		TenantID:   reg.Tenant.ID,
		ReceivedAt: time.Now().Add(1 * time.Hour),
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, result.RowsAffected(), int64(1))
}
func TestDeleteOldHeartbeats_TenantIsolation(t *testing.T) {
	reg1 := createRegisteredTenant(t)
	reg2 := createRegisteredTenant(t)
	env1 := createTestEnvironment(t, reg1.Tenant.ID, "production")
	env2 := createTestEnvironment(t, reg2.Tenant.ID, "production")

	_, err := testStore.InsertHeartbeat(context.Background(), InsertHeartbeatParams{
		EnvID: env1.ID, AgentID: "a1", Status: "healthy", Metadata: json.RawMessage(`{}`),
	})
	require.NoError(t, err)
	_, err = testStore.InsertHeartbeat(context.Background(), InsertHeartbeatParams{
		EnvID: env2.ID, AgentID: "a2", Status: "healthy", Metadata: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	// Delete tenant 1's heartbeats
	_, err = testStore.DeleteOldHeartbeats(context.Background(), DeleteOldHeartbeatsParams{
		TenantID:   reg1.Tenant.ID,
		ReceivedAt: time.Now().Add(1 * time.Hour),
	})
	require.NoError(t, err)

	// Tenant 2's heartbeat must survive
	hb2, err := testStore.GetLatestHeartbeat(context.Background(), env2.ID)
	require.NoError(t, err)
	require.Equal(t, env2.ID, hb2.EnvID)
}
func TestCheckEnvironmentNameExists(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	testCases := []struct {
		name     string
		params   CheckEnvironmentNameExistsParams
		expected bool
	}{
		{
			name: "Existing environment name",
			params: CheckEnvironmentNameExistsParams{
				Name:     env.Name,
				TenantID: reg.Tenant.ID,
				ID:       uuid.New(),
			},
			expected: true,
		},
		{
			name: "Non-existing environment name",
			params: CheckEnvironmentNameExistsParams{
				Name:     "non-existing-env",
				TenantID: reg.Tenant.ID,
				ID:       env.ID,
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			exists, err := testStore.CheckEnvironmentNameExists(context.Background(), tc.params)
			require.NoError(t, err)
			require.Equal(t, tc.expected, exists)
		})
	}
}

func TestUpdateEnvironment_PartialUpdate(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	newName := "Updated Name Partial"

	updated, err := testStore.UpdateEnvironment(context.Background(), UpdateEnvironmentParams{
		ID:       env.ID,
		Name:     &newName,
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)

	require.Equal(t, "Updated Name Partial", updated.Name)
	require.Equal(t, env.OdooUrl, updated.OdooUrl)
	require.Equal(t, env.DbName, updated.DbName)
}

func TestListStaleAgents_NoHeartbeat(t *testing.T) {
	reg := createRegisteredTenant(t)
	// Create a connected env with NO heartbeats → should be stale
	_ = createTestEnvironment(t, reg.Tenant.ID, "production") // status = "connected"

	// Interval of 5 minutes
	stale, err := testStore.ListStaleAgents(context.Background(), pgtype.Interval{
		Microseconds: 5 * 60 * 1_000_000, // 5 min
		Valid:        true,
	})
	require.NoError(t, err)
	// At least our env should appear (no heartbeat = stale)
	require.GreaterOrEqual(t, len(stale), 1)
}

// ═══════════════════════════════════════════════════════════════
//  CountEnvironmentsByTenantTypeAndStatus
// ═══════════════════════════════════════════════════════════════

func TestCountEnvironmentsByTenantTypeAndStatus_Basic(t *testing.T) {
	reg := createRegisteredTenant(t)

	// Create 2 production+connected, 1 staging+connected
	createTestEnvironment(t, reg.Tenant.ID, "production") // status = "connected" by helper
	createTestEnvironment(t, reg.Tenant.ID, "production")
	createTestEnvironment(t, reg.Tenant.ID, "staging")

	count, err := testStore.CountEnvironmentsByTenantTypeAndStatus(context.Background(), CountEnvironmentsByTenantTypeAndStatusParams{
		TenantID: reg.Tenant.ID,
		EnvType:  "production",
		Status:   "connected",
	})

	require.NoError(t, err)
	require.GreaterOrEqual(t, count, int64(2))
}

func TestCountEnvironmentsByTenantTypeAndStatus_NoMatch(t *testing.T) {
	reg := createRegisteredTenant(t)
	createTestEnvironment(t, reg.Tenant.ID, "production")

	count, err := testStore.CountEnvironmentsByTenantTypeAndStatus(context.Background(),
		CountEnvironmentsByTenantTypeAndStatusParams{
			TenantID: reg.Tenant.ID,
			EnvType:  "staging",
			Status:   "disconnected",
		})
	require.NoError(t, err)
	require.Equal(t, int64(0), count)

}
func TestCountEnvironmentsByTenantTypeAndStatus_TenantIsolation(t *testing.T) {
	reg1 := createRegisteredTenant(t)
	reg2 := createRegisteredTenant(t)

	createTestEnvironment(t, reg1.Tenant.ID, "production")
	createTestEnvironment(t, reg1.Tenant.ID, "production")

	count, err := testStore.CountEnvironmentsByTenantTypeAndStatus(context.Background(),
		CountEnvironmentsByTenantTypeAndStatusParams{
			TenantID: reg2.Tenant.ID,
			EnvType:  "production",
			Status:   "connected",
		})
	require.NoError(t, err)
	require.Equal(t, int64(0), count, "should not count another tenant's environments")
}

// ═══════════════════════════════════════════════════════════════
//
//	CountEnvironmentsByTenantAndType
//
// ═══════════════════════════════════════════════════════════════
func TestCountEnvironmentsByTenantAndType_Basic(t *testing.T) {
	reg := createRegisteredTenant(t)

	countBefore, err := testStore.CountEnvironmentsByTenantAndType(context.Background(),
		CountEnvironmentsByTenantAndTypeParams{TenantID: reg.Tenant.ID, EnvType: "staging"})
	require.NoError(t, err)

	createTestEnvironment(t, reg.Tenant.ID, "staging")
	createTestEnvironment(t, reg.Tenant.ID, "staging")
	createTestEnvironment(t, reg.Tenant.ID, "production") // should not be counted

	count, err := testStore.CountEnvironmentsByTenantAndType(context.Background(),
		CountEnvironmentsByTenantAndTypeParams{TenantID: reg.Tenant.ID, EnvType: "staging"})
	require.NoError(t, err)
	require.Equal(t, countBefore+2, count)
}
func TestCountEnvironmentsByTenantAndType_NoMatch(t *testing.T) {
	reg := createRegisteredTenant(t)
	createTestEnvironment(t, reg.Tenant.ID, "production")

	count, err := testStore.CountEnvironmentsByTenantAndType(context.Background(),
		CountEnvironmentsByTenantAndTypeParams{TenantID: reg.Tenant.ID, EnvType: "staging"})
	require.NoError(t, err)
	require.Equal(t, int64(0), count)
}
func TestCountEnvironmentsByTenantAndType_TenantIsolation(t *testing.T) {
	reg1 := createRegisteredTenant(t)
	reg2 := createRegisteredTenant(t)

	createTestEnvironment(t, reg1.Tenant.ID, "production")

	count, err := testStore.CountEnvironmentsByTenantAndType(context.Background(),
		CountEnvironmentsByTenantAndTypeParams{TenantID: reg2.Tenant.ID, EnvType: "production"})
	require.NoError(t, err)
	require.Equal(t, int64(0), count)
}

// ═══════════════════════════════════════════════════════════════
//
//	ListEnvironmentsByTenantTypeAndStatus
//
// ═══════════════════════════════════════════════════════════════
func TestListEnvironmentsByTenantTypeAndStatus_Basic(t *testing.T) {
	reg := createRegisteredTenant(t)

	env1 := createTestEnvironment(t, reg.Tenant.ID, "production")
	env2 := createTestEnvironment(t, reg.Tenant.ID, "production")
	createTestEnvironment(t, reg.Tenant.ID, "staging") // should not appear

	results, err := testStore.ListEnvironmentsByTenantTypeAndStatus(context.Background(),
		ListEnvironmentsByTenantTypeAndStatusParams{
			TenantID: reg.Tenant.ID,
			EnvType:  "production",
			Status:   "connected",
			Limit:    10,
			Offset:   0,
		})
	require.NoError(t, err)

	ids := map[uuid.UUID]bool{}
	for _, e := range results {
		ids[e.ID] = true
		require.Equal(t, "production", e.EnvType)
		require.Equal(t, "connected", e.Status)
		require.Equal(t, reg.Tenant.ID, e.TenantID)
	}
	require.True(t, ids[env1.ID])
	require.True(t, ids[env2.ID])
}

func TestListEnvironmentsByTenantTypeAndStatus_Pagination(t *testing.T) {
	reg := createRegisteredTenant(t)

	for i := 0; i < 5; i++ {
		createTestEnvironment(t, reg.Tenant.ID, "production")
	}

	page1, err := testStore.ListEnvironmentsByTenantTypeAndStatus(context.Background(),
		ListEnvironmentsByTenantTypeAndStatusParams{
			TenantID: reg.Tenant.ID,
			EnvType:  "production",
			Status:   "connected",
			Limit:    2,
			Offset:   0,
		})
	require.NoError(t, err)
	require.Len(t, page1, 2)

	page2, err := testStore.ListEnvironmentsByTenantTypeAndStatus(context.Background(),
		ListEnvironmentsByTenantTypeAndStatusParams{
			TenantID: reg.Tenant.ID,
			EnvType:  "production",
			Status:   "connected",
			Limit:    2,
			Offset:   2,
		})
	require.NoError(t, err)
	require.Len(t, page2, 2)

	// Pages must not overlap
	require.NotEqual(t, page1[0].ID, page2[0].ID)
	require.NotEqual(t, page1[1].ID, page2[1].ID)
}
func TestListEnvironmentsByTenantTypeAndStatus_OrderedByCreatedAtDesc(t *testing.T) {
	reg := createRegisteredTenant(t)

	for i := 0; i < 3; i++ {
		createTestEnvironment(t, reg.Tenant.ID, "production")
	}

	results, err := testStore.ListEnvironmentsByTenantTypeAndStatus(context.Background(),
		ListEnvironmentsByTenantTypeAndStatusParams{
			TenantID: reg.Tenant.ID,
			EnvType:  "production",
			Status:   "connected",
			Limit:    10,
			Offset:   0,
		})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(results), 2)

	for i := 1; i < len(results); i++ {
		require.False(t, results[i].CreatedAt.After(results[i-1].CreatedAt),
			"results must be ordered by created_at DESC")
	}
}

func TestListEnvironmentsByTenantTypeAndStatus_TenantIsolation(t *testing.T) {
	reg1 := createRegisteredTenant(t)
	reg2 := createRegisteredTenant(t)

	createTestEnvironment(t, reg1.Tenant.ID, "production")

	results, err := testStore.ListEnvironmentsByTenantTypeAndStatus(context.Background(),
		ListEnvironmentsByTenantTypeAndStatusParams{
			TenantID: reg2.Tenant.ID,
			EnvType:  "production",
			Status:   "connected",
			Limit:    10,
			Offset:   0,
		})
	require.NoError(t, err)
	require.Empty(t, results)
}

func TestListEnvironmentsByTenantTypeAndStatus_NoMatch(t *testing.T) {
	reg := createRegisteredTenant(t)
	createTestEnvironment(t, reg.Tenant.ID, "production")

	results, err := testStore.ListEnvironmentsByTenantTypeAndStatus(context.Background(),
		ListEnvironmentsByTenantTypeAndStatusParams{
			TenantID: reg.Tenant.ID,
			EnvType:  "staging",
			Status:   "disconnected",
			Limit:    10,
			Offset:   0,
		})
	require.NoError(t, err)
	require.Empty(t, results)
}

// ═══════════════════════════════════════════════════════════════
//
//	ListEnvironmentsByTenantAndType
//
// ═══════════════════════════════════════════════════════════════
func TestListEnvironmentsByTenantAndType_Basic(t *testing.T) {
	reg := createRegisteredTenant(t)

	env1 := createTestEnvironment(t, reg.Tenant.ID, "staging")
	env2 := createTestEnvironment(t, reg.Tenant.ID, "staging")
	createTestEnvironment(t, reg.Tenant.ID, "production") // must not appear

	results, err := testStore.ListEnvironmentsByTenantAndType(context.Background(),
		ListEnvironmentsByTenantAndTypeParams{
			TenantID: reg.Tenant.ID,
			EnvType:  "staging",
			Limit:    10,
			Offset:   0,
		})
	require.NoError(t, err)

	ids := map[uuid.UUID]bool{}
	for _, e := range results {
		ids[e.ID] = true
		require.Equal(t, "staging", e.EnvType)
		require.Equal(t, reg.Tenant.ID, e.TenantID)
	}
	require.True(t, ids[env1.ID])
	require.True(t, ids[env2.ID])
}
func TestListEnvironmentsByTenantAndType_Pagination(t *testing.T) {
	reg := createRegisteredTenant(t)

	for i := 0; i < 4; i++ {
		createTestEnvironment(t, reg.Tenant.ID, "staging")
	}

	page1, err := testStore.ListEnvironmentsByTenantAndType(context.Background(),
		ListEnvironmentsByTenantAndTypeParams{
			TenantID: reg.Tenant.ID,
			EnvType:  "staging",
			Limit:    2,
			Offset:   0,
		})
	require.NoError(t, err)
	require.Len(t, page1, 2)

	page2, err := testStore.ListEnvironmentsByTenantAndType(context.Background(),
		ListEnvironmentsByTenantAndTypeParams{
			TenantID: reg.Tenant.ID,
			EnvType:  "staging",
			Limit:    2,
			Offset:   2,
		})
	require.NoError(t, err)
	require.Len(t, page2, 2)

	require.NotEqual(t, page1[0].ID, page2[0].ID)
}
func TestListEnvironmentsByTenantAndType_OrderedByCreatedAtDesc(t *testing.T) {
	reg := createRegisteredTenant(t)

	for range 3 {
		createTestEnvironment(t, reg.Tenant.ID, "production")
	}

	results, err := testStore.ListEnvironmentsByTenantAndType(context.Background(),
		ListEnvironmentsByTenantAndTypeParams{
			TenantID: reg.Tenant.ID,
			EnvType:  "production",
			Limit:    10,
			Offset:   0,
		})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(results), 2)

	for i := 1; i < len(results); i++ {
		require.False(t, results[i].CreatedAt.After(results[i-1].CreatedAt),
			"results must be ordered by created_at DESC")
	}
}
func TestListEnvironmentsByTenantAndType_TenantIsolation(t *testing.T) {
	reg1 := createRegisteredTenant(t)
	reg2 := createRegisteredTenant(t)

	createTestEnvironment(t, reg1.Tenant.ID, "production")

	results, err := testStore.ListEnvironmentsByTenantAndType(context.Background(),
		ListEnvironmentsByTenantAndTypeParams{
			TenantID: reg2.Tenant.ID,
			EnvType:  "production",
			Limit:    10,
			Offset:   0,
		})
	require.NoError(t, err)
	require.Empty(t, results)
}

// ═══════════════════════════════════════════════════════════════
//
//	ListEnvironmentsByTenantAndStatus
//
// ═══════════════════════════════════════════════════════════════
func TestListEnvironmentsByTenantAndStatus_Basic(t *testing.T) {
	reg := createRegisteredTenant(t)

	env1 := createTestEnvironment(t, reg.Tenant.ID, "production") // connected
	env2 := createTestEnvironment(t, reg.Tenant.ID, "staging")    // connected

	results, err := testStore.ListEnvironmentsByTenantAndStatus(context.Background(),
		ListEnvironmentsByTenantAndStatusParams{
			TenantID: reg.Tenant.ID,
			Status:   "connected",
			Limit:    10,
			Offset:   0,
		})
	require.NoError(t, err)

	ids := map[uuid.UUID]bool{}
	for _, e := range results {
		ids[e.ID] = true
		require.Equal(t, "connected", e.Status)
		require.Equal(t, reg.Tenant.ID, e.TenantID)
	}
	require.True(t, ids[env1.ID])
	require.True(t, ids[env2.ID])
}

func TestListEnvironmentsByTenantAndStatus_FiltersOutOtherStatuses(t *testing.T) {
	reg := createRegisteredTenant(t)

	connected := createTestEnvironment(t, reg.Tenant.ID, "production") // connected

	// Manually set one env to disconnected via status update
	disconnected := createTestEnvironment(t, reg.Tenant.ID, "staging")
	err := testStore.UpdateEnvironmentStatus(context.Background(), UpdateEnvironmentStatusParams{
		ID:     disconnected.ID,
		Status: "disconnected",
	})
	require.NoError(t, err)

	results, err := testStore.ListEnvironmentsByTenantAndStatus(context.Background(),
		ListEnvironmentsByTenantAndStatusParams{
			TenantID: reg.Tenant.ID,
			Status:   "connected",
			Limit:    10,
			Offset:   0,
		})
	require.NoError(t, err)

	for _, e := range results {
		require.Equal(t, "connected", e.Status)
		require.NotEqual(t, disconnected.ID, e.ID)
	}

	ids := map[uuid.UUID]bool{}
	for _, e := range results {
		ids[e.ID] = true
	}
	require.True(t, ids[connected.ID])
}

func TestListEnvironmentsByTenantAndStatus_Pagination(t *testing.T) {
	reg := createRegisteredTenant(t)

	for i := 0; i < 5; i++ {
		createTestEnvironment(t, reg.Tenant.ID, "production")
	}

	page1, err := testStore.ListEnvironmentsByTenantAndStatus(context.Background(),
		ListEnvironmentsByTenantAndStatusParams{
			TenantID: reg.Tenant.ID,
			Status:   "connected",
			Limit:    2,
			Offset:   0,
		})
	require.NoError(t, err)
	require.Len(t, page1, 2)

	page2, err := testStore.ListEnvironmentsByTenantAndStatus(context.Background(),
		ListEnvironmentsByTenantAndStatusParams{
			TenantID: reg.Tenant.ID,
			Status:   "connected",
			Limit:    2,
			Offset:   2,
		})
	require.NoError(t, err)
	require.Len(t, page2, 2)

	require.NotEqual(t, page1[0].ID, page2[0].ID)
}

func TestListEnvironmentsByTenantAndStatus_OrderedByCreatedAtDesc(t *testing.T) {
	reg := createRegisteredTenant(t)

	for range 3 {
		createTestEnvironment(t, reg.Tenant.ID, "production")
	}

	results, err := testStore.ListEnvironmentsByTenantAndStatus(context.Background(),
		ListEnvironmentsByTenantAndStatusParams{
			TenantID: reg.Tenant.ID,
			Status:   "connected",
			Limit:    10,
			Offset:   0,
		})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(results), 2)

	for i := 1; i < len(results); i++ {
		require.False(t, results[i].CreatedAt.After(results[i-1].CreatedAt),
			"results must be ordered by created_at DESC")
	}
}

func TestListEnvironmentsByTenantAndStatus_TenantIsolation(t *testing.T) {
	reg1 := createRegisteredTenant(t)
	reg2 := createRegisteredTenant(t)

	createTestEnvironment(t, reg1.Tenant.ID, "production")

	results, err := testStore.ListEnvironmentsByTenantAndStatus(context.Background(),
		ListEnvironmentsByTenantAndStatusParams{
			TenantID: reg2.Tenant.ID,
			Status:   "connected",
			Limit:    10,
			Offset:   0,
		})
	require.NoError(t, err)
	require.Empty(t, results)
}

// ═══════════════════════════════════════════════════════════════
//  Registration Token: SetRegistrationToken / GetByToken / Clear
// ═══════════════════════════════════════════════════════════════

func TestSetRegistrationToken_Success(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	token := "reg_" + utils.RandomString(32)
	expiresAt := time.Now().Add(1 * time.Hour).UTC()

	updated, err := testStore.SetRegistrationToken(context.Background(), SetRegistrationTokenParams{
		ID:                         env.ID,
		RegistrationToken:          &token,
		RegistrationTokenExpiresAt: &expiresAt,
		TenantID:                   reg.Tenant.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, updated.RegistrationToken)
	require.Equal(t, token, *updated.RegistrationToken)
	require.NotNil(t, updated.RegistrationTokenExpiresAt)
	require.WithinDuration(t, expiresAt, *updated.RegistrationTokenExpiresAt, time.Second)
}

func TestSetRegistrationToken_WrongTenant_Fails(t *testing.T) {
	reg1 := createRegisteredTenant(t)
	reg2 := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg1.Tenant.ID, "production")

	token := "reg_" + utils.RandomString(32)
	expiresAt := time.Now().Add(1 * time.Hour).UTC()

	_, err := testStore.SetRegistrationToken(context.Background(), SetRegistrationTokenParams{
		ID:                         env.ID,
		RegistrationToken:          &token,
		RegistrationTokenExpiresAt: &expiresAt,
		TenantID:                   reg2.Tenant.ID, // wrong tenant
	})
	require.Error(t, err, "wrong tenant must not set registration token")
}

func TestSetRegistrationToken_OverwritesPreviousToken(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	token1 := "reg_" + utils.RandomString(32)
	expires := time.Now().Add(1 * time.Hour).UTC()

	_, err := testStore.SetRegistrationToken(context.Background(), SetRegistrationTokenParams{
		ID:                         env.ID,
		RegistrationToken:          &token1,
		RegistrationTokenExpiresAt: &expires,
		TenantID:                   reg.Tenant.ID,
	})
	require.NoError(t, err)

	token2 := "reg_" + utils.RandomString(32)
	updated, err := testStore.SetRegistrationToken(context.Background(), SetRegistrationTokenParams{
		ID:                         env.ID,
		RegistrationToken:          &token2,
		RegistrationTokenExpiresAt: &expires,
		TenantID:                   reg.Tenant.ID,
	})
	require.NoError(t, err)
	require.Equal(t, token2, *updated.RegistrationToken)
}

func TestGetEnvironmentByRegistrationToken_Found(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	token := "reg_" + utils.RandomString(32)
	expiresAt := time.Now().Add(1 * time.Hour).UTC()

	_, err := testStore.SetRegistrationToken(context.Background(), SetRegistrationTokenParams{
		ID:                         env.ID,
		RegistrationToken:          &token,
		RegistrationTokenExpiresAt: &expiresAt,
		TenantID:                   reg.Tenant.ID,
	})
	require.NoError(t, err)

	fetched, err := testStore.GetEnvironmentByRegistrationToken(context.Background(), &token)
	require.NoError(t, err)
	require.Equal(t, env.ID, fetched.ID)
	require.Equal(t, env.TenantID, fetched.TenantID)
}

func TestGetEnvironmentByRegistrationToken_Expired_Fails(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	token := "reg_" + utils.RandomString(32)
	// Already expired
	expiresAt := time.Now().Add(-1 * time.Hour).UTC()

	_, err := testStore.SetRegistrationToken(context.Background(), SetRegistrationTokenParams{
		ID:                         env.ID,
		RegistrationToken:          &token,
		RegistrationTokenExpiresAt: &expiresAt,
		TenantID:                   reg.Tenant.ID,
	})
	require.NoError(t, err)

	_, err = testStore.GetEnvironmentByRegistrationToken(context.Background(), &token)
	require.Error(t, err, "expired token must not be found")
	require.ErrorIs(t, err, pgx.ErrNoRows)
}

func TestGetEnvironmentByRegistrationToken_NonExistent_Fails(t *testing.T) {
	bogus := "reg_nonexistent_" + utils.RandomString(32)
	_, err := testStore.GetEnvironmentByRegistrationToken(context.Background(), &bogus)
	require.Error(t, err)
	require.ErrorIs(t, err, pgx.ErrNoRows)
}

func TestClearRegistrationToken_SetsAgentAndClearsToken(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	// Set a token first.
	token := "reg_" + utils.RandomString(32)
	expiresAt := time.Now().Add(1 * time.Hour).UTC()

	_, err := testStore.SetRegistrationToken(context.Background(), SetRegistrationTokenParams{
		ID:                         env.ID,
		RegistrationToken:          &token,
		RegistrationTokenExpiresAt: &expiresAt,
		TenantID:                   reg.Tenant.ID,
	})
	require.NoError(t, err)

	// Clear token and set agent_id.
	agentID := uuid.New().String()
	err = testStore.ClearRegistrationToken(context.Background(), ClearRegistrationTokenParams{
		ID:      env.ID,
		AgentID: &agentID,
	})
	require.NoError(t, err)

	// Verify: token cleared, agent_id set, status connected.
	fetched, err := testStore.GetEnvironmentByID(context.Background(), GetEnvironmentByIDParams{
		ID:       env.ID,
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)
	require.Nil(t, fetched.RegistrationToken, "token must be cleared")
	require.Nil(t, fetched.RegistrationTokenExpiresAt, "token expiry must be cleared")
	require.NotNil(t, fetched.AgentID)
	require.Equal(t, agentID, *fetched.AgentID)
	require.Equal(t, "connected", fetched.Status)
	require.NotNil(t, fetched.LastSync)
}

func TestClearRegistrationToken_TokenNoLongerFound(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	token := "reg_" + utils.RandomString(32)
	expiresAt := time.Now().Add(1 * time.Hour).UTC()

	_, err := testStore.SetRegistrationToken(context.Background(), SetRegistrationTokenParams{
		ID:                         env.ID,
		RegistrationToken:          &token,
		RegistrationTokenExpiresAt: &expiresAt,
		TenantID:                   reg.Tenant.ID,
	})
	require.NoError(t, err)

	// Clear the token.
	agentID := uuid.New().String()
	err = testStore.ClearRegistrationToken(context.Background(), ClearRegistrationTokenParams{
		ID:      env.ID,
		AgentID: &agentID,
	})
	require.NoError(t, err)

	// Token must no longer be resolvable.
	_, err = testStore.GetEnvironmentByRegistrationToken(context.Background(), &token)
	require.Error(t, err, "cleared token must not be found")
	require.ErrorIs(t, err, pgx.ErrNoRows)
}
