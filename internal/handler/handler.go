package handler

import (
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/middleware"
	"Intelligent_Dev_ToolKit_Odoo/internal/service"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

type Handler struct {
	Auth *AuthHandler
}

type AuthHandler struct {
	svc      *service.AuthService
	validate *validator.Validate
}

func NewHandler(services *service.Services) *Handler {
	v := validator.New()
	api.RegisterCustomValidations(v) // slug, env_type, odoo_version, …

	return &Handler{
		Auth: &AuthHandler{
			svc:      services.Auth,
			validate: v,
		},
	}
}

// =============================================================================
// AUTH HANDLERS
// =============================================================================

func (h *AuthHandler) HandleNotImplement(w http.ResponseWriter, r *http.Request) {
	h.svc.NotImplemented(w, r)
}

func (h *AuthHandler) HandleVersion(w http.ResponseWriter, r *http.Request) {
	h.svc.ServiceVersion(w, r)
}

// DecodeJSON decodes the request body into v. It rejects unknown fields.
func (h *AuthHandler) DecodeJSON(r *http.Request, v any) *api.APIError {

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
func (h *AuthHandler) ValidateRequest(v any) *api.APIError {
	if err := h.validate.Struct(v); err != nil {
		return api.FromValidationError(err)
	}
	return nil
}

// WriteErr writes an APIError response with request context (request-id, path …).
func (h *AuthHandler) WriteErr(w http.ResponseWriter, r *http.Request, err *api.APIError) {
	api.WriteErrorWithContext(w, r, err)
}

// MapErr converts any error (sentinel, APIError, pg, …) to an *api.APIError.
func (h *AuthHandler) MapErr(err error) *api.APIError {
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
func (h *AuthHandler) HandleErr(w http.ResponseWriter, r *http.Request, err error) {
	h.WriteErr(w, r, h.MapErr(err))
}

// MustUserID reads the user ID from context; writes 401 and returns false on failure.
func (h *AuthHandler) MustUserID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
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
func (h *AuthHandler) MustTenantID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
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

func (h *AuthHandler) ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	return r.RemoteAddr
}
func (h *AuthHandler) BearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(auth, "Bearer "); ok {
		return after
	}
	return ""
}
