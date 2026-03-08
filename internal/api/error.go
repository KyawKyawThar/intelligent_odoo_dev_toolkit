// Package api provides functionality for handling API errors, including defining standardized error codes,
// creating structured error responses, and providing helper functions for common HTTP errors.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/google/uuid"
)

// ErrorCode is a string that represents a specific API error code.
type ErrorCode string

const (
	// ErrCodeValidation indicates a validation error with the request.
	ErrCodeValidation ErrorCode = "VALIDATION_ERROR"
	// ErrCodeUnauthorized indicates that the request is not authenticated.
	ErrCodeUnauthorized ErrorCode = "UNAUTHORIZED"
	// ErrCodeForbidden indicates that the authenticated user does not have permission to perform the action.
	ErrCodeForbidden ErrorCode = "FORBIDDEN"
	// ErrCodeNotFound indicates that the requested resource could not be found.
	ErrCodeNotFound ErrorCode = "NOT_FOUND"
	// ErrCodeConflict indicates a conflict with the current state of the resource.
	ErrCodeConflict ErrorCode = "CONFLICT"
	// ErrCodeUnprocessable indicates that the server understands the content type of the request entity,
	// and the syntax of the request entity is correct but was unable to process the contained instructions.
	ErrCodeUnprocessable ErrorCode = "UNPROCESSABLE_ENTITY"
	// ErrCodeRateLimited indicates that the user has sent too many requests in a given amount of time.
	ErrCodeRateLimited ErrorCode = "RATE_LIMITED"
	// ErrCodeBadRequest indicates that the server cannot or will not process the request due to something that is perceived to be a client error.
	ErrCodeBadRequest ErrorCode = "BAD_REQUEST"
	// ErrCodePayloadTooLarge indicates that the request is larger than the server is willing or able to process.
	ErrCodePayloadTooLarge ErrorCode = "PAYLOAD_TOO_LARGE"

	// ErrCodeInternal indicates an internal server error.
	ErrCodeInternal ErrorCode = "INTERNAL_ERROR"
	// ErrCodeUnavailable indicates that the server is not ready to handle the request.
	ErrCodeUnavailable ErrorCode = "SERVICE_UNAVAILABLE"
	// ErrCodeTimeout indicates that the server did not receive a timely response from an upstream server.
	ErrCodeTimeout ErrorCode = "TIMEOUT"
	// ErrCodeDatabase indicates a database error.
	ErrCodeDatabase ErrorCode = "DATABASE_ERROR"

	// OdooDevTools-specific business logic errors

	// ErrCodeTenantNotFound indicates that the tenant could not be found.
	ErrCodeTenantNotFound ErrorCode = "TENANT_NOT_FOUND"
	// ErrCodeEnvironmentNotFound indicates that the environment could not be found.
	ErrCodeEnvironmentNotFound ErrorCode = "ENVIRONMENT_NOT_FOUND"
	// ErrCodeAgentNotConnected indicates that the agent is not connected.
	ErrCodeAgentNotConnected ErrorCode = "AGENT_NOT_CONNECTED"
	// ErrCodeAgentNotFound indicates that the agent could not be found.
	ErrCodeAgentNotFound ErrorCode = "AGENT_NOT_FOUND"
	// ErrCodeQuotaExceeded indicates that the quota has been exceeded.
	ErrCodeQuotaExceeded ErrorCode = "QUOTA_EXCEEDED"
	// ErrCodePlanLimitReached indicates that the plan limit has been reached.
	ErrCodePlanLimitReached ErrorCode = "PLAN_LIMIT_REACHED"
	// ErrCodeInvalidAPIKey indicates that the API key is invalid.
	ErrCodeInvalidAPIKey ErrorCode = "INVALID_API_KEY" //nolint:gosec // not a credential
	// ErrCodeExpiredAPIKey indicates that the API key has expired.
	ErrCodeExpiredAPIKey ErrorCode = "EXPIRED_API_KEY" //nolint:gosec // not a credential
	// ErrCodeInvalidToken indicates that the token is invalid.
	ErrCodeInvalidToken ErrorCode = "INVALID_TOKEN"
	// ErrCodeExpiredToken indicates that the token has expired.
	ErrCodeExpiredToken ErrorCode = "EXPIRED_TOKEN"
	// ErrCodeInvalidCredentials indicates that the credentials are invalid.
	ErrCodeInvalidCredentials ErrorCode = "INVALID_CREDENTIALS" //nolint:gosec // not a credential
	// ErrCodeEmailAlreadyExists indicates that the email already exists.
	ErrCodeEmailAlreadyExists ErrorCode = "EMAIL_ALREADY_EXISTS"
	// ErrCodeSlugAlreadyExists indicates that the slug already exists.
	ErrCodeSlugAlreadyExists ErrorCode = "SLUG_ALREADY_EXISTS"
	// ErrCodeInvalidScope indicates that the scope is invalid.
	ErrCodeInvalidScope ErrorCode = "INVALID_SCOPE"
	// ErrCodeSchemaSnapshotNotFound indicates that the schema snapshot could not be found.
	ErrCodeSchemaSnapshotNotFound ErrorCode = "SCHEMA_SNAPSHOT_NOT_FOUND"
	// ErrCodeMigrationScanNotFound indicates that the migration scan could not be found.
	ErrCodeMigrationScanNotFound ErrorCode = "MIGRATION_SCAN_NOT_FOUND"
	// ErrCodeErrorGroupNotFound indicates that the error group could not be found.
	ErrCodeErrorGroupNotFound ErrorCode = "ERROR_GROUP_NOT_FOUND"
	// ErrCodeAlertRuleNotFound indicates that the alert rule could not be found.
	ErrCodeAlertRuleNotFound ErrorCode = "ALERT_RULE_NOT_FOUND"
	// ErrCodeAlertChannelNotFound indicates that the alert channel could not be found.
	ErrCodeAlertChannelNotFound ErrorCode = "ALERT_CHANNEL_NOT_FOUND"
	// ErrCodeProfilerRecordingNotFound indicates that the profiler recording could not be found.
	ErrCodeProfilerRecordingNotFound ErrorCode = "PROFILER_RECORDING_NOT_FOUND"
	// ErrCodeAnonProfileNotFound indicates that the anon profile could not be found.
	ErrCodeAnonProfileNotFound ErrorCode = "ANON_PROFILE_NOT_FOUND"
	// ErrCodeAnonJobNotFound indicates that the anon job could not be found.
	ErrCodeAnonJobNotFound ErrorCode = "ANON_JOB_NOT_FOUND"
	// ErrCodeInvalidOdooVersion indicates that the Odoo version is invalid.
	ErrCodeInvalidOdooVersion ErrorCode = "INVALID_ODOO_VERSION"
	// ErrCodeInvalidDomain indicates that the domain is invalid.
	ErrCodeInvalidDomain ErrorCode = "INVALID_DOMAIN"
)

