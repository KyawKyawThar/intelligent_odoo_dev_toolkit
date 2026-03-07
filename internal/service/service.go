package service

import (
	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"

	"Intelligent_Dev_ToolKit_Odoo/internal/cache"

	"Intelligent_Dev_ToolKit_Odoo/internal/token"

	"time"

	"golang.org/x/crypto/bcrypt"
)

type Services struct {
	Auth AuthServicer

	// Future services:
	// Environment EnvironmentServicer
	// Profiler    ProfilerServicer
	// Alert       AlertServicer
	// Audit       AuditServicer
}

type Config struct {
	Auth AuthConfig
	// Future configs can be added here
}

// =============================================================================
// Auth Configuration
// =============================================================================
type AuthService struct {
	store      db.Store
	cache      *cache.RedisClient
	tokenMaker token.Maker
	config     *AuthConfig
}
type AuthConfig struct {
	AccessTokenDuration  time.Duration
	RefreshTokenDuration time.Duration
	PasswordMinLength    int
	BcryptCost           int

	// MailHog / SMTP
	SMTPHost   string
	SMTPPort   int
	SMTPFrom   string
	AppBaseURL string
}

func DefaultAuthConfig() *AuthConfig {
	return &AuthConfig{
		AccessTokenDuration:  15 * time.Minute,
		RefreshTokenDuration: 7 * 24 * time.Hour,
		PasswordMinLength:    8,
		BcryptCost:           bcrypt.DefaultCost,

		SMTPHost:   "localhost",
		SMTPPort:   1025,
		SMTPFrom:   "noreply@odoodevtools.com",
		AppBaseURL: "http://localhost:8080",
	}
}

// DefaultConfig returns sensible defaults for all services.
func DefaultConfig() *Config {
	return &Config{
		Auth: *DefaultAuthConfig(),
	}
}

func NewAuthService(store db.Store, redisCache *cache.RedisClient, tokenMaker token.Maker, cfg *AuthConfig) *AuthService {
	if cfg == nil {
		cfg = DefaultAuthConfig()
	}
	return &AuthService{
		store:      store,
		cache:      redisCache,
		tokenMaker: tokenMaker,
		config:     cfg,
	}
}

// NewServices creates all services with their dependencies.
func NewServices(store db.Store, redisCache *cache.RedisClient, tokenMaker token.Maker, cfg *Config) *Services {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Create the base auth service (implements multiple interfaces)
	authSvc := NewAuthService(store, redisCache, tokenMaker, &cfg.Auth)

	return &Services{
		Auth: authSvc,
	}
}
