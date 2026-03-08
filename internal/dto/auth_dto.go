// Package dto contains data transfer objects.
package dto

import (
	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/cache"
	"time"

	"github.com/google/uuid"
)

type MessageResponse struct {
	Message string `json:"message"`
}

func NewMessageResponse(msg string) *MessageResponse {
	return &MessageResponse{Message: msg}
}

type RegisterRequest struct {
	Email      string `json:"email"       validate:"required,email"`
	Password   string `json:"password"    validate:"required,min=8"`
	TenantName string `json:"tenant_name" validate:"required,min=2,max=100"`
	TenantSlug string `json:"tenant_slug" validate:"required,min=2,max=50,slug"`
	FullName   string `json:"full_name"   validate:"omitempty,max=100"`
}

type RegisterResponse struct {
	User         *UserResponse   `json:"user"`
	Tenant       *TenantResponse `json:"tenant"`
	AccessToken  string          `json:"access_token"`
	RefreshToken string          `json:"refresh_token"`
	ExpiresIn    int64           `json:"expires_in"`
	TokenType    string          `json:"token_type"`
}

// --- Login ---

type LoginRequest struct {
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type LoginResponse struct {
	AccessToken  string        `json:"access_token"`
	RefreshToken string        `json:"refresh_token"`
	ExpiresIn    int64         `json:"expires_in"`
	TokenType    string        `json:"token_type"`
	User         *UserResponse `json:"user"`
}

// --- Token Refresh ---

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type RefreshTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// --- Logout ---

type LogoutRequest struct {
	SessionID string `json:"session_id,omitempty"` // specific session; empty = current token only
	LogoutAll bool   `json:"logout_all"`           // true = revoke all sessions
}

// --- Password Management ---

type ForgotPasswordRequest struct {
	Email string `json:"email" validate:"required,email"`
}

type ResetPasswordRequest struct {
	Token       string `json:"token"        validate:"required"`
	NewPassword string `json:"new_password" validate:"required,min=8"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" validate:"required"`
	NewPassword     string `json:"new_password"     validate:"required,min=8"`
}

// --- Email Verification ---

type VerifyEmailRequest struct {
	Token string `json:"token" validate:"required"`
}

// --- User Profile ---

type UpdateUserRequest struct {
	Email    *string `json:"email,omitempty"     validate:"omitempty,email"`
	FullName *string `json:"full_name,omitempty" validate:"omitempty,max=100"`
}

// --- Sessions ---

type SessionResponse struct {
	ID           string    `json:"id"`
	UserAgent    string    `json:"user_agent,omitempty"`
	IPAddress    string    `json:"ip_address,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	LastActiveAt time.Time `json:"last_active_at"`
}

// =============================================================================
// User DTOs
// =============================================================================

type UserResponse struct {
	ID            string     `json:"id"`
	Email         string     `json:"email"`
	FullName      *string    `json:"full_name"`
	TenantID      string     `json:"tenant_id"`
	EmailVerified bool       `json:"email_verified"`
	IsActive      bool       `json:"is_active"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	LastLoginAt   *time.Time `json:"last_login_at,omitempty"`
}

type UserListItem struct {
	ID            string     `json:"id"`
	Email         string     `json:"email"`
	FullName      *string    `json:"full_name"`
	EmailVerified bool       `json:"email_verified"`
	IsActive      bool       `json:"is_active"`
	CreatedAt     time.Time  `json:"created_at"`
	LastLoginAt   *time.Time `json:"last_login_at,omitempty"`
}

type CreateUserRequest struct {
	Email    string  `json:"email"     validate:"required,email"`
	Password string  `json:"password"  validate:"required,min=8"`
	FullName *string `json:"full_name" validate:"omitempty,max=100"`
}

type AdminUpdateUserRequest struct {
	Email         *string `json:"email,omitempty"          validate:"omitempty,email"`
	FullName      *string `json:"full_name,omitempty"      validate:"omitempty,max=100"`
	EmailVerified *bool   `json:"email_verified,omitempty"`
	IsActive      *bool   `json:"is_active,omitempty"`
}

type UserGlobalRowResponse struct {
	ID            uuid.UUID  `db:"id" json:"id"`
	TenantID      uuid.UUID  `db:"tenant_id" json:"tenant_id"`
	Email         string     `db:"email" json:"email"`
	PasswordHash  string     `db:"password_hash" json:"password_hash"`
	FullName      *string    `db:"full_name" json:"full_name"`
	EmailVerified bool       `db:"email_verified" json:"email_verified"`
	IsActive      bool       `db:"is_active" json:"is_active"`
	LastLoginAt   *time.Time `db:"last_login_at" json:"last_login_at"`
	CreatedAt     time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time  `db:"updated_at" json:"updated_at"`
	TenantSlug    string     `db:"tenant_slug" json:"tenant_slug"`
	TenantPlan    string     `db:"tenant_plan" json:"tenant_plan"`
}

// toUserResponse maps db.User → UserResponse.
func ToUserResponse(u db.User) *UserResponse {
	return &UserResponse{
		ID:       u.ID.String(),
		Email:    u.Email,
		FullName: u.FullName,

		TenantID:      u.TenantID.String(),
		EmailVerified: u.EmailVerified,
		IsActive:      u.IsActive,
		CreatedAt:     u.CreatedAt,
		UpdatedAt:     u.UpdatedAt,
		LastLoginAt:   u.LastLoginAt,
	}
}
func ToUserResponseFromGlobal(u db.GetUserByEmailGlobalRow) *UserResponse {
	return &UserResponse{
		ID:            u.ID.String(),
		Email:         u.Email,
		FullName:      u.FullName,
		TenantID:      u.TenantID.String(),
		EmailVerified: u.EmailVerified,
		IsActive:      u.IsActive,
		CreatedAt:     u.CreatedAt,
		UpdatedAt:     u.UpdatedAt,
		LastLoginAt:   u.LastLoginAt,
	}
}

// UserGlobalRowResponse.
func ToUserGlobalRowResponse(u db.GetUserByEmailGlobalRow) *UserGlobalRowResponse {
	return &UserGlobalRowResponse{
		ID:            u.ID,
		TenantID:      u.TenantID,
		Email:         u.Email,
		PasswordHash:  u.PasswordHash,
		FullName:      u.FullName,
		EmailVerified: u.EmailVerified,
		IsActive:      u.IsActive,
		LastLoginAt:   u.LastLoginAt,
		CreatedAt:     u.CreatedAt,
		UpdatedAt:     u.UpdatedAt,
		TenantSlug:    u.TenantSlug,
		TenantPlan:    u.TenantPlan,
	}
}

// toTenantResponse maps db.Tenant → TenantResponse.
func ToTenantResponse(t db.Tenant) *TenantResponse {
	return &TenantResponse{
		ID:         t.ID.String(),
		Name:       t.Name,
		Slug:       t.Slug,
		Plan:       t.Plan,
		PlanStatus: t.PlanStatus,
		CreatedAt:  t.CreatedAt,
	}
}

func CacheSessionToResponse(s *cache.Session) *SessionResponse {
	return &SessionResponse{
		ID:           s.ID,
		UserAgent:    s.UserAgent,
		IPAddress:    s.IPAddress,
		CreatedAt:    s.CreatedAt,
		ExpiresAt:    s.ExpiresAt,
		LastActiveAt: s.LastActiveAt,
	}
}

func DbSessionToResponse(s db.Session) *SessionResponse {
	r := &SessionResponse{
		ID:           s.ID.String(),
		CreatedAt:    s.CreatedAt,
		ExpiresAt:    s.ExpiresAt,
		LastActiveAt: s.LastUsedAt,
	}
	if s.UserAgent != nil {
		r.UserAgent = *s.UserAgent
	}
	if s.IpAddress != nil {
		r.IPAddress = s.IpAddress.String()
	}
	return r
}
