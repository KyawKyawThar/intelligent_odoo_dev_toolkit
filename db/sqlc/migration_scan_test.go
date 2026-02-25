package db

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func createTestMigrationScan(t *testing.T, envID uuid.UUID, triggeredBy *uuid.UUID) MigrationScan {
	t.Helper()
	issues := json.RawMessage(`[{"module":"sale","type":"breaking","description":"field removed"}]`)
	scan, err := testStore.CreateMigrationScan(context.Background(), CreateMigrationScanParams{
		EnvID:         envID,
		TriggeredBy:   triggeredBy,
		FromVersion:   "16.0",
		ToVersion:     "17.0",
		Issues:        issues,
		BreakingCount: 1,
		WarningCount:  2,
		MinorCount:    5,
		Status:        "completed",
	})
	require.NoError(t, err)
	require.NotZero(t, scan.ID)
	return scan
}
func TestCreateMigrationScan_AllFields(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	issues := json.RawMessage(`[{"module":"stock","type":"warning","description":"deprecated API"}]`)

	scan, err := testStore.CreateMigrationScan(context.Background(), CreateMigrationScanParams{
		EnvID:         env.ID,
		TriggeredBy:   &reg.User.ID,
		FromVersion:   "15.0",
		ToVersion:     "17.0",
		Issues:        issues,
		BreakingCount: 3,
		WarningCount:  7,
		MinorCount:    12,
		Status:        "completed",
	})
	require.NoError(t, err)

	require.NotZero(t, scan.ID)
	require.Equal(t, env.ID, scan.EnvID)
	require.NotNil(t, scan.TriggeredBy)
	require.Equal(t, reg.User.ID, *scan.TriggeredBy)
	require.Equal(t, "15.0", scan.FromVersion)
	require.Equal(t, "17.0", scan.ToVersion)
	require.Equal(t, int32(3), scan.BreakingCount)
	require.Equal(t, int32(7), scan.WarningCount)
	require.Equal(t, int32(12), scan.MinorCount)
	require.Equal(t, "completed", scan.Status)
	require.NotZero(t, scan.ScannedAt)

	var parsedIssues []map[string]any
	require.NoError(t, json.Unmarshal(scan.Issues, &parsedIssues))
	require.Len(t, parsedIssues, 1)
	require.Equal(t, "stock", parsedIssues[0]["module"])
}
func TestCreateMigrationScan_NilTriggeredBy(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	scan, err := testStore.CreateMigrationScan(context.Background(), CreateMigrationScanParams{
		EnvID:         env.ID,
		TriggeredBy:   nil,
		FromVersion:   "16.0",
		ToVersion:     "17.0",
		Issues:        json.RawMessage(`[]`),
		BreakingCount: 0,
		WarningCount:  0,
		MinorCount:    0,
		Status:        "completed",
	})
	require.NoError(t, err)
	require.Nil(t, scan.TriggeredBy)
}
func TestCreateMigrationScan_ZeroCounts(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	scan, err := testStore.CreateMigrationScan(context.Background(), CreateMigrationScanParams{
		EnvID:         env.ID,
		FromVersion:   "17.0",
		ToVersion:     "17.0",
		Issues:        json.RawMessage(`[]`),
		BreakingCount: 0,
		WarningCount:  0,
		MinorCount:    0,
		Status:        "completed",
	})
	require.NoError(t, err)
	require.Equal(t, int32(0), scan.BreakingCount)
	require.Equal(t, int32(0), scan.WarningCount)
	require.Equal(t, int32(0), scan.MinorCount)
}
func TestCreateMigrationScan_InvalidEnvID_Fails(t *testing.T) {
	reg := createRegisteredTenant(t)

	_, err := testStore.CreateMigrationScan(context.Background(), CreateMigrationScanParams{
		EnvID:       uuid.New(),
		TriggeredBy: &reg.User.ID,
		FromVersion: "16.0",
		ToVersion:   "17.0",
		Issues:      json.RawMessage(`[]`),
		Status:      "completed",
	})
	require.Error(t, err)
}
func TestCreateMigrationScan_StatusRunning(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	scan, err := testStore.CreateMigrationScan(context.Background(), CreateMigrationScanParams{
		EnvID:       env.ID,
		FromVersion: "16.0",
		ToVersion:   "17.0",
		Issues:      json.RawMessage(`[]`),
		Status:      "running",
	})
	require.NoError(t, err)
	require.Equal(t, "running", scan.Status)
}
func TestGetMigrationScan_Found(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	created := createTestMigrationScan(t, env.ID, &reg.User.ID)

	fetched, err := testStore.GetMigrationScan(context.Background(), GetMigrationScanParams{
		ID:    created.ID,
		EnvID: env.ID,
	})
	require.NoError(t, err)
	require.Equal(t, created.ID, fetched.ID)
	require.Equal(t, created.FromVersion, fetched.FromVersion)
	require.Equal(t, created.ToVersion, fetched.ToVersion)
	require.Equal(t, created.BreakingCount, fetched.BreakingCount)
}
func TestGetMigrationScan_WrongEnvID(t *testing.T) {
	reg := createRegisteredTenant(t)
	env1 := createTestEnvironment(t, reg.Tenant.ID, "production")
	env2 := createTestEnvironment(t, reg.Tenant.ID, "production")
	scan := createTestMigrationScan(t, env1.ID, &reg.User.ID)

	_, err := testStore.GetMigrationScan(context.Background(), GetMigrationScanParams{
		ID:    scan.ID,
		EnvID: env2.ID,
	})
	require.Error(t, err)
}
func TestGetMigrationScan_NotFound(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	_, err := testStore.GetMigrationScan(context.Background(), GetMigrationScanParams{
		ID:    uuid.New(),
		EnvID: env.ID,
	})
	require.Error(t, err)
}

