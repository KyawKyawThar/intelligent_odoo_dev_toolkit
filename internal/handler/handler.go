package handler

import (
	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"
	"Intelligent_Dev_ToolKit_Odoo/internal/middleware"
	"Intelligent_Dev_ToolKit_Odoo/internal/service"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// Handlers is the main container for all HTTP handlers.
type Handlers struct {
	Auth        *AuthHandler
	Ws          *WsHandler
	Environment *EnvironmentHandler

	// Future handlers:
	// Profiler    *ProfilerHandler
	// Alert       *AlertHandler
	//	APIKey *APIKeyHandler
	// Tenant *TenantHandler
	// User   *UserHandler
}

// NewHandlers creates all handlers with their service dependencies.
func NewHandlers(services *service.Services, store db.Store, logger *zerolog.Logger) *Handlers {
	v := validator.New()
	if err := api.RegisterCustomValidations(v); err != nil {
		panic(err)
	}

	base := &BaseHandler{validate: v, logger: logger}

	authSvc, ok := services.Auth.(*service.AuthService)
	if !ok {
		panic("invalid auth service type")
	}
	envSvc, ok := services.Environment.(*service.EnvironmentService)
	if !ok {
		panic("invalid environment service type")
	}

	return &Handlers{
		Auth:        NewAuthHandler(authSvc, base),
		Environment: NewEnviromentHandler(*envSvc, base),
		Ws:          NewWsHandler(base, store),
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
	logger   *zerolog.Logger
}

// =============================================================================
// AUTH HANDLERS
// =============================================================================

func (h *BaseHandler) HandleNotImplement(w http.ResponseWriter, r *http.Request) {
	api.HandleError(w, r, api.NewError(
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
func (h *BaseHandler) DecodeJSON(r *http.Request, v any) *api.Error {

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
func (h *BaseHandler) ValidateRequest(v any) *api.Error {
	if err := h.validate.Struct(v); err != nil {
		return api.FromValidationError(err)
	}
	return nil
}

// DecodeAndValidate is a convenience helper that decodes and validates a request.
func (h *BaseHandler) DecodeAndValidate(w http.ResponseWriter, r *http.Request, v any) bool {
	if apiErr := h.DecodeJSON(r, v); apiErr != nil {
		h.WriteErr(w, r, apiErr)
		return false
	}
	if apiErr := h.ValidateRequest(v); apiErr != nil {
		h.WriteErr(w, r, apiErr)
		return false
	}
	return true
}

// WriteErr writes an APIError response with request context (request-id, path …).
func (h *BaseHandler) WriteErr(w http.ResponseWriter, r *http.Request, err *api.Error) {
	api.WriteErrorWithContext(w, r, err)
}

// MapErr converts any error (sentinel, APIError, pg, …) to an *api.Error.
func (h *BaseHandler) MapErr(err error) *api.Error {
	if err == nil {
		return nil
	}

	// Already a typed API error (from api.FromPgError, api.ErrXxx, …)
	var apiErr *api.Error
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

func ParseQueryInt32(r *http.Request, key string, defaultVal int32) int32 {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return defaultVal
	}
	v, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return defaultVal
	}
	return int32(v)
}

// MustUUIDParam extracts a UUID path parameter. Returns false and writes an
// error response if the parameter is missing or not a valid UUID.
func (h *EnvironmentHandler) MustUUIDParam(w http.ResponseWriter, r *http.Request, param string) (uuid.UUID, bool) {
	raw := chi.URLParam(r, param)
	if raw == "" {
		h.WriteErr(w, r, api.ErrBadRequest("missing "+param+" path parameter"))
		return uuid.Nil, false
	}

	id, err := uuid.Parse(raw)
	if err != nil {
		h.WriteErr(w, r, api.ErrBadRequest(param+" must be a valid UUID"))
		return uuid.Nil, false
	}

	return id, true
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
