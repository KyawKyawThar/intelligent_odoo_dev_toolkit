package db

import (
	"Intelligent_Dev_ToolKit_Odoo/utils"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
)

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
		EnvType:      "production",
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
		EnvType:      "staging",
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

	updated, err := testStore.UpdateEnvironment(context.Background(), UpdateEnvironmentParams{
		ID:          env.ID,
		Name:        "Updated Name",
		OdooUrl:     "https://updated.example.com",
		DbName:      "odoo_updated",
		OdooVersion: &newVersion,
		EnvType:     "staging",
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

	_, err := testStore.UpdateEnvironment(context.Background(), UpdateEnvironmentParams{
		ID:       env.ID,
		Name:     "Hacked",
		OdooUrl:  "https://hacked.com",
		DbName:   "hacked",
		EnvType:  "production",
		TenantID: reg2.Tenant.ID,
	})
	require.Error(t, err)
}
func TestUpdateEnvironment_PreservesStatusAndAgent(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	updated, err := testStore.UpdateEnvironment(context.Background(), UpdateEnvironmentParams{
		ID:       env.ID,
		Name:     "New Name",
		OdooUrl:  env.OdooUrl,
		DbName:   env.DbName,
		EnvType:  env.EnvType,
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
		Status:       "healthy",
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
	for i := 0; i < 3; i++ {
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
