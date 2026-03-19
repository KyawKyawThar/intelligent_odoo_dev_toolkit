package service

import (
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"
	"Intelligent_Dev_ToolKit_Odoo/internal/token"
	"context"

	"github.com/google/uuid"
)

// AuthServicer defines the contract for authentication operations.
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

// EnvironmentServicer defines the business operations for environments.
type EnvironmentServicer interface {
	Create(ctx context.Context, tenantID uuid.UUID, req *dto.CreateEnvironmentRequest) (*dto.EnvironmentResponse, error)
	GetByID(ctx context.Context, tenantID, envID uuid.UUID) (*dto.EnvironmentResponse, error)
	List(ctx context.Context, tenantID uuid.UUID, req *dto.ListEnvironmentsRequest) (*dto.EnvironmentListResponse, error)
	Update(ctx context.Context, tenantID, envID uuid.UUID, req *dto.UpdateEnvironmentRequest) (*dto.EnvironmentResponse, error)
	Delete(ctx context.Context, tenantID, envID uuid.UUID) error
	RegisterAgent(ctx context.Context, tenantID, envID uuid.UUID, req *dto.RegisterAgentRequest) (*dto.RegisterAgentResponse, error)
}

// ErrorServicer defines the agent error ingestion and query operations.
type ErrorServicer interface {
	IngestBatch(ctx context.Context, tenantID uuid.UUID, req *dto.IngestErrorsRequest) error
	ListErrorGroups(ctx context.Context, tenantID, envID uuid.UUID, req *dto.ListErrorGroupsRequest) (*dto.ErrorGroupListResponse, error)
	GetErrorGroup(ctx context.Context, tenantID, envID, errorID uuid.UUID, includeTrace bool) (*dto.ErrorGroupDetailResponse, error)
	GetErrorGroupBySignature(ctx context.Context, tenantID, envID uuid.UUID, signature string) (*dto.ErrorGroupDetailResponse, error)
}

// APIKeyServicer defines operations for managing agent API keys.
type APIKeyServicer interface {
	Create(ctx context.Context, tenantID, envID, userID uuid.UUID, req *dto.CreateAPIKeyRequest) (*dto.APIKeyCreatedResponse, error)
	List(ctx context.Context, tenantID, envID uuid.UUID) (*dto.APIKeyListResponse, error)
	Revoke(ctx context.Context, tenantID, keyID uuid.UUID) error
}

// AgentRegisterServicer handles agent self-registration with one-time tokens.
type AgentRegisterServicer interface {
	SelfRegister(ctx context.Context, req *dto.AgentSelfRegisterRequest) (*dto.AgentSelfRegisterResponse, error)
}

// SchemaServicer defines the business operations for schema snapshots.
type SchemaServicer interface {
	// StoreSchema is called by the agent to persist a collected schema snapshot.
	StoreSchema(ctx context.Context, tenantID uuid.UUID, req *dto.StoreSchemaRequest) (*dto.SchemaSnapshotResponse, error)
	// GetLatest returns the most recent snapshot for an environment.
	GetLatest(ctx context.Context, tenantID, envID uuid.UUID) (*dto.SchemaSnapshotResponse, error)
	// ListSnapshots returns a lightweight list of snapshots for an environment.
	ListSnapshots(ctx context.Context, tenantID, envID uuid.UUID, limit int32) (*dto.SchemaSnapshotListResponse, error)
	// SearchModels searches models within the latest snapshot for an environment.
	SearchModels(ctx context.Context, tenantID, envID uuid.UUID, q string, limit, offset int32) (*dto.SearchModelsResponse, error)
}
