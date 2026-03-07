package cache

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	ErrSessionNotFound = errors.New("session not found")
	ErrTokenNotFound   = errors.New("token not found")
	ErrKeyNotFound     = errors.New("key not found")
)

const blacklistPrefix = "blacklist"
const rateLimitPrefix = "ratelimit"

type RedisClient struct {
	Client *redis.Client
	Prefix string // Key prefix for namespacing (e.g., "odt:")
}

type RedisConfig struct {
	// Address may contain a full redis URI (redis:// or rediss://) or a
	// simple host:port pair.  If provided, it takes precedence over Host/Port
	// fields and is parsed internally by NewRedisClient.
	Address string

	Host     string
	Port     int
	Password string
	DB       int
	Prefix   string
	TLS      bool
}

// ParseRedisConfig takes either a simple host:port string or a full Redis URL
// (e.g. redis://:password@host:6379/0) and returns a RedisConfig that can be
// passed to NewRedisClient.  This helper is primarily used when loading
// configuration from environment variables where some platforms supply a
// complete URL.  The returned config will fill Host, Port, Password and DB
// fields.  If the input string is empty an error is returned.
func ParseRedisConfig(urlStr string) (RedisConfig, error) {
	if urlStr == "" {
		return RedisConfig{}, fmt.Errorf("empty redis url")
	}

	if strings.HasPrefix(urlStr, "redis://") || strings.HasPrefix(urlStr, "rediss://") {
		return parseRedisURL(urlStr)
	}

	return parseSimpleRedisAddr(urlStr)
}

func parseRedisURL(urlStr string) (RedisConfig, error) {
	cfg := RedisConfig{Port: 6379}
	u, err := url.Parse(urlStr)
	if err != nil {
		return cfg, fmt.Errorf("invalid redis url: %w", err)
	}

	if u.Scheme != "redis" && u.Scheme != "rediss" {
		return cfg, fmt.Errorf("invalid redis scheme: %s", u.Scheme)
	}

	cfg.Host = u.Hostname()
	if p := u.Port(); p != "" {
		if pi, err := strconv.Atoi(p); err == nil {
			cfg.Port = pi
		}
	}

	if u.User != nil {
		if pass, ok := u.User.Password(); ok {
			cfg.Password = pass
		}
	}

	if u.Path != "" && u.Path != "/" {
		dbStr := strings.TrimPrefix(u.Path, "/")
		if di, err := strconv.Atoi(dbStr); err == nil {
			cfg.DB = di
		}
	}
	return cfg, nil
}

func parseSimpleRedisAddr(urlStr string) (RedisConfig, error) {
	cfg := RedisConfig{Port: 6379}
	host := urlStr
	if parts := strings.Split(urlStr, ":"); len(parts) == 2 {
		host = parts[0]
		if p, err := strconv.Atoi(parts[1]); err == nil {
			cfg.Port = p
		}
	}
	cfg.Host = host
	return cfg, nil
}

func NewRedisClient(cfg RedisConfig) (*RedisClient, error) {
	// if an address string is provided, parse it to fill missing fields.
	if cfg.Address != "" {
		parsed, err := ParseRedisConfig(cfg.Address)
		if err != nil {
			return nil, fmt.Errorf("invalid redis address: %w", err)
		}
		// only override hosts/port/password/db when not explicitly set
		if cfg.Host == "" {
			cfg.Host = parsed.Host
		}
		if cfg.Port == 0 {
			cfg.Port = parsed.Port
		}
		if cfg.Password == "" {
			cfg.Password = parsed.Password
		}
		if cfg.DB == 0 {
			cfg.DB = parsed.DB
		}
	}

	if cfg.Port == 0 {
		cfg.Port = 6379
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	opts := &redis.Options{
		Addr:     addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	}

	if cfg.TLS {
		opts.TLSConfig = &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: false,
			ServerName:         cfg.Host,
		}
	}

	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}
	prefix := cfg.Prefix
	if prefix == "" {
		prefix = "odt:"
	}
	return &RedisClient{
		Client: client,
		Prefix: prefix,
	}, nil
}
func (r *RedisClient) Close() error {
	return r.Client.Close()
}

// Client returns the underlying redis client (for testing)
func (r *RedisClient) GetClient() *redis.Client {
	return r.Client
}

func (r *RedisClient) Key(parts ...string) string {
	result := r.Prefix
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
	return r.Client.Set(ctx, r.Key(key), data, expiration).Err()
}
func (r *RedisClient) Get(ctx context.Context, key string, dest any) error {
	data, err := r.Client.Get(ctx, r.Key(key)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return ErrKeyNotFound
		}
		return err
	}
	return json.Unmarshal(data, dest)
}

