// Package dto contains data transfer objects.
package dto

import (
	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/cache"
	"time"

	"github.com/google/uuid"
)

// MessageResponse is a generic JSON envelope carrying a single message string.
type MessageResponse struct {
	Message string `json:"message"`
}

// NewMessageResponse creates a MessageResponse with the given text.
func NewMessageResponse(msg string) *MessageResponse {
	return &MessageResponse{Message: msg}
}

// RegisterRequest carries the fields needed to create a new tenant + owner user.
type RegisterRequest struct {
	Email      string `json:"email"       validate:"required,email"`
	Password   string `json:"password"    validate:"required,min=8"`
	TenantName string `json:"tenant_name" validate:"required,min=2,max=100"`
	TenantSlug string `json:"tenant_slug" validate:"required,min=2,max=50,slug"`
	FullName   string `json:"full_name"   validate:"omitempty,max=100"`
}

// RegisterResponse is returned after successful registration with user, tenant, and tokens.
type RegisterResponse struct {
	User         *UserResponse   `json:"user"`
	Tenant       *TenantResponse `json:"tenant"`
	AccessToken  string          `json:"access_token"`
	RefreshToken string          `json:"refresh_token"`
	ExpiresIn    int64           `json:"expires_in"`
	TokenType    string          `json:"token_type"`
}

// --- Login ---

// LoginRequest carries email and password for authentication.
type LoginRequest struct {
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// LoginResponse is returned after successful authentication with tokens and user info.
type LoginResponse struct {
	AccessToken  string        `json:"access_token"`
	RefreshToken string        `json:"refresh_token"`
	ExpiresIn    int64         `json:"expires_in"`
	TokenType    string        `json:"token_type"`
	User         *UserResponse `json:"user"`
}

// --- Token Refresh ---

// RefreshTokenRequest carries the refresh token to obtain a new access token.
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// RefreshTokenResponse is returned after a successful token refresh.
type RefreshTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// --- Logout ---

// LogoutRequest optionally targets a specific session or all sessions for the user.
type LogoutRequest struct {
	SessionID string `json:"session_id,omitempty"` // specific session; empty = current token only
	LogoutAll bool   `json:"logout_all"`           // true = revoke all sessions
}

// --- Password Management ---

// ForgotPasswordRequest initiates a password-reset flow for the given email.
type ForgotPasswordRequest struct {
	Email string `json:"email" validate:"required,email"`
}

// ResetPasswordRequest completes a password reset using a one-time token.
type ResetPasswordRequest struct {
	Token       string `json:"token"        validate:"required"`
	NewPassword string `json:"new_password" validate:"required,min=8"`
}

// ChangePasswordRequest lets an authenticated user update their password.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" validate:"required"`
	NewPassword     string `json:"new_password"     validate:"required,min=8"`
}

// --- Email Verification ---

// VerifyEmailRequest carries the verification token sent to the user's inbox.
type VerifyEmailRequest struct {
	Token string `json:"token" validate:"required"`
}

// --- User Profile ---

// UpdateUserRequest carries optional fields a user may change on their own profile.
type UpdateUserRequest struct {
	Email    *string `json:"email,omitempty"     validate:"omitempty,email"`
	FullName *string `json:"full_name,omitempty" validate:"omitempty,max=100"`
}

// --- Sessions ---

// SessionResponse represents a single active session returned to the client.
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

// UserResponse is the public representation of a user returned by the API.
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

// UserListItem is a trimmed user representation used in list/search endpoints.
type UserListItem struct {
	ID            string     `json:"id"`
	Email         string     `json:"email"`
	FullName      *string    `json:"full_name"`
	EmailVerified bool       `json:"email_verified"`
	IsActive      bool       `json:"is_active"`
	CreatedAt     time.Time  `json:"created_at"`
	LastLoginAt   *time.Time `json:"last_login_at,omitempty"`
}

// CreateUserRequest carries the fields an admin needs to create a new user.
type CreateUserRequest struct {
	Email    string  `json:"email"     validate:"required,email"`
	Password string  `json:"password"  validate:"required,min=8"`
	FullName *string `json:"full_name" validate:"omitempty,max=100"`
}

// AdminUpdateUserRequest carries optional fields an admin may change on any user.
type AdminUpdateUserRequest struct {
	Email         *string `json:"email,omitempty"          validate:"omitempty,email"`
	FullName      *string `json:"full_name,omitempty"      validate:"omitempty,max=100"`
	EmailVerified *bool   `json:"email_verified,omitempty"`
	IsActive      *bool   `json:"is_active,omitempty"`
}

// UserGlobalRowResponse maps the cross-tenant user lookup row including tenant metadata.
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

// ToUserResponse maps db.User to a client-facing UserResponse.
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

// ToUserResponseFromGlobal maps a global-lookup row to a client-facing UserResponse.
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

// ToUserGlobalRowResponse maps a global-lookup row to a UserGlobalRowResponse.
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

// ToTenantResponse maps db.Tenant to a client-facing TenantResponse.
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

// CacheSessionToResponse converts a cached session into a SessionResponse.
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

// DbSessionToResponse converts a database session row into a SessionResponse.
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
