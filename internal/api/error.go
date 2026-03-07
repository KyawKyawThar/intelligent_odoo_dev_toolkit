package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/google/uuid"
)

type ErrorCode string

const (
	// Client errors (4xx)
	ErrCodeValidation      ErrorCode = "VALIDATION_ERROR"
	ErrCodeUnauthorized    ErrorCode = "UNAUTHORIZED"
	ErrCodeForbidden       ErrorCode = "FORBIDDEN"
	ErrCodeNotFound        ErrorCode = "NOT_FOUND"
	ErrCodeConflict        ErrorCode = "CONFLICT"
	ErrCodeUnprocessable   ErrorCode = "UNPROCESSABLE_ENTITY"
	ErrCodeRateLimited     ErrorCode = "RATE_LIMITED"
	ErrCodeBadRequest      ErrorCode = "BAD_REQUEST"
	ErrCodePayloadTooLarge ErrorCode = "PAYLOAD_TOO_LARGE"

	// Server errors (5xx)
	ErrCodeInternal    ErrorCode = "INTERNAL_ERROR"
	ErrCodeUnavailable ErrorCode = "SERVICE_UNAVAILABLE"
	ErrCodeTimeout     ErrorCode = "TIMEOUT"
	ErrCodeDatabase    ErrorCode = "DATABASE_ERROR"

	// OdooDevTools-specific business logic errors
	ErrCodeTenantNotFound            ErrorCode = "TENANT_NOT_FOUND"
	ErrCodeEnvironmentNotFound       ErrorCode = "ENVIRONMENT_NOT_FOUND"
	ErrCodeAgentNotConnected         ErrorCode = "AGENT_NOT_CONNECTED"
	ErrCodeAgentNotFound             ErrorCode = "AGENT_NOT_FOUND"
	ErrCodeQuotaExceeded             ErrorCode = "QUOTA_EXCEEDED"
	ErrCodePlanLimitReached          ErrorCode = "PLAN_LIMIT_REACHED"
	ErrCodeInvalidAPIKey             ErrorCode = "INVALID_API_KEY"
	ErrCodeExpiredAPIKey             ErrorCode = "EXPIRED_API_KEY"
	ErrCodeInvalidToken              ErrorCode = "INVALID_TOKEN"
	ErrCodeExpiredToken              ErrorCode = "EXPIRED_TOKEN"
	ErrCodeInvalidCredentials        ErrorCode = "INVALID_CREDENTIALS"
	ErrCodeEmailAlreadyExists        ErrorCode = "EMAIL_ALREADY_EXISTS"
	ErrCodeSlugAlreadyExists         ErrorCode = "SLUG_ALREADY_EXISTS"
	ErrCodeInvalidScope              ErrorCode = "INVALID_SCOPE"
	ErrCodeSchemaSnapshotNotFound    ErrorCode = "SCHEMA_SNAPSHOT_NOT_FOUND"
	ErrCodeMigrationScanNotFound     ErrorCode = "MIGRATION_SCAN_NOT_FOUND"
	ErrCodeErrorGroupNotFound        ErrorCode = "ERROR_GROUP_NOT_FOUND"
	ErrCodeAlertRuleNotFound         ErrorCode = "ALERT_RULE_NOT_FOUND"
	ErrCodeAlertChannelNotFound      ErrorCode = "ALERT_CHANNEL_NOT_FOUND"
	ErrCodeProfilerRecordingNotFound ErrorCode = "PROFILER_RECORDING_NOT_FOUND"
	ErrCodeAnonProfileNotFound       ErrorCode = "ANON_PROFILE_NOT_FOUND"
	ErrCodeAnonJobNotFound           ErrorCode = "ANON_JOB_NOT_FOUND"
	ErrCodeInvalidOdooVersion        ErrorCode = "INVALID_ODOO_VERSION"
	ErrCodeInvalidDomain             ErrorCode = "INVALID_DOMAIN"
)

type ErrorDetail struct {
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
	Value   any    `json:"value,omitempty"`
}

type APIError struct {
	Code      ErrorCode     `json:"code"`
	Message   string        `json:"message"`
	Details   []ErrorDetail `json:"details,omitempty"`
	RequestID string        `json:"request_id,omitempty"`

	// Internal fields (not serialized to JSON)
	HTTPStatus int       `json:"-"`
	Internal   error     `json:"-"`
	Stack      string    `json:"-"`
	Timestamp  time.Time `json:"-"`
	Path       string    `json:"-"`
	Method     string    `json:"-"`
}

