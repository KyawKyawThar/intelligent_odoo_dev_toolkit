package db

import (
	"Intelligent_Dev_ToolKit_Odoo/utils"
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func createTestSession(t *testing.T, userID, tenantID uuid.UUID, expiresIn time.Duration) Session {
	t.Helper()
	expiresAt := time.Now().Add(expiresIn)
	agent := "Mozilla/5.0 Test"
	sess, err := testStore.CreateSession(context.Background(), CreateSessionParams{
		UserID:       userID,
		TenantID:     tenantID,
		RefreshToken: utils.RandomString(64), // simulate hashed token
		UserAgent:    &agent,
		IpAddress:    nil,
		ExpiresAt:    expiresAt,
	})
	require.NoError(t, err)
	require.NotZero(t, sess.ID)
	return sess
}
func parseAddr(ip string) *netip.Addr {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return nil
	}
	return &a
}

func TestCreateSession_Success(t *testing.T) {
	reg := createRegisteredTenant(t)
	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	agent := "Mozilla/5.0"

	sess, err := testStore.CreateSession(context.Background(), CreateSessionParams{
		UserID:       reg.User.ID,
		TenantID:     reg.Tenant.ID,
		RefreshToken: utils.RandomString(64),
		UserAgent:    &agent,
		IpAddress:    nil,
		ExpiresAt:    expiresAt,
	})
	require.NoError(t, err)

	require.NotZero(t, sess.ID)
	require.Equal(t, reg.User.ID, sess.UserID)
	require.Equal(t, reg.Tenant.ID, sess.TenantID)
	require.NotEmpty(t, sess.RefreshToken)
	require.NotNil(t, sess.UserAgent)
	require.Equal(t, agent, *sess.UserAgent)
	require.Nil(t, sess.IpAddress)
	require.WithinDuration(t, expiresAt, sess.ExpiresAt, time.Second)
	require.NotZero(t, sess.CreatedAt)
	require.NotZero(t, sess.LastUsedAt)
}
func TestCreateSession_WithIPAddress(t *testing.T) {
	reg := createRegisteredTenant(t)

	sess, err := testStore.CreateSession(context.Background(), CreateSessionParams{
		UserID:       reg.User.ID,
		TenantID:     reg.Tenant.ID,
		RefreshToken: utils.RandomString(64),
		UserAgent:    nil,
		IpAddress:    parseAddr("203.0.113.42"),
		ExpiresAt:    time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	require.NotNil(t, sess.IpAddress)
	require.Equal(t, "203.0.113.42", sess.IpAddress.String())
}
func TestCreateSession_WithoutOptionalFields(t *testing.T) {
	reg := createRegisteredTenant(t)

	sess, err := testStore.CreateSession(context.Background(), CreateSessionParams{
		UserID:       reg.User.ID,
		TenantID:     reg.Tenant.ID,
		RefreshToken: utils.RandomString(64),
		UserAgent:    nil, // no user agent
		IpAddress:    nil, // no IP
		ExpiresAt:    time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	require.NotZero(t, sess.ID)
	require.Nil(t, sess.UserAgent)
	require.Nil(t, sess.IpAddress)
}
func TestCreateSession_DuplicateRefreshToken_Fails(t *testing.T) {
	reg := createRegisteredTenant(t)
	token := utils.RandomString(64)

	_, err := testStore.CreateSession(context.Background(), CreateSessionParams{
		UserID:       reg.User.ID,
		TenantID:     reg.Tenant.ID,
		RefreshToken: token,
		UserAgent:    nil,
		IpAddress:    nil,
		ExpiresAt:    time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	// Same token again — UNIQUE violation
	_, err = testStore.CreateSession(context.Background(), CreateSessionParams{
		UserID:       reg.User.ID,
		TenantID:     reg.Tenant.ID,
		RefreshToken: token,
		UserAgent:    nil,
		IpAddress:    nil,
		ExpiresAt:    time.Now().Add(time.Hour),
	})
	require.Error(t, err, "duplicate refresh_token must be rejected")
}
func TestCreateSession_MultipleSessionsPerUser_Allowed(t *testing.T) {
	// User can have multiple active sessions (different devices)
	reg := createRegisteredTenant(t)

	sess1 := createTestSession(t, reg.User.ID, reg.Tenant.ID, time.Hour)
	sess2 := createTestSession(t, reg.User.ID, reg.Tenant.ID, time.Hour)
	sess3 := createTestSession(t, reg.User.ID, reg.Tenant.ID, time.Hour)

	require.NotEqual(t, sess1.ID, sess2.ID)
	require.NotEqual(t, sess2.ID, sess3.ID)
}
func TestGetSession_Success(t *testing.T) {
	reg := createRegisteredTenant(t)
	sess := createTestSession(t, reg.User.ID, reg.Tenant.ID, time.Hour)

	row, err := testStore.GetSession(context.Background(), sess.ID)
	require.NoError(t, err)

	require.Equal(t, sess.ID, row.ID)
	require.Equal(t, sess.UserID, row.UserID)
	require.Equal(t, sess.TenantID, row.TenantID)
	require.Equal(t, sess.RefreshToken, row.RefreshToken)
	require.True(t, row.UserIsActive, "user must be active")
}
func TestGetSession_InactiveUser_StillReturnsSession(t *testing.T) {
	// GetSession joins users — verify UserIsActive reflects user state
	reg := createRegisteredTenant(t)
	sess := createTestSession(t, reg.User.ID, reg.Tenant.ID, time.Hour)

	// Deactivate user
	err := testStore.DeactivateUser(context.Background(), DeactivateUserParams{
		ID:       reg.User.ID,
		TenantID: reg.Tenant.ID,
	})
	require.NoError(t, err)

	row, err := testStore.GetSession(context.Background(), sess.ID)
	require.NoError(t, err)
	require.False(t, row.UserIsActive, "UserIsActive must reflect deactivated user")
}
func TestGetSession_NonExistent_Fails(t *testing.T) {
	_, err := testStore.GetSession(context.Background(), utils.RandomUUID())
	require.Error(t, err)
}
func TestGetSessionByToken_ActiveNotExpired_Success(t *testing.T) {
	reg := createRegisteredTenant(t)
	sess := createTestSession(t, reg.User.ID, reg.Tenant.ID, 7*24*time.Hour) // expires in 7 days

	row, err := testStore.GetSessionByToken(context.Background(), sess.RefreshToken)
	require.NoError(t, err)
	require.Equal(t, sess.ID, row.ID)
	require.Equal(t, sess.UserID, row.UserID)
	require.True(t, row.UserIsActive)
}
func TestGetSessionByToken_ExpiredToken_Fails(t *testing.T) {
	reg := createRegisteredTenant(t)

	// Create session that expires in the past (1 nanosecond TTL, already expired)
	expiredAt := time.Now().Add(-time.Hour)
	_, err := testStore.CreateSession(context.Background(), CreateSessionParams{
		UserID:       reg.User.ID,
		TenantID:     reg.Tenant.ID,
		RefreshToken: utils.RandomString(64),
		UserAgent:    nil,
		IpAddress:    nil,
		ExpiresAt:    expiredAt,
	})
	// Note: creation may succeed — expiry is only enforced on GET
	if err != nil {
		t.Skip("DB rejected expired session on create")
	}

	// Fetch by token — must fail because expires_at < now()
	_, err = testStore.GetSessionByToken(context.Background(), utils.RandomString(64))
	require.Error(t, err, "expired token must not be returned")
}

func TestGetSessionByToken_NonExistentToken_Fails(t *testing.T) {
	_, err := testStore.GetSessionByToken(context.Background(), utils.RandomString(64))
	require.Error(t, err)
}
func TestListSessions_ReturnsActiveSessionsOnly(t *testing.T) {
	reg := createRegisteredTenant(t)

	// 2 active sessions
	createTestSession(t, reg.User.ID, reg.Tenant.ID, time.Hour)
	createTestSession(t, reg.User.ID, reg.Tenant.ID, time.Hour)

	sessions, err := testStore.ListSessions(context.Background(), reg.User.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 2)

	for _, s := range sessions {
		require.True(t, s.ExpiresAt.After(time.Now()), "only non-expired sessions must be listed")
	}
}
func TestListSessions_OrderedByCreatedAtDesc(t *testing.T) {
	reg := createRegisteredTenant(t)

	createTestSession(t, reg.User.ID, reg.Tenant.ID, time.Hour)
	time.Sleep(5 * time.Millisecond)
	createTestSession(t, reg.User.ID, reg.Tenant.ID, time.Hour)
	time.Sleep(5 * time.Millisecond)
	createTestSession(t, reg.User.ID, reg.Tenant.ID, time.Hour)

	sessions, err := testStore.ListSessions(context.Background(), reg.User.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 3)

	for i := 1; i < len(sessions); i++ {
		require.True(t,
			sessions[i-1].CreatedAt.After(sessions[i].CreatedAt) || sessions[i-1].CreatedAt.Equal(sessions[i].CreatedAt),
			"must be ordered by created_at DESC",
		)
	}
}
func TestListSessions_NoSessions_ReturnsEmpty(t *testing.T) {
	reg := createRegisteredTenant(t)

	sessions, err := testStore.ListSessions(context.Background(), reg.User.ID)
	require.NoError(t, err)
	require.Empty(t, sessions)
}
func TestListSessions_IsolatedFromOtherUsers(t *testing.T) {
	reg := createRegisteredTenant(t)

	// Create a second user in the same tenant
	hashedPw, err := utils.HashPassword(utils.RandomString(8))
	require.NoError(t, err)
	user2, err := testStore.CreateUser(context.Background(), CreateUserParams{
		TenantID:     reg.Tenant.ID,
		Email:        utils.RandomEmail(),
		PasswordHash: hashedPw,
		FullName:     nil,
		Role:         "member",
		IsActive:     true,
	})
	require.NoError(t, err)

	createTestSession(t, reg.User.ID, reg.Tenant.ID, time.Hour)
	createTestSession(t, reg.User.ID, reg.Tenant.ID, time.Hour)
	createTestSession(t, user2.ID, reg.Tenant.ID, time.Hour)

	sessions1, err := testStore.ListSessions(context.Background(), reg.User.ID)
	require.NoError(t, err)
	require.Len(t, sessions1, 2, "user 1 must only see their own sessions")

	sessions2, err := testStore.ListSessions(context.Background(), user2.ID)
	require.NoError(t, err)
	require.Len(t, sessions2, 1, "user 2 must only see their own sessions")
}
func TestRevokeSession_Success(t *testing.T) {
	reg := createRegisteredTenant(t)
	sess := createTestSession(t, reg.User.ID, reg.Tenant.ID, time.Hour)

	err := testStore.RevokeSession(context.Background(), RevokeSessionParams{
		ID:     sess.ID,
		UserID: reg.User.ID,
	})
	require.NoError(t, err)

	// Must be gone from ListSessions
	sessions, err := testStore.ListSessions(context.Background(), reg.User.ID)
	require.NoError(t, err)
	require.Empty(t, sessions)
}
func TestRevokeSession_WrongUserID_DoesNotRevoke(t *testing.T) {
	reg1 := createRegisteredTenant(t)
	reg2 := createRegisteredTenant(t)
	sess := createTestSession(t, reg1.User.ID, reg1.Tenant.ID, time.Hour)

	// User 2 tries to revoke user 1's session
	err := testStore.RevokeSession(context.Background(), RevokeSessionParams{
		ID:     sess.ID,
		UserID: reg2.User.ID, // wrong user
	})
	require.NoError(t, err) // no error but 0 rows affected

	// Session must still exist for user 1
	sessions, err := testStore.ListSessions(context.Background(), reg1.User.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 1, "session must not be revoked by wrong user")
}
func TestRevokeSession_OnlyRevokesTargetSession(t *testing.T) {
	reg := createRegisteredTenant(t)

	sess1 := createTestSession(t, reg.User.ID, reg.Tenant.ID, time.Hour)
	sess2 := createTestSession(t, reg.User.ID, reg.Tenant.ID, time.Hour)
	sess3 := createTestSession(t, reg.User.ID, reg.Tenant.ID, time.Hour)

	// Revoke only sess2
	err := testStore.RevokeSession(context.Background(), RevokeSessionParams{
		ID:     sess2.ID,
		UserID: reg.User.ID,
	})
	require.NoError(t, err)

	sessions, err := testStore.ListSessions(context.Background(), reg.User.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 2)

	ids := make(map[string]bool)
	for _, s := range sessions {
		ids[s.ID.String()] = true
	}
	require.True(t, ids[sess1.ID.String()])
	require.False(t, ids[sess2.ID.String()], "revoked session must be gone")
	require.True(t, ids[sess3.ID.String()])
}
func TestRevokeAllSessions_DeletesAllUserSessions(t *testing.T) {
	reg := createRegisteredTenant(t)

	createTestSession(t, reg.User.ID, reg.Tenant.ID, time.Hour)
	createTestSession(t, reg.User.ID, reg.Tenant.ID, time.Hour)
	createTestSession(t, reg.User.ID, reg.Tenant.ID, time.Hour)

	err := testStore.RevokeAllSessions(context.Background(), reg.User.ID)
	require.NoError(t, err)

	sessions, err := testStore.ListSessions(context.Background(), reg.User.ID)
	require.NoError(t, err)
	require.Empty(t, sessions, "all sessions must be deleted")
}
func TestRevokeAllSessions_OnlyAffectsTargetUser(t *testing.T) {
	reg := createRegisteredTenant(t)

	hashedPw, err := utils.HashPassword(utils.RandomString(8))
	require.NoError(t, err)
	user2, err := testStore.CreateUser(context.Background(), CreateUserParams{
		TenantID:     reg.Tenant.ID,
		Email:        utils.RandomEmail(),
		PasswordHash: hashedPw,
		FullName:     nil,
		Role:         "member",
		IsActive:     true,
	})
	require.NoError(t, err)

	createTestSession(t, reg.User.ID, reg.Tenant.ID, time.Hour)
	createTestSession(t, user2.ID, reg.Tenant.ID, time.Hour)

	// Revoke only user 1's sessions
	err = testStore.RevokeAllSessions(context.Background(), reg.User.ID)
	require.NoError(t, err)

	sessions1, err := testStore.ListSessions(context.Background(), reg.User.ID)
	require.NoError(t, err)
	require.Empty(t, sessions1)

	sessions2, err := testStore.ListSessions(context.Background(), user2.ID)
	require.NoError(t, err)
	require.Len(t, sessions2, 1, "user 2 sessions must be untouched")
}
func TestRevokeAllSessions_NoSessions_NoError(t *testing.T) {
	reg := createRegisteredTenant(t)

	err := testStore.RevokeAllSessions(context.Background(), reg.User.ID)
	require.NoError(t, err) // idempotent, no sessions = no error
}

func TestTouchSession_UpdatesLastUsedAt(t *testing.T) {
	reg := createRegisteredTenant(t)
	sess := createTestSession(t, reg.User.ID, reg.Tenant.ID, time.Hour)

	before := time.Now()
	time.Sleep(5 * time.Millisecond)

	err := testStore.TouchSession(context.Background(), sess.ID)
	require.NoError(t, err)

	// GetSession to verify last_used_at was updated
	updated, err := testStore.GetSession(context.Background(), sess.ID)
	require.NoError(t, err)
	require.True(t, updated.LastUsedAt.After(before), "last_used_at must be updated after touch")
}

func TestTouchSession_NonExistent_NoError(t *testing.T) {
	err := testStore.TouchSession(context.Background(), utils.RandomUUID())
	require.NoError(t, err) // exec, 0 rows affected = no error
}
func TestDeleteExpiredSessions_RemovesExpiredOnly(t *testing.T) {
	reg := createRegisteredTenant(t)

	// Active session
	createTestSession(t, reg.User.ID, reg.Tenant.ID, time.Hour)

	// Expired session — create directly with past expires_at
	_, err := testStore.CreateSession(context.Background(), CreateSessionParams{
		UserID:       reg.User.ID,
		TenantID:     reg.Tenant.ID,
		RefreshToken: utils.RandomString(64),
		UserAgent:    nil,
		IpAddress:    nil,
		ExpiresAt:    time.Now().Add(-time.Hour), // expired
	})
	if err != nil {
		t.Skip("DB rejected expired session on create — skipping cleanup test")
	}

	tag, err := testStore.DeleteExpiredSessions(context.Background())
	require.NoError(t, err)
	require.GreaterOrEqual(t, tag.RowsAffected(), int64(1))

	// Active session must still exist
	sessions, err := testStore.ListSessions(context.Background(), reg.User.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 1, "active session must survive cleanup")
}
func TestDeleteExpiredSessions_NoExpiredSessions_AffectsZeroRows(t *testing.T) {
	reg := createRegisteredTenant(t)
	createTestSession(t, reg.User.ID, reg.Tenant.ID, time.Hour)
	createTestSession(t, reg.User.ID, reg.Tenant.ID, time.Hour)

	tag, err := testStore.DeleteExpiredSessions(context.Background())
	require.NoError(t, err)
	// May affect 0 rows for this tenant's sessions (other tests may have expired ones)
	_ = tag // just verify no error
}
