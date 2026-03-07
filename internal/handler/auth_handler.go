package handler

import (
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"
	"Intelligent_Dev_ToolKit_Odoo/internal/service"
	"net/http"
)

type AuthHandler struct {
	*BaseHandler
	svc *service.AuthService
}

func NewAuthHandler(authService *service.AuthService, base *BaseHandler) *AuthHandler {
	return &AuthHandler{
		BaseHandler: base,
		svc:         authService,
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/auth/register
// ---------------------------------------------------------------------------

// HandleRegister creates a new tenant + owner user in a single transaction,
// issues an access + refresh token pair, and triggers a verification email
// via MailHog.
// @Summary      Register a new user
// @Description  Creates a new tenant and owner user, and returns an access and refresh token.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request body dto.RegisterRequest true "User registration details"
// @Success      201  {object}  dto.LoginResponse
// @Failure      400  {object}  api.APIError
// @Failure      409  {object}  api.APIError
// @Failure      500  {object}  api.APIError
// @Router       /auth/register [post]
func (h *AuthHandler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	var req dto.RegisterRequest
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

	dto.WriteCreated(w, r, resp)
}

// ---------------------------------------------------------------------------
// POST /api/v1/auth/login
// ---------------------------------------------------------------------------

// HandleLogin authenticates a user by email + password, enforces brute-force
// protection via Redis, and returns a token pair.
// @Summary      Login
// @Description  Authenticates a user by email and password. Enforces brute-force protection via Redis. Returns an access and refresh token pair.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request body dto.LoginRequest true "Login credentials"
// @Success      200  {object}  dto.LoginResponse
// @Failure      400  {object}  api.APIError
// @Failure      401  {object}  api.APIError
// @Failure      429  {object}  api.APIError
// @Failure      500  {object}  api.APIError
// @Security     none
// @Router       /auth/login [post]
func (h *AuthHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	var req dto.LoginRequest
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

	dto.WriteSuccess(w, r, resp)
}

// ---------------------------------------------------------------------------
// POST /api/v1/auth/refresh
// ---------------------------------------------------------------------------

// HandleRefreshToken rotates the refresh token (old one is blacklisted in Redis)
// and issues a new access + refresh pair.
// @Summary      Refresh token
// @Description  Rotates the refresh token (old one is blacklisted in Redis) and issues a new access + refresh pair.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request body dto.RefreshTokenRequest true "Refresh token"
// @Success      200  {object}  dto.RefreshTokenResponse
// @Failure      400  {object}  api.APIError
// @Failure      401  {object}  api.APIError
// @Failure      500  {object}  api.APIError
// @Router       /auth/refresh [post]
// and issues a new access + refresh pair.
func (h *AuthHandler) HandleRefreshToken(w http.ResponseWriter, r *http.Request) {
	var req dto.RefreshTokenRequest
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

	dto.WriteSuccess(w, r, resp)
}

// ---------------------------------------------------------------------------
// POST /api/v1/auth/logout
// ---------------------------------------------------------------------------

// HandleLogout invalidates the current session (or all sessions when
// logout_all=true).  Requires a valid Bearer token in the Authorization header.
// @Summary      Logout
// @Description  Invalidates the current session or all sessions when logout_all=true. Requires a valid Bearer token.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body dto.LogoutRequest false "Logout options (optional)"
// @Success      204
// @Failure      401  {object}  api.APIError
// @Failure      500  {object}  api.APIError
// @Router       /auth/logout [post]
func (h *AuthHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	BearerToken := h.BearerToken(r)
	if BearerToken == "" {
		h.WriteErr(w, r, api.ErrMissingAuthHeader())
		return
	}

	// Body is optional — default to single-token logout
	var req dto.LogoutRequest
	_ = h.DecodeJSON(r, &req) //nolint:errcheck // error is handled in DecodeJSON

	if err := h.svc.Logout(r.Context(), BearerToken, &req); err != nil {
		h.HandleErr(w, r, err)
		return
	}

	dto.WriteNoContent(w)
}

// ---------------------------------------------------------------------------
// POST /api/v1/auth/forgot-password
// ---------------------------------------------------------------------------

// HandleForgotPassword always returns HTTP 200 regardless of whether the
// email is registered (prevents user enumeration). An email is sent via
// MailHog when the account exists.
// @Summary      Forgot password
// @Description  Always returns HTTP 200 regardless of whether the email is registered (prevents user enumeration). Sends a reset link via MailHog when the account exists.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request body dto.ForgotPasswordRequest true "Email address"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  api.APIError
// @Router       /auth/forgot-password [post]
func (h *AuthHandler) HandleForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req dto.ForgotPasswordRequest
	if apiErr := h.DecodeJSON(r, &req); apiErr != nil {
		h.WriteErr(w, r, apiErr)
		return
	}
	if apiErr := h.ValidateRequest(&req); apiErr != nil {
		h.WriteErr(w, r, apiErr)
		return
	}

	// Intentionally ignore the error — security requirement
	_ = h.svc.ForgotPassword(r.Context(), &req) //nolint:errcheck // error is not critical for this operation

	dto.WriteSuccess(w, r, map[string]any{
		"message": "If that email address is registered you will receive a reset link shortly.",
	})
}

// ---------------------------------------------------------------------------
// POST /api/v1/auth/reset-password
// ---------------------------------------------------------------------------

// HandleResetPassword consumes the one-time Redis token and updates the
// user's password.  All existing sessions are revoked on success.
// @Summary      Reset password
// @Description  Consumes the one-time Redis token and updates the user's password. All existing sessions are revoked on success.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request body dto.ResetPasswordRequest true "Reset token and new password"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  api.APIError
// @Failure      500  {object}  api.APIError
// @Router       /auth/reset-password [post]
func (h *AuthHandler) HandleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req dto.ResetPasswordRequest
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

	dto.WriteSuccess(w, r, map[string]any{
		"message": "Password has been reset. Please log in with your new password.",
	})
}