func (r *RedisClient) GetString(ctx context.Context, key string) (string, error) {
	result, err := r.Client.Get(ctx, r.Key(key)).Result()
	if err == redis.Nil {
		return "", ErrKeyNotFound
	}
	return result, err
}

// SetString stores a string value with expiration
func (r *RedisClient) SetString(ctx context.Context, key, value string, expiration time.Duration) error {
	return r.Client.Set(ctx, r.Key(key), value, expiration).Err()
}

func (r *RedisClient) Delete(ctx context.Context, keys ...string) error {
	prefixedKeys := make([]string, len(keys))
	for i, k := range keys {
		prefixedKeys[i] = r.Key(k)
	}
	return r.Client.Del(ctx, prefixedKeys...).Err()
}

func (r *RedisClient) Exists(ctx context.Context, key string) (bool, error) {
	result, err := r.Client.Exists(ctx, r.Key(key)).Result()
	return result > 0, err
}

func (r *RedisClient) Expire(ctx context.Context, key string, expiration time.Duration) error {
	return r.Client.Expire(ctx, r.Key(key), expiration).Err()
}

func (r *RedisClient) TTL(ctx context.Context, key string) (time.Duration, error) {
	return r.Client.TTL(ctx, r.Key(key)).Result()
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
	sessionKey := r.Key(sessionPrefix, session.ID)
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	ttl := time.Until(session.ExpiresAt)
	if ttl <= 0 {
		return fmt.Errorf("session expired")
	}
	if err := r.Client.Set(ctx, sessionKey, data, ttl).Err(); err != nil {
		return fmt.Errorf("failed to store session: %w", err)
	}

	// Add session ID to user's session set (for listing all sessions)
	userSessionsKey := r.Key(userSessionPrefix, session.UserID)
	if err := r.Client.SAdd(ctx, userSessionsKey, session.ID).Err(); err != nil {
		return fmt.Errorf("failed to add session to user set: %w", err)
	}

	// Set expiry on user sessions set (auto-cleanup)
	r.Client.Expire(ctx, userSessionsKey, ttl+time.Hour)

	return nil
}

// GetSession retrieves a session by ID
func (r *RedisClient) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	sessionKey := r.Key(sessionPrefix, sessionID)
	data, err := r.Client.Get(ctx, sessionKey).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrSessionNotFound // Session not found
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
	sessionKey := r.Key(sessionPrefix, sessionID)
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	ttl := time.Until(session.ExpiresAt)
	return r.Client.Set(ctx, sessionKey, data, ttl).Err()
}

// DeleteSession removes a session
func (r *RedisClient) DeleteSession(ctx context.Context, sessionID, userID string) error {
	sessionKey := r.Key(sessionPrefix, sessionID)
	userSessionsKey := r.Key(userSessionPrefix, userID)

	pipe := r.Client.Pipeline()
	pipe.Del(ctx, sessionKey)
	pipe.SRem(ctx, userSessionsKey, sessionID)
	_, err := pipe.Exec(ctx)
	return err
}

func (r *RedisClient) RevokeSession(ctx context.Context, userID, sessionID string) error {
	return r.DeleteSession(ctx, sessionID, userID)
}

func (r *RedisClient) RevokeAllSessions(ctx context.Context, userID string) error {
	return r.DeleteAllUserSessions(ctx, userID)
}

// DeleteAllUserSessions removes all sessions for a user (logout everywhere)
func (r *RedisClient) DeleteAllUserSessions(ctx context.Context, userID string) error {
	userSessionsKey := r.Key(userSessionPrefix, userID)

	// Get all session IDs for the user
	sessionIDs, err := r.Client.SMembers(ctx, userSessionsKey).Result()
	if err != nil {
		return fmt.Errorf("failed to get user sessions: %w", err)
	}

	if len(sessionIDs) == 0 {
		return nil
	}

	// Delete all sessions
	pipe := r.Client.Pipeline()
	for _, sid := range sessionIDs {
		pipe.Del(ctx, r.Key(sessionPrefix, sid))
	}
	pipe.Del(ctx, userSessionsKey)
	_, err = pipe.Exec(ctx)
	return err
}

// GetUserSessions returns all active sessions for a user
func (r *RedisClient) GetUserSessions(ctx context.Context, userID string) ([]*Session, error) {
	userSessionsKey := r.Key(userSessionPrefix, userID)

	sessionIDs, err := r.Client.SMembers(ctx, userSessionsKey).Result()
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

// BlacklistToken adds a token to the blacklist
func (r *RedisClient) BlacklistToken(ctx context.Context, tokenID string, expiresAt time.Time) error {
	key := r.Key(blacklistPrefix, tokenID)
	ttl := time.Until(expiresAt)
	if ttl <= 0 {
		return nil // Token already expired, no need to blacklist
	}
	return r.Client.Set(ctx, key, "1", ttl).Err()
}

// IsTokenBlacklisted checks if a token is blacklisted
func (r *RedisClient) IsTokenBlacklisted(ctx context.Context, tokenID string) (bool, error) {
	key := r.Key(blacklistPrefix, tokenID)
	exists, err := r.Client.Exists(ctx, key).Result()
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
	key := r.Key(resetTokenPrefix, token.Token)
	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("failed to marshal reset token: %w", err)
	}
	return r.Client.Set(ctx, key, data, resetTokenTTL).Err()
}

