package db

import (
	"Intelligent_Dev_ToolKit_Odoo/utils"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func createTestErrorGroup(t *testing.T, envID uuid.UUID) ErrorGroup {
	t.Helper()
	sig := "sig_" + utils.RandomString(12)
	module := "sale"
	model := "sale.order"
	traceRef := "s3://traces/" + utils.RandomString(10) + ".json.gz"

	eg, err := testStore.UpsertErrorGroup(context.Background(), UpsertErrorGroupParams{
		EnvID:         envID,
		Signature:     sig,
		ErrorType:     "ValidationError",
		Message:       "Test error: " + sig,
		Module:        &module,
		Model:         &model,
		FirstSeen:     time.Now(),
		AffectedUsers: []int32{1, 2, 3},
		RawTraceRef:   &traceRef,
	})
	require.NoError(t, err)
	require.NotZero(t, eg.ID)
	return eg
}
func TestUpsertErrorGroup_NewSignature_Creates(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	module := "stock"
	model := "stock.picking"
	traceRef := "s3://traces/new.json.gz"
	now := time.Now().UTC().Truncate(time.Microsecond)

	eg, err := testStore.UpsertErrorGroup(context.Background(), UpsertErrorGroupParams{
		EnvID:         env.ID,
		Signature:     "sig_unique_new_" + utils.RandomString(8),
		ErrorType:     "ValueError",
		Message:       "Something went wrong",
		Module:        &module,
		Model:         &model,
		FirstSeen:     now,
		AffectedUsers: []int32{10, 20},
		RawTraceRef:   &traceRef,
	})
	require.NoError(t, err)

	require.NotZero(t, eg.ID)
	require.Equal(t, env.ID, eg.EnvID)
	require.Equal(t, "ValueError", eg.ErrorType)
	require.Equal(t, "Something went wrong", eg.Message)
	require.Equal(t, "stock", *eg.Module)
	require.Equal(t, "stock.picking", *eg.Model)
	require.Equal(t, int32(1), eg.OccurrenceCount)
	require.Equal(t, "open", eg.Status)
	require.Nil(t, eg.ResolvedBy)
	require.Nil(t, eg.ResolvedAt)
	require.NotZero(t, eg.CreatedAt)
}
func TestUpsertErrorGroup_SameSignature_IncrementsCount(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	sig := "sig_dup_" + utils.RandomString(8)
	module := "sale"

	first, err := testStore.UpsertErrorGroup(context.Background(), UpsertErrorGroupParams{
		EnvID:         env.ID,
		Signature:     sig,
		ErrorType:     "ValidationError",
		Message:       "First message",
		Module:        &module,
		FirstSeen:     time.Now(),
		AffectedUsers: []int32{1},
	})
	require.NoError(t, err)
	require.Equal(t, int32(1), first.OccurrenceCount)

	// Second upsert with same env_id + signature
	second, err := testStore.UpsertErrorGroup(context.Background(), UpsertErrorGroupParams{
		EnvID:         env.ID,
		Signature:     sig,
		ErrorType:     "ValidationError",
		Message:       "Updated message",
		Module:        &module,
		FirstSeen:     time.Now(),
		AffectedUsers: []int32{2},
	})
	require.NoError(t, err)

	require.Equal(t, first.ID, second.ID, "same row must be updated")
	require.Equal(t, int32(2), second.OccurrenceCount)
	require.Equal(t, "Updated message", second.Message, "message must be updated on conflict")
	// first_seen must NOT change
	require.Equal(t, first.FirstSeen.Unix(), second.FirstSeen.Unix())
}
func TestUpsertErrorGroup_SameSignatureDifferentEnv_CreatesSeparate(t *testing.T) {
	reg := createRegisteredTenant(t)
	env1 := createTestEnvironment(t, reg.Tenant.ID, "production")
	env2 := createTestEnvironment(t, reg.Tenant.ID, "production")
	sig := "sig_shared_" + utils.RandomString(8)

	eg1, err := testStore.UpsertErrorGroup(context.Background(), UpsertErrorGroupParams{
		EnvID: env1.ID, Signature: sig, ErrorType: "E", Message: "m",
		FirstSeen: time.Now(), AffectedUsers: []int32{1},
	})
	require.NoError(t, err)

	eg2, err := testStore.UpsertErrorGroup(context.Background(), UpsertErrorGroupParams{
		EnvID: env2.ID, Signature: sig, ErrorType: "E", Message: "m",
		FirstSeen: time.Now(), AffectedUsers: []int32{1},
	})
	require.NoError(t, err)

	require.NotEqual(t, eg1.ID, eg2.ID, "same sig in different envs must be separate rows")
}
func TestGetErrorGroupByID_Found(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	created := createTestErrorGroup(t, env.ID)

	fetched, err := testStore.GetErrorGroupByID(context.Background(), GetErrorGroupByIDParams{
		ID:    created.ID,
		EnvID: env.ID,
	})
	require.NoError(t, err)
	require.Equal(t, created.ID, fetched.ID)
	require.Equal(t, created.Signature, fetched.Signature)
}
func TestGetErrorGroupByID_WrongEnv(t *testing.T) {
	reg := createRegisteredTenant(t)
	env1 := createTestEnvironment(t, reg.Tenant.ID, "production")
	env2 := createTestEnvironment(t, reg.Tenant.ID, "production")
	eg := createTestErrorGroup(t, env1.ID)

	_, err := testStore.GetErrorGroupByID(context.Background(), GetErrorGroupByIDParams{
		ID:    eg.ID,
		EnvID: env2.ID,
	})
	require.Error(t, err)
}

// ═══════════════════════════════════════════════════════════════.
//  GetErrorGroupBySignature
// ═══════════════════════════════════════════════════════════════.

func TestGetErrorGroupBySignature_Found(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	eg := createTestErrorGroup(t, env.ID)

	fetched, err := testStore.GetErrorGroupBySignature(context.Background(), GetErrorGroupBySignatureParams{
		EnvID:     env.ID,
		Signature: eg.Signature,
	})
	require.NoError(t, err)
	require.Equal(t, eg.ID, fetched.ID)
}

func TestGetErrorGroupBySignature_NotFound(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	_, err := testStore.GetErrorGroupBySignature(context.Background(), GetErrorGroupBySignatureParams{
		EnvID:     env.ID,
		Signature: "nonexistent_sig",
	})
	require.Error(t, err)
}
func TestListErrorGroups_PaginationAndOrder(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	for i := 0; i < 5; i++ {
		createTestErrorGroup(t, env.ID)
	}

	page1, err := testStore.ListErrorGroups(context.Background(), ListErrorGroupsParams{
		EnvID:  env.ID,
		Limit:  3,
		Offset: 0,
	})
	require.NoError(t, err)
	require.Len(t, page1, 3)

	page2, err := testStore.ListErrorGroups(context.Background(), ListErrorGroupsParams{
		EnvID:  env.ID,
		Limit:  3,
		Offset: 3,
	})
	require.NoError(t, err)
	require.Len(t, page2, 2)

	// No overlap
	require.NotEqual(t, page1[0].ID, page2[0].ID)
}
func TestListErrorGroups_Empty(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	groups, err := testStore.ListErrorGroups(context.Background(), ListErrorGroupsParams{
		EnvID: env.ID, Limit: 10, Offset: 0,
	})
	require.NoError(t, err)
	require.Empty(t, groups)
}

// ═══════════════════════════════════════════════════════════════.
//  CountErrorGroupsByEnv
// ═══════════════════════════════════════════════════════════════.

func TestCountErrorGroupsByEnv(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	for range 3 {
		createTestErrorGroup(t, env.ID)
	}

	count, err := testStore.CountErrorGroupsByEnv(context.Background(), env.ID)
	require.NoError(t, err)
	require.Equal(t, int64(3), count)
}

// ═══════════════════════════════════════════════════════════════.
//  ListErrorGroupsByStatus / CountErrorGroupsByStatus
// ═══════════════════════════════════════════════════════════════.

func TestListErrorGroupsByStatus(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	eg1 := createTestErrorGroup(t, env.ID)
	_ = createTestErrorGroup(t, env.ID) // stays open

	// Resolve eg1
	_, err := testStore.ResolveErrorGroup(context.Background(), ResolveErrorGroupParams{
		ID:         eg1.ID,
		ResolvedBy: &reg.User.ID,
		EnvID:      env.ID,
	})
	require.NoError(t, err)

	openGroups, err := testStore.ListErrorGroupsByStatus(context.Background(), ListErrorGroupsByStatusParams{
		EnvID: env.ID, Status: "open", Limit: 10, Offset: 0,
	})
	require.NoError(t, err)
	require.Len(t, openGroups, 1)

	resolvedGroups, err := testStore.ListErrorGroupsByStatus(context.Background(), ListErrorGroupsByStatusParams{
		EnvID: env.ID, Status: "resolved", Limit: 10, Offset: 0,
	})
	require.NoError(t, err)
	require.Len(t, resolvedGroups, 1)
}
func TestCountErrorGroupsByStatus(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	createTestErrorGroup(t, env.ID)
	createTestErrorGroup(t, env.ID)

	count, err := testStore.CountErrorGroupsByStatus(context.Background(), CountErrorGroupsByStatusParams{
		EnvID: env.ID, Status: "open",
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), count)
}

// ═══════════════════════════════════════════════════════════════.
//  ListErrorGroupsByType
// ═══════════════════════════════════════════════════════════════.

func TestListErrorGroupsByType(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	// createTestErrorGroup uses "ValidationError"
	createTestErrorGroup(t, env.ID)
	createTestErrorGroup(t, env.ID)

	// Create one with a different type
	_, err := testStore.UpsertErrorGroup(context.Background(), UpsertErrorGroupParams{
		EnvID: env.ID, Signature: "sig_type_" + utils.RandomString(8),
		ErrorType: "AccessError", Message: "Access denied",
		FirstSeen: time.Now(), AffectedUsers: []int32{1},
	})
	require.NoError(t, err)

	valErrors, err := testStore.ListErrorGroupsByType(context.Background(), ListErrorGroupsByTypeParams{
		EnvID: env.ID, ErrorType: "ValidationError", Limit: 10, Offset: 0,
	})
	require.NoError(t, err)
	require.Len(t, valErrors, 2)
	for _, eg := range valErrors {
		require.Equal(t, "ValidationError", eg.ErrorType)
	}

	accessErrors, err := testStore.ListErrorGroupsByType(context.Background(), ListErrorGroupsByTypeParams{
		EnvID: env.ID, ErrorType: "AccessError", Limit: 10, Offset: 0,
	})
	require.NoError(t, err)
	require.Len(t, accessErrors, 1)
}

// ═══════════════════════════════════════════════════════════════.
//
//	SearchErrorGroups
//
// ═══════════════════════════════════════════════════════════════.
func TestSearchErrorGroups_ByMessage(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	uniqueWord := "xyzUnique" + utils.RandomString(6)

	_, err := testStore.UpsertErrorGroup(context.Background(), UpsertErrorGroupParams{
		EnvID: env.ID, Signature: "sig_search_" + utils.RandomString(8),
		ErrorType: "E", Message: "Error with " + uniqueWord + " keyword",
		FirstSeen: time.Now(), AffectedUsers: []int32{1},
	})
	require.NoError(t, err)

	// Another group without the keyword
	createTestErrorGroup(t, env.ID)

	results, err := testStore.SearchErrorGroups(context.Background(), SearchErrorGroupsParams{
		EnvID:   env.ID,
		Column2: &uniqueWord,
		Limit:   10,
		Offset:  0,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Contains(t, results[0].Message, uniqueWord)
}
func TestSearchErrorGroups_ByModule(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	uniqueModule := "uniquemod_" + utils.RandomString(6)

	_, err := testStore.UpsertErrorGroup(context.Background(), UpsertErrorGroupParams{
		EnvID: env.ID, Signature: "sig_mod_" + utils.RandomString(8),
		ErrorType: "E", Message: "generic error",
		Module:    &uniqueModule,
		FirstSeen: time.Now(), AffectedUsers: []int32{1},
	})
	require.NoError(t, err)

	results, err := testStore.SearchErrorGroups(context.Background(), SearchErrorGroupsParams{
		EnvID: env.ID, Column2: &uniqueModule, Limit: 10, Offset: 0,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
}
func TestSearchErrorGroups_NoMatch(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	createTestErrorGroup(t, env.ID)

	query := "zzz_no_match_ever_" + utils.RandomString(10)
	results, err := testStore.SearchErrorGroups(context.Background(), SearchErrorGroupsParams{
		EnvID: env.ID, Column2: &query, Limit: 10, Offset: 0,
	})
	require.NoError(t, err)
	require.Empty(t, results)
}

// ═══════════════════════════════════════════════════════════════.
//  AcknowledgeErrorGroup
// ═══════════════════════════════════════════════════════════════.

func TestAcknowledgeErrorGroup(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	eg := createTestErrorGroup(t, env.ID)
	require.Equal(t, "open", eg.Status)

	acked, err := testStore.AcknowledgeErrorGroup(context.Background(), AcknowledgeErrorGroupParams{
		ID:    eg.ID,
		EnvID: env.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "acknowledged", acked.Status)
}
func TestAcknowledgeErrorGroup_WrongEnv(t *testing.T) {
	reg := createRegisteredTenant(t)
	env1 := createTestEnvironment(t, reg.Tenant.ID, "production")
	env2 := createTestEnvironment(t, reg.Tenant.ID, "production")
	eg := createTestErrorGroup(t, env1.ID)

	_, err := testStore.AcknowledgeErrorGroup(context.Background(), AcknowledgeErrorGroupParams{
		ID:    eg.ID,
		EnvID: env2.ID,
	})
	require.Error(t, err)
}

// ═══════════════════════════════════════════════════════════════.
//  ResolveErrorGroup
// ═══════════════════════════════════════════════════════════════.

func TestResolveErrorGroup(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	eg := createTestErrorGroup(t, env.ID)

	resolved, err := testStore.ResolveErrorGroup(context.Background(), ResolveErrorGroupParams{
		ID:         eg.ID,
		ResolvedBy: &reg.User.ID,
		EnvID:      env.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "resolved", resolved.Status)
	require.NotNil(t, resolved.ResolvedBy)
	require.Equal(t, reg.User.ID, *resolved.ResolvedBy)
	require.NotNil(t, resolved.ResolvedAt)
}

// ═══════════════════════════════════════════════════════════════.
//  ReopenErrorGroup
// ═══════════════════════════════════════════════════════════════.

func TestReopenErrorGroup(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	eg := createTestErrorGroup(t, env.ID)

	// Resolve first
	_, err := testStore.ResolveErrorGroup(context.Background(), ResolveErrorGroupParams{
		ID: eg.ID, ResolvedBy: &reg.User.ID, EnvID: env.ID,
	})
	require.NoError(t, err)

	// Reopen
	reopened, err := testStore.ReopenErrorGroup(context.Background(), ReopenErrorGroupParams{
		ID:    eg.ID,
		EnvID: env.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "open", reopened.Status)
	require.Nil(t, reopened.ResolvedBy)
	require.Nil(t, reopened.ResolvedAt)
}

// ═══════════════════════════════════════════════════════════════.
//  Full lifecycle: open → acknowledged → resolved → reopened
// ═══════════════════════════════════════════════════════════════.

func TestErrorGroupLifecycle(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	eg := createTestErrorGroup(t, env.ID)
	require.Equal(t, "open", eg.Status)

	// Acknowledge
	acked, err := testStore.AcknowledgeErrorGroup(context.Background(), AcknowledgeErrorGroupParams{
		ID: eg.ID, EnvID: env.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "acknowledged", acked.Status)

	// Resolve
	resolved, err := testStore.ResolveErrorGroup(context.Background(), ResolveErrorGroupParams{
		ID: eg.ID, ResolvedBy: &reg.User.ID, EnvID: env.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "resolved", resolved.Status)
	require.NotNil(t, resolved.ResolvedAt)

	// Reopen
	reopened, err := testStore.ReopenErrorGroup(context.Background(), ReopenErrorGroupParams{
		ID: eg.ID, EnvID: env.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "open", reopened.Status)
	require.Nil(t, reopened.ResolvedBy)
	require.Nil(t, reopened.ResolvedAt)
}

// ═══════════════════════════════════════════════════════════════.
//  AppendAffectedUsers
// ═══════════════════════════════════════════════════════════════.

func TestAppendAffectedUsers_MergesAndDeduplicates(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	eg := createTestErrorGroup(t, env.ID) // starts with [1, 2, 3]

	err := testStore.AppendAffectedUsers(context.Background(), AppendAffectedUsersParams{
		ID:      eg.ID,
		UserIds: []int32{3, 4, 5}, // 3 is duplicate
	})
	require.NoError(t, err)

	fetched, err := testStore.GetErrorGroupByID(context.Background(), GetErrorGroupByIDParams{
		ID: eg.ID, EnvID: env.ID,
	})
	require.NoError(t, err)

	// Must contain {1, 2, 3, 4, 5} — deduplicated
	require.Len(t, fetched.AffectedUsers, 5)
	userSet := map[int32]bool{}
	for _, u := range fetched.AffectedUsers {
		userSet[u] = true
	}
	for _, expected := range []int32{1, 2, 3, 4, 5} {
		require.True(t, userSet[expected], "expected user %d in affected_users", expected)
	}
}

// ═══════════════════════════════════════════════════════════════.
//  DeleteOldErrorGroups
// ═══════════════════════════════════════════════════════════════.

func TestDeleteOldErrorGroups_OnlyDeletesNonOpen(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")

	openEG := createTestErrorGroup(t, env.ID)     // status = open
	resolvedEG := createTestErrorGroup(t, env.ID) // will be resolved

	_, err := testStore.ResolveErrorGroup(context.Background(), ResolveErrorGroupParams{
		ID: resolvedEG.ID, ResolvedBy: &reg.User.ID, EnvID: env.ID,
	})
	require.NoError(t, err)

	// Delete with future cutoff
	result, err := testStore.DeleteOldErrorGroups(context.Background(), DeleteOldErrorGroupsParams{
		TenantID: reg.Tenant.ID,
		LastSeen: time.Now().Add(1 * time.Hour),
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, result.RowsAffected(), int64(1))

	// Open group must survive
	_, err = testStore.GetErrorGroupByID(context.Background(), GetErrorGroupByIDParams{
		ID: openEG.ID, EnvID: env.ID,
	})
	require.NoError(t, err)

	// Resolved group must be deleted
	_, err = testStore.GetErrorGroupByID(context.Background(), GetErrorGroupByIDParams{
		ID: resolvedEG.ID, EnvID: env.ID,
	})
	require.Error(t, err, "resolved group must be deleted")
}
func TestDeleteOldErrorGroups_TenantIsolation(t *testing.T) {
	reg1 := createRegisteredTenant(t)
	reg2 := createRegisteredTenant(t)
	env1 := createTestEnvironment(t, reg1.Tenant.ID, "production")
	env2 := createTestEnvironment(t, reg2.Tenant.ID, "production")

	eg1 := createTestErrorGroup(t, env1.ID)
	eg2 := createTestErrorGroup(t, env2.ID)

	// Resolve both so they're eligible for deletion
	_, err := testStore.ResolveErrorGroup(context.Background(), ResolveErrorGroupParams{
		ID: eg1.ID, ResolvedBy: &reg1.User.ID, EnvID: env1.ID,
	})
	require.NoError(t, err)
	_, err = testStore.ResolveErrorGroup(context.Background(), ResolveErrorGroupParams{
		ID: eg2.ID, ResolvedBy: &reg2.User.ID, EnvID: env2.ID,
	})
	require.NoError(t, err)

	// Delete only tenant 1's
	_, err = testStore.DeleteOldErrorGroups(context.Background(), DeleteOldErrorGroupsParams{
		TenantID: reg1.Tenant.ID,
		LastSeen: time.Now().Add(1 * time.Hour),
	})
	require.NoError(t, err)

	// Tenant 2's group must survive
	_, err = testStore.GetErrorGroupByID(context.Background(), GetErrorGroupByIDParams{
		ID: eg2.ID, EnvID: env2.ID,
	})
	require.NoError(t, err)
}
func TestDeleteOldErrorGroups_CutoffInPast_DeletesNothing(t *testing.T) {
	reg := createRegisteredTenant(t)
	env := createTestEnvironment(t, reg.Tenant.ID, "production")
	eg := createTestErrorGroup(t, env.ID)

	// Resolve so it's eligible
	_, err := testStore.ResolveErrorGroup(context.Background(), ResolveErrorGroupParams{
		ID: eg.ID, ResolvedBy: &reg.User.ID, EnvID: env.ID,
	})
	require.NoError(t, err)

	// Cutoff far in the past → nothing qualifies
	result, err := testStore.DeleteOldErrorGroups(context.Background(), DeleteOldErrorGroupsParams{
		TenantID: reg.Tenant.ID,
		LastSeen: time.Now().Add(-24 * time.Hour),
	})
	require.NoError(t, err)
	require.Equal(t, int64(0), result.RowsAffected())
}