// Error implements the error interface
func (e *APIError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying error
func (e *APIError) Unwrap() error {
	return e.Internal
}

// =============================================================================
// Chainable Methods (Builder Pattern)
// =============================================================================
func (e *APIError) WithRequestID(requestID string) *APIError {
	e.RequestID = requestID
	return e
}

func (e *APIError) WithPath(method, path string) *APIError {
	e.Method = method
	e.Path = path
	return e
}

func (e *APIError) WithDetails(details ...ErrorDetail) *APIError {
	e.Details = append(e.Details, details...)
	return e
}

func (e *APIError) WithDetail(field, message string) *APIError {
	e.Details = append(e.Details, ErrorDetail{Field: field, Message: message})
	return e
}

func (e *APIError) WithInternal(err error) *APIError {
	e.Internal = err
	return e
}

func (e *APIError) WithStack() *APIError {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	e.Stack = string(buf[:n])
	return e
}

// =============================================================================
// Response Methods
// =============================================================================

func (e *APIError) ToResponse() map[string]any {
	response := map[string]any{
		"code":    e.Code,
		"message": e.Message,
	}
	if len(e.Details) > 0 {
		response["details"] = e.Details
	}
	if e.RequestID != "" {
		response["request_id"] = e.RequestID
	}
	return map[string]any{"error": response}
}

func (e *APIError) ToJSON() []byte {
	data, err := json.Marshal(e.ToResponse())
	if err != nil {
		log.Error().Err(err).Msg("could not marshal error response")
		return []byte(`{"error":{"code":"INTERNAL_ERROR","message":"Failed to serialize error response"}}`)
	}
	return data
}

// LogFields returns fields for structured logging
func (e *APIError) LogFields() map[string]any {
	fields := map[string]any{
		"error_code":    e.Code,
		"error_message": e.Message,
		"http_status":   e.HTTPStatus,
	}
	if e.RequestID != "" {
		fields["request_id"] = e.RequestID
	}
	if e.Path != "" {
		fields["path"] = e.Path
		fields["method"] = e.Method
	}
	if e.Internal != nil {
		fields["internal_error"] = e.Internal.Error()
	}
	return fields
}

// =============================================================================
// Error Constructor
// =============================================================================

func NewAPIError(code ErrorCode, message string, httpStatus int) *APIError {
	return &APIError{
		Code:       code,
		Message:    message,
		HTTPStatus: httpStatus,
		Timestamp:  time.Now().UTC(),
	}
}

// =============================================================================
// Client Error Constructors (4xx)
// =============================================================================

// --- 400 Bad Request ---

func ErrBadRequest(message string) *APIError {
	return NewAPIError(ErrCodeBadRequest, message, http.StatusBadRequest)
}

func ErrValidation(message string) *APIError {
	return NewAPIError(ErrCodeValidation, message, http.StatusBadRequest)
}

func ErrInvalidJSON(err error) *APIError {
	return NewAPIError(ErrCodeValidation, "Invalid JSON payload", http.StatusBadRequest).
		WithInternal(err)
}

func ErrInvalidQueryParam(param, message string) *APIError {
	return NewAPIError(ErrCodeValidation, fmt.Sprintf("Invalid query parameter '%s': %s", param, message), http.StatusBadRequest).
		WithDetail(param, message)
}

func ErrInvalidPathParam(param, message string) *APIError {
	return NewAPIError(ErrCodeValidation, fmt.Sprintf("Invalid path parameter '%s': %s", param, message), http.StatusBadRequest).
		WithDetail(param, message)
}

func ErrInvalidUUID(field string) *APIError {
	return NewAPIError(ErrCodeValidation, fmt.Sprintf("Invalid UUID format for '%s'", field), http.StatusBadRequest).
		WithDetail(field, "must be a valid UUID")
}

func ErrMissingRequired(field string) *APIError {
	return NewAPIError(ErrCodeValidation, fmt.Sprintf("Missing required field '%s'", field), http.StatusBadRequest).
		WithDetail(field, "is required")
}

// --- 401 Unauthorized ---

func ErrUnauthorized(message string) *APIError {
	if message == "" {
		message = "Authentication required"
	}
	return NewAPIError(ErrCodeUnauthorized, message, http.StatusUnauthorized)
}

func ErrInvalidToken(message string) *APIError {
	if message == "" {
		message = "Invalid or malformed token"
	}
	return NewAPIError(ErrCodeInvalidToken, message, http.StatusUnauthorized)
}

func ErrExpiredToken() *APIError {
	return NewAPIError(ErrCodeExpiredToken, "Token has expired", http.StatusUnauthorized)
}

func ErrInvalidAPIKey() *APIError {
	return NewAPIError(ErrCodeInvalidAPIKey, "Invalid API key", http.StatusUnauthorized)
}

func ErrExpiredAPIKey() *APIError {
	return NewAPIError(ErrCodeExpiredAPIKey, "API key has expired", http.StatusUnauthorized)
}

func ErrInvalidCredentials() *APIError {
	return NewAPIError(ErrCodeInvalidCredentials, "Invalid email or password", http.StatusUnauthorized)
}

func ErrMissingAuthHeader() *APIError {
	return NewAPIError(ErrCodeUnauthorized, "Missing Authorization header", http.StatusUnauthorized)
}

func ErrMissingAPIKey() *APIError {
	return NewAPIError(ErrCodeUnauthorized, "Missing X-API-Key header", http.StatusUnauthorized)
}

// --- 403 Forbidden ---

func ErrForbidden(message string) *APIError {
	if message == "" {
		message = "You don't have permission to perform this action"
	}
	return NewAPIError(ErrCodeForbidden, message, http.StatusForbidden)
}

func ErrInsufficientScope(required string) *APIError {
	return NewAPIError(ErrCodeInvalidScope, fmt.Sprintf("Insufficient scope. Required: %s", required), http.StatusForbidden)
}

func ErrTenantMismatch() *APIError {
	return NewAPIError(ErrCodeForbidden, "Resource belongs to a different tenant", http.StatusForbidden)
}

func ErrAdminRequired() *APIError {
	return NewAPIError(ErrCodeForbidden, "Admin privileges required", http.StatusForbidden)
}

// --- 404 Not Found ---

func ErrNotFound(resource string) *APIError {
	return NewAPIError(ErrCodeNotFound, fmt.Sprintf("%s not found", resource), http.StatusNotFound)
}

func ErrTenantNotFound() *APIError {
	return NewAPIError(ErrCodeTenantNotFound, "Tenant not found", http.StatusNotFound)
}

func ErrEnvironmentNotFound() *APIError {
	return NewAPIError(ErrCodeEnvironmentNotFound, "Environment not found", http.StatusNotFound)
}

func ErrUserNotFound() *APIError {
	return NewAPIError(ErrCodeNotFound, "User not found", http.StatusNotFound)
}

func ErrAPIKeyNotFound() *APIError {
	return NewAPIError(ErrCodeNotFound, "API key not found", http.StatusNotFound)
}

func ErrErrorGroupNotFound() *APIError {
	return NewAPIError(ErrCodeErrorGroupNotFound, "Error group not found", http.StatusNotFound)
}

func ErrAlertRuleNotFound() *APIError {
	return NewAPIError(ErrCodeAlertRuleNotFound, "Alert rule not found", http.StatusNotFound)
}

func ErrSchemaSnapshotNotFound() *APIError {
	return NewAPIError(ErrCodeSchemaSnapshotNotFound, "Schema snapshot not found", http.StatusNotFound)
}

func ErrMigrationScanNotFound() *APIError {
	return NewAPIError(ErrCodeMigrationScanNotFound, "Migration scan not found", http.StatusNotFound)
}

func ErrProfilerRecordingNotFound() *APIError {
	return NewAPIError(ErrCodeProfilerRecordingNotFound, "Profiler recording not found", http.StatusNotFound)
}

func ErrAgentNotFound() *APIError {
	return NewAPIError(ErrCodeAgentNotFound, "Agent not found", http.StatusNotFound)
}

// --- 409 Conflict ---

func ErrConflict(message string) *APIError {
	return NewAPIError(ErrCodeConflict, message, http.StatusConflict)
}

func ErrEmailAlreadyExists() *APIError {
	return NewAPIError(ErrCodeEmailAlreadyExists, "Email address is already registered", http.StatusConflict).
		WithDetail("email", "already exists")
}

func ErrSlugAlreadyExists() *APIError {
	return NewAPIError(ErrCodeSlugAlreadyExists, "Slug is already taken", http.StatusConflict).
		WithDetail("slug", "already exists")
}

func ErrDuplicateResource(resource, field string) *APIError {
	return NewAPIError(ErrCodeConflict, fmt.Sprintf("%s with this %s already exists", resource, field), http.StatusConflict).
		WithDetail(field, "already exists")
}

// --- 422 Unprocessable Entity ---

func ErrUnprocessable(message string) *APIError {
	return NewAPIError(ErrCodeUnprocessable, message, http.StatusUnprocessableEntity)
}

func ErrAgentNotConnected(envName string) *APIError {
	msg := "Agent is not connected"
	if envName != "" {
		msg = fmt.Sprintf("Agent is not connected for environment '%s'", envName)
	}
	return NewAPIError(ErrCodeAgentNotConnected, msg, http.StatusUnprocessableEntity)
}

func ErrQuotaExceeded(resource string, limit int) *APIError {
	return NewAPIError(ErrCodeQuotaExceeded, fmt.Sprintf("Quota exceeded for %s. Limit: %d", resource, limit), http.StatusUnprocessableEntity)
}

func ErrPlanLimitReached(feature string) *APIError {
	return NewAPIError(ErrCodePlanLimitReached, fmt.Sprintf("Plan limit reached for %s. Please upgrade your plan.", feature), http.StatusUnprocessableEntity)
}

func ErrInvalidOdooVersion(version string) *APIError {
	return NewAPIError(ErrCodeInvalidOdooVersion, fmt.Sprintf("Unsupported Odoo version: %s", version), http.StatusUnprocessableEntity)
}

func ErrInvalidReference(field, reference string) *APIError {
	return NewAPIError(ErrCodeUnprocessable, fmt.Sprintf("Invalid reference in '%s': %s does not exist", field, reference), http.StatusUnprocessableEntity).
		WithDetail(field, "references non-existent record")
}

// --- 429 Rate Limited ---

func ErrRateLimited(retryAfter int) *APIError {
	return NewAPIError(ErrCodeRateLimited, fmt.Sprintf("Rate limit exceeded. Retry after %d seconds", retryAfter), http.StatusTooManyRequests)
}

// =============================================================================
// Server Error Constructors (5xx)
// =============================================================================

func ErrInternal(err error) *APIError {
	return NewAPIError(ErrCodeInternal, "An internal error occurred", http.StatusInternalServerError).
		WithInternal(err).
		WithStack()
}

func ErrDatabase(err error) *APIError {
	return NewAPIError(ErrCodeDatabase, "A database error occurred", http.StatusInternalServerError).
		WithInternal(err).
		WithStack()
}

func ErrUnavailable(message string) *APIError {
	if message == "" {
		message = "Service temporarily unavailable"
	}
	return NewAPIError(ErrCodeUnavailable, message, http.StatusServiceUnavailable)
}

func ErrDatabaseUnavailable() *APIError {
	return NewAPIError(ErrCodeUnavailable, "Database is temporarily unavailable", http.StatusServiceUnavailable)
}

func ErrTimeout(operation string) *APIError {
	return NewAPIError(ErrCodeTimeout, fmt.Sprintf("Operation timed out: %s", operation), http.StatusGatewayTimeout)
}

func ErrAgentTimeout(envName string) *APIError {
	return NewAPIError(ErrCodeTimeout, fmt.Sprintf("Agent did not respond in time for environment '%s'", envName), http.StatusGatewayTimeout)
}

// =============================================================================
// HTTP Response Helpers
//

func WriteError(w http.ResponseWriter, err *APIError) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(err.HTTPStatus)
	if err := json.NewEncoder(w).Encode(err.ToResponse()); err != nil {
		log.Error().Err(err).Msg("could not write error response")
	}
}

func WriteErrorWithContext(w http.ResponseWriter, r *http.Request, err *APIError) {
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = uuid.New().String()
	}
	w.Header().Set("X-Request-ID", requestID)
	err = err.WithRequestID(requestID).WithPath(r.Method, r.URL.Path)
	WriteError(w, err)
}

func HandleError(w http.ResponseWriter, r *http.Request, err error) {
	apiErr := ToAPIError(err)
	WriteErrorWithContext(w, r, apiErr)
}

func ToAPIError(err error) *APIError {
	if err == nil {
		return nil
	}
	if apiErr, ok := err.(*APIError); ok {
		return apiErr
	}
	return ErrInternal(err)
}
