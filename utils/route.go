package utils

const (
	// Authentication & Identity
	AuthRegister = "/auth/register"
	AuthLogin    = "/auth/login"
	AuthRefresh  = "/auth/refresh"
	AuthLogout   = "/auth/logout"
	AuthForgotPassword = "/auth/forgot-password" //nolint:gosec // not a credential
	AuthResetPassword = "/auth/reset-password" //nolint:gosec // not a credential
)
