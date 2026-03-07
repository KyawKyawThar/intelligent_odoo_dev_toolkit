package handler

import (
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"
	"Intelligent_Dev_ToolKit_Odoo/internal/middleware"
	"Intelligent_Dev_ToolKit_Odoo/internal/service"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

// Handlers is the main container for all HTTP handlers.
type Handlers struct {
	Auth *AuthHandler

	// Future handlers:
	// Environment *EnvironmentHandler
	// Profiler    *ProfilerHandler
	// Alert       *AlertHandler
	//	APIKey *APIKeyHandler
	//Tenant *TenantHandler
	//User   *UserHandler
}

// NewHandlers creates all handlers with their service dependencies.
func NewHandlers(services *service.Services) *Handlers {
	v := validator.New()
	api.RegisterCustomValidations(v)

	base := &BaseHandler{validate: v}

	return &Handlers{
		Auth: NewAuthHandler(services.Auth.(*service.AuthService), base),
		// APIKey: NewAPIKeyHandler(services.APIKey, base),
		// Tenant: NewTenantHandler(services.Tenant, base),
		// User:   NewUserHandler(services.User, base),
	}
}

// =============================================================================
// Base Handler (common utilities)
// =============================================================================

// BaseHandler provides common functionality for all handlers.
type BaseHandler struct {
	validate *validator.Validate
}

// =============================================================================
// AUTH HANDLERS
// =============================================================================

func (h *BaseHandler) HandleNotImplement(w http.ResponseWriter, r *http.Request) {
	api.HandleError(w, r, api.NewAPIError(
		api.ErrCodeInternal,
		"This endpoint is not yet implemented",
		http.StatusNotImplemented,
	))
}

func (h *BaseHandler) HandleVersion(w http.ResponseWriter, r *http.Request) {
	dto.WriteSuccess(w, r, map[string]string{
		"version":     "1.0.0",
		"api_version": "v1",
		"go_version":  "1.21",
	})
}

// DecodeJSON decodes the request body into v. It rejects unknown fields.
func (h *BaseHandler) DecodeJSON(r *http.Request, v any) *api.APIError {

	if r.Body == nil {
		return api.ErrBadRequest("Request body is empty")
	}

	dec := json.NewDecoder(r.Body)

	dec.DisallowUnknownFields()

	if err := dec.Decode(v); err != nil {
		return api.ErrInvalidJSON(err)
	}
	return nil
}

// ValidateRequest validates v against struct tags and returns a rich APIError.
func (h *BaseHandler) ValidateRequest(v any) *api.APIError {
	if err := h.validate.Struct(v); err != nil {
		return api.FromValidationError(err)
	}
	return nil
}

// WriteErr writes an APIError response with request context (request-id, path …).
func (h *BaseHandler) WriteErr(w http.ResponseWriter, r *http.Request, err *api.APIError) {
	api.WriteErrorWithContext(w, r, err)
}

// MapErr converts any error (sentinel, APIError, pg, …) to an *api.APIError.
func (h *BaseHandler) MapErr(err error) *api.APIError {
	if err == nil {
		return nil
	}

	// Already a typed API error (from api.FromPgError, api.ErrXxx, …)
	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		return apiErr
	}

	// Validator.ValidationErrors coming from service layer (rare but safe)
	var valErrs validator.ValidationErrors
	if errors.As(err, &valErrs) {
		return api.FromValidationError(err)
	}

	// Anything else → 500
	return api.ErrInternal(err)
}

// HandleErr is a convenience that maps then writes.
func (h *BaseHandler) HandleErr(w http.ResponseWriter, r *http.Request, err error) {
	h.WriteErr(w, r, h.MapErr(err))
}

// MustUserID reads the user ID from context; writes 401 and returns false on failure.
func (h *BaseHandler) MustUserID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	raw := middleware.GetUserID(r.Context())
	if raw == "" {
		h.WriteErr(w, r, api.ErrUnauthorized(""))
		return uuid.Nil, false
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		h.WriteErr(w, r, api.ErrUnauthorized("Malformed user ID in token"))
		return uuid.Nil, false
	}
	return id, true
}

// MustTenantID reads the tenant ID from context; writes 401 and returns false on failure.
func (h *BaseHandler) MustTenantID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	raw := middleware.GetTenantID(r.Context())
	if raw == "" {
		h.WriteErr(w, r, api.ErrUnauthorized("Tenant ID missing from token"))
		return uuid.Nil, false
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		h.WriteErr(w, r, api.ErrUnauthorized("Malformed tenant ID in token"))
		return uuid.Nil, false
	}
	return id, true
}

func (h *BaseHandler) ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	return r.RemoteAddr
}
func (h *BaseHandler) BearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(auth, "Bearer "); ok {
		return after
	}
	return ""
}

// =============================================================================
// Response Helpers
// =============================================================================

// WriteSuccess writes a successful JSON response with status 200.
func (h *BaseHandler) WriteSuccess(w http.ResponseWriter, r *http.Request, data any) {
	dto.WriteSuccess(w, r, data)
}

// WriteCreated writes a successful JSON response with status 201.
func (h *BaseHandler) WriteCreated(w http.ResponseWriter, r *http.Request, data any) {
	dto.WriteCreated(w, r, data)
}

// WriteNoContent writes a 204 No Content response.
func (h *BaseHandler) WriteNoContent(w http.ResponseWriter) {
	dto.WriteNoContent(w)
}

// WriteMessage writes a simple message response.
func (h *BaseHandler) WriteMessage(w http.ResponseWriter, r *http.Request, msg string) {
	dto.WriteSuccess(w, r, dto.NewMessageResponse(msg))
}
