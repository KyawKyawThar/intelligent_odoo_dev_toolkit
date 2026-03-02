package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisClient struct {
	client *redis.Client
	prefix string // Key prefix for namespacing (e.g., "odt:")
}

type RedisConfig struct {
	Host     string
	Port     int
	Password string
	DB       int
	Prefix   string
}

func NewRedisClient(cfg RedisConfig) (*RedisClient, error) {

	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}
	prefix := cfg.Prefix
	if prefix == "" {
		prefix = "odt:"
	}
	return &RedisClient{}, nil
}
func (r *RedisClient) Close() error {
	return r.client.Close()
}
func (r *RedisClient) key(parts ...string) string {
	result := r.prefix
	for _, part := range parts {
		result += part + ":"
	}
	return result[:len(result)-1] // Remove trailing colon
}
func (r *RedisClient) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}
	return r.client.Set(ctx, r.key(key), data, expiration).Err()
}
func (r *RedisClient) Get(ctx context.Context, key string, dest any) error {
	data, err := r.client.Get(ctx, r.key(key)).Bytes()
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

func (r *RedisClient) GetString(ctx context.Context, key string) (string, error) {
	return r.client.Get(ctx, r.key(key)).Result()
}

func (r *RedisClient) SetString(ctx context.Context, key, value string, expiration time.Duration) error {
	return r.client.Set(ctx, r.key(key), value, expiration).Err()
}

func (r *RedisClient) Delete(ctx context.Context, keys ...string) error {
	prefixedKeys := make([]string, len(keys))
	for i, k := range keys {
		prefixedKeys[i] = r.key(k)
	}
	return r.client.Del(ctx, prefixedKeys...).Err()
}

func (r *RedisClient) Exists(ctx context.Context, key string) (bool, error) {
	result, err := r.client.Exists(ctx, r.key(key)).Result()
	return result > 0, err
}

func (r *RedisClient) Expire(ctx context.Context, key string, expiration time.Duration) error {
	return r.client.Expire(ctx, r.key(key), expiration).Err()
}

func (r *RedisClient) TTL(ctx context.Context, key string) (time.Duration, error) {
	return r.client.TTL(ctx, r.key(key)).Result()
}

// =============================================================================
// Session Management
// =============================================================================
const (
	sessionPrefix     = "session"
	userSessionPrefix = "user_sessions"
)

type Session struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	TenantID     string    `json:"tenant_id"`
	RefreshToken string    `json:"refresh_token"`
	UserAgent    string    `json:"user_agent"`
	IPAddress    string    `json:"ip_address"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	LastActiveAt time.Time `json:"last_active_at"`
}

// CreateSession stores a new session in Redis
func (r *RedisClient) CreateSession(ctx context.Context, session *Session) error {
	// Store session by ID
	sessionKey := r.key(sessionPrefix, session.ID)
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	ttl := time.Until(session.ExpiresAt)
	if err := r.client.Set(ctx, sessionKey, data, ttl).Err(); err != nil {
		return fmt.Errorf("failed to store session: %w", err)
	}

	// Add session ID to user's session set (for listing all sessions)
	userSessionsKey := r.key(userSessionPrefix, session.UserID)
	if err := r.client.SAdd(ctx, userSessionsKey, session.ID).Err(); err != nil {
		return fmt.Errorf("failed to add session to user set: %w", err)
	}

	// Set expiry on user sessions set (auto-cleanup)
	r.client.Expire(ctx, userSessionsKey, ttl+time.Hour)

	return nil
}

// GetSession retrieves a session by ID
func (r *RedisClient) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	sessionKey := r.key(sessionPrefix, sessionID)
	data, err := r.client.Get(ctx, sessionKey).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // Session not found
		}
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return &session, nil
}

// UpdateSessionActivity updates the last active timestamp
func (r *RedisClient) UpdateSessionActivity(ctx context.Context, sessionID string) error {
	session, err := r.GetSession(ctx, sessionID)
	if err != nil || session == nil {
		return err
	}

	session.LastActiveAt = time.Now().UTC()
	sessionKey := r.key(sessionPrefix, sessionID)
	data, _ := json.Marshal(session)

	ttl := time.Until(session.ExpiresAt)
	return r.client.Set(ctx, sessionKey, data, ttl).Err()
}

// DeleteSession removes a session
func (r *RedisClient) DeleteSession(ctx context.Context, sessionID, userID string) error {
	sessionKey := r.key(sessionPrefix, sessionID)
	userSessionsKey := r.key(userSessionPrefix, userID)

	pipe := r.client.Pipeline()
	pipe.Del(ctx, sessionKey)
	pipe.SRem(ctx, userSessionsKey, sessionID)
	_, err := pipe.Exec(ctx)
	return err
}

// DeleteAllUserSessions removes all sessions for a user (logout everywhere)
func (r *RedisClient) DeleteAllUserSessions(ctx context.Context, userID string) error {
	userSessionsKey := r.key(userSessionPrefix, userID)

	// Get all session IDs for the user
	sessionIDs, err := r.client.SMembers(ctx, userSessionsKey).Result()
	if err != nil {
		return fmt.Errorf("failed to get user sessions: %w", err)
	}

	if len(sessionIDs) == 0 {
		return nil
	}

	// Delete all sessions
	pipe := r.client.Pipeline()
	for _, sid := range sessionIDs {
		pipe.Del(ctx, r.key(sessionPrefix, sid))
	}
	pipe.Del(ctx, userSessionsKey)
	_, err = pipe.Exec(ctx)
	return err
}

// GetUserSessions returns all active sessions for a user
func (r *RedisClient) GetUserSessions(ctx context.Context, userID string) ([]*Session, error) {
	userSessionsKey := r.key(userSessionPrefix, userID)

	sessionIDs, err := r.client.SMembers(ctx, userSessionsKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get user session IDs: %w", err)
	}

	sessions := make([]*Session, 0, len(sessionIDs))
	for _, sid := range sessionIDs {
		session, err := r.GetSession(ctx, sid)
		if err != nil {
			continue // Skip errored sessions
		}
		if session != nil {
			sessions = append(sessions, session)
		}
	}

	return sessions, nil
}

const blacklistPrefix = "blacklist"

// BlacklistToken adds a token to the blacklist
func (r *RedisClient) BlacklistToken(ctx context.Context, tokenID string, expiresAt time.Time) error {
	key := r.key(blacklistPrefix, tokenID)
	ttl := time.Until(expiresAt)
	if ttl <= 0 {
		return nil // Token already expired, no need to blacklist
	}
	return r.client.Set(ctx, key, "1", ttl).Err()
}

// IsTokenBlacklisted checks if a token is blacklisted
func (r *RedisClient) IsTokenBlacklisted(ctx context.Context, tokenID string) (bool, error) {
	key := r.key(blacklistPrefix, tokenID)
	exists, err := r.client.Exists(ctx, key).Result()
	return exists > 0, err
}

// =============================================================================
// Password Reset Tokens
// =============================================================================
const (
	resetTokenPrefix = "reset_token"
	resetTokenTTL    = 1 * time.Hour
)

type ResetToken struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Token     string    `json:"token"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// StoreResetToken stores a password reset token
func (r *RedisClient) StoreResetToken(ctx context.Context, token *ResetToken) error {
	key := r.key(resetTokenPrefix, token.Token)
	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("failed to marshal reset token: %w", err)
	}
	return r.client.Set(ctx, key, data, resetTokenTTL).Err()
}

