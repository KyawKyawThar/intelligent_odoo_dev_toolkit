package service

import (
	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/cache"
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"
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

func (s *AuthService) Register(ctx context.Context, req *dto.RegisterRequest, ipAddress, userAgent string) (*dto.RegisterResponse, error) {

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

	// Verify email can be delivered before proceeding further.
	if err := s.sendVerificationEmail(ctx, result.User.ID.String(), email); err != nil {
		_ = s.store.DeleteTenant(ctx, result.Tenant.ID)
		return nil, api.ErrInternal(fmt.Errorf("send verification email: %w", err))
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

	return &dto.RegisterResponse{
		User:         dto.ToUserResponse(result.User),
		Tenant:       dto.ToTenantResponse(result.Tenant),
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(s.config.AccessTokenDuration.Seconds()),
		TokenType:    "Bearer",
	}, nil
}

func (s *AuthService) Login(
	ctx context.Context,
	req *dto.LoginRequest,
	ipAddress, userAgent string,
) (*dto.LoginResponse, error) {
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
	if !user.EmailVerified {
		return nil, api.NewAPIError(api.ErrCodeForbidden, "Please verify your email address before logging in", http.StatusForbidden)
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

	return &dto.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(s.config.AccessTokenDuration.Seconds()),
		TokenType:    "Bearer",
		User:         dto.ToUserResponseFromGlobal(user),
	}, nil
}

func (s *AuthService) RefreshToken(
	ctx context.Context,
	req *dto.RefreshTokenRequest,
	ipAddress, userAgent string,
) (*dto.RefreshTokenResponse, error) {
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

	return &dto.RefreshTokenResponse{
		AccessToken:  newAccessToken,
		RefreshToken: newRefreshToken,
		ExpiresIn:    int64(s.config.AccessTokenDuration.Seconds()),
		TokenType:    "Bearer",
	}, nil
}

func (s *AuthService) Logout(ctx context.Context, accessToken string, req *dto.LogoutRequest) error {
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

func (s *AuthService) ForgotPassword(ctx context.Context, req *dto.ForgotPasswordRequest) error {
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

func (s *AuthService) ResetPassword(ctx context.Context, req *dto.ResetPasswordRequest) error {
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
	req *dto.ChangePasswordRequest,
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

func (s *AuthService) VerifyEmail(ctx context.Context, req *dto.VerifyEmailRequest) error {
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

	// send synchronously so errors bubble up
	if err := s.sendVerificationEmail(ctx, userID.String(), user.Email); err != nil {
		return api.ErrInternal(fmt.Errorf("send verification email: %w", err))
	}
	return nil
}

func (s *AuthService) GetCurrentUser(ctx context.Context, userID, tenantID uuid.UUID) (*dto.UserResponse, error) {
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
	return dto.ToUserResponse(user), nil
}

func (s *AuthService) UpdateCurrentUser(
	ctx context.Context,
	userID, tenantID uuid.UUID,
	req *dto.UpdateUserRequest,
) (*dto.UserResponse, error) {
	// Get the current user state before any updates
	userBeforeUpdate, err := s.store.GetUserByID(ctx, db.GetUserByIDParams{ID: userID, TenantID: tenantID})
	if err != nil {
		if api.IsRecordNotFound(err) {
			return nil, api.ErrUserNotFound()
		}
		return nil, api.FromPgError(err)
	}

	params := db.UpdateUserProfileParams{
		ID:       userID,
		TenantID: tenantID,
	}
	emailChanged := false
	var newEmail string

	if req.FullName != nil {
		fullName := strings.TrimSpace(*req.FullName)
		if fullName != "" {
			params.FullName = &fullName
		}
	}

	if req.Email != nil {
		email := strings.ToLower(strings.TrimSpace(*req.Email))
		if email != "" && email != userBeforeUpdate.Email {
			params.Email = &email
			newEmail = email
			emailChanged = true
		}
	}

	// a transaction would be better here
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

	if emailChanged {
		// Email was changed, now un-verify it and send a new verification email.
		err = s.store.UnverifyUserEmail(ctx, db.UnverifyUserEmailParams{
			ID:       userID,
			TenantID: tenantID,
		})
		if err != nil {
			// Attempt to roll back the email change on failure.
			// This is best-effort and highlights why a transaction would be ideal.
			_, _ = s.store.UpdateUserProfile(ctx, db.UpdateUserProfileParams{
				ID:       userID,
				TenantID: tenantID,
				Email:    &userBeforeUpdate.Email,
			})
			return nil, api.FromPgError(err)
		}
		user.EmailVerified = false // Reflect the change in the returned object

		if err := s.sendVerificationEmail(ctx, userID.String(), newEmail); err != nil {
			return nil, api.ErrInternal(fmt.Errorf("send verification email: %w", err))
		}
	}

	return dto.ToUserResponse(user), nil
}

func (s *AuthService) GetUserSessions(ctx context.Context, userID uuid.UUID) ([]*dto.SessionResponse, error) {
	// Fast path: Redis
	cacheSessions, err := s.cache.GetUserSessions(ctx, userID.String())
	if err == nil && len(cacheSessions) > 0 {
		out := make([]*dto.SessionResponse, 0, len(cacheSessions))
		for _, cs := range cacheSessions {
			out = append(out, dto.CacheSessionToResponse(cs))
		}
		return out, nil
	}

	// Fallback: DB (e.g. after a Redis flush)
	dbSessions, err := s.store.ListSessions(ctx, userID)
	if err != nil {
		return nil, api.FromPgError(err)
	}

	out := make([]*dto.SessionResponse, 0, len(dbSessions))
	for _, ds := range dbSessions {
		out = append(out, dto.DbSessionToResponse(ds))
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
