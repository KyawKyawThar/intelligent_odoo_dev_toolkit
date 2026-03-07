package db

import (
	"context"
	"encoding/json"
	"net/netip"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

//lint:ignore U1000 unused
func createTestAuditLog(t *testing.T, tenantID uuid.UUID, userID *uuid.UUID, action string) AuditLog {
	t.Helper()
	resource := "test_resource"
	resourceID := uuid.New().String()
	log, err := testStore.CreateAuditLog(context.Background(), CreateAuditLogParams{
		TenantID:   tenantID,
		UserID:     userID,
		Action:     action,
		Resource:   &resource,
		ResourceID: &resourceID,
		Metadata:   json.RawMessage(`{"test": true}`),
	})
	require.NoError(t, err)
	require.NotZero(t, log.ID)
	_ = log
	return log
}

func TestCreateAuditLog_AllFields(t *testing.T) {
	reg := createRegisteredTenant(t)
	ip := netip.MustParseAddr("192.168.1.100")
	resource := "environments"
	resourceID := uuid.New().String()
	beforeJSON := json.RawMessage(`{"status":"draft"}`)
	afterJSON := json.RawMessage(`{"status":"active"}`)

	log, err := testStore.CreateAuditLog(context.Background(), CreateAuditLogParams{
		TenantID:   reg.Tenant.ID,
		UserID:     &reg.User.ID,
		IpAddress:  &ip,
		Action:     "env.update",
		Resource:   &resource,
		ResourceID: &resourceID,
		Before:     &beforeJSON,
		After:      &afterJSON,
		Metadata:   json.RawMessage(`{"reason":"test"}`),
	})
	require.NoError(t, err)

	require.NotZero(t, log.ID)
	require.Equal(t, reg.Tenant.ID, log.TenantID)
	require.Equal(t, reg.User.ID, *log.UserID)
	require.NotNil(t, log.IpAddress)
	require.Equal(t, ip, *log.IpAddress)
	require.Equal(t, "env.update", log.Action)
	require.Equal(t, resource, *log.Resource)
	require.Equal(t, resourceID, *log.ResourceID)
	require.NotNil(t, log.Before)
	require.NotNil(t, log.After)
	require.NotZero(t, log.CreatedAt)
}
func TestCreateAuditLog_MinimalFields(t *testing.T) {
	reg := createRegisteredTenant(t)

	log, err := testStore.CreateAuditLog(context.Background(), CreateAuditLogParams{
		TenantID: reg.Tenant.ID,
		Action:   "system.ping",
		Metadata: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	require.NotZero(t, log.ID)
	require.Equal(t, "system.ping", log.Action)
	require.Nil(t, log.UserID)
	require.Nil(t, log.IpAddress)
	require.Nil(t, log.Resource)
	require.Nil(t, log.ResourceID)
	require.Nil(t, log.Before)
	require.Nil(t, log.After)
}
func TestCreateAuditLog_MetadataRoundTrip(t *testing.T) {
	reg := createRegisteredTenant(t)
	inputMeta := json.RawMessage(`{"profile_id":"abc","tags":["a","b"],"count":42}`)

	log, err := testStore.CreateAuditLog(context.Background(), CreateAuditLogParams{
		TenantID: reg.Tenant.ID,
		Action:   "meta.test",
		Metadata: inputMeta,
	})
	require.NoError(t, err)

	var expected, actual map[string]any
	require.NoError(t, json.Unmarshal(inputMeta, &expected))
	require.NoError(t, json.Unmarshal(log.Metadata, &actual))
	require.Equal(t, expected, actual)
}
func TestCreateAuditLog_BeforeAfterJSON(t *testing.T) {
	reg := createRegisteredTenant(t)
	before := json.RawMessage(`{"name":"old"}`)
	after := json.RawMessage(`{"name":"new"}`)

	log, err := testStore.CreateAuditLog(context.Background(), CreateAuditLogParams{
		TenantID: reg.Tenant.ID,
		Action:   "resource.update",
		Before:   &before,
		After:    &after,
		Metadata: json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	require.NotNil(t, log.Before)
	require.NotNil(t, log.After)

	var b, a map[string]any
	require.NoError(t, json.Unmarshal(*log.Before, &b))
	require.NoError(t, json.Unmarshal(*log.After, &a))
	require.Equal(t, "old", b["name"])
	require.Equal(t, "new", a["name"])
}
func TestCountAuditLogs_Empty(t *testing.T) {
	reg := createRegisteredTenant(t)

	count, err := testStore.CountAuditLogs(context.Background(), reg.Tenant.ID)
	require.NoError(t, err)
	require.Equal(t, int64(0), count)
}
func TestCountAuditLogs_Multiple(t *testing.T) {
	reg := createRegisteredTenant(t)

	for range 5 {
		createTestAuditLog(t, reg.Tenant.ID, &reg.User.ID, "test.action")
	}

	count, err := testStore.CountAuditLogs(context.Background(), reg.Tenant.ID)
	require.NoError(t, err)
	require.Equal(t, int64(5), count)
}

func TestCountAuditLogs_TenantIsolation(t *testing.T) {
	reg1 := createRegisteredTenant(t)
	reg2 := createRegisteredTenant(t)

	createTestAuditLog(t, reg1.Tenant.ID, &reg1.User.ID, "action1")
	createTestAuditLog(t, reg1.Tenant.ID, &reg1.User.ID, "action2")
	createTestAuditLog(t, reg2.Tenant.ID, &reg2.User.ID, "action3")

	count1, err := testStore.CountAuditLogs(context.Background(), reg1.Tenant.ID)
	require.NoError(t, err)
	require.Equal(t, int64(2), count1)

	count2, err := testStore.CountAuditLogs(context.Background(), reg2.Tenant.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), count2)
}
func TestGetAuditLogsByTenant_FiltersCorrectly(t *testing.T) {
	reg := createRegisteredTenant(t)

	createTestAuditLog(t, reg.Tenant.ID, &reg.User.ID, "anon.running")
	createTestAuditLog(t, reg.Tenant.ID, &reg.User.ID, "anon.completed")
	createTestAuditLog(t, reg.Tenant.ID, &reg.User.ID, "anon.running")

	logs, err := testStore.GetAuditLogsByTenant(context.Background(), GetAuditLogsByTenantParams{
		TenantID: reg.Tenant.ID,
		Action:   "anon.running",
	})
	require.NoError(t, err)
	require.Len(t, logs, 2)
	for _, l := range logs {
		require.Equal(t, "anon.running", l.Action)
	}
}
func TestGetAuditLogsByTenant_OrderByCreatedAtDesc(t *testing.T) {
	reg := createRegisteredTenant(t)

	for range 3 {
		createTestAuditLog(t, reg.Tenant.ID, &reg.User.ID, "ordered.action")
	}

	logs, err := testStore.GetAuditLogsByTenant(context.Background(), GetAuditLogsByTenantParams{
		TenantID: reg.Tenant.ID,
		Action:   "ordered.action",
	})
	require.NoError(t, err)
	require.Len(t, logs, 3)

	for i := 1; i < len(logs); i++ {
		require.True(t, logs[i-1].CreatedAt.After(logs[i].CreatedAt) ||
			logs[i-1].CreatedAt.Equal(logs[i].CreatedAt),
			"logs must be ordered by created_at DESC")
	}
}
func TestGetAuditLogsByTenant_NoMatch(t *testing.T) {
	reg := createRegisteredTenant(t)
	createTestAuditLog(t, reg.Tenant.ID, &reg.User.ID, "something.else")

	logs, err := testStore.GetAuditLogsByTenant(context.Background(), GetAuditLogsByTenantParams{
		TenantID: reg.Tenant.ID,
		Action:   "nonexistent.action",
	})
	require.NoError(t, err)
	require.Empty(t, logs)
}
func TestGetAuditLogsByTenant_TenantIsolation(t *testing.T) {
	reg1 := createRegisteredTenant(t)
	reg2 := createRegisteredTenant(t)

	createTestAuditLog(t, reg1.Tenant.ID, &reg1.User.ID, "shared.action")
	createTestAuditLog(t, reg2.Tenant.ID, &reg2.User.ID, "shared.action")

	logs1, err := testStore.GetAuditLogsByTenant(context.Background(), GetAuditLogsByTenantParams{
		TenantID: reg1.Tenant.ID,
		Action:   "shared.action",
	})
	require.NoError(t, err)
	require.Len(t, logs1, 1)
	require.Equal(t, reg1.Tenant.ID, logs1[0].TenantID)
}
func TestListAuditLogs_Pagination(t *testing.T) {
	reg := createRegisteredTenant(t)

	for i := 0; i < 5; i++ {
		createTestAuditLog(t, reg.Tenant.ID, &reg.User.ID, "page.action")
	}

	// Page 1: first 2
	page1, err := testStore.ListAuditLogs(context.Background(), ListAuditLogsParams{
		TenantID: reg.Tenant.ID,
		Limit:    2,
		Offset:   0,
	})
	require.NoError(t, err)
	require.Len(t, page1, 2)

	// Page 2: next 2
	page2, err := testStore.ListAuditLogs(context.Background(), ListAuditLogsParams{
		TenantID: reg.Tenant.ID,
		Limit:    2,
		Offset:   2,
	})
	require.NoError(t, err)
	require.Len(t, page2, 2)

	// Pages must not overlap
	require.NotEqual(t, page1[0].ID, page2[0].ID)
	require.NotEqual(t, page1[1].ID, page2[1].ID)

	// Page 3: last 1
	page3, err := testStore.ListAuditLogs(context.Background(), ListAuditLogsParams{
		TenantID: reg.Tenant.ID,
		Limit:    2,
		Offset:   4,
	})
	require.NoError(t, err)
	require.Len(t, page3, 1)
}

func TestListAuditLogs_OrderByCreatedAtDesc(t *testing.T) {
	reg := createRegisteredTenant(t)

	for i := 0; i < 3; i++ {
		createTestAuditLog(t, reg.Tenant.ID, &reg.User.ID, "list.order")
	}

	logs, err := testStore.ListAuditLogs(context.Background(), ListAuditLogsParams{
		TenantID: reg.Tenant.ID,
		Limit:    10,
		Offset:   0,
	})
	require.NoError(t, err)
	for i := 1; i < len(logs); i++ {
		require.True(t, logs[i-1].CreatedAt.After(logs[i].CreatedAt) ||
			logs[i-1].CreatedAt.Equal(logs[i].CreatedAt))
	}
}
func TestListAuditLogs_Empty(t *testing.T) {
	reg := createRegisteredTenant(t)

	logs, err := testStore.ListAuditLogs(context.Background(), ListAuditLogsParams{
		TenantID: reg.Tenant.ID,
		Limit:    10,
		Offset:   0,
	})
	require.NoError(t, err)
	require.Empty(t, logs)
}
func TestListAuditLogsByAction_FilterAndPagination(t *testing.T) {
	reg := createRegisteredTenant(t)

	for range 4 {
		createTestAuditLog(t, reg.Tenant.ID, &reg.User.ID, "target.action")
	}
	createTestAuditLog(t, reg.Tenant.ID, &reg.User.ID, "other.action")

	logs, err := testStore.ListAuditLogsByAction(context.Background(), ListAuditLogsByActionParams{
		TenantID: reg.Tenant.ID,
		Action:   "target.action",
		Limit:    2,
		Offset:   0,
	})
	require.NoError(t, err)
	require.Len(t, logs, 2)
	for _, l := range logs {
		require.Equal(t, "target.action", l.Action)
	}

	// Page 2
	page2, err := testStore.ListAuditLogsByAction(context.Background(), ListAuditLogsByActionParams{
		TenantID: reg.Tenant.ID,
		Action:   "target.action",
		Limit:    2,
		Offset:   2,
	})
	require.NoError(t, err)
	require.Len(t, page2, 2)
}
func TestListAuditLogsByUser_FiltersByUser(t *testing.T) {
	reg := createRegisteredTenant(t)
	reg2 := createRegisteredTenant(t)
	otherUserID := reg2.User.ID

	createTestAuditLog(t, reg.Tenant.ID, &reg.User.ID, "user.action")
	createTestAuditLog(t, reg.Tenant.ID, &reg.User.ID, "user.action")
	createTestAuditLog(t, reg.Tenant.ID, &otherUserID, "user.action")

	logs, err := testStore.ListAuditLogsByUser(context.Background(), ListAuditLogsByUserParams{
		TenantID: reg.Tenant.ID,
		UserID:   &reg.User.ID,
		Limit:    10,
		Offset:   0,
	})
	require.NoError(t, err)
	require.Len(t, logs, 2)
	for _, l := range logs {
		require.Equal(t, reg.User.ID, *l.UserID)
	}
}

func TestListAuditLogsByUser_Pagination(t *testing.T) {
	reg := createRegisteredTenant(t)

	for i := 0; i < 5; i++ {
		createTestAuditLog(t, reg.Tenant.ID, &reg.User.ID, "user.page")
	}

	page1, err := testStore.ListAuditLogsByUser(context.Background(), ListAuditLogsByUserParams{
		TenantID: reg.Tenant.ID,
		UserID:   &reg.User.ID,
		Limit:    3,
		Offset:   0,
	})
	require.NoError(t, err)
	require.Len(t, page1, 3)

	page2, err := testStore.ListAuditLogsByUser(context.Background(), ListAuditLogsByUserParams{
		TenantID: reg.Tenant.ID,
		UserID:   &reg.User.ID,
		Limit:    3,
		Offset:   3,
	})
	require.NoError(t, err)
	require.Len(t, page2, 2)
}
func TestListAuditLogsByResource_FiltersCorrectly(t *testing.T) {
	reg := createRegisteredTenant(t)
	resource := "anon_profiles"
	targetID := uuid.New().String()
	otherID := uuid.New().String()

	// 2 logs for target resource
	for i := 0; i < 2; i++ {
		_, err := testStore.CreateAuditLog(context.Background(), CreateAuditLogParams{
			TenantID:   reg.Tenant.ID,
			UserID:     &reg.User.ID,
			Action:     "anon.completed",
			Resource:   &resource,
			ResourceID: &targetID,
			Metadata:   json.RawMessage(`{}`),
		})
		require.NoError(t, err)
	}
	// 1 log for different resource_id
	_, err := testStore.CreateAuditLog(context.Background(), CreateAuditLogParams{
		TenantID:   reg.Tenant.ID,
		UserID:     &reg.User.ID,
		Action:     "anon.completed",
		Resource:   &resource,
		ResourceID: &otherID,
		Metadata:   json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	logs, err := testStore.ListAuditLogsByResource(context.Background(), ListAuditLogsByResourceParams{
		TenantID:   reg.Tenant.ID,
		Resource:   &resource,
		ResourceID: &targetID,
		Limit:      10,
	})
	require.NoError(t, err)
	require.Len(t, logs, 2)
	for _, l := range logs {
		require.Equal(t, targetID, *l.ResourceID)
	}
}
func TestListAuditLogsByResource_TenantIsolation(t *testing.T) {
	reg1 := createRegisteredTenant(t)
	reg2 := createRegisteredTenant(t)
	resource := "shared_resource"
	resID := uuid.New().String()

	_, err := testStore.CreateAuditLog(context.Background(), CreateAuditLogParams{
		TenantID:   reg1.Tenant.ID,
		UserID:     &reg1.User.ID,
		Action:     "res.create",
		Resource:   &resource,
		ResourceID: &resID,
		Metadata:   json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	// Tenant 2 must not see tenant 1's logs
	logs, err := testStore.ListAuditLogsByResource(context.Background(), ListAuditLogsByResourceParams{
		TenantID:   reg2.Tenant.ID,
		Resource:   &resource,
		ResourceID: &resID,
		Limit:      10,
	})
	require.NoError(t, err)
	require.Empty(t, logs)
}
func TestListAuditLogsBetween_DateRange(t *testing.T) {
	reg := createRegisteredTenant(t)

	// Create logs — they'll all have created_at ≈ now
	for range 3 {
		createTestAuditLog(t, reg.Tenant.ID, &reg.User.ID, "between.action")
	}

	now := time.Now()
	logs, err := testStore.ListAuditLogsBetween(context.Background(), ListAuditLogsBetweenParams{
		TenantID:    reg.Tenant.ID,
		CreatedAt:   now.Add(-1 * time.Hour),
		CreatedAt_2: now.Add(1 * time.Hour),
		Limit:       10,
		Offset:      0,
	})
	require.NoError(t, err)
	require.Len(t, logs, 3)
}
func TestListAuditLogsBetween_OutOfRange(t *testing.T) {
	reg := createRegisteredTenant(t)
	createTestAuditLog(t, reg.Tenant.ID, &reg.User.ID, "range.action")

	// Query a range in the past — should find nothing
	pastStart := time.Now().Add(-48 * time.Hour)
	pastEnd := time.Now().Add(-24 * time.Hour)

	logs, err := testStore.ListAuditLogsBetween(context.Background(), ListAuditLogsBetweenParams{
		TenantID:    reg.Tenant.ID,
		CreatedAt:   pastStart,
		CreatedAt_2: pastEnd,
		Limit:       10,
		Offset:      0,
	})
	require.NoError(t, err)
	require.Empty(t, logs)
}
func TestListAuditLogsBetween_Pagination(t *testing.T) {
	reg := createRegisteredTenant(t)

	for i := 0; i < 5; i++ {
		createTestAuditLog(t, reg.Tenant.ID, &reg.User.ID, "between.page")
	}

	now := time.Now()
	page1, err := testStore.ListAuditLogsBetween(context.Background(), ListAuditLogsBetweenParams{
		TenantID:    reg.Tenant.ID,
		CreatedAt:   now.Add(-1 * time.Hour),
		CreatedAt_2: now.Add(1 * time.Hour),
		Limit:       2,
		Offset:      0,
	})
	require.NoError(t, err)
	require.Len(t, page1, 2)

	page2, err := testStore.ListAuditLogsBetween(context.Background(), ListAuditLogsBetweenParams{
		TenantID:    reg.Tenant.ID,
		CreatedAt:   now.Add(-1 * time.Hour),
		CreatedAt_2: now.Add(1 * time.Hour),
		Limit:       2,
		Offset:      2,
	})
	require.NoError(t, err)
	require.Len(t, page2, 2)
	require.NotEqual(t, page1[0].ID, page2[0].ID)
}
func TestDeleteOldAuditLogs_DeletesOldKeepsNew(t *testing.T) {
	reg := createRegisteredTenant(t)

	// Create logs (all have created_at ≈ now)
	createTestAuditLog(t, reg.Tenant.ID, &reg.User.ID, "delete.test")
	createTestAuditLog(t, reg.Tenant.ID, &reg.User.ID, "delete.test")

	// Delete logs older than 1 hour in the future → deletes everything
	result, err := testStore.DeleteOldAuditLogs(context.Background(), DeleteOldAuditLogsParams{
		TenantID:  reg.Tenant.ID,
		CreatedAt: time.Now().Add(1 * time.Hour),
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), result.RowsAffected())

	count, err := testStore.CountAuditLogs(context.Background(), reg.Tenant.ID)
	require.NoError(t, err)
	require.Equal(t, int64(0), count)
}

func TestDeleteOldAuditLogs_CutoffInPast_DeletesNothing(t *testing.T) {
	reg := createRegisteredTenant(t)
	createTestAuditLog(t, reg.Tenant.ID, &reg.User.ID, "keep.test")

	// Cutoff in the past → nothing qualifies
	result, err := testStore.DeleteOldAuditLogs(context.Background(), DeleteOldAuditLogsParams{
		TenantID:  reg.Tenant.ID,
		CreatedAt: time.Now().Add(-24 * time.Hour),
	})
	require.NoError(t, err)
	require.Equal(t, int64(0), result.RowsAffected())

	count, err := testStore.CountAuditLogs(context.Background(), reg.Tenant.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
}

func TestDeleteOldAuditLogs_TenantIsolation(t *testing.T) {
	reg1 := createRegisteredTenant(t)
	reg2 := createRegisteredTenant(t)

	createTestAuditLog(t, reg1.Tenant.ID, &reg1.User.ID, "iso.action")
	createTestAuditLog(t, reg2.Tenant.ID, &reg2.User.ID, "iso.action")

	// Delete all of tenant 1's logs
	_, err := testStore.DeleteOldAuditLogs(context.Background(), DeleteOldAuditLogsParams{
		TenantID:  reg1.Tenant.ID,
		CreatedAt: time.Now().Add(1 * time.Hour),
	})
	require.NoError(t, err)

	// Tenant 1: gone
	count1, err := testStore.CountAuditLogs(context.Background(), reg1.Tenant.ID)
	require.NoError(t, err)
	require.Equal(t, int64(0), count1)

	// Tenant 2: untouched
	count2, err := testStore.CountAuditLogs(context.Background(), reg2.Tenant.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), count2)
}
