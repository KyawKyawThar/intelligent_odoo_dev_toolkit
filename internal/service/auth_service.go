package service

import (
	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/cache"
	"Intelligent_Dev_ToolKit_Odoo/internal/token"
	"Intelligent_Dev_ToolKit_Odoo/utils"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// Request / Response Types
// =============================================================================

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

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type RefreshTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

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

type UpdateUserRequest struct {
	Email    *string `json:"email,omitempty" validate:"omitempty,email"`
	FullName *string `json:"full_name,omitempty" validate:"omitempty,max=100"`
}

type VerifyEmailRequest struct {
	Token string `json:"token" validate:"required"`
}

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

type TenantResponse struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Slug       string    `json:"slug"`
	Plan       string    `json:"plan"`
	PlanStatus string    `json:"plan_status"`
	CreatedAt  time.Time `json:"created_at"`
}
type LogoutRequest struct {
	SessionID string `json:"session_id,omitempty"` // specific session; empty = current token only
	LogoutAll bool   `json:"logout_all"`           // true = revoke all sessions
}

type SessionResponse struct {
	ID           string    `json:"id"`
	UserAgent    string    `json:"user_agent,omitempty"`
	IPAddress    string    `json:"ip_address,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	LastActiveAt time.Time `json:"last_active_at"`
}

func (s *AuthService) Register(ctx context.Context, req *RegisterRequest, ipAddress, userAgent string) (*RegisterResponse, error) {

	email := strings.ToLower(strings.TrimSpace(req.Email))
	slug := strings.ToLower(strings.TrimSpace(req.TenantSlug))
	if err := utils.ValidatePassword(req.Password, s.config.PasswordMinLength); err != nil {
		return nil, api.ErrValidation(err.Error())
	}
	hashedPassword, err := utils.HashPassword(req.Password)
	if err != nil {
		return nil, api.ErrInternal(fmt.Errorf("hash password: %w", err))
	}
	// Atomic: tenant + owner user + subscription in one transaction
	result, err := s.store.RegisterTenantTx(ctx, db.RegisterTenantParams{
		TenantName:   req.TenantName,
		Slug:         slug,
		Plan:         "cloud",
		OwnerEmail:   email,
		PasswordHash: hashedPassword,
		FullName:     req.FullName,
	})

	if err != nil {
		if api.IsUniqueViolation(err) {
			// The DB tx creates tenant first (slug), then user (email).
			// We check the error detail field returned by api.FromPgError.
			pgAPIErr := api.FromPgError(err)
			for _, d := range pgAPIErr.Details {
				if d.Field == "slug" {
					return nil, api.ErrSlugAlreadyExists()
				}
				if d.Field == "email" {
					return nil, api.ErrEmailAlreadyExists()
				}
			}
			return nil, api.ErrSlugAlreadyExists() // safe fallback (tenant is step 1)
		}
		return nil, api.FromPgError(err)
	}

	// Issue tokens
	accessToken, _, err := s.tokenMaker.CreateToken(result.User.ID.String(), s.config.AccessTokenDuration)
	if err != nil {
		return nil, api.ErrInternal(fmt.Errorf("create access token: %w", err))
	}

	refreshToken, refreshPayload, err := s.tokenMaker.CreateToken(result.User.ID.String(), s.config.RefreshTokenDuration)
	if err != nil {
		return nil, api.ErrInternal(fmt.Errorf("create refresh token: %w", err))
	}

	// Persist session to DB + Redis
	if err := s.persistSession(ctx, db.CreateSessionParams{
		UserID:       result.User.ID,
		TenantID:     result.Tenant.ID,
		RefreshToken: refreshToken,
		UserAgent:    &userAgent,
		IpAddress:    parseIP(ipAddress),
		ExpiresAt:    refreshPayload.ExpiredAt,
	}, ipAddress, userAgent); err != nil {
		return nil, err
	}

	// Send verification email via MailHog (async)
	go s.sendVerificationEmail(context.Background(), result.User.ID.String(), email)

	return &RegisterResponse{
		User:         toUserResponse(result.User),
		Tenant:       toTenantResponse(result.Tenant),
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(s.config.AccessTokenDuration.Seconds()),
		TokenType:    "Bearer",
	}, nil
}

func (s *AuthService) Login(
	ctx context.Context,
	req *LoginRequest,
	ipAddress, userAgent string,
) (*LoginResponse, error) {
	email := strings.ToLower(strings.TrimSpace(req.Email))

	// Brute-force / lockout check
	locked, lockedUntil, err := s.cache.IsLoginLocked(ctx, email)

	if err == nil && locked {
		msg := "Account temporarily locked due to too many failed attempts. Try again later."
		if lockedUntil != nil {
			msg = fmt.Sprintf("Account locked until %s.", lockedUntil.Format(time.RFC3339))
		}
		return nil, api.NewAPIError(api.ErrCodeRateLimited, msg, http.StatusTooManyRequests)
	}

	user, err := s.store.GetUserByEmailGlobal(ctx, email)

	if err != nil {
		s.cache.RecordLoginAttempt(ctx, email, false) //nolint:errcheck
		return nil, api.ErrInvalidCredentials()
	}
	if !user.IsActive {
		return nil, api.NewAPIError(api.ErrCodeForbidden, "Account has been deactivated", http.StatusForbidden)
	}
	if err := utils.CheckPassword(req.Password, user.PasswordHash); err != nil {
		result, _ := s.cache.RecordLoginAttempt(ctx, email, false)
		if result != nil && !result.Allowed {
			msg := "Too many failed login attempts."
			if result.LockoutDuration > 0 {
				msg = fmt.Sprintf("Too many failed attempts. Account locked for %v.", result.LockoutDuration)
			}
			return nil, api.NewAPIError(api.ErrCodeRateLimited, msg, http.StatusTooManyRequests)
		}
		return nil, api.ErrInvalidCredentials()
	}
	// Clear lockout on success
	s.cache.RecordLoginAttempt(ctx, email, true)                  //nolint:errcheck
	go s.store.UpdateUserLastLogin(context.Background(), user.ID) //nolint:errcheck

	// Issue tokens
	accessToken, _, err := s.tokenMaker.CreateToken(user.ID.String(), s.config.AccessTokenDuration)
	if err != nil {
		return nil, api.ErrInternal(fmt.Errorf("create access token: %w", err))
	}

	refreshToken, refreshPayload, err := s.tokenMaker.CreateToken(user.ID.String(), s.config.RefreshTokenDuration)
	if err != nil {
		return nil, api.ErrInternal(fmt.Errorf("create refresh token: %w", err))
	}

	if err := s.persistSession(ctx, db.CreateSessionParams{
		UserID:       user.ID,
		TenantID:     user.TenantID,
		RefreshToken: refreshToken,
		UserAgent:    &userAgent,
		IpAddress:    parseIP(ipAddress),
		ExpiresAt:    refreshPayload.ExpiredAt,
	}, ipAddress, userAgent); err != nil {
		return nil, err
	}

	return &LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(s.config.AccessTokenDuration.Seconds()),
		TokenType:    "Bearer",
		User:         toUserResponseFromGlobal(user),
	}, nil
}

func (s *AuthService) RefreshToken(
	ctx context.Context,
	req *RefreshTokenRequest,
	ipAddress, userAgent string,
) (*RefreshTokenResponse, error) {
	refreshPayload, err := s.tokenMaker.VerifyToken(req.RefreshToken)
	if err != nil {
		return nil, api.ErrInvalidToken("Invalid or expired refresh token")
	}

	blacklisted, _ := s.cache.IsTokenBlacklisted(ctx, refreshPayload.ID.String())
	if blacklisted {
		return nil, api.ErrInvalidToken("Refresh token has been revoked")
	}
	session, err := s.store.GetSessionByToken(ctx, req.RefreshToken)
	if err != nil {
		return nil, api.ErrInvalidToken("Session not found or already expired")
	}
	// Ownership guard
	if session.UserID.String() != refreshPayload.Username {
		return nil, api.ErrInvalidToken("Token/session mismatch")
	}

	if !session.UserIsActive {
		return nil, api.NewAPIError(api.ErrCodeForbidden, "Account has been deactivated", http.StatusForbidden)
	}
	// Rotate — blacklist old token, issue new pair
	s.cache.BlacklistToken(ctx, refreshPayload.ID.String(), refreshPayload.ExpiredAt) //nolint:errcheck

	newAccessToken, _, err := s.tokenMaker.CreateToken(session.UserID.String(), s.config.AccessTokenDuration)
	if err != nil {
		return nil, api.ErrInternal(fmt.Errorf("create access token: %w", err))
	}

	newRefreshToken, newRefreshPayload, err := s.tokenMaker.CreateToken(session.UserID.String(), s.config.RefreshTokenDuration)
	if err != nil {
		return nil, api.ErrInternal(fmt.Errorf("create refresh token: %w", err))
	}
	// Persist new refresh token in DB session
	if err := s.store.UpdateSessionToken(ctx, db.UpdateSessionTokenParams{
		ID:           session.ID,
		RefreshToken: newRefreshToken,
		ExpiresAt:    newRefreshPayload.ExpiredAt,
	}); err != nil {
		return nil, api.FromPgError(err)
	}

	// Refresh Redis entry
	s.cache.DeleteSession(ctx, session.ID.String(), session.UserID.String()) //nolint:errcheck
	_ = s.cache.CreateSession(ctx, &cache.Session{
		ID:           session.ID.String(),
		UserID:       session.UserID.String(),
		TenantID:     session.TenantID.String(),
		RefreshToken: newRefreshToken,
		UserAgent:    userAgent,
		IPAddress:    ipAddress,
		CreatedAt:    session.CreatedAt,
		ExpiresAt:    newRefreshPayload.ExpiredAt,
		LastActiveAt: time.Now().UTC(),
	})

	return &RefreshTokenResponse{
		AccessToken:  newAccessToken,
		RefreshToken: newRefreshToken,
		ExpiresIn:    int64(s.config.AccessTokenDuration.Seconds()),
		TokenType:    "Bearer",
	}, nil
}

func (s *AuthService) Logout(ctx context.Context, accessToken string, req *LogoutRequest) error {
	payload, err := s.tokenMaker.VerifyToken(accessToken)
	if err != nil {
		return nil // already invalid — treat as success
	}
	userID := payload.Username

	if req.LogoutAll {
		if err := s.store.RevokeAllSessions(ctx, uuid.MustParse(userID)); err != nil {
			return api.FromPgError(err)
		}
		return s.cache.DeleteAllUserSessions(ctx, userID)
	}
	// Specific session or current-token-only logout
	if req.SessionID != "" {
		sessUUID, err := uuid.Parse(req.SessionID)
		if err != nil {
			return api.ErrInvalidPathParam("session_id", "must be a valid UUID")
		}
		if err := s.store.RevokeSession(ctx, db.RevokeSessionParams{
			ID:     sessUUID,
			UserID: uuid.MustParse(userID),
		}); err != nil {
			return api.FromPgError(err)
		}
		s.cache.DeleteSession(ctx, req.SessionID, userID) //nolint:errcheck
		return nil
	}

	// No explicit session ID — blacklist the current access token
	s.cache.BlacklistToken(ctx, payload.ID.String(), payload.ExpiredAt) //nolint:errcheck
	return nil
}

func (s *AuthService) ForgotPassword(ctx context.Context, req *ForgotPasswordRequest) error {
	email := strings.ToLower(strings.TrimSpace(req.Email))
	// Always succeed to prevent user enumeration
	user, err := s.store.GetUserByEmailGlobal(ctx, email)
	if err != nil {
		return nil
	}

	tokenBytes := make([]byte, 32)

	if _, err := rand.Read(tokenBytes); err != nil {
		return api.ErrInternal(fmt.Errorf("generate reset token: %w", err))
	}
	tokenStr := hex.EncodeToString(tokenBytes)

	if err := s.cache.StoreResetToken(ctx, &cache.ResetToken{
		UserID:    user.ID.String(),
		Email:     email,
		Token:     tokenStr,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		return api.ErrInternal(fmt.Errorf("store reset token: %w", err))
	}

	// Send via MailHog (async — never block the HTTP response)
	go s.sendPasswordResetEmail(context.Background(), email, tokenStr)

	return nil
}

func (s *AuthService) ResetPassword(ctx context.Context, req *ResetPasswordRequest) error {
	resetToken, err := s.cache.GetResetToken(ctx, req.Token)
	if err != nil || resetToken == nil {
		return api.ErrInvalidToken("Invalid or expired password reset token")
	}

	userID, err := uuid.Parse(resetToken.UserID)
	if err != nil {
		return api.ErrInternal(fmt.Errorf("parse userID from reset token: %w", err))
	}

	if err := utils.ValidatePassword(req.NewPassword, s.config.PasswordMinLength); err != nil {
		return api.ErrValidation(err.Error())
	}

	hashedPassword, err := utils.HashPassword(req.NewPassword)
	if err != nil {
		return api.ErrInternal(fmt.Errorf("hash password: %w", err))
	}
	// Need tenantID for the scoped UpdateUserPassword query
	user, err := s.store.GetUserByIDGlobal(ctx, userID)
	if err != nil {
		if api.IsRecordNotFound(err) {
			return api.ErrUserNotFound()
		}
		return api.FromPgError(err)
	}

	if err := s.store.UpdateUserPassword(ctx, db.UpdateUserPasswordParams{
		ID:           userID,
		TenantID:     user.TenantID,
		PasswordHash: hashedPassword,
	}); err != nil {
		return api.FromPgError(err)
	}

	// One-time token — consume it
	s.cache.DeleteResetToken(ctx, req.Token) //nolint:errcheck

	// Invalidate all sessions (force re-login everywhere)
	s.store.RevokeAllSessions(ctx, userID)              //nolint:errcheck
	s.cache.DeleteAllUserSessions(ctx, userID.String()) //nolint:errcheck

	return nil
}

func (s *AuthService) ChangePassword(
	ctx context.Context,
	userID, tenantID uuid.UUID,
	req *ChangePasswordRequest,
) error {

	user, err := s.store.GetUserByID(ctx, db.GetUserByIDParams{
		ID:       userID,
		TenantID: tenantID,
	})
	if err != nil {
		if api.IsRecordNotFound(err) {
			return api.ErrUserNotFound()
		}
		return api.FromPgError(err)
	}

	if err := utils.CheckPassword(req.CurrentPassword, user.PasswordHash); err != nil {
		return api.ErrValidation("Current password is incorrect").
			WithDetail("current_password", "does not match")
	}
	// Guard against same password reuse
	if err := utils.CheckPassword(req.NewPassword, user.PasswordHash); err == nil {
		return api.ErrValidation("New password must be different from the current password")
	}

	if err := utils.ValidatePassword(req.NewPassword, s.config.PasswordMinLength); err != nil {
		return api.ErrValidation(err.Error())
	}

	hashedPassword, err := utils.HashPassword(req.NewPassword)
	if err != nil {
		return api.ErrInternal(fmt.Errorf("hash password: %w", err))
	}

	if err := s.store.UpdateUserPassword(ctx, db.UpdateUserPasswordParams{
		ID:           userID,
		TenantID:     tenantID,
		PasswordHash: hashedPassword,
	}); err != nil {
		return api.FromPgError(err)
	}

	// Revoke all sessions so all devices must re-login
	s.store.RevokeAllSessions(ctx, userID)              //nolint:errcheck
	s.cache.DeleteAllUserSessions(ctx, userID.String()) //nolint:errcheck

	return nil
}

func (s *AuthService) VerifyEmail(ctx context.Context, req *VerifyEmailRequest) error {
	verifyToken, err := s.cache.GetVerifyEmailToken(ctx, req.Token)
	if err != nil || verifyToken == nil {
		return api.ErrInvalidToken("Invalid or expired email verification token")
	}

	userID, err := uuid.Parse(verifyToken.UserID)
	if err != nil {
		return api.ErrInternal(fmt.Errorf("parse userID from verify token: %w", err))
	}

	user, err := s.store.GetUserByIDGlobal(ctx, userID)
	if err != nil {
		if api.IsRecordNotFound(err) {
			return api.ErrUserNotFound()
		}
		return api.FromPgError(err)
	}

	if !user.EmailVerified {
		if err := s.store.VerifyUserEmail(ctx, db.VerifyUserEmailParams{
			ID:       userID,
			TenantID: user.TenantID,
		}); err != nil {
			return api.FromPgError(err)
		}
	}

	// One-time use — always delete
	s.cache.DeleteVerifyEmailToken(ctx, req.Token) //nolint:errcheck
	return nil
}

// ResendVerificationEmail sends a fresh verification email to an unverified user.
func (s *AuthService) ResendVerificationEmail(ctx context.Context, userID, tenantID uuid.UUID) error {
	user, err := s.store.GetUserByID(ctx, db.GetUserByIDParams{
		ID:       userID,
		TenantID: tenantID,
	})
	if err != nil {
		if api.IsRecordNotFound(err) {
			return api.ErrUserNotFound()
		}
		return api.FromPgError(err)
	}

	if user.EmailVerified {
		return api.NewAPIError(api.ErrCodeConflict, "Email is already verified", http.StatusConflict)
	}

	go s.sendVerificationEmail(context.Background(), userID.String(), user.Email)
	return nil
}

func (s *AuthService) GetCurrentUser(ctx context.Context, userID, tenantID uuid.UUID) (*UserResponse, error) {
	user, err := s.store.GetUserByID(ctx, db.GetUserByIDParams{
		ID:       userID,
		TenantID: tenantID,
	})
	if err != nil {
		if api.IsRecordNotFound(err) {
			return nil, api.ErrUserNotFound()
		}
		return nil, api.FromPgError(err)
	}
	return toUserResponse(user), nil
}

func (s *AuthService) UpdateCurrentUser(
	ctx context.Context,
	userID, tenantID uuid.UUID,
	req *UpdateUserRequest,
) (*UserResponse, error) {

	params := db.UpdateUserProfileParams{
		ID:       userID,
		TenantID: tenantID,
	}
	if req.FullName != nil {
		params.FullName = req.FullName
	}
	if req.Email != nil {
		e := strings.ToLower(strings.TrimSpace(*req.Email))
		params.Email = e
	}

	user, err := s.store.UpdateUserProfile(ctx, params)
	if err != nil {
		if api.IsUniqueViolation(err) {
			return nil, api.ErrEmailAlreadyExists()
		}
		if api.IsRecordNotFound(err) {
			return nil, api.ErrUserNotFound()
		}
		return nil, api.FromPgError(err)
	}

	// Email changed → trigger a new verification email
	if req.Email != nil {
		go s.sendVerificationEmail(context.Background(), userID.String(), user.Email)
	}

	return toUserResponse(user), nil
}

func (s *AuthService) GetUserSessions(ctx context.Context, userID uuid.UUID) ([]*SessionResponse, error) {
	// Fast path: Redis
	cacheSessions, err := s.cache.GetUserSessions(ctx, userID.String())
	if err == nil && len(cacheSessions) > 0 {
		out := make([]*SessionResponse, 0, len(cacheSessions))
		for _, cs := range cacheSessions {
			out = append(out, cacheSessionToResponse(cs))
		}
		return out, nil
	}

	// Fallback: DB (e.g. after a Redis flush)
	dbSessions, err := s.store.ListSessions(ctx, userID)
	if err != nil {
		return nil, api.FromPgError(err)
	}

	out := make([]*SessionResponse, 0, len(dbSessions))
	for _, ds := range dbSessions {
		out = append(out, dbSessionToResponse(ds))
	}
	return out, nil
}
func (s *AuthService) RevokeSession(ctx context.Context, userID uuid.UUID, sessionID string) error {
	sessUUID, err := uuid.Parse(sessionID)
	if err != nil {
		return api.ErrInvalidPathParam("session_id", "must be a valid UUID")
	}

	if err := s.store.RevokeSession(ctx, db.RevokeSessionParams{
		ID:     sessUUID,
		UserID: userID,
	}); err != nil {
		if api.IsRecordNotFound(err) {
			return api.ErrNotFound("Session")
		}
		return api.FromPgError(err)
	}

	s.cache.DeleteSession(ctx, sessionID, userID.String()) //nolint:errcheck
	return nil
}
func (s *AuthService) ValidateAccessToken(ctx context.Context, tokenStr string) (*token.Payload, error) {
	payload, err := s.tokenMaker.VerifyToken(tokenStr)
	if err != nil {
		return nil, api.ErrInvalidToken("")
	}

	blacklisted, err := s.cache.IsTokenBlacklisted(ctx, payload.ID.String())
	if err == nil && blacklisted {
		return nil, api.ErrInvalidToken("Token has been revoked")
	}

	return payload, nil
}
