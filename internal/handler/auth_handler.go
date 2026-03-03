package handler

import (
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/service"
	"net/http"
)

// ---------------------------------------------------------------------------
// POST /api/v1/auth/register
// ---------------------------------------------------------------------------

// HandleRegister creates a new tenant + owner user in a single transaction,
// issues an access + refresh token pair, and triggers a verification email
// via MailHog.
func (h *AuthHandler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	var req service.RegisterRequest
	if apiErr := h.DecodeJSON(r, &req); apiErr != nil {
		h.WriteErr(w, r, apiErr)
		return
	}
	if apiErr := h.ValidateRequest(&req); apiErr != nil {
		h.WriteErr(w, r, apiErr)
		return
	}

	resp, err := h.svc.Register(r.Context(), &req, h.ClientIP(r), r.Header.Get("User-Agent"))
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	api.WriteCreated(w, r, resp)
}

// ---------------------------------------------------------------------------
// POST /api/v1/auth/login
// ---------------------------------------------------------------------------

// HandleLogin authenticates a user by email + password, enforces brute-force
// protection via Redis, and returns a token pair.
func (h *AuthHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	var req service.LoginRequest
	if apiErr := h.DecodeJSON(r, &req); apiErr != nil {
		h.WriteErr(w, r, apiErr)
		return
	}
	if apiErr := h.ValidateRequest(&req); apiErr != nil {
		h.WriteErr(w, r, apiErr)
		return
	}

	resp, err := h.svc.Login(r.Context(), &req, h.ClientIP(r), r.Header.Get("User-Agent"))
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	api.WriteSuccess(w, r, resp)
}

// ---------------------------------------------------------------------------
// POST /api/v1/auth/refresh
// ---------------------------------------------------------------------------

// HandleRefreshToken rotates the refresh token (old one is blacklisted in Redis)
// and issues a new access + refresh pair.
func (h *AuthHandler) HandleRefreshToken(w http.ResponseWriter, r *http.Request) {
	var req service.RefreshTokenRequest
	if apiErr := h.DecodeJSON(r, &req); apiErr != nil {
		h.WriteErr(w, r, apiErr)
		return
	}
	if apiErr := h.ValidateRequest(&req); apiErr != nil {
		h.WriteErr(w, r, apiErr)
		return
	}

	resp, err := h.svc.RefreshToken(r.Context(), &req, h.ClientIP(r), r.Header.Get("User-Agent"))
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	api.WriteSuccess(w, r, resp)
}

// ---------------------------------------------------------------------------
// POST /api/v1/auth/logout
// ---------------------------------------------------------------------------

// HandleLogout invalidates the current session (or all sessions when
// logout_all=true).  Requires a valid Bearer token in the Authorization header.
func (h *AuthHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	BearerToken := h.BearerToken(r)
	if BearerToken == "" {
		h.WriteErr(w, r, api.ErrMissingAuthHeader())
		return
	}

	// Body is optional — default to single-token logout
	var req service.LogoutRequest
	_ = h.DecodeJSON(r, &req) // intentionally ignoring parse errors

	if err := h.svc.Logout(r.Context(), BearerToken, &req); err != nil {
		h.HandleErr(w, r, err)
		return
	}

	api.WriteNoContent(w)
}

// ---------------------------------------------------------------------------
// POST /api/v1/auth/forgot-password
// ---------------------------------------------------------------------------

// HandleForgotPassword always returns HTTP 200 regardless of whether the
// email is registered (prevents user enumeration). An email is sent via
// MailHog when the account exists.
func (h *AuthHandler) HandleForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req service.ForgotPasswordRequest
	if apiErr := h.DecodeJSON(r, &req); apiErr != nil {
		h.WriteErr(w, r, apiErr)
		return
	}
	if apiErr := h.ValidateRequest(&req); apiErr != nil {
		h.WriteErr(w, r, apiErr)
		return
	}

	// Intentionally ignore the error — security requirement
	_ = h.svc.ForgotPassword(r.Context(), &req)

	api.WriteSuccess(w, r, map[string]any{
		"message": "If that email address is registered you will receive a reset link shortly.",
	})
}

// ---------------------------------------------------------------------------
// POST /api/v1/auth/reset-password
// ---------------------------------------------------------------------------

// HandleResetPassword consumes the one-time Redis token and updates the
// user's password.  All existing sessions are revoked on success.
func (h *AuthHandler) HandleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req service.ResetPasswordRequest
	if apiErr := h.DecodeJSON(r, &req); apiErr != nil {
		h.WriteErr(w, r, apiErr)
		return
	}
	if apiErr := h.ValidateRequest(&req); apiErr != nil {
		h.WriteErr(w, r, apiErr)
		return
	}

	if err := h.svc.ResetPassword(r.Context(), &req); err != nil {
		h.HandleErr(w, r, err)
		return
	}

	api.WriteSuccess(w, r, map[string]any{
		"message": "Password has been reset. Please log in with your new password.",
	})
}

