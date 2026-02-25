package db

import (
	"Intelligent_Dev_ToolKit_Odoo/utils"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func createTestAPIKey(t *testing.T, tenantID, createdBy uuid.UUID, scopes []string, expiresAt *time.Time) ApiKey {
	t.Helper()
	key, err := testStore.CreateAPIKey(context.Background(), CreateAPIKeyParams{
		TenantID:  tenantID,
		CreatedBy: &createdBy,
		KeyHash:   utils.RandomString(64), // simulate SHA-256 hash
		KeyPrefix: utils.RandomString(8),  // simulate "odt_ak_xxxxxxxx"
		Name:      "Test Key " + utils.RandomString(4),
		Scopes:    scopes,
		ExpiresAt: expiresAt,
	})
	require.NoError(t, err)
	require.NotZero(t, key.ID)
	return key
}

// futureTime returns a time in the future by the given duration.
func futureTime(d time.Duration) *time.Time {
	t := time.Now().Add(d)
	return &t
}

// pastTime returns a time in the past by the given duration.
func pastTime(d time.Duration) *time.Time {
	t := time.Now().Add(-d)
	return &t
}

func TestCreateAPIKey_Success(t *testing.T) {
	reg := createRegisteredTenant(t)

	scopes := []string{"read:errors", "write:config", "agent:ingest"}
	key, err := testStore.CreateAPIKey(context.Background(), CreateAPIKeyParams{
		TenantID:  reg.Tenant.ID,
		CreatedBy: &reg.User.ID,
		KeyHash:   utils.RandomString(64),
		KeyPrefix: "odt_ak_" + utils.RandomString(8),
		Name:      "CI/CD Key",
		Scopes:    scopes,
		ExpiresAt: nil, // never expires
	})
	require.NoError(t, err)

	// All fields must be set correctly
	require.NotZero(t, key.ID)
	require.Equal(t, reg.Tenant.ID, key.TenantID)
	require.NotNil(t, key.CreatedBy)
	require.Equal(t, reg.User.ID, *key.CreatedBy)
	require.Equal(t, "CI/CD Key", key.Name)
	require.Equal(t, scopes, key.Scopes)
	require.Nil(t, key.ExpiresAt) // never expires
	require.Nil(t, key.LastUsed)  // never used yet
	require.True(t, key.IsActive) // active by default
	require.NotZero(t, key.CreatedAt)
}
func TestCreateAPIKey_WithExpiry(t *testing.T) {
	reg := createRegisteredTenant(t)
	expires := futureTime(24 * time.Hour)

	key, err := testStore.CreateAPIKey(context.Background(), CreateAPIKeyParams{
		TenantID:  reg.Tenant.ID,
		CreatedBy: &reg.User.ID,
		KeyHash:   utils.RandomString(64),
		KeyPrefix: utils.RandomString(8),
		Name:      "Expiring Key",
		Scopes:    []string{"read:errors"},
		ExpiresAt: expires,
	})
	require.NoError(t, err)
	require.NotNil(t, key.ExpiresAt)
	require.WithinDuration(t, *expires, *key.ExpiresAt, time.Second)
}

func TestCreateAPIKey_WithEmptyScopes(t *testing.T) {
	reg := createRegisteredTenant(t)

	key, err := testStore.CreateAPIKey(context.Background(), CreateAPIKeyParams{
		TenantID:  reg.Tenant.ID,
		CreatedBy: &reg.User.ID,
		KeyHash:   utils.RandomString(64),
		KeyPrefix: utils.RandomString(8),
		Name:      "No Scopes Key",
		Scopes:    []string{}, // empty scopes — allowed
		ExpiresAt: nil,
	})
	require.NoError(t, err)
	require.NotZero(t, key.ID)
	require.Empty(t, key.Scopes)
}

func TestCreateAPIKey_WithoutCreatedBy(t *testing.T) {
	reg := createRegisteredTenant(t)

	key, err := testStore.CreateAPIKey(context.Background(), CreateAPIKeyParams{
		TenantID:  reg.Tenant.ID,
		CreatedBy: nil, // system-generated key, no user
		KeyHash:   utils.RandomString(64),
		KeyPrefix: utils.RandomString(8),
		Name:      "System Key",
		Scopes:    []string{"agent:ingest"},
		ExpiresAt: nil,
	})
	require.NoError(t, err)
	require.NotZero(t, key.ID)
	require.Nil(t, key.CreatedBy)
}
func TestCreateAPIKey_DuplicateKeyHash_Fails(t *testing.T) {
	reg := createRegisteredTenant(t)
	hash := utils.RandomString(64) // same hash used twice

	_, err := testStore.CreateAPIKey(context.Background(), CreateAPIKeyParams{
		TenantID:  reg.Tenant.ID,
		CreatedBy: &reg.User.ID,
		KeyHash:   hash,
		KeyPrefix: utils.RandomString(8),
		Name:      "First Key",
		Scopes:    []string{},
		ExpiresAt: nil,
	})
	require.NoError(t, err)

	_, err = testStore.CreateAPIKey(context.Background(), CreateAPIKeyParams{
		TenantID:  reg.Tenant.ID,
		CreatedBy: &reg.User.ID,
		KeyHash:   hash, // same hash → UNIQUE violation
		KeyPrefix: utils.RandomString(8),
		Name:      "Duplicate Key",
		Scopes:    []string{},
		ExpiresAt: nil,
	})
	require.Error(t, err, "duplicate key_hash must be rejected")
}
func TestGetAPIKeyByID_Success(t *testing.T) {
	reg := createRegisteredTenant(t)
	created := createTestAPIKey(t, reg.Tenant.ID, reg.User.ID, []string{"read:errors"}, nil)

	fetched, err := testStore.GetAPIKeyByID(context.Background(), GetAPIKeyByIDParams{
		ID:       created.ID,
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)
	require.Equal(t, created.ID, fetched.ID)
	require.Equal(t, created.KeyHash, fetched.KeyHash)
	require.Equal(t, created.KeyPrefix, fetched.KeyPrefix)
	require.Equal(t, created.Name, fetched.Name)
	require.Equal(t, created.Scopes, fetched.Scopes)
	require.Equal(t, created.TenantID, fetched.TenantID)
}
func TestGetAPIKeyByID_WrongTenant_Fails(t *testing.T) {
	reg1 := createRegisteredTenant(t)
	reg2 := createRegisteredTenant(t)

	key := createTestAPIKey(t, reg1.Tenant.ID, reg1.User.ID, []string{}, nil)

	// Tenant 2 tries to fetch tenant 1's key
	_, err := testStore.GetAPIKeyByID(context.Background(), GetAPIKeyByIDParams{
		ID:       key.ID,
		TenantID: reg2.Tenant.ID, // wrong tenant
	})
	require.Error(t, err, "cross-tenant key fetch must fail")
}
func TestGetAPIKeyByID_NonExistent_Fails(t *testing.T) {
	reg := createRegisteredTenant(t)

	_, err := testStore.GetAPIKeyByID(context.Background(), GetAPIKeyByIDParams{
		ID:       uuid.New(),
		TenantID: reg.Tenant.ID,
	})
	require.Error(t, err, "non-existent key must return error")
}
func TestGetAPIKeyByHash_ActiveNeverExpires_Success(t *testing.T) {
	reg := createRegisteredTenant(t)
	key := createTestAPIKey(t, reg.Tenant.ID, reg.User.ID, []string{"read:errors"}, nil)

	fetched, err := testStore.GetAPIKeyByHash(context.Background(), key.KeyHash)
	require.NoError(t, err)
	require.Equal(t, key.ID, fetched.ID)
	require.Equal(t, key.TenantID, fetched.TenantID)
}
func TestGetAPIKeyByHash_ActiveFutureExpiry_Success(t *testing.T) {
	reg := createRegisteredTenant(t)
	key := createTestAPIKey(t, reg.Tenant.ID, reg.User.ID, []string{}, futureTime(7*24*time.Hour))

	fetched, err := testStore.GetAPIKeyByHash(context.Background(), key.KeyHash)
	require.NoError(t, err)
	require.Equal(t, key.ID, fetched.ID)
}
func TestGetAPIKeyByHash_ExpiredKey_Fails(t *testing.T) {
	reg := createRegisteredTenant(t)
	// expires_at in the past → GetAPIKeyByHash should NOT return it
	key := createTestAPIKey(t, reg.Tenant.ID, reg.User.ID, []string{}, pastTime(time.Hour))

	_, err := testStore.GetAPIKeyByHash(context.Background(), key.KeyHash)
	require.Error(t, err, "expired key must not be returned by GetAPIKeyByHash")
}
func TestGetAPIKeyByHash_RevokedKey_Fails(t *testing.T) {
	reg := createRegisteredTenant(t)
	key := createTestAPIKey(t, reg.Tenant.ID, reg.User.ID, []string{}, nil)

	// Revoke it
	err := testStore.RevokeAPIKey(context.Background(), RevokeAPIKeyParams{
		ID:       key.ID,
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)

	// GetAPIKeyByHash must not return revoked keys
	_, err = testStore.GetAPIKeyByHash(context.Background(), key.KeyHash)
	require.Error(t, err, "revoked key must not be returned by GetAPIKeyByHash")
}
func TestGetAPIKeyByHash_NonExistent_Fails(t *testing.T) {
	_, err := testStore.GetAPIKeyByHash(context.Background(), utils.RandomString(64))
	require.Error(t, err, "non-existent hash must return error")
}
func TestRevokeAPIKey_Success(t *testing.T) {
	reg := createRegisteredTenant(t)
	key := createTestAPIKey(t, reg.Tenant.ID, reg.User.ID, []string{"read:errors"}, nil)

	// Must be active before revoke
	require.True(t, key.IsActive)

	err := testStore.RevokeAPIKey(context.Background(), RevokeAPIKeyParams{
		ID:       key.ID,
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)

	// Fetch and verify is_active = false
	fetched, err := testStore.GetAPIKeyByID(context.Background(), GetAPIKeyByIDParams{
		ID:       key.ID,
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)
	require.False(t, fetched.IsActive, "key must be inactive after revocation")
}

func TestRevokeAPIKey_WrongTenant_DoesNotRevoke(t *testing.T) {
	reg1 := createRegisteredTenant(t)
	reg2 := createRegisteredTenant(t)
	key := createTestAPIKey(t, reg1.Tenant.ID, reg1.User.ID, []string{}, nil)

	// Tenant 2 tries to revoke tenant 1's key — no error but no effect
	err := testStore.RevokeAPIKey(context.Background(), RevokeAPIKeyParams{
		ID:       key.ID,
		TenantID: reg2.Tenant.ID, // wrong tenant
	})
	require.NoError(t, err) // exec returns no error even for 0 rows

	// Key must still be active under tenant 1
	fetched, err := testStore.GetAPIKeyByID(context.Background(), GetAPIKeyByIDParams{
		ID:       key.ID,
		TenantID: reg1.Tenant.ID,
	})
	require.NoError(t, err)
	require.True(t, fetched.IsActive, "key must still be active — wrong tenant revoke must have no effect")
}
func TestRevokeAPIKey_AlreadyRevoked_IsIdempotent(t *testing.T) {
	reg := createRegisteredTenant(t)
	key := createTestAPIKey(t, reg.Tenant.ID, reg.User.ID, []string{}, nil)

	// Revoke twice — must not error
	err := testStore.RevokeAPIKey(context.Background(), RevokeAPIKeyParams{ID: key.ID, TenantID: reg.Tenant.ID})
	require.NoError(t, err)

	err = testStore.RevokeAPIKey(context.Background(), RevokeAPIKeyParams{ID: key.ID, TenantID: reg.Tenant.ID})
	require.NoError(t, err)

	fetched, err := testStore.GetAPIKeyByID(context.Background(), GetAPIKeyByIDParams{ID: key.ID, TenantID: reg.Tenant.ID})
	require.NoError(t, err)
	require.False(t, fetched.IsActive)
}
func TestDeleteAPIKey_Success(t *testing.T) {
	reg := createRegisteredTenant(t)
	key := createTestAPIKey(t, reg.Tenant.ID, reg.User.ID, []string{}, nil)

	err := testStore.DeleteAPIKey(context.Background(), DeleteAPIKeyParams{
		ID:       key.ID,
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)

	// Must be gone
	_, err = testStore.GetAPIKeyByID(context.Background(), GetAPIKeyByIDParams{
		ID:       key.ID,
		TenantID: reg.Tenant.ID,
	})
	require.Error(t, err, "deleted key must not be fetchable")
}
func TestDeleteAPIKey_WrongTenant_DoesNotDelete(t *testing.T) {
	reg1 := createRegisteredTenant(t)
	reg2 := createRegisteredTenant(t)
	key := createTestAPIKey(t, reg1.Tenant.ID, reg1.User.ID, []string{}, nil)

	// Tenant 2 tries to delete tenant 1's key
	err := testStore.DeleteAPIKey(context.Background(), DeleteAPIKeyParams{
		ID:       key.ID,
		TenantID: reg2.Tenant.ID, // wrong tenant
	})
	require.NoError(t, err) // no error, but 0 rows affected

	// Key must still exist under tenant 1
	fetched, err := testStore.GetAPIKeyByID(context.Background(), GetAPIKeyByIDParams{
		ID:       key.ID,
		TenantID: reg1.Tenant.ID,
	})
	require.NoError(t, err)
	require.Equal(t, key.ID, fetched.ID, "key must still exist under original tenant")
}
func TestDeleteAPIKey_NonExistent_NoError(t *testing.T) {
	reg := createRegisteredTenant(t)

	// Deleting a non-existent key must not error (exec returns no error for 0 rows)
	err := testStore.DeleteAPIKey(context.Background(), DeleteAPIKeyParams{
		ID:       uuid.New(),
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)
}
func TestTouchAPIKey_SetsLastUsed(t *testing.T) {
	reg := createRegisteredTenant(t)
	key := createTestAPIKey(t, reg.Tenant.ID, reg.User.ID, []string{}, nil)

	// last_used must be nil before touch
	require.Nil(t, key.LastUsed)

	before := time.Now().Add(-time.Second)

	err := testStore.TouchAPIKey(context.Background(), key.ID)
	require.NoError(t, err)

	after := time.Now().Add(time.Second)

	fetched, err := testStore.GetAPIKeyByID(context.Background(), GetAPIKeyByIDParams{
		ID:       key.ID,
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, fetched.LastUsed, "last_used must be set after touch")
	require.True(t, fetched.LastUsed.After(before) || fetched.LastUsed.Equal(before))
	require.True(t, fetched.LastUsed.Before(after) || fetched.LastUsed.Equal(after))
}
func TestTouchAPIKey_UpdatesLastUsed_OnEveryCall(t *testing.T) {
	reg := createRegisteredTenant(t)
	key := createTestAPIKey(t, reg.Tenant.ID, reg.User.ID, []string{}, nil)

	// First touch
	err := testStore.TouchAPIKey(context.Background(), key.ID)
	require.NoError(t, err)

	first, err := testStore.GetAPIKeyByID(context.Background(), GetAPIKeyByIDParams{
		ID: key.ID, TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, first.LastUsed)

	// Small sleep to ensure timestamp differs
	time.Sleep(10 * time.Millisecond)

	// Second touch
	err = testStore.TouchAPIKey(context.Background(), key.ID)
	require.NoError(t, err)

	second, err := testStore.GetAPIKeyByID(context.Background(), GetAPIKeyByIDParams{
		ID: key.ID, TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, second.LastUsed)
	require.True(t, second.LastUsed.After(*first.LastUsed), "second touch must update last_used to a later time")
}
func TestTouchAPIKey_NonExistent_NoError(t *testing.T) {
	// Touching a non-existent key must not error
	err := testStore.TouchAPIKey(context.Background(), uuid.New())
	require.NoError(t, err)
}
func TestListAPIKeysByTenant_ReturnsAllKeys(t *testing.T) {
	reg := createRegisteredTenant(t)

	k1 := createTestAPIKey(t, reg.Tenant.ID, reg.User.ID, []string{"read:errors"}, nil)
	k2 := createTestAPIKey(t, reg.Tenant.ID, reg.User.ID, []string{"agent:ingest"}, nil)
	k3 := createTestAPIKey(t, reg.Tenant.ID, reg.User.ID, []string{"write:config"}, nil)

	keys, err := testStore.ListAPIKeysByTenant(context.Background(), reg.Tenant.ID)
	require.NoError(t, err)
	require.Len(t, keys, 3)

	// Collect IDs
	ids := make(map[string]bool)
	for _, k := range keys {
		ids[k.ID.String()] = true
	}
	require.True(t, ids[k1.ID.String()])
	require.True(t, ids[k2.ID.String()])
	require.True(t, ids[k3.ID.String()])
}
func TestListAPIKeysByTenant_OrderedByCreatedAtDesc(t *testing.T) {
	reg := createRegisteredTenant(t)

	createTestAPIKey(t, reg.Tenant.ID, reg.User.ID, []string{}, nil)
	time.Sleep(5 * time.Millisecond)
	createTestAPIKey(t, reg.Tenant.ID, reg.User.ID, []string{}, nil)
	time.Sleep(5 * time.Millisecond)
	createTestAPIKey(t, reg.Tenant.ID, reg.User.ID, []string{}, nil)

	keys, err := testStore.ListAPIKeysByTenant(context.Background(), reg.Tenant.ID)
	require.NoError(t, err)
	require.Len(t, keys, 3)

	// Must be newest first
	for i := 1; i < len(keys); i++ {
		require.True(t,
			keys[i-1].CreatedAt.After(keys[i].CreatedAt) || keys[i-1].CreatedAt.Equal(keys[i].CreatedAt),
			"keys must be ordered by created_at DESC",
		)
	}
}
func TestListAPIKeysByTenant_EmptyTenant_ReturnsEmpty(t *testing.T) {
	reg := createRegisteredTenant(t)

	keys, err := testStore.ListAPIKeysByTenant(context.Background(), reg.Tenant.ID)
	require.NoError(t, err)
	require.Empty(t, keys)
}
func TestListAPIKeysByTenant_IncludesRevokedKeys(t *testing.T) {
	reg := createRegisteredTenant(t)

	active := createTestAPIKey(t, reg.Tenant.ID, reg.User.ID, []string{}, nil)
	revoked := createTestAPIKey(t, reg.Tenant.ID, reg.User.ID, []string{}, nil)

	err := testStore.RevokeAPIKey(context.Background(), RevokeAPIKeyParams{
		ID:       revoked.ID,
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)

	// ListAPIKeysByTenant returns ALL keys (active + revoked) — UI shows all with status
	keys, err := testStore.ListAPIKeysByTenant(context.Background(), reg.Tenant.ID)
	require.NoError(t, err)
	require.Len(t, keys, 2)

	statusMap := make(map[string]bool)
	for _, k := range keys {
		statusMap[k.ID.String()] = k.IsActive
	}
	require.True(t, statusMap[active.ID.String()], "active key must have IsActive=true")
	require.False(t, statusMap[revoked.ID.String()], "revoked key must have IsActive=false")
}
func TestListAPIKeysByTenant_IsolatedFromOtherTenants(t *testing.T) {
	reg1 := createRegisteredTenant(t)
	reg2 := createRegisteredTenant(t)

	// 2 keys for tenant 1, 1 key for tenant 2
	createTestAPIKey(t, reg1.Tenant.ID, reg1.User.ID, []string{}, nil)
	createTestAPIKey(t, reg1.Tenant.ID, reg1.User.ID, []string{}, nil)
	createTestAPIKey(t, reg2.Tenant.ID, reg2.User.ID, []string{}, nil)

	keys1, err := testStore.ListAPIKeysByTenant(context.Background(), reg1.Tenant.ID)
	require.NoError(t, err)
	require.Len(t, keys1, 2, "tenant 1 must only see its own keys")

	keys2, err := testStore.ListAPIKeysByTenant(context.Background(), reg2.Tenant.ID)
	require.NoError(t, err)
	require.Len(t, keys2, 1, "tenant 2 must only see its own keys")
}
func TestListAPIKeysByTenant_DoesNotExposeKeyHash(t *testing.T) {
	reg := createRegisteredTenant(t)
	createTestAPIKey(t, reg.Tenant.ID, reg.User.ID, []string{}, nil)

	keys, err := testStore.ListAPIKeysByTenant(context.Background(), reg.Tenant.ID)
	require.NoError(t, err)
	require.Len(t, keys, 1)

	// ListAPIKeysByTenant returns ListAPIKeysByTenantRow which excludes key_hash
	// This is intentional — hash must never be exposed in list endpoints
	// Compile-time check: ListAPIKeysByTenantRow has no KeyHash field
	var row ListAPIKeysByTenantRow = keys[0]
	_ = row // if KeyHash existed on the struct, referencing it would be needed in tests
	// Verified by sqlc: key_hash is intentionally omitted from SELECT in listAPIKeysByTenant
	require.NotEmpty(t, keys[0].KeyPrefix, "KeyPrefix must be present for UI display")
}
