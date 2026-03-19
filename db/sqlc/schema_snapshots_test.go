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

func ptr[T any](v T) *T { return &v }

func createTestSnapshot(t *testing.T, envID uuid.UUID, modelCount, fieldCount int32) SchemaSnapshot {
	t.Helper()
	snap, err := testStore.CreateSchemaSnapshot(context.Background(), CreateSchemaSnapshotParams{
		EnvID:   envID,
		Version: ptr("17.0"),
		Models: json.RawMessage(`{
			"res.partner": {
				"model": "res.partner",
				"name": "Contact",
				"fields": {
					"name": {"type":"char","string":"Name","required":true},
					"email": {"type":"char","string":"Email"}
				},
				"accesses": [
					{"group_id":"base.group_user","perm_read":true,"perm_write":true,"perm_create":true,"perm_unlink":false}
				],
				"rules": [
					{"name":"res.partner.rule","domain":"[('user_id','=',user.id)]","perm_read":true,"perm_write":true,"perm_create":true,"perm_unlink":true}
				]
			}
		}`),
		ModelCount: &modelCount,
		FieldCount: &fieldCount,
		DiffRef:    nil,
	})
	require.NoError(t, err)
	require.NotZero(t, snap.ID)
	return snap
}

func TestCreateSchemaSnapshot_MinimalFields(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	snap, err := testStore.CreateSchemaSnapshot(context.Background(), CreateSchemaSnapshotParams{
		EnvID:      env.ID,
		Version:    nil,
		Models:     json.RawMessage(`{}`),
		ModelCount: nil,
		FieldCount: nil,
		DiffRef:    nil,
	})
	require.NoError(t, err)
	require.NotZero(t, snap.ID)
	require.Equal(t, env.ID, snap.EnvID)
	require.Nil(t, snap.ModelCount)
	require.Nil(t, snap.FieldCount)
	require.Nil(t, snap.DiffRef)
	require.Nil(t, snap.Version)
	require.NotZero(t, snap.CapturedAt)
}

func TestCreateSchemaSnapshot_AllFields(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	diffRef := "s3://bucket/schema/" + env.ID.String() + "/diff.json.gz"
	mc := int32(142)
	fc := int32(3891)

	snap, err := testStore.CreateSchemaSnapshot(context.Background(), CreateSchemaSnapshotParams{
		EnvID:   env.ID,
		Version: ptr("17.0"),
		Models: json.RawMessage(`{
			"res.partner": {
				"model": "res.partner",
				"name": "Contact",
				"fields": {"name":{"type":"char","string":"Name","required":true}},
				"accesses": [{"group_id":"base.group_user","perm_read":true,"perm_write":true,"perm_create":true,"perm_unlink":false}],
				"rules": []
			},
			"sale.order": {
				"model": "sale.order",
				"name": "Sales Order",
				"fields": {"name":{"type":"char","string":"Order Reference","required":true}},
				"accesses": [{"group_id":"sales_team.group_sale_salesman","perm_read":true,"perm_write":true,"perm_create":false,"perm_unlink":false}],
				"rules": [{"name":"sale.order.rule","domain":"[('user_id','=',user.id)]","perm_read":true,"perm_write":true,"perm_create":true,"perm_unlink":true}]
			}
		}`),
		ModelCount: &mc,
		FieldCount: &fc,
		DiffRef:    &diffRef,
	})
	require.NoError(t, err)
	require.NotZero(t, snap.ID)
	require.NotNil(t, snap.Version)
	require.Equal(t, "17.0", *snap.Version)
	require.NotNil(t, snap.ModelCount)
	require.Equal(t, int32(142), *snap.ModelCount)
	require.NotNil(t, snap.FieldCount)
	require.Equal(t, int32(3891), *snap.FieldCount)
	require.NotNil(t, snap.DiffRef)
	require.Equal(t, diffRef, *snap.DiffRef)
}

