package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// setupTestRedis creates a miniredis instance and returns a RedisClient for testing
func setupTestRedis(t *testing.T) (*RedisClient, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	rc := &RedisClient{
		client: client,
		prefix: "test:",
	}

	return rc, mr
}

// -----------------------------------------------------------------------------
// ParseRedisConfig tests
// -----------------------------------------------------------------------------
func TestParseRedisConfig_SimpleHostPort(t *testing.T) {
	cfg, err := ParseRedisConfig("localhost:6380")
	require.NoError(t, err)
	require.Equal(t, "localhost", cfg.Host)
	require.Equal(t, 6380, cfg.Port)
	// defaults
	require.Equal(t, "", cfg.Password)
	require.Equal(t, 0, cfg.DB)
}

func TestParseRedisConfig_URLWithPasswordAndDB(t *testing.T) {
	input := "redis://:mypassword@redis.local:6381/2"
	cfg, err := ParseRedisConfig(input)
	require.NoError(t, err)
	require.Equal(t, "redis.local", cfg.Host)
	require.Equal(t, 6381, cfg.Port)
	require.Equal(t, "mypassword", cfg.Password)
	require.Equal(t, 2, cfg.DB)
}

func TestParseRedisConfig_URLWithoutPortOrDB(t *testing.T) {
	cfg, err := ParseRedisConfig("redis://redis.local")
	require.NoError(t, err)
	require.Equal(t, "redis.local", cfg.Host)
	// default port is 6379
	require.Equal(t, 6379, cfg.Port)
}

// new tests
func TestNewRedisClientWithAddress(t *testing.T) {
	// use miniredis to verify the client can connect via address parsing
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	addr := mr.Addr()
	// supply a URI-like address (without scheme) to NewRedisClient
	client, err := NewRedisClient(RedisConfig{
		Address: addr,
		Prefix:  "test:",
	})
	require.NoError(t, err)
	require.NotNil(t, client)
	client.Close()
}

func TestNewRedisClientWithURLAddress(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	url := fmt.Sprintf("redis://:%s@%s/%d", "", mr.Addr(), 0)
	client, err := NewRedisClient(RedisConfig{Address: url})
	require.NoError(t, err)
	require.NotNil(t, client)
	client.Close()
}

func teardownTestRedis(rc *RedisClient, mr *miniredis.Miniredis) {
	rc.Close()
	mr.Close()
}
func TestRedisClient_SetAndGet(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()

	// Test Set and Get with struct
	type TestData struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	data := TestData{Name: "test", Value: 42}
	err := rc.Set(ctx, "mykey", data, time.Minute)
	require.NoError(t, err)

	var retrieved TestData
	err = rc.Get(ctx, "mykey", &retrieved)
	require.NoError(t, err)
	require.Equal(t, data.Name, retrieved.Name)
	require.Equal(t, data.Value, retrieved.Value)
}
func TestRedisClient_GetNotFound(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()

	var data string
	err := rc.Get(ctx, "nonexistent", &data)

	require.Error(t, err)
	require.ErrorIs(t, err, ErrKeyNotFound)
}
func TestRedisClient_SetStringAndGetString(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()

	err := rc.SetString(ctx, "strkey", "hello world", time.Minute)
	require.NoError(t, err)

	val, err := rc.GetString(ctx, "strkey")
	require.NoError(t, err)
	require.Equal(t, "hello world", val)
}
func TestRedisClient_Delete(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()

	// Set a key
	err := rc.SetString(ctx, "deletekey", "value", time.Minute)
	require.NoError(t, err)

	// Verify it exists
	exists, err := rc.Exists(ctx, "deletekey")
	require.NoError(t, err)
	require.True(t, exists)

	// Delete it
	err = rc.Delete(ctx, "deletekey")
	require.NoError(t, err)

	// Verify it's gone
	exists, err = rc.Exists(ctx, "deletekey")
	require.NoError(t, err)
	require.False(t, exists)
}
func TestRedisClient_Exists(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()

	// Key doesn't exist
	exists, err := rc.Exists(ctx, "nokey")
	require.NoError(t, err)
	require.False(t, exists)

	// Create key
	err = rc.SetString(ctx, "nokey", "value", time.Minute)
	require.NoError(t, err)

	// Now it exists
	exists, err = rc.Exists(ctx, "nokey")
	require.NoError(t, err)
	require.True(t, exists)
}

// =============================================================================
// Session Management Tests
// =============================================================================

