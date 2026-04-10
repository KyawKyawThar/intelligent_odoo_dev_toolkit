package handler

import (
	"net/http"
	"time"

	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/service"

	"github.com/go-chi/chi/v5"
)

// AuditHandler handles audit log REST endpoints.
type AuditHandler struct {
	*BaseHandler
	svc *service.AuditService
}

// NewAuditHandler creates an AuditHandler.
func NewAuditHandler(svc *service.AuditService, base *BaseHandler) *AuditHandler {
	return &AuditHandler{BaseHandler: base, svc: svc}
}

// HandleList returns a paginated list of audit logs for the authenticated tenant.
//
//	@Summary      List audit logs
//	@Description  Returns a paginated list of all audit log entries for the tenant
//	@Tags         audit
//	@Produce      json
//	@Param        limit   query  int  false  "Max entries to return (default 50, max 200)"
//	@Param        offset  query  int  false  "Pagination offset (default 0)"
//	@Success      200  {object}  dto.AuditLogListResponse
//	@Failure      401  {object}  api.Error
//	@Router       /audit-logs [get]
//	@Security     BearerAuth
func (h *AuditHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.MustTenantID(w, r)
	if !ok {
		return
	}

	result, err := h.svc.List(
		r.Context(),
		tenantID,
		ParseQueryInt32(r, "limit", 50),
		ParseQueryInt32(r, "offset", 0),
	)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, result)
}

// HandleListByAction returns audit logs filtered to a specific action.
//
//	@Summary      List audit logs by action
//	@Description  Returns audit logs matching the given action string (e.g. "POST /api/v1/environments")
//	@Tags         audit
//	@Produce      json
//	@Param        action  path   string  true   "Action string"
//	@Param        limit   query  int     false  "Max entries to return (default 50)"
//	@Param        offset  query  int     false  "Pagination offset (default 0)"
//	@Success      200  {object}  dto.AuditLogListResponse
//	@Failure      401  {object}  api.Error
//	@Router       /audit-logs/by-action/{action} [get]
//	@Security     BearerAuth
func (h *AuditHandler) HandleListByAction(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.MustTenantID(w, r)
	if !ok {
		return
	}

	action := chi.URLParam(r, "action")

	result, err := h.svc.ListByAction(
		r.Context(),
		tenantID,
		action,
		ParseQueryInt32(r, "limit", 50),
		ParseQueryInt32(r, "offset", 0),
	)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, result)
}

// HandleListByUser returns audit logs for a specific user.
//
//	@Summary      List audit logs by user
//	@Description  Returns audit logs performed by the specified user
//	@Tags         audit
//	@Produce      json
//	@Param        user_id  path   string  true   "User UUID"
//	@Param        limit    query  int     false  "Max entries to return (default 50)"
//	@Param        offset   query  int     false  "Pagination offset (default 0)"
//	@Success      200  {object}  dto.AuditLogListResponse
//	@Failure      400  {object}  api.Error
//	@Failure      401  {object}  api.Error
//	@Router       /audit-logs/by-user/{user_id} [get]
//	@Security     BearerAuth
func (h *AuditHandler) HandleListByUser(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.MustTenantID(w, r)
	if !ok {
		return
	}

	userID, ok := h.MustUUIDParam(w, r, "user_id")
	if !ok {
		return
	}

	result, err := h.svc.ListByUser(
		r.Context(),
		tenantID,
		userID,
		ParseQueryInt32(r, "limit", 50),
		ParseQueryInt32(r, "offset", 0),
	)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, result)
}

// HandleListBetween returns audit logs within a time range.
//
//	@Summary      List audit logs between timestamps
//	@Description  Returns audit logs created between `from` and `to` (RFC3339 format)
//	@Tags         audit
//	@Produce      json
//	@Param        from    query  string  true   "Start time (RFC3339)"
//	@Param        to      query  string  true   "End time (RFC3339)"
//	@Param        limit   query  int     false  "Max entries to return (default 50)"
//	@Param        offset  query  int     false  "Pagination offset (default 0)"
//	@Success      200  {object}  dto.AuditLogListResponse
//	@Failure      400  {object}  api.Error
//	@Failure      401  {object}  api.Error
//	@Router       /audit-logs/between [get]
//	@Security     BearerAuth
func (h *AuditHandler) HandleListBetween(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.MustTenantID(w, r)
	if !ok {
		return
	}

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	from, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		h.WriteErr(w, r, api.ErrBadRequest("'from' must be a valid RFC3339 timestamp"))
		return
	}
	to, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		h.WriteErr(w, r, api.ErrBadRequest("'to' must be a valid RFC3339 timestamp"))
		return
	}

	result, svcErr := h.svc.ListBetween(
		r.Context(),
		tenantID,
		from, to,
		ParseQueryInt32(r, "limit", 50),
		ParseQueryInt32(r, "offset", 0),
	)
	if svcErr != nil {
		h.HandleErr(w, r, svcErr)
		return
	}

	h.WriteSuccess(w, r, result)
}

// HandleListByResource returns audit logs for a specific resource.
//
//	@Summary      List audit logs by resource
//	@Description  Returns audit logs for a specific resource type and ID
//	@Tags         audit
//	@Produce      json
//	@Param        resource     query  string  true   "Resource type (e.g. environment, api_key)"
//	@Param        resource_id  query  string  true   "Resource ID"
//	@Param        limit        query  int     false  "Max entries to return (default 50)"
//	@Success      200  {object}  dto.AuditLogListResponse
//	@Failure      400  {object}  api.Error
//	@Failure      401  {object}  api.Error
//	@Router       /audit-logs/by-resource [get]
//	@Security     BearerAuth
func (h *AuditHandler) HandleListByResource(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.MustTenantID(w, r)
	if !ok {
		return
	}

	resource := r.URL.Query().Get("resource")
	resourceID := r.URL.Query().Get("resource_id")
	if resource == "" || resourceID == "" {
		h.WriteErr(w, r, api.ErrBadRequest("'resource' and 'resource_id' query params are required"))
		return
	}

	result, err := h.svc.ListByResource(
		r.Context(),
		tenantID,
		resource,
		resourceID,
		ParseQueryInt32(r, "limit", 50),
	)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, result)
}