func TestCreateSchemaSnapshot_ModelsContainAccessesAndRules(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	mc := int32(1)
	fc := int32(3)

	snap, err := testStore.CreateSchemaSnapshot(context.Background(), CreateSchemaSnapshotParams{
		EnvID:   env.ID,
		Version: ptr("17.0"),
		Models: json.RawMessage(`{
			"res.partner": {
				"model": "res.partner",
				"name": "Contact",
				"fields": {
					"name": {"type":"char","string":"Name","required":true},
					"email": {"type":"char","string":"Email","required":false},
					"is_company": {"type":"boolean","string":"Is a Company","default":false}
				},
				"accesses": [
					{"group_id":"base.group_user","perm_read":true,"perm_write":true,"perm_create":true,"perm_unlink":false}
				],
				"rules": [
					{"name":"res.partner.rule.private.employee","domain":"['|',('user_id','=',user.id),('user_id','=',False)]","perm_read":true,"perm_write":true,"perm_create":true,"perm_unlink":true}
				]
			}
		}`),
		ModelCount: &mc,
		FieldCount: &fc,
		DiffRef:    nil,
	})
	require.NoError(t, err)

	// Verify stored JSON preserves accesses and rules inside each model.
	var models map[string]json.RawMessage
	err = json.Unmarshal(snap.Models, &models)
	require.NoError(t, err)
	require.Contains(t, models, "res.partner")

	var partner struct {
		Model    string          `json:"model"`
		Name     string          `json:"name"`
		Fields   json.RawMessage `json:"fields"`
		Accesses []struct {
			GroupID    string `json:"group_id"`
			PermRead   bool   `json:"perm_read"`
			PermWrite  bool   `json:"perm_write"`
			PermCreate bool   `json:"perm_create"`
			PermUnlink bool   `json:"perm_unlink"`
		} `json:"accesses"`
		Rules []struct {
			Name       string `json:"name"`
			Domain     string `json:"domain"`
			PermRead   bool   `json:"perm_read"`
			PermWrite  bool   `json:"perm_write"`
			PermCreate bool   `json:"perm_create"`
			PermUnlink bool   `json:"perm_unlink"`
		} `json:"rules"`
	}
	err = json.Unmarshal(models["res.partner"], &partner)
	require.NoError(t, err)

	require.Equal(t, "res.partner", partner.Model)
	require.Equal(t, "Contact", partner.Name)

	// Verify accesses
	require.Len(t, partner.Accesses, 1)
	require.Equal(t, "base.group_user", partner.Accesses[0].GroupID)
	require.True(t, partner.Accesses[0].PermRead)
	require.True(t, partner.Accesses[0].PermWrite)
	require.True(t, partner.Accesses[0].PermCreate)
	require.False(t, partner.Accesses[0].PermUnlink)

	// Verify rules
	require.Len(t, partner.Rules, 1)
	require.Equal(t, "res.partner.rule.private.employee", partner.Rules[0].Name)
	require.True(t, partner.Rules[0].PermRead)
}

func TestGetSchemaByID_Success(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	created := createTestSnapshot(t, env.ID, 10, 100)

	fetched, err := testStore.GetSchemaByID(context.Background(), GetSchemaByIDParams{
		ID:    created.ID,
		EnvID: env.ID,
	})
	require.NoError(t, err)
	require.Equal(t, created.ID, fetched.ID)
	require.Equal(t, created.EnvID, fetched.EnvID)
	require.Equal(t, *created.ModelCount, *fetched.ModelCount)
}

func TestGetSchemaByID_WrongEnvID_Fails(t *testing.T) {
	reg := createRegisteredTenant(t)
	env1 := createTestEnvironment(t, reg.Tenant.ID, "production")
	env2 := createTestEnvironment(t, reg.Tenant.ID, "staging")
	snap := createTestSnapshot(t, env1.ID, 10, 100)

	_, err := testStore.GetSchemaByID(context.Background(), GetSchemaByIDParams{
		ID:    snap.ID,
		EnvID: env2.ID, // wrong env
	})
	require.Error(t, err, "wrong env_id must return error")
}

func TestGetSchemaByID_NonExistent_Fails(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	_, err := testStore.GetSchemaByID(context.Background(), GetSchemaByIDParams{
		ID:    utils.RandomUUID(),
		EnvID: env.ID,
	})
	require.Error(t, err)
}

func TestGetSchemaSnapshotByID_Success(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	created := createTestSnapshot(t, env.ID, 5, 50)

	fetched, err := testStore.GetSchemaSnapshotByID(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, fetched.ID)
}

func TestGetSchemaSnapshotByID_NonExistent_Fails(t *testing.T) {
	_, err := testStore.GetSchemaSnapshotByID(context.Background(), uuid.New())
	require.Error(t, err)
}