func TestRedisClient_CreateAndGetSession(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()

	session := &Session{
		ID:           "session-123",
		UserID:       "user-456",
		TenantID:     "tenant-789",
		RefreshToken: "refresh-token-abc",
		UserAgent:    "Mozilla/5.0",
		IPAddress:    "192.168.1.1",
		CreatedAt:    time.Now().UTC(),
		ExpiresAt:    time.Now().UTC().Add(time.Hour),
		LastActiveAt: time.Now().UTC(),
	}

	// Create session
	err := rc.CreateSession(ctx, session)
	require.NoError(t, err)

	// Get session
	retrieved, err := rc.GetSession(ctx, "session-123")
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	require.Equal(t, session.ID, retrieved.ID)
	require.Equal(t, session.UserID, retrieved.UserID)
	require.Equal(t, session.TenantID, retrieved.TenantID)
	require.Equal(t, session.RefreshToken, retrieved.RefreshToken)
	require.Equal(t, session.UserAgent, retrieved.UserAgent)
	require.Equal(t, session.IPAddress, retrieved.IPAddress)
}
func TestRedisClient_GetSessionNotFound(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()

	session, err := rc.GetSession(ctx, "nonexistent-session")
	require.NoError(t, err) // Not found is not an error
	require.Nil(t, session)
}
func TestRedisClient_DeleteSession(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()

	session := &Session{
		ID:        "session-to-delete",
		UserID:    "user-123",
		TenantID:  "tenant-123",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}

	err := rc.CreateSession(ctx, session)
	require.NoError(t, err)

	// Verify session exists
	retrieved, err := rc.GetSession(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	// Delete session
	err = rc.DeleteSession(ctx, session.ID, session.UserID)
	require.NoError(t, err)

	// Verify session is gone
	retrieved, err = rc.GetSession(ctx, session.ID)
	require.NoError(t, err)
	require.Nil(t, retrieved)
}
func TestRedisClient_GetUserSessions(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()
	userID := "user-multi-session"

	// Create multiple sessions for the same user
	session1 := &Session{
		ID:        "session-1",
		UserID:    userID,
		TenantID:  "tenant-123",
		UserAgent: "Chrome",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	session2 := &Session{
		ID:        "session-2",
		UserID:    userID,
		TenantID:  "tenant-123",
		UserAgent: "Firefox",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}

	err := rc.CreateSession(ctx, session1)
	require.NoError(t, err)
	err = rc.CreateSession(ctx, session2)
	require.NoError(t, err)

	// Get all sessions for user
	sessions, err := rc.GetUserSessions(ctx, userID)
	require.NoError(t, err)
	require.Len(t, sessions, 2)
}
func TestRedisClient_DeleteAllUserSessions(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()
	userID := "user-logout-all"

	// Create multiple sessions
	for i := range 3 {
		session := &Session{
			ID:        fmt.Sprintf("session-%d", i),
			UserID:    userID,
			TenantID:  "tenant-123",
			ExpiresAt: time.Now().UTC().Add(time.Hour),
		}
		err := rc.CreateSession(ctx, session)
		require.NoError(t, err)
	}

	// Verify sessions exist
	sessions, err := rc.GetUserSessions(ctx, userID)
	require.NoError(t, err)
	require.Len(t, sessions, 3)

	// Delete all sessions
	err = rc.DeleteAllUserSessions(ctx, userID)
	require.NoError(t, err)

	// Verify all sessions are gone
	sessions, err = rc.GetUserSessions(ctx, userID)
	require.NoError(t, err)
	require.Len(t, sessions, 0)
}
func TestRedisClient_RevokeSession(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()

	session := &Session{
		ID:        "session-revoke",
		UserID:    "user-revoke",
		TenantID:  "tenant-123",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}

	err := rc.CreateSession(ctx, session)
	require.NoError(t, err)

	// RevokeSession is alias for DeleteSession with swapped params
	err = rc.RevokeSession(ctx, session.UserID, session.ID)
	require.NoError(t, err)

	retrieved, err := rc.GetSession(ctx, session.ID)
	require.NoError(t, err)
	require.Nil(t, retrieved)
}
func TestRedisClient_RevokeAllSessions(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()
	userID := "user-revoke-all"

	session := &Session{
		ID:        "session-revoke-all",
		UserID:    userID,
		TenantID:  "tenant-123",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}

	err := rc.CreateSession(ctx, session)
	require.NoError(t, err)

	// RevokeAllSessions is alias for DeleteAllUserSessions
	err = rc.RevokeAllSessions(ctx, userID)
	require.NoError(t, err)

	sessions, err := rc.GetUserSessions(ctx, userID)
	require.NoError(t, err)
	require.Len(t, sessions, 0)
}
func TestRedisClient_CreateExpiredSession(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()

	session := &Session{
		ID:        "expired-session",
		UserID:    "user-123",
		TenantID:  "tenant-123",
		ExpiresAt: time.Now().UTC().Add(-time.Hour), // Already expired
	}

	err := rc.CreateSession(ctx, session)
	fmt.Println("error is:", err)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expired")
}
func TestRedisClient_BlacklistToken(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()
	tokenID := "token-to-blacklist"

	// Initially not blacklisted
	blacklisted, err := rc.IsTokenBlacklisted(ctx, tokenID)
	require.NoError(t, err)
	require.False(t, blacklisted)

	// Blacklist the token
	err = rc.BlacklistToken(ctx, tokenID, time.Now().Add(time.Hour))
	require.NoError(t, err)

	// Now it's blacklisted
	blacklisted, err = rc.IsTokenBlacklisted(ctx, tokenID)
	require.NoError(t, err)
	require.True(t, blacklisted)
}
func TestRedisClient_BlacklistExpiredToken(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()
	tokenID := "already-expired-token"

	// Blacklisting an already expired token should be a no-op
	err := rc.BlacklistToken(ctx, tokenID, time.Now().Add(-time.Hour))
	require.NoError(t, err)

	// Should not be blacklisted (it was a no-op)
	blacklisted, err := rc.IsTokenBlacklisted(ctx, tokenID)
	require.NoError(t, err)
	require.False(t, blacklisted)
}
func TestRedisClient_ResetToken(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()

	resetToken := &ResetToken{
		UserID:    "user-reset",
		Email:     "user@example.com",
		Token:     "reset-token-xyz",
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}

	// Store token
	err := rc.StoreResetToken(ctx, resetToken)
	require.NoError(t, err)

	// Get token
	retrieved, err := rc.GetResetToken(ctx, resetToken.Token)
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	require.Equal(t, resetToken.UserID, retrieved.UserID)
	require.Equal(t, resetToken.Email, retrieved.Email)

	// Delete token
	err = rc.DeleteResetToken(ctx, resetToken.Token)
	require.NoError(t, err)

	// Verify it's gone
	retrieved, err = rc.GetResetToken(ctx, resetToken.Token)
	require.NoError(t, err)
	require.Nil(t, retrieved)
}
func TestRedisClient_GetResetTokenNotFound(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()

	token, err := rc.GetResetToken(ctx, "nonexistent-token")
	require.NoError(t, err)
	require.Nil(t, token)
}
func TestRedisClient_VerifyEmailToken(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()

	verifyToken := &VerifyEmailToken{
		UserID:    "user-verify",
		Email:     "verify@example.com",
		Token:     "verify-token-abc",
		CreatedAt: time.Now().UTC(),
	}

	// Store token
	err := rc.StoreVerifyEmailToken(ctx, verifyToken)
	require.NoError(t, err)

	// Get token
	retrieved, err := rc.GetVerifyEmailToken(ctx, verifyToken.Token)
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	require.Equal(t, verifyToken.UserID, retrieved.UserID)
	require.Equal(t, verifyToken.Email, retrieved.Email)

	// Delete token
	err = rc.DeleteVerifyEmailToken(ctx, verifyToken.Token)
	require.NoError(t, err)

	// Verify it's gone
	retrieved, err = rc.GetVerifyEmailToken(ctx, verifyToken.Token)
	require.NoError(t, err)
	require.Nil(t, retrieved)
}
func TestRedisClient_RateLimit(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()
	key := "api:user:123"
	limit := int64(3)
	window := time.Minute

	// First request - should be allowed
	result, err := rc.CheckRateLimit(ctx, key, limit, window)
	require.NoError(t, err)
	require.True(t, result.Allowed)
	require.Equal(t, int64(2), result.Remaining)

	// Second request - should be allowed
	result, err = rc.CheckRateLimit(ctx, key, limit, window)
	require.NoError(t, err)
	require.True(t, result.Allowed)
	require.Equal(t, int64(1), result.Remaining)

	// Third request - should be allowed
	result, err = rc.CheckRateLimit(ctx, key, limit, window)
	require.NoError(t, err)
	require.True(t, result.Allowed)
	require.Equal(t, int64(0), result.Remaining)

	// Fourth request - should be rate limited
	result, err = rc.CheckRateLimit(ctx, key, limit, window)
	require.NoError(t, err)
	require.False(t, result.Allowed)
	require.Equal(t, int64(0), result.Remaining)
	require.Greater(t, result.RetryAfter, int64(0))
}
func TestRedisClient_LoginAttempts(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()
	identifier := "user@example.com"

	// First failed attempt
	result, err := rc.RecordLoginAttempt(ctx, identifier, false)
	require.NoError(t, err)
	require.True(t, result.Allowed)
	require.Equal(t, 4, result.AttemptsLeft) // 5 max - 1 = 4

	// More failed attempts
	for range 3 {
		result, err = rc.RecordLoginAttempt(ctx, identifier, false)
		require.NoError(t, err)
		require.True(t, result.Allowed)
	}

	// 5th attempt - should trigger lockout
	result, err = rc.RecordLoginAttempt(ctx, identifier, false)
	require.NoError(t, err)
	require.False(t, result.Allowed)
	require.NotNil(t, result.LockedUntil)
	require.Equal(t, 0, result.AttemptsLeft)

	// Check if locked
	locked, lockedUntil, err := rc.IsLoginLocked(ctx, identifier)
	require.NoError(t, err)
	require.True(t, locked)
	require.NotNil(t, lockedUntil)
}
func TestRedisClient_FeatureFlags(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()
	tenantID := "tenant-flags"
	envID := "production"

	flags := map[string]bool{
		"new_dashboard":   true,
		"beta_feature":    false,
		"experimental_ui": true,
	}

	// Cache flags
	err := rc.CacheFeatureFlags(ctx, tenantID, envID, flags, time.Hour)
	require.NoError(t, err)

	// Get flags
	retrieved, err := rc.GetFeatureFlags(ctx, tenantID, envID)
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	require.Equal(t, flags["new_dashboard"], retrieved["new_dashboard"])
	require.Equal(t, flags["beta_feature"], retrieved["beta_feature"])
	require.Equal(t, flags["experimental_ui"], retrieved["experimental_ui"])
}
func TestRedisClient_GetFeatureFlagsNotFound(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()

	flags, err := rc.GetFeatureFlags(ctx, "nonexistent", "env")
	require.NoError(t, err)
	require.Nil(t, flags)
}
func TestRedisClient_KeyPrefix(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	// Test key building
	key := rc.key("session", "abc123")
	require.Equal(t, "test:session:abc123", key)

	key = rc.key("user_sessions", "user-1")
	require.Equal(t, "test:user_sessions:user-1", key)

	key = rc.key("a", "b", "c")
	require.Equal(t, "test:a:b:c", key)
}

func TestRedisClient_Client(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	// Verify Client() returns the underlying client
	client := rc.Client()
	require.NotNil(t, client)

	// Use it directly
	ctx := context.Background()
	err := client.Set(ctx, "direct-key", "direct-value", time.Minute).Err()
	require.NoError(t, err)
}

func TestRedisClient_Publish(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()

	type Message struct {
		Type    string `json:"type"`
		Payload string `json:"payload"`
	}

	msg := Message{Type: "test", Payload: "hello"}

	err := rc.Publish(ctx, "notifications", msg)
	require.NoError(t, err)
}
func TestRedisClient_PublishInvalidMessage(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()

	// Create an unmarshalable value (channel cannot be marshaled)
	ch := make(chan int)

	err := rc.Publish(ctx, "notifications", ch)
	require.Error(t, err)
}
func TestRedisClient_Subscribe(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()

	// Subscribe to channel
	pubsub := rc.Subscribe(ctx, "events")
	require.NotNil(t, pubsub)
	defer pubsub.Close()

	// Verify subscription
	_, err := pubsub.Receive(ctx)
	require.NoError(t, err)
}
func TestRedisClient_SubscribeMultipleChannels(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()

	// Subscribe to multiple channels
	pubsub := rc.Subscribe(ctx, "channel1", "channel2", "channel3")
	require.NotNil(t, pubsub)
	defer pubsub.Close()
}
func TestRedisClient_PubSubIntegration(t *testing.T) {
	rc, mr := setupTestRedis(t)
	defer teardownTestRedis(rc, mr)

	ctx := context.Background()

	type Event struct {
		Action string `json:"action"`
		UserID string `json:"user_id"`
	}

	// Subscribe first
	pubsub := rc.Subscribe(ctx, "user_events")
	defer pubsub.Close()

	// Wait for subscription confirmation
	_, err := pubsub.Receive(ctx)
	require.NoError(t, err)

	// Publish message
	event := Event{Action: "login", UserID: "user-123"}
	err = rc.Publish(ctx, "user_events", event)
	require.NoError(t, err)

	// Receive message
	msg, err := pubsub.ReceiveMessage(ctx)
	require.NoError(t, err)
	require.Equal(t, "test:user_events", msg.Channel)

	// Verify payload
	var received Event
	err = json.Unmarshal([]byte(msg.Payload), &received)
	require.NoError(t, err)
	require.Equal(t, event.Action, received.Action)
	require.Equal(t, event.UserID, received.UserID)
}
