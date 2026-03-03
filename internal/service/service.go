package service

import (
	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/cache"
	"Intelligent_Dev_ToolKit_Odoo/internal/token"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Services contains all the services for the application.
type Services struct {
	Auth *AuthService
}

func NewServices(store db.Store, cache *cache.RedisClient, tokenMaker token.Maker, cfg *AuthConfig) *Services {
	return &Services{
		Auth: NewAuthService(store, cache, tokenMaker, cfg),
	}
}

type AuthConfig struct {
	AccessTokenDuration  time.Duration
	RefreshTokenDuration time.Duration
	PasswordMinLength    int
	BcryptCost           int

	// MailHog / SMTP — no auth required for MailHog
	SMTPHost   string // e.g. "localhost"
	SMTPPort   int    // MailHog default: 1025
	SMTPFrom   string // e.g. "noreply@odoodevtools.com"
	AppBaseURL string // e.g. "http://localhost:8080" — used to build verify/reset links
}

type AuthService struct {
	store      db.Store
	cache      *cache.RedisClient
	tokenMaker token.Maker
	config     *AuthConfig
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
func NewAuthService(store db.Store, cache *cache.RedisClient, tokenMaker token.Maker, cfg *AuthConfig) *AuthService {
	if cfg == nil {
		cfg = DefaultAuthConfig()
	}
	return &AuthService{
		store:      store,
		cache:      cache,
		tokenMaker: tokenMaker,
		config:     cfg,
	}
}

// Helper for not-yet-implemented handlers
func (s *AuthService) NotImplemented(w http.ResponseWriter, r *http.Request) {
	api.HandleError(w, r, api.NewAPIError(
		api.ErrCodeInternal,
		"This endpoint is not yet implemented",
		http.StatusNotImplemented,
	))
}
func (s *AuthService) ServiceVersion(w http.ResponseWriter, r *http.Request) {
	api.WriteJSON(w, http.StatusOK, map[string]string{
		"version":     "1.0.0",
		"api_version": "v1",
		"go_version":  "1.21",
	})
}