func TestGetLatestSchema_ReturnsNewest(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	createTestSnapshot(t, env.ID, 10, 100)
	time.Sleep(5 * time.Millisecond)
	createTestSnapshot(t, env.ID, 20, 200)
	time.Sleep(5 * time.Millisecond)
	newest := createTestSnapshot(t, env.ID, 30, 300)

	latest, err := testStore.GetLatestSchema(context.Background(), env.ID)
	require.NoError(t, err)
	require.Equal(t, newest.ID, latest.ID)
	require.Equal(t, int32(30), *latest.ModelCount)
}

func TestGetLatestSchema_SingleSnapshot_ReturnsIt(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	snap := createTestSnapshot(t, env.ID, 5, 50)

	latest, err := testStore.GetLatestSchema(context.Background(), env.ID)
	require.NoError(t, err)
	require.Equal(t, snap.ID, latest.ID)
}

func TestGetLatestSchema_NoSnapshots_Fails(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	_, err := testStore.GetLatestSchema(context.Background(), env.ID)
	require.Error(t, err, "no snapshots must return error")
}

func TestGetLatestSchema_IncludesVersion(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	createTestSnapshot(t, env.ID, 10, 100)

	latest, err := testStore.GetLatestSchema(context.Background(), env.ID)
	require.NoError(t, err)
	require.NotNil(t, latest.Version)
	require.Equal(t, "17.0", *latest.Version)
}

func TestListSchemaSnapshots_OrderedByCapturedAtDesc(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	createTestSnapshot(t, env.ID, 10, 100)
	time.Sleep(5 * time.Millisecond)
	createTestSnapshot(t, env.ID, 20, 200)
	time.Sleep(5 * time.Millisecond)
	createTestSnapshot(t, env.ID, 30, 300)

	rows, err := testStore.ListSchemaSnapshots(context.Background(), ListSchemaSnapshotsParams{
		EnvID: env.ID,
		Limit: 10,
	})
	require.NoError(t, err)
	require.Len(t, rows, 3)

	for i := 1; i < len(rows); i++ {
		require.True(t,
			rows[i-1].CapturedAt.After(rows[i].CapturedAt) || rows[i-1].CapturedAt.Equal(rows[i].CapturedAt),
			"must be ordered by captured_at DESC",
		)
	}
}

func TestListSchemaSnapshots_RespectsLimit(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	for i := range 5 {
		createTestSnapshot(t, env.ID, int32(i+1)*10, int32(i+1)*100)
	}

	rows, err := testStore.ListSchemaSnapshots(context.Background(), ListSchemaSnapshotsParams{
		EnvID: env.ID,
		Limit: 3, // only return 3 most recent
	})
	require.NoError(t, err)
	require.Len(t, rows, 3)
}

func TestListSchemaSnapshots_IncludesVersion(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	createTestSnapshot(t, env.ID, 10, 100)

	rows, err := testStore.ListSchemaSnapshots(context.Background(), ListSchemaSnapshotsParams{
		EnvID: env.ID,
		Limit: 10,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].Version)
	require.Equal(t, "17.0", *rows[0].Version)
}

