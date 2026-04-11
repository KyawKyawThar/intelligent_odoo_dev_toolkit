package handler

import (
	db "Intelligent_Dev_ToolKit_Odoo/db/sqlc"
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"
	"Intelligent_Dev_ToolKit_Odoo/internal/middleware"
	"Intelligent_Dev_ToolKit_Odoo/internal/service"
	"Intelligent_Dev_ToolKit_Odoo/internal/storage"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

const trueString = "true"

// Handlers is the main container for all HTTP handlers.
type Handlers struct {
	Auth                *AuthHandler
	Ws                  *WsHandler
	Environment         *EnvironmentHandler
	Schema              *SchemaHandler
	Error               *ErrorHandler
	APIKey              *APIKeyHandler
	Batch               *BatchHandler
	AgentRegister       *AgentRegisterHandler
	AgentDistribution   *AgentDistributionHandler
	ACL                 *ACLHandler
	Profiler            *ProfilerHandler
	N1                  *N1Handler
	Budget              *BudgetHandler
	Alert               *AlertHandler
	Overview            *OverviewHandler
	Migration           *MigrationHandler
	Audit               *AuditHandler
	NotificationChannel *NotificationChannelHandler
}

// HandlerDeps holds optional dependencies for handler construction.
type HandlerDeps struct {
	// RedisClient is the underlying redis.Client for stream-based handlers.
	// If nil, stream-based handlers (e.g. BatchHandler) will not be created.
	RedisClient *redis.Client
	// IngestStreamName overrides the Redis stream name for batch ingestion.
	IngestStreamName string
	// S3Client enables agent binary distribution endpoints.
	// If nil, those endpoints are not registered.
	S3Client *storage.S3Client
}

func mustCast[T any](svc any, name string) *T {
	v, ok := svc.(*T)
	if !ok {
		panic("invalid " + name + " service type")
	}
	return v
}

// NewHandlers creates all handlers with their service dependencies.
// NewHandlers creates all handlers with their service dependencies.
func NewHandlers(
	services *service.Services,
	store db.Store,
	logger *zerolog.Logger,
	deps *HandlerDeps,
) *Handlers {

	v := validator.New()
	if err := api.RegisterCustomValidations(v); err != nil {
		panic(err)
	}

	base := &BaseHandler{
		validate: v,
		logger:   logger,
	}

	// --- Extract services (NO cyclomatic explosion) ---
	authSvc := mustCast[service.AuthService](services.Auth, "auth")
	envSvc := mustCast[service.EnvironmentService](services.Environment, "environment")
	schemaSvc := mustCast[service.SchemaService](services.Schema, "schema")
	errorSvc := mustCast[service.ErrorService](services.Error, "error")
	apiKeySvc := mustCast[service.APIKeyService](services.APIKey, "api key")
	agentRegSvc := mustCast[service.AgentRegisterService](services.AgentRegister, "agent register")
	aclSvc := mustCast[service.ACLService](services.ACL, "acl")
	profilerSvc := mustCast[service.ProfilerService](services.Profiler, "profiler")
	n1Svc := mustCast[service.N1Service](services.N1, "n1")
	budgetSvc := mustCast[service.BudgetService](services.Budget, "budget")
	alertSvc := mustCast[service.AlertService](services.Alert, "alert")
	overviewSvc := mustCast[service.OverviewService](services.Overview, "overview")
	migrationSvc := mustCast[service.MigrationService](services.Migration, "migration")

	h := &Handlers{
		Auth:                NewAuthHandler(authSvc, base),
		Environment:         NewEnviromentHandler(*envSvc, base),
		Schema:              NewSchemaHandler(schemaSvc, base),
		Error:               NewErrorHandler(errorSvc, base),
		APIKey:              NewAPIKeyHandler(apiKeySvc, base),
		Ws:                  NewWsHandler(base, store),
		AgentRegister:       NewAgentRegisterHandler(agentRegSvc, base),
		ACL:                 NewACLHandler(aclSvc, base),
		Profiler:            NewProfilerHandler(profilerSvc, base),
		N1:                  NewN1Handler(n1Svc, base),
		Budget:              NewBudgetHandler(budgetSvc, base),
		Alert:               NewAlertHandler(alertSvc, base),
		Overview:            NewOverviewHandler(overviewSvc, base),
		Migration:           NewMigrationHandler(migrationSvc, base),
		Audit:               NewAuditHandler(services.Audit, base),
		NotificationChannel: NewNotificationChannelHandler(services.Notification, base),
	}

	// Optional dependencies
	if deps != nil {
		if deps.RedisClient != nil {
			h.Batch = NewBatchHandler(base, deps.RedisClient, deps.IngestStreamName)
		}
		if deps.S3Client != nil {
			h.AgentDistribution = NewAgentDistributionHandler(deps.S3Client, base)
		}
	}

	return h
}

// func NewHandlers(services *service.Services, store db.Store, logger *zerolog.Logger, deps *HandlerDeps) *Handlers {
// 	v := validator.New()
// 	if err := api.RegisterCustomValidations(v); err != nil {
// 		panic(err)
// 	}

// 	base := &BaseHandler{validate: v, logger: logger}

// 	authSvc, ok := services.Auth.(*service.AuthService)
// 	if !ok {
// 		panic("invalid auth service type")
// 	}
// 	envSvc, ok := services.Environment.(*service.EnvironmentService)
// 	if !ok {
// 		panic("invalid environment service type")
// 	}
// 	schemaSvc, ok := services.Schema.(*service.SchemaService)
// 	if !ok {
// 		panic("invalid schema service type")
// 	}
// 	errorSvc, ok := services.Error.(*service.ErrorService)
// 	if !ok {
// 		panic("invalid error service type")
// 	}
// 	apiKeySvc, ok := services.APIKey.(*service.APIKeyService)
// 	if !ok {
// 		panic("invalid api key service type")
// 	}
// 	agentRegSvc, ok := services.AgentRegister.(*service.AgentRegisterService)
// 	if !ok {
// 		panic("invalid agent register service type")
// 	}
// 	aclSvc, ok := services.ACL.(*service.ACLService)
// 	if !ok {
// 		panic("invalid acl service type")
// 	}
// 	profilerSvc, ok := services.Profiler.(*service.ProfilerService)
// 	if !ok {
// 		panic("invalid profiler service type")
// 	}
// 	n1Svc, ok := services.N1.(*service.N1Service)
// 	if !ok {
// 		panic("invalid n1 service type")
// 	}
// 	budgetSvc, ok := services.Budget.(*service.BudgetService)
// 	if !ok {
// 		panic("invalid budget service type")
// 	}
// 	alertSvc, ok := services.Alert.(*service.AlertService)
// 	if !ok {
// 		panic("invalid alert service type")
// 	}
// 	overviewSvc, ok := services.Overview.(*service.OverviewService)
// 	if !ok {
// 		panic("invalid overview service type")
// 	}
// 	migrationSvc, ok := services.Migration.(*service.MigrationService)
// 	if !ok {
// 		panic("invalid migration service type")
// 	}

// 	h := &Handlers{
// 		Auth:          NewAuthHandler(authSvc, base),
// 		Environment:   NewEnviromentHandler(*envSvc, base),
// 		Schema:        NewSchemaHandler(schemaSvc, base),
// 		Error:         NewErrorHandler(errorSvc, base),
// 		APIKey:        NewAPIKeyHandler(apiKeySvc, base),
// 		Ws:            NewWsHandler(base, store),
// 		AgentRegister: NewAgentRegisterHandler(agentRegSvc, base),
// 		ACL:           NewACLHandler(aclSvc, base),
// 		Profiler:      NewProfilerHandler(profilerSvc, base),
// 		N1:            NewN1Handler(n1Svc, base),
// 		Budget:        NewBudgetHandler(budgetSvc, base),
// 		Alert:         NewAlertHandler(alertSvc, base),
// 		Overview:      NewOverviewHandler(overviewSvc, base),
// 		Migration:     NewMigrationHandler(migrationSvc, base),
// 	}

// 	// Wire up stream-based handlers when Redis is available.
// 	if deps != nil && deps.RedisClient != nil {
// 		h.Batch = NewBatchHandler(base, deps.RedisClient, deps.IngestStreamName)
// 	}

// 	return h
// }

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
	if r.Body == nil || r.Body == http.NoBody {
		return api.ErrBadRequest("Request body is empty")
	}

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(v); err != nil {
		// io.EOF means the body was present but contained no data at all
		// (e.g. Content-Length: 0). Distinguish this from a mid-stream
		// truncation (io.ErrUnexpectedEOF) or a JSON syntax error.
		if errors.Is(err, io.EOF) {
			return api.ErrBadRequest("Request body is empty")
		}
		h.logger.Error().Err(err).
			Str("path", r.URL.Path).
			Msg("JSON decode failed")
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

// MustTenantAndEnvID extracts both the tenant ID (from context) and the env_id
// path parameter. Returns false and writes an error if either fails.
func (h *BaseHandler) MustTenantAndEnvID(w http.ResponseWriter, r *http.Request) (tenantID, envID uuid.UUID, ok bool) {
	tenantID, ok = h.MustTenantID(w, r)
	if !ok {
		return
	}
	envID, ok = h.MustUUIDParam(w, r, "env_id")
	return
}

// MustTenantEnvAndExtraID extracts tenant ID, env_id, and one additional UUID
// path parameter. Returns false and writes an error if any fails.
func (h *BaseHandler) MustTenantEnvAndExtraID(w http.ResponseWriter, r *http.Request, param string) (tenantID, envID, extraID uuid.UUID, ok bool) {
	tenantID, envID, ok = h.MustTenantAndEnvID(w, r)
	if !ok {
		return
	}
	extraID, ok = h.MustUUIDParam(w, r, param)
	return
}

// MustUUIDParam extracts a UUID path parameter. Returns false and writes an
// error response if the parameter is missing or not a valid UUID.
func (h *BaseHandler) MustUUIDParam(w http.ResponseWriter, r *http.Request, param string) (uuid.UUID, bool) {
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
