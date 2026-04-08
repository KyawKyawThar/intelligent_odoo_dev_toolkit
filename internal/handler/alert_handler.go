package handler

import (
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"
	"Intelligent_Dev_ToolKit_Odoo/internal/service"
	"net/http"
)

// AlertHandler handles alert REST endpoints.
type AlertHandler struct {
	*BaseHandler
	svc *service.AlertService
}

// NewAlertHandler creates an AlertHandler.
func NewAlertHandler(svc *service.AlertService, base *BaseHandler) *AlertHandler {
	return &AlertHandler{BaseHandler: base, svc: svc}
}

// HandleList returns a paginated list of alerts for an environment.
//
//	@Summary      List alerts
//	@Description  Returns alerts for the environment, optionally filtered to unacknowledged only
//	@Tags         alerts
//	@Produce      json
//	@Param        env_id           path   string  true   "Environment ID"
//	@Param        limit            query  int     false  "Max alerts to return (default 50, max 200)"
//	@Param        offset           query  int     false  "Pagination offset (default 0)"
//	@Param        unacknowledged   query  bool    false  "If true, return only unacknowledged alerts"
//	@Success      200  {object}  dto.AlertListResponse
//	@Failure      401  {object}  api.Error
//	@Failure      404  {object}  api.Error
//	@Router       /environments/{env_id}/alerts [get]
//	@Security     BearerAuth
func (h *AlertHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	tenantID, envID, ok := h.MustTenantAndEnvID(w, r)
	if !ok {
		return
	}

	req := &dto.ListAlertsRequest{
		Limit:          ParseQueryInt32(r, "limit", 50),
		Offset:         ParseQueryInt32(r, "offset", 0),
		Unacknowledged: r.URL.Query().Get("unacknowledged") == trueString,
	}

	result, err := h.svc.List(r.Context(), tenantID, envID, req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, result)
}

// HandleGet returns a single alert by ID.
//
//	@Summary      Get alert
//	@Description  Returns a single alert by its ID, scoped to the environment
//	@Tags         alerts
//	@Produce      json
//	@Param        env_id    path  string  true  "Environment ID"
//	@Param        alert_id  path  string  true  "Alert ID"
//	@Success      200  {object}  dto.AlertResponse
//	@Failure      401  {object}  api.Error
//	@Failure      404  {object}  api.Error
//	@Router       /environments/{env_id}/alerts/{alert_id} [get]
//	@Security     BearerAuth
func (h *AlertHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	tenantID, envID, alertID, ok := h.MustTenantEnvAndExtraID(w, r, "alert_id")
	if !ok {
		return
	}

	result, err := h.svc.GetByID(r.Context(), tenantID, envID, alertID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, result)
}

// HandleAcknowledge marks a single alert as acknowledged.
//
//	@Summary      Acknowledge alert
//	@Description  Marks the alert as acknowledged by the authenticated user
//	@Tags         alerts
//	@Produce      json
//	@Param        env_id    path  string  true  "Environment ID"
//	@Param        alert_id  path  string  true  "Alert ID"
//	@Success      200  {object}  dto.AlertResponse
//	@Failure      401  {object}  api.Error
//	@Failure      404  {object}  api.Error
//	@Router       /environments/{env_id}/alerts/{alert_id}/acknowledge [post]
//	@Security     BearerAuth
func (h *AlertHandler) HandleAcknowledge(w http.ResponseWriter, r *http.Request) {
	tenantID, envID, alertID, ok := h.MustTenantEnvAndExtraID(w, r, "alert_id")
	if !ok {
		return
	}

	userID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	result, err := h.svc.Acknowledge(r.Context(), tenantID, envID, alertID, userID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, result)
}

// HandleAcknowledgeAll marks all unacknowledged alerts as acknowledged.
//
//	@Summary      Acknowledge all alerts
//	@Description  Marks every unacknowledged alert in the environment as acknowledged
//	@Tags         alerts
//	@Produce      json
//	@Param        env_id  path  string  true  "Environment ID"
//	@Success      200  {object}  dto.AcknowledgeAllResponse
//	@Failure      401  {object}  api.Error
//	@Failure      404  {object}  api.Error
//	@Router       /environments/{env_id}/alerts/acknowledge-all [post]
//	@Security     BearerAuth
func (h *AlertHandler) HandleAcknowledgeAll(w http.ResponseWriter, r *http.Request) {
	tenantID, envID, ok := h.MustTenantAndEnvID(w, r)
	if !ok {
		return
	}

	userID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	result, err := h.svc.AcknowledgeAll(r.Context(), tenantID, envID, userID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, result)
}

// HandleCount returns the number of unacknowledged alerts (for UI badge).
//
//	@Summary      Count unacknowledged alerts
//	@Description  Returns the count of unacknowledged alerts for the environment, useful for notification badges
//	@Tags         alerts
//	@Produce      json
//	@Param        env_id  path  string  true  "Environment ID"
//	@Success      200  {object}  dto.AlertCountResponse
//	@Failure      401  {object}  api.Error
//	@Failure      404  {object}  api.Error
//	@Router       /environments/{env_id}/alerts/count [get]
//	@Security     BearerAuth
func (h *AlertHandler) HandleCount(w http.ResponseWriter, r *http.Request) {
	tenantID, envID, ok := h.MustTenantAndEnvID(w, r)
	if !ok {
		return
	}

	result, err := h.svc.CountUnacknowledged(r.Context(), tenantID, envID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, result)
}