func TestListSchemaSnapshots_DoesNotExposeModels(t *testing.T) {
	// ListSchemaSnapshots returns a lightweight row (no models JSONB)
	// This is intentional — full data fetched via GetSchemaByID
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	createTestSnapshot(t, env.ID, 10, 100)

	rows, err := testStore.ListSchemaSnapshots(context.Background(), ListSchemaSnapshotsParams{
		EnvID: env.ID,
		Limit: 10,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)

	// ListSchemaSnapshotsRow has no Models field
	// Compile-time verified by sqlc struct
	var row ListSchemaSnapshotsRow = rows[0]
	require.NotZero(t, row.ID)
	require.NotZero(t, row.CapturedAt)
}

func TestListSchemaSnapshots_EmptyEnv_ReturnsEmpty(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	rows, err := testStore.ListSchemaSnapshots(context.Background(), ListSchemaSnapshotsParams{
		EnvID: env.ID,
		Limit: 10,
	})
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestCountSchemasByEnv_CorrectCount(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	count, err := testStore.CountSchemasByEnv(context.Background(), env.ID)
	require.NoError(t, err)
	require.Equal(t, int64(0), count)

	createTestSnapshot(t, env.ID, 10, 100)
	createTestSnapshot(t, env.ID, 20, 200)

	count, err = testStore.CountSchemasByEnv(context.Background(), env.ID)
	require.NoError(t, err)
	require.Equal(t, int64(2), count)
}

func TestGetTwoSchemasForDiff_ReturnsBothOrderedByTime(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	old := createTestSnapshot(t, env.ID, 10, 100)
	time.Sleep(5 * time.Millisecond)
	new_ := createTestSnapshot(t, env.ID, 20, 200)

	results, err := testStore.GetTwoSchemasForDiff(context.Background(), []uuid.UUID{old.ID, new_.ID})
	require.NoError(t, err)
	require.Len(t, results, 2)

	// Ordered by captured_at ASC — oldest first
	require.Equal(t, old.ID, results[0].ID)
	require.Equal(t, new_.ID, results[1].ID)
}

func TestGetTwoSchemasForDiff_OneNonExistent_ReturnsOne(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	snap := createTestSnapshot(t, env.ID, 10, 100)

	results, err := testStore.GetTwoSchemasForDiff(context.Background(), []uuid.UUID{
		snap.ID,
		uuid.New(), // does not exist
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, snap.ID, results[0].ID)
}

func TestGetTwoSchemasForDiff_EmptyInput_ReturnsEmpty(t *testing.T) {
	results, err := testStore.GetTwoSchemasForDiff(context.Background(), []uuid.UUID{})
	require.NoError(t, err)
	require.Empty(t, results)
}

func TestDeleteOldSchemaSnapshots_KeepsNMostRecent(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	// Create 5 snapshots
	for i := range 5 {
		createTestSnapshot(t, env.ID, int32(i+1)*10, int32(i+1)*100)
		time.Sleep(3 * time.Millisecond)
	}

	count, err := testStore.CountSchemasByEnv(context.Background(), env.ID)
	require.NoError(t, err)
	require.Equal(t, int64(5), count)

	// Keep only 3 most recent
	_, err = testStore.DeleteOldSchemaSnapshots(context.Background(), DeleteOldSchemaSnapshotsParams{
		TenantID: reg.Tenant.ID,
		Limit:    3,
	})
	require.NoError(t, err)

	count, err = testStore.CountSchemasByEnv(context.Background(), env.ID)
	require.NoError(t, err)
	require.Equal(t, int64(3), count)
}

func TestDeleteOldSchemaSnapshots_OnlyAffectsOwnTenant(t *testing.T) {
	reg1 := createRegisteredTenant(t)
	reg2 := createRegisteredTenant(t)
	env1 := createTestEnvironment(t, reg1.Tenant.ID, "production")
	env2 := createTestEnvironment(t, reg2.Tenant.ID, "production")

	for i := 0; i < 4; i++ {
		createTestSnapshot(t, env1.ID, int32(i+1)*10, 100)
		createTestSnapshot(t, env2.ID, int32(i+1)*10, 100)
	}

	// Delete for tenant 1, keep 2
	_, err := testStore.DeleteOldSchemaSnapshots(context.Background(), DeleteOldSchemaSnapshotsParams{
		TenantID: reg1.Tenant.ID,
		Limit:    2,
	})
	require.NoError(t, err)

	count1, err := testStore.CountSchemasByEnv(context.Background(), env1.ID)
	require.NoError(t, err)
	require.Equal(t, int64(2), count1, "tenant 1 must have only 2 snapshots")

	count2, err := testStore.CountSchemasByEnv(context.Background(), env2.ID)
	require.NoError(t, err)
	require.Equal(t, int64(4), count2, "tenant 2 must be untouched")
}

func TestDeleteOldSchemaSnapshots_KeepMoreThanExist_DeletesNothing(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	createTestSnapshot(t, env.ID, 10, 100)
	createTestSnapshot(t, env.ID, 20, 200)

	// Keep 10, only 2 exist — nothing should be deleted
	_, err := testStore.DeleteOldSchemaSnapshots(context.Background(), DeleteOldSchemaSnapshotsParams{
		TenantID: reg.Tenant.ID,
		Limit:    10,
	})
	require.NoError(t, err)

	count, err := testStore.CountSchemasByEnv(context.Background(), env.ID)
	require.NoError(t, err)
	require.Equal(t, int64(2), count)
}