// ═══════════════════════════════════════════════════════════════
//  GetLatestMigrationScan
// ═══════════════════════════════════════════════════════════════

func TestGetLatestMigrationScan_ReturnsNewest(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	_ = createTestMigrationScan(t, env.ID, &reg.User.ID)
	_ = createTestMigrationScan(t, env.ID, &reg.User.ID)
	latest := createTestMigrationScan(t, env.ID, &reg.User.ID)

	fetched, err := testStore.GetLatestMigrationScan(context.Background(), env.ID)
	require.NoError(t, err)
	require.Equal(t, latest.ID, fetched.ID)
}
func TestGetLatestMigrationScan_NoScans(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	_, err := testStore.GetLatestMigrationScan(context.Background(), env.ID)
	require.Error(t, err)
}

func TestGetLatestMigrationScan_EnvIsolation(t *testing.T) {
	reg := createRegisteredTenant(t)
	env1 := createTestEnvironment(t, reg.Tenant.ID, "production")
	env2 := createTestEnvironment(t, reg.Tenant.ID, "production")

	scan1 := createTestMigrationScan(t, env1.ID, &reg.User.ID)
	_ = createTestMigrationScan(t, env2.ID, &reg.User.ID)

	fetched, err := testStore.GetLatestMigrationScan(context.Background(), env1.ID)
	require.NoError(t, err)
	require.Equal(t, scan1.ID, fetched.ID)
	require.Equal(t, env1.ID, fetched.EnvID)
}

// ═══════════════════════════════════════════════════════════════
//  ListMigrationScans
// ═══════════════════════════════════════════════════════════════

