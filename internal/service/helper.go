package service

import (
	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/cache"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/smtp"

	"time"
)

// persistSession writes a session to both PostgreSQL and Redis.
func (s *AuthService) persistSession(
	ctx context.Context,
	params db.CreateSessionParams,
	ipAddress, userAgent string,
) error {
	dbSession, err := s.store.CreateSession(ctx, params)
	if err != nil {
		return api.FromPgError(err)
	}

	_ = s.cache.CreateSession(ctx, &cache.Session{
		ID:           dbSession.ID.String(),
		UserID:       params.UserID.String(),
		TenantID:     params.TenantID.String(),
		RefreshToken: params.RefreshToken,
		UserAgent:    userAgent,
		IPAddress:    ipAddress,
		CreatedAt:    dbSession.CreatedAt,
		ExpiresAt:    dbSession.ExpiresAt,
		LastActiveAt: dbSession.LastUsedAt,
	})

	return nil
}

// =============================================================================
// Email delivery — MailHog SMTP (no auth)
// =============================================================================

// sendVerificationEmail generates a one-time token, stores it in Redis,
// and delivers an email via MailHog. An error is returned if delivery fails
// so callers (such as registration) can rollback any database changes.
func (s *AuthService) sendVerificationEmail(ctx context.Context, userID, email string) error {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		fmt.Printf("[email] ERROR generating verify token for %s: %v\n", email, err)
		return err
	}
	tokenStr := hex.EncodeToString(tokenBytes)

	if err := s.cache.StoreVerifyEmailToken(ctx, &cache.VerifyEmailToken{
		UserID:    userID,
		Email:     email,
		Token:     tokenStr,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		fmt.Printf("[email] ERROR storing verify token for %s: %v\n", email, err)
		return err
	}

	verifyURL := fmt.Sprintf(
		"%s/api/v1/auth/verify-email?token=%s",
		s.config.AppBaseURL, tokenStr,
	)
	body := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: Verify your email address\r\n\r\n"+
			"Hello,\r\n\r\n"+
			"Please verify your email address by visiting the link below:\r\n\r\n"+
			"  %s\r\n\r\n"+
			"This link expires in 24 hours.\r\n\r\n"+
			"If you did not create an account, you can safely ignore this email.\r\n",
		s.config.SMTPFrom, email, verifyURL,
	)

	addr := fmt.Sprintf("%s:%d", s.config.SMTPHost, s.config.SMTPPort)
	//gosec:G107
	if err := smtp.SendMail(addr, nil, s.config.SMTPFrom, []string{email}, []byte(body)); err != nil {
		fmt.Printf("[email] ERROR sending verify email to %s via %s: %v\n", email, addr, err)
		return err
	}
	fmt.Printf("[email] Sent verify email to %s (via MailHog %s)\n", email, addr)
	return nil
}

// sendPasswordResetEmail sends the reset link via MailHog.
func (s *AuthService) sendPasswordResetEmail(_ context.Context, email, resetToken string) {
	resetURL := fmt.Sprintf(
		"%s/reset-password?token=%s",
		s.config.AppBaseURL, resetToken,
	)
	body := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: Reset your password\r\n\r\n"+
			"Hello,\r\n\r\n"+
			"Click the link below to reset your password:\r\n\r\n"+
			"  %s\r\n\r\n"+
			"This link expires in 1 hour.\r\n\r\n"+
			"If you did not request a password reset, please ignore this email.\r\n",
		s.config.SMTPFrom, email, resetURL,
	)

	addr := fmt.Sprintf("%s:%d", s.config.SMTPHost, s.config.SMTPPort)
	//gosec:G107
	if err := smtp.SendMail(addr, nil, s.config.SMTPFrom, []string{email}, []byte(body)); err != nil {
		fmt.Printf("[email] ERROR sending reset email to %s via %s: %v\n", email, addr, err)
		return
	}
	fmt.Printf("[email] Sent password reset email to %s (via MailHog %s)\n", email, addr)
}
