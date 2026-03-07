package service

import (
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"
	"Intelligent_Dev_ToolKit_Odoo/internal/token"
	"context"

	"github.com/google/uuid"
)

// AuthServicer defines the contract for authentication operations.
// This interface allows for easy mocking in tests.

type AuthServicer interface {
	Register(ctx context.Context, req *dto.RegisterRequest, ipAddress, userAgent string) (*dto.RegisterResponse, error)
	Login(ctx context.Context, req *dto.LoginRequest, ipAddress, userAgent string) (*dto.LoginResponse, error)
	RefreshToken(ctx context.Context, req *dto.RefreshTokenRequest, ipAddress, userAgent string) (*dto.RefreshTokenResponse, error)

	Logout(ctx context.Context, accessToken string, req *dto.LogoutRequest) error
	ForgotPassword(ctx context.Context, req *dto.ForgotPasswordRequest) error
	ResetPassword(ctx context.Context, req *dto.ResetPasswordRequest) error
	ChangePassword(ctx context.Context, userID, tenantID uuid.UUID, req *dto.ChangePasswordRequest) error
	VerifyEmail(ctx context.Context, req *dto.VerifyEmailRequest) error
	ResendVerificationEmail(ctx context.Context, userID, tenantID uuid.UUID) error
	GetCurrentUser(ctx context.Context, userID, tenantID uuid.UUID) (*dto.UserResponse, error)
	UpdateCurrentUser(ctx context.Context, userID, tenantID uuid.UUID, req *dto.UpdateUserRequest) (*dto.UserResponse, error)
	GetUserSessions(ctx context.Context, userID uuid.UUID) ([]*dto.SessionResponse, error)
	RevokeSession(ctx context.Context, userID uuid.UUID, sessionID string) error
	ValidateAccessToken(ctx context.Context, tokenStr string) (*token.Payload, error)
}