// ErrorDetail provides more specific information about an error.
type ErrorDetail struct {
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
	Value   any    `json:"value,omitempty"`
}

// Error represents a structured error response.
type Error struct {
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
func (e *Error) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying error
func (e *Error) Unwrap() error {
	return e.Internal
}

// =============================================================================
// Chainable Methods (Builder Pattern)
// =============================================================================

// WithRequestID adds a request ID to the error.
func (e *Error) WithRequestID(requestID string) *Error {
	e.RequestID = requestID
	return e
}

// WithPath adds the HTTP method and path to the error.
func (e *Error) WithPath(method, path string) *Error {
	e.Method = method
	e.Path = path
	return e
}

// WithDetails adds details to the error.
func (e *Error) WithDetails(details ...ErrorDetail) *Error {
	e.Details = append(e.Details, details...)
	return e
}

// WithDetail adds a single detail to the error.
func (e *Error) WithDetail(field, message string) *Error {
	e.Details = append(e.Details, ErrorDetail{Field: field, Message: message})
	return e
}

// WithInternal adds an internal error.
func (e *Error) WithInternal(err error) *Error {
	e.Internal = err
	return e
}

// WithStack adds a stack trace to the error.
func (e *Error) WithStack() *Error {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	e.Stack = string(buf[:n])
	return e
}

// =============================================================================
// Response Methods
// =============================================================================

// ToResponse converts the error to a map suitable for a JSON response.
func (e *Error) ToResponse() map[string]any {
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

// ToJSON converts the error to a JSON byte slice.
func (e *Error) ToJSON() []byte {
	data, err := json.Marshal(e.ToResponse())
	if err != nil {
		log.Error().Err(err).Msg("could not marshal error response")
		return []byte(`{"error":{"code":"INTERNAL_ERROR","message":"Failed to serialize error response"}}`)
	}
	return data
}

// LogFields returns fields for structured logging
func (e *Error) LogFields() map[string]any {
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

// NewError creates a new Error.
func NewError(code ErrorCode, message string, httpStatus int) *Error {
	return &Error{
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

// ErrBadRequest creates a new 400 Bad Request error.
func ErrBadRequest(message string) *Error {
	return NewError(ErrCodeBadRequest, message, http.StatusBadRequest)
}

// ErrValidation creates a new 400 Validation Error.
func ErrValidation(message string) *Error {
	return NewError(ErrCodeValidation, message, http.StatusBadRequest)
}

// ErrInvalidJSON creates a new 400 Invalid JSON error.
func ErrInvalidJSON(err error) *Error {
	return NewError(ErrCodeValidation, "Invalid JSON payload", http.StatusBadRequest).
		WithInternal(err)
}

// ErrInvalidQueryParam creates a new 400 Invalid Query Param error.
func ErrInvalidQueryParam(param, message string) *Error {
	return NewError(ErrCodeValidation, fmt.Sprintf("Invalid query parameter '%s': %s", param, message), http.StatusBadRequest).
		WithDetail(param, message)
}

// ErrInvalidPathParam creates a new 400 Invalid Path Param error.
func ErrInvalidPathParam(param, message string) *Error {
	return NewError(ErrCodeValidation, fmt.Sprintf("Invalid path parameter '%s': %s", param, message), http.StatusBadRequest).
		WithDetail(param, message)
}

// ErrInvalidUUID creates a new 400 Invalid UUID error.
func ErrInvalidUUID(field string) *Error {
	return NewError(ErrCodeValidation, fmt.Sprintf("Invalid UUID format for '%s'", field), http.StatusBadRequest).
		WithDetail(field, "must be a valid UUID")
}

// ErrMissingRequired creates a new 400 Missing Required Field error.
func ErrMissingRequired(field string) *Error {
	return NewError(ErrCodeValidation, fmt.Sprintf("Missing required field '%s'", field), http.StatusBadRequest).
		WithDetail(field, "is required")
}

// --- 401 Unauthorized ---

// ErrUnauthorized creates a new 401 Unauthorized error.
func ErrUnauthorized(message string) *Error {
	if message == "" {
		message = "Authentication required"
	}
	return NewError(ErrCodeUnauthorized, message, http.StatusUnauthorized)
}

// ErrInvalidToken creates a new 401 Invalid Token error.
func ErrInvalidToken(message string) *Error {
	if message == "" {
		message = "Invalid or malformed token"
	}
	return NewError(ErrCodeInvalidToken, message, http.StatusUnauthorized)
}

// ErrExpiredToken creates a new 401 Expired Token error.
func ErrExpiredToken() *Error {
	return NewError(ErrCodeExpiredToken, "Token has expired", http.StatusUnauthorized)
}

// ErrInvalidAPIKey creates a new 401 Invalid API Key error.
func ErrInvalidAPIKey() *Error {
	return NewError(ErrCodeInvalidAPIKey, "Invalid API key", http.StatusUnauthorized)
}

// ErrExpiredAPIKey creates a new 401 Expired API Key error.
func ErrExpiredAPIKey() *Error {
	return NewError(ErrCodeExpiredAPIKey, "API key has expired", http.StatusUnauthorized)
}

// ErrInvalidCredentials creates a new 401 Invalid Credentials error.
func ErrInvalidCredentials() *Error {
	return NewError(ErrCodeInvalidCredentials, "Invalid email or password", http.StatusUnauthorized)
}

// ErrMissingAuthHeader creates a new 401 Missing Auth Header error.
func ErrMissingAuthHeader() *Error {
	return NewError(ErrCodeUnauthorized, "Missing Authorization header", http.StatusUnauthorized)
}

// ErrMissingAPIKey creates a new 401 Missing API Key error.
func ErrMissingAPIKey() *Error {
	return NewError(ErrCodeUnauthorized, "Missing X-API-Key header", http.StatusUnauthorized)
}

// --- 403 Forbidden ---

// ErrForbidden creates a new 403 Forbidden error.
func ErrForbidden(message string) *Error {
	if message == "" {
		message = "You don't have permission to perform this action"
	}
	return NewError(ErrCodeForbidden, message, http.StatusForbidden)
}

// ErrInsufficientScope creates a new 403 Insufficient Scope error.
func ErrInsufficientScope(required string) *Error {
	return NewError(ErrCodeInvalidScope, fmt.Sprintf("Insufficient scope. Required: %s", required), http.StatusForbidden)
}

// ErrTenantMismatch creates a new 403 Tenant Mismatch error.
func ErrTenantMismatch() *Error {
	return NewError(ErrCodeForbidden, "Resource belongs to a different tenant", http.StatusForbidden)
}

// ErrAdminRequired creates a new 403 Admin Required error.
func ErrAdminRequired() *Error {
	return NewError(ErrCodeForbidden, "Admin privileges required", http.StatusForbidden)
}

// --- 404 Not Found ---

// ErrNotFound creates a new 404 Not Found error.
func ErrNotFound(resource string) *Error {
	return NewError(ErrCodeNotFound, fmt.Sprintf("%s not found", resource), http.StatusNotFound)
}

// ErrTenantNotFound creates a new 404 Tenant Not Found error.
func ErrTenantNotFound() *Error {
	return NewError(ErrCodeTenantNotFound, "Tenant not found", http.StatusNotFound)
}

// ErrEnvironmentNotFound creates a new 404 Environment Not Found error.
func ErrEnvironmentNotFound() *Error {
	return NewError(ErrCodeEnvironmentNotFound, "Environment not found", http.StatusNotFound)
}

// ErrUserNotFound creates a new 404 User Not Found error.
func ErrUserNotFound() *Error {
	return NewError(ErrCodeNotFound, "User not found", http.StatusNotFound)
}

// ErrAPIKeyNotFound creates a new 404 API Key Not Found error.
func ErrAPIKeyNotFound() *Error {
	return NewError(ErrCodeNotFound, "API key not found", http.StatusNotFound)
}

// ErrErrorGroupNotFound creates a new 404 Error Group Not Found error.
func ErrErrorGroupNotFound() *Error {
	return NewError(ErrCodeErrorGroupNotFound, "Error group not found", http.StatusNotFound)
}

// ErrAlertRuleNotFound creates a new 404 Alert Rule Not Found error.
func ErrAlertRuleNotFound() *Error {
	return NewError(ErrCodeAlertRuleNotFound, "Alert rule not found", http.StatusNotFound)
}

// ErrSchemaSnapshotNotFound creates a new 404 Schema Snapshot Not Found error.
func ErrSchemaSnapshotNotFound() *Error {
	return NewError(ErrCodeSchemaSnapshotNotFound, "Schema snapshot not found", http.StatusNotFound)
}

// ErrMigrationScanNotFound creates a new 404 Migration Scan Not Found error.
func ErrMigrationScanNotFound() *Error {
	return NewError(ErrCodeMigrationScanNotFound, "Migration scan not found", http.StatusNotFound)
}

// ErrProfilerRecordingNotFound creates a new 404 Profiler Recording Not Found error.
func ErrProfilerRecordingNotFound() *Error {
	return NewError(ErrCodeProfilerRecordingNotFound, "Profiler recording not found", http.StatusNotFound)
}

// ErrAgentNotFound creates a new 404 Agent Not Found error.
func ErrAgentNotFound() *Error {
	return NewError(ErrCodeAgentNotFound, "Agent not found", http.StatusNotFound)
}

// --- 409 Conflict ---

// ErrConflict creates a new 409 Conflict error.
func ErrConflict(message string) *Error {
	return NewError(ErrCodeConflict, message, http.StatusConflict)
}

// ErrEmailAlreadyExists creates a new 409 Email Already Exists error.
func ErrEmailAlreadyExists() *Error {
	return NewError(ErrCodeEmailAlreadyExists, "Email address is already registered", http.StatusConflict).
		WithDetail("email", "already exists")
}

// ErrSlugAlreadyExists creates a new 409 Slug Already Exists error.
func ErrSlugAlreadyExists() *Error {
	return NewError(ErrCodeSlugAlreadyExists, "Slug is already taken", http.StatusConflict).
		WithDetail("slug", "already exists")
}

// ErrDuplicateResource creates a new 409 Duplicate Resource error.
func ErrDuplicateResource(resource, field string) *Error {
	return NewError(ErrCodeConflict, fmt.Sprintf("%s with this %s already exists", resource, field), http.StatusConflict).
		WithDetail(field, "already exists")
}

// --- 422 Unprocessable Entity ---

// ErrUnprocessable creates a new 422 Unprocessable Entity error.
func ErrUnprocessable(message string) *Error {
	return NewError(ErrCodeUnprocessable, message, http.StatusUnprocessableEntity)
}

// ErrAgentNotConnected creates a new 422 Agent Not Connected error.
func ErrAgentNotConnected(envName string) *Error {
	msg := "Agent is not connected"
	if envName != "" {
		msg = fmt.Sprintf("Agent is not connected for environment '%s'", envName)
	}
	return NewError(ErrCodeAgentNotConnected, msg, http.StatusUnprocessableEntity)
}

// ErrQuotaExceeded creates a new 422 Quota Exceeded error.
func ErrQuotaExceeded(resource string, limit int) *Error {
	return NewError(ErrCodeQuotaExceeded, fmt.Sprintf("Quota exceeded for %s. Limit: %d", resource, limit), http.StatusUnprocessableEntity)
}

// ErrPlanLimitReached creates a new 422 Plan Limit Reached error.
func ErrPlanLimitReached(feature string) *Error {
	return NewError(ErrCodePlanLimitReached, fmt.Sprintf("Plan limit reached for %s. Please upgrade your plan.", feature), http.StatusUnprocessableEntity)
}

// ErrInvalidOdooVersion creates a new 422 Invalid Odoo Version error.
func ErrInvalidOdooVersion(version string) *Error {
	return NewError(ErrCodeInvalidOdooVersion, fmt.Sprintf("Unsupported Odoo version: %s", version), http.StatusUnprocessableEntity)
}

// ErrInvalidReference creates a new 422 Invalid Reference error.
func ErrInvalidReference(field, reference string) *Error {
	return NewError(ErrCodeUnprocessable, fmt.Sprintf("Invalid reference in '%s': %s does not exist", field, reference), http.StatusUnprocessableEntity).
		WithDetail(field, "references non-existent record")
}

// --- 429 Rate Limited ---

// ErrRateLimited creates a new 429 Rate Limited error.
func ErrRateLimited(retryAfter int) *Error {
	return NewError(ErrCodeRateLimited, fmt.Sprintf("Rate limit exceeded. Retry after %d seconds", retryAfter), http.StatusTooManyRequests)
}

// =============================================================================
// Server Error Constructors (5xx)
// =============================================================================

// ErrInternal creates a new 500 Internal Server Error.
func ErrInternal(err error) *Error {
	return NewError(ErrCodeInternal, "An internal error occurred", http.StatusInternalServerError).
		WithInternal(err).
		WithStack()
}

// ErrDatabase creates a new 500 Database Error.
func ErrDatabase(err error) *Error {
	return NewError(ErrCodeDatabase, "A database error occurred", http.StatusInternalServerError).
		WithInternal(err).
		WithStack()
}

// ErrUnavailable creates a new 503 Service Unavailable error.
func ErrUnavailable(message string) *Error {
	if message == "" {
		message = "Service temporarily unavailable"
	}
	return NewError(ErrCodeUnavailable, message, http.StatusServiceUnavailable)
}

// ErrDatabaseUnavailable creates a new 503 Database Unavailable error.
func ErrDatabaseUnavailable() *Error {
	return NewError(ErrCodeUnavailable, "Database is temporarily unavailable", http.StatusServiceUnavailable)
}

// ErrTimeout creates a new 504 Gateway Timeout error.
func ErrTimeout(operation string) *Error {
	return NewError(ErrCodeTimeout, fmt.Sprintf("Operation timed out: %s", operation), http.StatusGatewayTimeout)
}

// ErrAgentTimeout creates a new 504 Agent Timeout error.
func ErrAgentTimeout(envName string) *Error {
	return NewError(ErrCodeTimeout, fmt.Sprintf("Agent did not respond in time for environment '%s'", envName), http.StatusGatewayTimeout)
}

// =============================================================================
// HTTP Response Helpers
//

// WriteError writes an Error to the response.
func WriteError(w http.ResponseWriter, err *Error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(err.HTTPStatus)
	if err := json.NewEncoder(w).Encode(err.ToResponse()); err != nil {
		log.Error().Err(err).Msg("could not write error response")
	}
}

// WriteErrorWithContext writes an Error to the response with context.
func WriteErrorWithContext(w http.ResponseWriter, r *http.Request, err *Error) {
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = uuid.New().String()
	}
	w.Header().Set("X-Request-ID", requestID)
	err = err.WithRequestID(requestID).WithPath(r.Method, r.URL.Path)
	WriteError(w, err)
}

// HandleError handles an error and writes it to the response.
func HandleError(w http.ResponseWriter, r *http.Request, err error) {
	apiErr := ToError(err)
	WriteErrorWithContext(w, r, apiErr)
}

// ToError converts an error to an Error.
func ToError(err error) *Error {
	if err == nil {
		return nil
	}
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return apiErr
	}
	return ErrInternal(err)
}