// GetResetToken retrieves a password reset token
func (r *RedisClient) GetResetToken(ctx context.Context, token string) (*ResetToken, error) {
	key := r.key(resetTokenPrefix, token)
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}

	var resetToken ResetToken
	if err := json.Unmarshal(data, &resetToken); err != nil {
		return nil, err
	}
	return &resetToken, nil
}

// DeleteResetToken removes a password reset token (after use)
func (r *RedisClient) DeleteResetToken(ctx context.Context, token string) error {
	key := r.key(resetTokenPrefix, token)
	return r.client.Del(ctx, key).Err()
}

// =============================================================================
// Email Verification Tokens
// =============================================================================

const (
	verifyEmailPrefix = "verify_email"
	verifyEmailTTL    = 24 * time.Hour
)

type VerifyEmailToken struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Token     string    `json:"token"`
	CreatedAt time.Time `json:"created_at"`
}

// StoreVerifyEmailToken stores an email verification token
func (r *RedisClient) StoreVerifyEmailToken(ctx context.Context, token *VerifyEmailToken) error {
	key := r.key(verifyEmailPrefix, token.Token)
	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("failed to marshal verify email token: %w", err)
	}
	return r.client.Set(ctx, key, data, verifyEmailTTL).Err()
}

// GetVerifyEmailToken retrieves an email verification token
func (r *RedisClient) GetVerifyEmailToken(ctx context.Context, token string) (*VerifyEmailToken, error) {
	key := r.key(verifyEmailPrefix, token)
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}

	var verifyToken VerifyEmailToken
	if err := json.Unmarshal(data, &verifyToken); err != nil {
		return nil, err
	}
	return &verifyToken, nil
}

// DeleteVerifyEmailToken removes a verification token (after use)
func (r *RedisClient) DeleteVerifyEmailToken(ctx context.Context, token string) error {
	key := r.key(verifyEmailPrefix, token)
	return r.client.Del(ctx, key).Err()
}

// =============================================================================
// Rate Limiting
// =============================================================================

const rateLimitPrefix = "ratelimit"

type RateLimitResult struct {
	Allowed    bool
	Remaining  int64
	ResetAt    time.Time
	RetryAfter int64 // seconds
}

