package service

import (
	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/cache"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/smtp"
	"time"

	"github.com/rs/zerolog/log"
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

	_ = s.cache.CreateSession(ctx, &cache.Session{ //nolint:errcheck // fire-and-forget cache op.
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

// sendVerificationEmail generates a 6-digit verification code, stores it in
// Redis, and delivers it via SMTP. An error is returned if delivery fails so
// callers (such as registration) can rollback any database changes.
func (s *AuthService) sendVerificationEmail(ctx context.Context, userID, email string) error {
	code, err := generate6DigitCode()
	if err != nil {
		log.Error().Err(err).Str("email", email).Msg("failed to generate verification code")
		return err
	}

	if err := s.cache.StoreVerifyEmailToken(ctx, &cache.VerifyEmailToken{
		UserID:    userID,
		Email:     email,
		Code:      code,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		log.Error().Err(err).Str("email", email).Msg("failed to store verification code")
		return err
	}

	from := s.config.SMTPFrom
	body := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: Verify your email address\r\n\r\n"+
			"Hello,\r\n\r\n"+
			"Your email verification code is:\r\n\r\n"+
			"  %s\r\n\r\n"+
			"Enter this 6-digit code to verify your email address.\r\n\r\n"+
			"This code expires in 10 minutes.\r\n\r\n"+
			"If you did not create an account, you can safely ignore this email.\r\n",
		from, email, code,
	)

	addr, auth := s.config.smtpSettings()
	//gosec:G107
	if err := smtp.SendMail(addr, auth, from, []string{email}, []byte(body)); err != nil {
		log.Error().Err(err).Str("email", email).Str("addr", addr).Msg("failed to send verify email")
		return err
	}
	log.Info().Str("email", email).Str("addr", addr).Str("env", s.config.Environment).Msg("sent verification code email")
	return nil
}

// buildSMTPAuth returns PlainAuth when credentials are set, nil otherwise
// (nil = unauthenticated relay, e.g. MailHog in development).
func buildSMTPAuth(username, password, host string) smtp.Auth {
	if username == "" {
		return nil
	}
	return smtp.PlainAuth("", username, password, host)
}

// generate6DigitCode returns a cryptographically random 6-digit numeric string.
func generate6DigitCode() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	// Convert to uint32 and mod 1_000_000 to get a 6-digit number
	n := (uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])) % 1_000_000
	return fmt.Sprintf("%06d", n), nil
}

// sendPasswordResetEmail sends the reset link via SMTP.
func (s *AuthService) sendPasswordResetEmail(_ context.Context, email, resetToken string) {
	resetURL := fmt.Sprintf("%s/reset-password?token=%s", s.config.AppBaseURL, resetToken)
	from := s.config.SMTPFrom
	body := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: Reset your password\r\n\r\n"+
			"Hello,\r\n\r\n"+
			"Click the link below to reset your password:\r\n\r\n"+
			"  %s\r\n\r\n"+
			"This link expires in 1 hour.\r\n\r\n"+
			"If you did not request a password reset, please ignore this email.\r\n",
		from, email, resetURL,
	)

	addr, auth := s.config.smtpSettings()
	//gosec:G107
	if err := smtp.SendMail(addr, auth, from, []string{email}, []byte(body)); err != nil {
		log.Error().Err(err).Str("email", email).Str("addr", addr).Msg("failed to send reset email")
		return
	}
	log.Info().Str("email", email).Str("addr", addr).Str("env", s.config.Environment).Msg("sent password reset email")
}
func defaultFeatureFlags(envType string) json.RawMessage {
	switch envType {
	case "development":
		return json.RawMessage(`{
			"sampling_mode": "full",
			"sample_rate": 1.0,
			"slow_threshold_ms": 50,
			"collect_orm": true,
			"collect_sql": true,
			"collect_errors": true,
			"collect_profiler": true,
			"max_events_per_batch": 1000,
			"max_bytes_per_minute": 5242880,
			"flush_interval_sec": 10,
			"strip_pii": false,
			"redact_fields": []
		}`)
	case "staging":
		return json.RawMessage(`{
			"sampling_mode": "sampled",
			"sample_rate": 0.25,
			"slow_threshold_ms": 100,
			"collect_orm": true,
			"collect_sql": true,
			"collect_errors": true,
			"collect_profiler": true,
			"max_events_per_batch": 500,
			"max_bytes_per_minute": 1048576,
			"flush_interval_sec": 30,
			"strip_pii": true,
			"redact_fields": ["partner_name", "email", "phone"]
		}`)
	default: // production
		return json.RawMessage(`{
			"sampling_mode": "aggregated_only",
			"sample_rate": 0.05,
			"slow_threshold_ms": 200,
			"collect_orm": true,
			"collect_sql": false,
			"collect_errors": true,
			"collect_profiler": false,
			"max_events_per_batch": 200,
			"max_bytes_per_minute": 524288,
			"flush_interval_sec": 60,
			"strip_pii": true,
			"redact_fields": ["partner_name", "email", "phone", "vat", "street"]
		}`)
	}
}