func TestListMigrationScans_Pagination(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	for i := 0; i < 5; i++ {
		createTestMigrationScan(t, env.ID, &reg.User.ID)
	}

	page1, err := testStore.ListMigrationScans(context.Background(), ListMigrationScansParams{
		EnvID: env.ID, Limit: 2, Offset: 0,
	})
	require.NoError(t, err)
	require.Len(t, page1, 2)

	page2, err := testStore.ListMigrationScans(context.Background(), ListMigrationScansParams{
		EnvID: env.ID, Limit: 2, Offset: 2,
	})
	require.NoError(t, err)
	require.Len(t, page2, 2)
	require.NotEqual(t, page1[0].ID, page2[0].ID)

	page3, err := testStore.ListMigrationScans(context.Background(), ListMigrationScansParams{
		EnvID: env.ID, Limit: 2, Offset: 4,
	})
	require.NoError(t, err)
	require.Len(t, page3, 1)
}
func TestListMigrationScans_OrderByScannedAtDesc(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	for i := 0; i < 3; i++ {
		createTestMigrationScan(t, env.ID, &reg.User.ID)
	}

	scans, err := testStore.ListMigrationScans(context.Background(), ListMigrationScansParams{
		EnvID: env.ID, Limit: 10, Offset: 0,
	})
	require.NoError(t, err)
	for i := 1; i < len(scans); i++ {
		require.True(t, scans[i-1].ScannedAt.After(scans[i].ScannedAt) ||
			scans[i-1].ScannedAt.Equal(scans[i].ScannedAt))
	}
}
func TestListMigrationScans_Empty(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	scans, err := testStore.ListMigrationScans(context.Background(), ListMigrationScansParams{
		EnvID: env.ID, Limit: 10, Offset: 0,
	})
	require.NoError(t, err)
	require.Empty(t, scans)
}
func TestListMigrationScans_EnvIsolation(t *testing.T) {
	reg := createRegisteredTenant(t)
	env1 := createTestEnvironment(t, reg.Tenant.ID, "production")
	env2 := createTestEnvironment(t, reg.Tenant.ID, "production")

	createTestMigrationScan(t, env1.ID, &reg.User.ID)
	createTestMigrationScan(t, env1.ID, &reg.User.ID)
	createTestMigrationScan(t, env2.ID, &reg.User.ID)

	scans1, err := testStore.ListMigrationScans(context.Background(), ListMigrationScansParams{
		EnvID: env1.ID, Limit: 10, Offset: 0,
	})
	require.NoError(t, err)
	require.Len(t, scans1, 2)
	for _, s := range scans1 {
		require.Equal(t, env1.ID, s.EnvID)
	}
}

// ═══════════════════════════════════════════════════════════════
//  DeleteMigrationScan
// ═══════════════════════════════════════════════════════════════

func TestDeleteMigrationScan(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	scan := createTestMigrationScan(t, env.ID, &reg.User.ID)

	err := testStore.DeleteMigrationScan(context.Background(), DeleteMigrationScanParams{
		ID:    scan.ID,
		EnvID: env.ID,
	})
	require.NoError(t, err)

	_, err = testStore.GetMigrationScan(context.Background(), GetMigrationScanParams{
		ID:    scan.ID,
		EnvID: env.ID,
	})
	require.Error(t, err)
}
func TestDeleteMigrationScan_WrongEnvID_NoOp(t *testing.T) {
	reg := createRegisteredTenant(t)
	env1 := createTestEnvironment(t, reg.Tenant.ID, "production")
	env2 := createTestEnvironment(t, reg.Tenant.ID, "production")
	scan := createTestMigrationScan(t, env1.ID, &reg.User.ID)

	err := testStore.DeleteMigrationScan(context.Background(), DeleteMigrationScanParams{
		ID:    scan.ID,
		EnvID: env2.ID,
	})
	require.NoError(t, err)

	// Must still exist
	_, err = testStore.GetMigrationScan(context.Background(), GetMigrationScanParams{
		ID:    scan.ID,
		EnvID: env1.ID,
	})
	require.NoError(t, err)
}

// ═══════════════════════════════════════════════════════════════
//  Issues JSON round-trip
// ═══════════════════════════════════════════════════════════════

func TestCreateMigrationScan_IssuesRoundTrip(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	issues := json.RawMessage(`[
		{"module":"sale","type":"breaking","description":"field removed","field":"x_custom"},
		{"module":"stock","type":"warning","description":"deprecated method"}
	]`)

	scan, err := testStore.CreateMigrationScan(context.Background(), CreateMigrationScanParams{
		EnvID:         env.ID,
		FromVersion:   "16.0",
		ToVersion:     "17.0",
		Issues:        issues,
		BreakingCount: 1,
		WarningCount:  1,
		MinorCount:    0,
		Status:        "completed",
	})
	require.NoError(t, err)

	var expected, actual []any
	require.NoError(t, json.Unmarshal(issues, &expected))
	require.NoError(t, json.Unmarshal(scan.Issues, &actual))
	require.Equal(t, expected, actual)

	fetched, err := testStore.GetMigrationScan(context.Background(), GetMigrationScanParams{
		ID: scan.ID, EnvID: env.ID,
	})
	require.NoError(t, err)
	var fetchedIssues []any
	require.NoError(t, json.Unmarshal(fetched.Issues, &fetchedIssues))
	require.Equal(t, expected, fetchedIssues)
}