// ---------------------------------------------------------------------------
// POST /api/v1/auth/change-password  (authenticated)
// ---------------------------------------------------------------------------

// HandleChangePassword lets a logged-in user change their own password.
// Requires the current password for verification.  All sessions are revoked
// on success (forces re-login on all devices).
func (h *AuthHandler) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.MustTenantID(w, r)
	if !ok {
		return
	}

	var req service.ChangePasswordRequest
	if apiErr := h.DecodeJSON(r, &req); apiErr != nil {
		h.WriteErr(w, r, apiErr)
		return
	}
	if apiErr := h.ValidateRequest(&req); apiErr != nil {
		h.WriteErr(w, r, apiErr)
		return
	}

	if err := h.svc.ChangePassword(r.Context(), userID, tenantID, &req); err != nil {
		h.HandleErr(w, r, err)
		return
	}

	api.WriteSuccess(w, r, map[string]any{
		"message": "Password changed successfully. Please log in again.",
	})
}

// ---------------------------------------------------------------------------
// POST /api/v1/auth/verify-email
// ---------------------------------------------------------------------------

// HandleVerifyEmail consumes the one-time Redis verification token and marks
// the user's email as verified in the database.
func (h *AuthHandler) HandleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req service.VerifyEmailRequest
	if apiErr := h.DecodeJSON(r, &req); apiErr != nil {
		h.WriteErr(w, r, apiErr)
		return
	}
	if apiErr := h.ValidateRequest(&req); apiErr != nil {
		h.WriteErr(w, r, apiErr)
		return
	}

	if err := h.svc.VerifyEmail(r.Context(), &req); err != nil {
		h.HandleErr(w, r, err)
		return
	}

	api.WriteSuccess(w, r, map[string]any{
		"message": "Email verified successfully.",
	})
}

// ---------------------------------------------------------------------------
// POST /api/v1/auth/resend-verification  (authenticated)
// ---------------------------------------------------------------------------

// HandleResendVerification sends a fresh verification email via MailHog.
// Returns 409 if the email is already verified.
func (h *AuthHandler) HandleResendVerification(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.MustTenantID(w, r)
	if !ok {
		return
	}

	if err := h.svc.ResendVerificationEmail(r.Context(), userID, tenantID); err != nil {
		h.HandleErr(w, r, err)
		return
	}

	api.WriteSuccess(w, r, map[string]any{
		"message": "Verification email sent.",
	})
}

// ---------------------------------------------------------------------------
// GET /api/v1/auth/me  (authenticated)
// ---------------------------------------------------------------------------

// HandleGetCurrentUser returns the profile of the authenticated user.
func (h *AuthHandler) HandleGetCurrentUser(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.MustTenantID(w, r)
	if !ok {
		return
	}

	user, err := h.svc.GetCurrentUser(r.Context(), userID, tenantID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	api.WriteSuccess(w, r, user)
}

// ---------------------------------------------------------------------------
// PATCH /api/v1/auth/me  (authenticated)
// ---------------------------------------------------------------------------

// HandleUpdateCurrentUser updates full_name and/or email for the authenticated
// user.  If the email is changed a new verification email is sent via MailHog.
func (h *AuthHandler) HandleUpdateCurrentUser(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.MustTenantID(w, r)
	if !ok {
		return
	}

	var req service.UpdateUserRequest
	if apiErr := h.DecodeJSON(r, &req); apiErr != nil {
		h.WriteErr(w, r, apiErr)
		return
	}
	if apiErr := h.ValidateRequest(&req); apiErr != nil {
		h.WriteErr(w, r, apiErr)
		return
	}

	user, err := h.svc.UpdateCurrentUser(r.Context(), userID, tenantID, &req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	api.WriteSuccess(w, r, user)
}

// ---------------------------------------------------------------------------
// GET /api/v1/auth/sessions  (authenticated)
// ---------------------------------------------------------------------------

// HandleGetSessions returns all active sessions for the authenticated user.
// Redis is checked first; the DB is used as a fallback.
func (h *AuthHandler) HandleGetSessions(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	sessions, err := h.svc.GetUserSessions(r.Context(), userID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	api.WriteSuccess(w, r, map[string]any{"sessions": sessions})
}

// ---------------------------------------------------------------------------
// DELETE /api/v1/auth/sessions/{session_id}  (authenticated)
// ---------------------------------------------------------------------------

// HandleRevokeSession invalidates a specific session by ID.
// The session must belong to the authenticated user.
func (h *AuthHandler) HandleRevokeSession(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	sessionID := r.PathValue("session_id")
	if sessionID == "" {
		h.WriteErr(w, r, api.ErrMissingRequired("session_id"))
		return
	}

	if err := h.svc.RevokeSession(r.Context(), userID, sessionID); err != nil {
		h.HandleErr(w, r, err)
		return
	}

	api.WriteNoContent(w)
}