// ---------------------------------------------------------------------------
// POST /api/v1/auth/change-password  (authenticated)
// ---------------------------------------------------------------------------

// HandleChangePassword lets a logged-in user change their own password.
// Requires the current password for verification.  All sessions are revoked
// on success (forces re-login on all devices).
// @Summary      Change password
// @Description  Lets a logged-in user change their own password. Requires the current password for verification. All sessions are revoked on success.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body dto.ChangePasswordRequest true "Current and new password"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  api.APIError
// @Failure      401  {object}  api.APIError
// @Failure      500  {object}  api.APIError
// @Router       /auth/change-password [post]
func (h *AuthHandler) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.MustTenantID(w, r)
	if !ok {
		return
	}

	var req dto.ChangePasswordRequest
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

	dto.WriteSuccess(w, r, map[string]any{
		"message": "Password changed successfully. Please log in again.",
	})
}

// ---------------------------------------------------------------------------
// POST /api/v1/auth/verify-email
// ---------------------------------------------------------------------------

// HandleVerifyEmail consumes the one-time Redis verification token and marks
// the user's email as verified in the database.
// @Summary      Verify email
// @Description  Consumes the one-time Redis verification token and marks the user's email as verified in the database.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request body dto.VerifyEmailRequest true "Verification token"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  api.APIError
// @Failure      500  {object}  api.APIError
// @Router       /auth/verify-email [post]
func (h *AuthHandler) HandleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req dto.VerifyEmailRequest
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

	dto.WriteSuccess(w, r, map[string]any{
		"message": "Email verified successfully.",
	})
}

// ---------------------------------------------------------------------------
// POST /api/v1/auth/resend-verification  (authenticated)
// ---------------------------------------------------------------------------

// HandleResendVerification sends a fresh verification email via MailHog.
// Returns 409 if the email is already verified.
// @Summary      Resend verification email
// @Description  Sends a fresh verification email via MailHog. Returns 409 if the email is already verified.
// @Tags         Auth
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  map[string]string
// @Failure      401  {object}  api.APIError
// @Failure      409  {object}  api.APIError
// @Failure      500  {object}  api.APIError
// @Router       /auth/resend-verification [post]
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

	dto.WriteSuccess(w, r, map[string]any{
		"message": "Verification email sent.",
	})
}

// ---------------------------------------------------------------------------
// GET /api/v1/auth/me  (authenticated)
// ---------------------------------------------------------------------------

// HandleGetCurrentUser returns the profile of the authenticated user.
// @Summary      Get current user
// @Description  Returns the profile of the authenticated user.
// @Tags         Auth
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  dto.UserResponse
// @Failure      401  {object}  api.APIError
// @Failure      404  {object}  api.APIError
// @Failure      500  {object}  api.APIError
// @Router       /auth/me [get]
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

	dto.WriteSuccess(w, r, user)
}

// ---------------------------------------------------------------------------
// PATCH /api/v1/auth/me  (authenticated)
// ---------------------------------------------------------------------------

// HandleUpdateCurrentUser updates full_name and/or email for the authenticated
// user.  If the email is changed a new verification email is sent via MailHog.
// @Summary      Update current user
// @Description  Updates full_name and/or email for the authenticated user. If the email is changed a new verification email is sent via MailHog.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body dto.UpdateUserRequest true "Fields to update"
// @Success      200  {object}  dto.UserResponse
// @Failure      400  {object}  api.APIError
// @Failure      401  {object}  api.APIError
// @Failure      409  {object}  api.APIError
// @Failure      500  {object}  api.APIError
// @Router       /auth/me [patch]
func (h *AuthHandler) HandleUpdateCurrentUser(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.MustTenantID(w, r)
	if !ok {
		return
	}

	var req dto.UpdateUserRequest
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

	dto.WriteSuccess(w, r, user)
}

// ---------------------------------------------------------------------------
// GET /api/v1/auth/sessions  (authenticated)
// ---------------------------------------------------------------------------

// HandleGetSessions returns all active sessions for the authenticated user.
// Redis is checked first; the DB is used as a fallback.
// @Summary      Get active sessions
// @Description  Returns all active sessions for the authenticated user. Redis is checked first; the DB is used as a fallback.
// @Tags         Auth
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  map[string][]dto.SessionResponse
// @Failure      401  {object}  api.APIError
// @Failure      500  {object}  api.APIError
// @Router       /auth/sessions [get]
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

	dto.WriteSuccess(w, r, map[string]any{"sessions": sessions})
}

// ---------------------------------------------------------------------------
// DELETE /api/v1/auth/sessions/{session_id}  (authenticated)
// ---------------------------------------------------------------------------

// HandleRevokeSession invalidates a specific session by ID.
// The session must belong to the authenticated user.
// @Summary      Revoke session
// @Description  Invalidates a specific session by ID. The session must belong to the authenticated user.
// @Tags         Auth
// @Produce      json
// @Security     BearerAuth
// @Param        session_id path string true "Session ID to revoke"
// @Success      204
// @Failure      400  {object}  api.APIError
// @Failure      401  {object}  api.APIError
// @Failure      404  {object}  api.APIError
// @Failure      500  {object}  api.APIError
// @Router       /auth/sessions/{session_id} [delete]
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

	dto.WriteNoContent(w)
}