// GetResetToken retrieves a password reset token
func (r *RedisClient) GetResetToken(ctx context.Context, token string) (*ResetToken, error) {
	key := r.Key(resetTokenPrefix, token)
	data, err := r.Client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrTokenNotFound
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
	key := r.Key(resetTokenPrefix, token)
	return r.Client.Del(ctx, key).Err()
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
	key := r.Key(verifyEmailPrefix, token.Token)
	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("failed to marshal verify email token: %w", err)
	}
	return r.Client.Set(ctx, key, data, verifyEmailTTL).Err()
}

// GetVerifyEmailToken retrieves an email verification token
func (r *RedisClient) GetVerifyEmailToken(ctx context.Context, token string) (*VerifyEmailToken, error) {
	key := r.Key(verifyEmailPrefix, token)
	data, err := r.Client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrTokenNotFound
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
	key := r.Key(verifyEmailPrefix, token)
	return r.Client.Del(ctx, key).Err()
}

// =============================================================================
// Rate Limiting
// =============================================================================

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

	redisKey := r.Key(rateLimitPrefix, key)

	pipe := r.Client.Pipeline()

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
	key := r.Key(loginAttemptPrefix, identifier)

	if success {
		// Clear attempts on successful login
		r.Client.Del(ctx, key)
		return &LoginAttemptResult{Allowed: true, AttemptsLeft: maxLoginAttempts}, nil
	}

	// Increment failed attempts
	count, err := r.Client.Incr(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	// Set expiry on first attempt
	if count == 1 {
		r.Client.Expire(ctx, key, loginAttemptWindow)
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
		lockoutKey := r.Key(loginAttemptPrefix, "lockout", identifier)
		lockedUntil := time.Now().Add(loginLockoutDuration)
		r.Client.Set(ctx, lockoutKey, lockedUntil.Unix(), loginLockoutDuration)
		result.LockedUntil = &lockedUntil
		result.LockoutDuration = loginLockoutDuration
	}

	return result, nil
}

// IsLoginLocked checks if an identifier is locked out
func (r *RedisClient) IsLoginLocked(ctx context.Context, identifier string) (bool, *time.Time, error) {
	lockoutKey := r.Key(loginAttemptPrefix, "lockout", identifier)
	timestamp, err := r.Client.Get(ctx, lockoutKey).Int64()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false, nil, nil
		}
		return false, nil, err
	}

	lockedUntil := time.Unix(timestamp, 0)
	if time.Now().After(lockedUntil) {
		// Lockout expired, clean up
		r.Client.Del(ctx, lockoutKey)
		return false, nil, nil
	}

	return true, &lockedUntil, nil
}

// ClearLoginAttempts clears login attempts for an identifier
func (r *RedisClient) ClearLoginAttempts(ctx context.Context, identifier string) error {
	key := r.Key(loginAttemptPrefix, identifier)
	lockoutKey := r.Key(loginAttemptPrefix, "lockout", identifier)
	return r.Client.Del(ctx, key, lockoutKey).Err()
}

// =============================================================================
// Feature Flags Cache
// =============================================================================

const featureFlagPrefix = "feature_flags"

// CacheFeatureFlags caches feature flags for a tenant/environment
func (r *RedisClient) CacheFeatureFlags(ctx context.Context, tenantID, envID string, flags map[string]bool, ttl time.Duration) error {
	key := r.Key(featureFlagPrefix, tenantID, envID)
	data, err := json.Marshal(flags)
	if err != nil {
		return err
	}
	return r.Client.Set(ctx, key, data, ttl).Err()
}

// GetFeatureFlags retrieves cached feature flags
func (r *RedisClient) GetFeatureFlags(ctx context.Context, tenantID, envID string) (map[string]bool, error) {
	key := r.Key(featureFlagPrefix, tenantID, envID)
	data, err := r.Client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrTokenNotFound
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
	return r.Client.Publish(ctx, r.Key(channel), data).Err()
}

// Subscribe returns a subscription to a channel
func (r *RedisClient) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	prefixedChannels := make([]string, len(channels))
	for i, ch := range channels {
		prefixedChannels[i] = r.Key(ch)
	}
	return r.Client.Subscribe(ctx, prefixedChannels...)
}