// CheckRateLimit implements sliding window rate limiting
func (r *RedisClient) CheckRateLimit(ctx context.Context, key string, limit int64, window time.Duration) (*RateLimitResult, error) {
	now := time.Now()
	windowStart := now.Add(-window)

	redisKey := r.key(rateLimitPrefix, key)

	pipe := r.client.Pipeline()

	// Remove old entries outside the window
	pipe.ZRemRangeByScore(ctx, redisKey, "0", fmt.Sprintf("%d", windowStart.UnixNano()))

	// Count current entries in window
	countCmd := pipe.ZCard(ctx, redisKey)

	// Add current request
	pipe.ZAdd(ctx, redisKey, redis.Z{
		Score:  float64(now.UnixNano()),
		Member: fmt.Sprintf("%d", now.UnixNano()),
	})

	// Set expiry on the key
	pipe.Expire(ctx, redisKey, window)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("rate limit check failed: %w", err)
	}

	count := countCmd.Val()
	remaining := max(limit-count-1, 0)

	result := &RateLimitResult{
		Allowed:   count < limit,
		Remaining: remaining,
		ResetAt:   now.Add(window),
	}

	if !result.Allowed {
		result.RetryAfter = int64(window.Seconds())
	}

	return result, nil
}

// =============================================================================
// Login Attempt Tracking (Brute Force Protection)
// =============================================================================

const (
	loginAttemptPrefix   = "login_attempts"
	loginAttemptWindow   = 15 * time.Minute
	maxLoginAttempts     = 5
	loginLockoutDuration = 30 * time.Minute
)

type LoginAttemptResult struct {
	Allowed         bool
	AttemptsLeft    int
	LockedUntil     *time.Time
	LockoutDuration time.Duration
}

// RecordLoginAttempt tracks failed login attempts
func (r *RedisClient) RecordLoginAttempt(ctx context.Context, identifier string, success bool) (*LoginAttemptResult, error) {
	key := r.key(loginAttemptPrefix, identifier)

	if success {
		// Clear attempts on successful login
		r.client.Del(ctx, key)
		return &LoginAttemptResult{Allowed: true, AttemptsLeft: maxLoginAttempts}, nil
	}

	// Increment failed attempts
	count, err := r.client.Incr(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	// Set expiry on first attempt
	if count == 1 {
		r.client.Expire(ctx, key, loginAttemptWindow)
	}

	attemptsLeft := maxLoginAttempts - int(count)
	if attemptsLeft < 0 {
		attemptsLeft = 0
	}

	result := &LoginAttemptResult{
		Allowed:      count < maxLoginAttempts,
		AttemptsLeft: attemptsLeft,
	}

	// If max attempts reached, set lockout
	if count >= maxLoginAttempts {
		lockoutKey := r.key(loginAttemptPrefix, "lockout", identifier)
		lockedUntil := time.Now().Add(loginLockoutDuration)
		r.client.Set(ctx, lockoutKey, lockedUntil.Unix(), loginLockoutDuration)
		result.LockedUntil = &lockedUntil
		result.LockoutDuration = loginLockoutDuration
	}

	return result, nil
}

// IsLoginLocked checks if an identifier is locked out
func (r *RedisClient) IsLoginLocked(ctx context.Context, identifier string) (bool, *time.Time, error) {
	lockoutKey := r.key(loginAttemptPrefix, "lockout", identifier)
	timestamp, err := r.client.Get(ctx, lockoutKey).Int64()
	if err != nil {
		if err == redis.Nil {
			return false, nil, nil
		}
		return false, nil, err
	}

	lockedUntil := time.Unix(timestamp, 0)
	if time.Now().After(lockedUntil) {
		// Lockout expired, clean up
		r.client.Del(ctx, lockoutKey)
		return false, nil, nil
	}

	return true, &lockedUntil, nil
}

// ClearLoginAttempts clears login attempts for an identifier
func (r *RedisClient) ClearLoginAttempts(ctx context.Context, identifier string) error {
	key := r.key(loginAttemptPrefix, identifier)
	lockoutKey := r.key(loginAttemptPrefix, "lockout", identifier)
	return r.client.Del(ctx, key, lockoutKey).Err()
}

// =============================================================================
// Feature Flags Cache
// =============================================================================

const featureFlagPrefix = "feature_flags"

// CacheFeatureFlags caches feature flags for a tenant/environment
func (r *RedisClient) CacheFeatureFlags(ctx context.Context, tenantID, envID string, flags map[string]bool, ttl time.Duration) error {
	key := r.key(featureFlagPrefix, tenantID, envID)
	data, err := json.Marshal(flags)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, key, data, ttl).Err()
}

// GetFeatureFlags retrieves cached feature flags
func (r *RedisClient) GetFeatureFlags(ctx context.Context, tenantID, envID string) (map[string]bool, error) {
	key := r.key(featureFlagPrefix, tenantID, envID)
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}

	var flags map[string]bool
	if err := json.Unmarshal(data, &flags); err != nil {
		return nil, err
	}
	return flags, nil
}

// =============================================================================
// Pub/Sub for Real-time Updates
// =============================================================================

// Publish publishes a message to a channel
func (r *RedisClient) Publish(ctx context.Context, channel string, message any) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return r.client.Publish(ctx, r.key(channel), data).Err()
}

// Subscribe returns a subscription to a channel
func (r *RedisClient) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	prefixedChannels := make([]string, len(channels))
	for i, ch := range channels {
		prefixedChannels[i] = r.key(ch)
	}
	return r.client.Subscribe(ctx, prefixedChannels...)
}
