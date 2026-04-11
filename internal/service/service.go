package service

import (
	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/cache"
	"Intelligent_Dev_ToolKit_Odoo/internal/token"
	"fmt"
	"net/smtp"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type Services struct {
	Auth          AuthServicer
	Environment   EnvironmentServicer
	Schema        SchemaServicer
	Error         ErrorServicer
	APIKey        APIKeyServicer
	AgentRegister AgentRegisterServicer
	ACL           ACLServicer
	Profiler      ProfilerServicer
	N1            N1Servicer
	Budget        BudgetServicer
	Alert         AlertServicer
	Overview      OverviewServicer
	Migration     MigrationServicer
	Audit         *AuditService
	Notification  *NotificationService
}

type Config struct {
	Auth AuthConfig
	// Future configs can be added here
}

// AuthService provides authentication related services.
type AuthService struct {
	store      db.Store
	cache      *cache.RedisClient
	tokenMaker token.Maker
	config     *AuthConfig
}

type EnvironmentService struct {
	store db.Store
}
type AuthConfig struct {
	AccessTokenDuration  time.Duration
	RefreshTokenDuration time.Duration
	PasswordMinLength    int
	BcryptCost           int

	Environment string // "development" | "staging" | "production"

	// SMTP — production (Resend, SendGrid, etc.)
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string

	// MailHog — used automatically when Environment == "development"
	MailhogHost string
	MailhogPort int

	AppBaseURL string
}

func DefaultAuthConfig() *AuthConfig {
	return &AuthConfig{
		AccessTokenDuration:  15 * time.Minute,
		RefreshTokenDuration: 7 * 24 * time.Hour,
		PasswordMinLength:    8,
		BcryptCost:           bcrypt.DefaultCost,

		Environment: "development",
		MailhogHost: "localhost",
		MailhogPort: 1025,
		SMTPFrom:    "noreply@odoodevtools.com",
		AppBaseURL:  "http://localhost:8080",
	}
}

// smtpSettings returns the correct host, port, and auth based on environment.
// Development → MailHog (no auth). Staging/Production → real SMTP with auth.
func (c *AuthConfig) smtpSettings() (addr string, auth smtp.Auth) {
	if c.Environment == "development" {
		return fmt.Sprintf("%s:%d", c.MailhogHost, c.MailhogPort), nil
	}
	a := buildSMTPAuth(c.SMTPUsername, c.SMTPPassword, c.SMTPHost)
	return fmt.Sprintf("%s:%d", c.SMTPHost, c.SMTPPort), a
}

// DefaultConfig returns sensible defaults for all services.
func DefaultConfig() *Config {
	return &Config{
		Auth: *DefaultAuthConfig(),
	}
}

func NewEnvironmentService(store db.Store) *EnvironmentService {
	return &EnvironmentService{store: store}
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
	envSvc := NewEnvironmentService(store)
	schemaSvc := NewSchemaService(store)
	errorSvc := NewErrorService(store)
	apiKeySvc := NewAPIKeyService(store)
	agentRegSvc := NewAgentRegisterService(store)
	aclSvc := NewACLService(store)
	profilerSvc := NewProfilerService(store)
	n1Svc := NewN1Service(store)
	budgetSvc := NewBudgetService(store)
	alertSvc := NewAlertService(store)
	overviewSvc := NewOverviewService(store)
	migrationSvc := NewMigrationService(store)
	auditSvc := NewAuditService(store)
	notificationSvc := NewNotificationService(store)

	return &Services{
		Auth:          authSvc,
		Environment:   envSvc,
		Schema:        schemaSvc,
		Error:         errorSvc,
		APIKey:        apiKeySvc,
		AgentRegister: agentRegSvc,
		ACL:           aclSvc,
		Profiler:      profilerSvc,
		N1:            n1Svc,
		Budget:        budgetSvc,
		Alert:         alertSvc,
		Overview:      overviewSvc,
		Migration:     migrationSvc,
		Audit:         auditSvc,
		Notification:  notificationSvc,
	}
}
