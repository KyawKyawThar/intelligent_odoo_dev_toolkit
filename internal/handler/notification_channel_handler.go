package handler

import (
	"net/http"

	"Intelligent_Dev_ToolKit_Odoo/internal/dto"
	"Intelligent_Dev_ToolKit_Odoo/internal/service"
)

// NotificationChannelHandler handles CRUD for notification channels.
type NotificationChannelHandler struct {
	*BaseHandler
	svc *service.NotificationService
}

// NewNotificationChannelHandler creates a NotificationChannelHandler.
func NewNotificationChannelHandler(svc *service.NotificationService, base *BaseHandler) *NotificationChannelHandler {
	return &NotificationChannelHandler{BaseHandler: base, svc: svc}
}

// HandleList returns all notification channels for the tenant.
//
//	@Summary      List notification channels
//	@Description  Returns all notification channels configured for the authenticated tenant
//	@Tags         notification-channels
//	@Produce      json
//	@Success      200  {object}  dto.NotificationChannelListResponse
//	@Failure      401  {object}  api.Error
//	@Router       /notification-channels [get]
//	@Security     BearerAuth
func (h *NotificationChannelHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.MustTenantID(w, r)
	if !ok {
		return
	}

	result, err := h.svc.List(r.Context(), tenantID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, result)
}

// HandleCreate creates a new notification channel.
//
//	@Summary      Create notification channel
//	@Description  Adds a new notification channel (slack, webhook, or email) for the tenant
//	@Tags         notification-channels
//	@Accept       json
//	@Produce      json
//	@Param        body  body      dto.CreateNotificationChannelRequest  true  "Channel config"
//	@Success      201   {object}  dto.NotificationChannelResponse
//	@Failure      400   {object}  api.Error
//	@Failure      401   {object}  api.Error
//	@Router       /notification-channels [post]
//	@Security     BearerAuth
func (h *NotificationChannelHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.MustTenantID(w, r)
	if !ok {
		return
	}

	var req dto.CreateNotificationChannelRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	result, err := h.svc.Create(r.Context(), tenantID, &req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteCreated(w, r, result)
}

// HandleGet returns a single notification channel by ID.
//
//	@Summary      Get notification channel
//	@Description  Returns a single notification channel by its ID
//	@Tags         notification-channels
//	@Produce      json
//	@Param        channel_id  path  string  true  "Channel UUID"
//	@Success      200  {object}  dto.NotificationChannelResponse
//	@Failure      401  {object}  api.Error
//	@Failure      404  {object}  api.Error
//	@Router       /notification-channels/{channel_id} [get]
//	@Security     BearerAuth
func (h *NotificationChannelHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.MustTenantID(w, r)
	if !ok {
		return
	}

	channelID, ok := h.MustUUIDParam(w, r, "channel_id")
	if !ok {
		return
	}

	result, err := h.svc.Get(r.Context(), tenantID, channelID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, result)
}

// HandleUpdate replaces a notification channel's configuration.
//
//	@Summary      Update notification channel
//	@Description  Replaces the channel name, type, config, and active status
//	@Tags         notification-channels
//	@Accept       json
//	@Produce      json
//	@Param        channel_id  path  string                                true  "Channel UUID"
//	@Param        body        body  dto.UpdateNotificationChannelRequest  true  "Updated channel"
//	@Success      200  {object}  dto.NotificationChannelResponse
//	@Failure      400  {object}  api.Error
//	@Failure      401  {object}  api.Error
//	@Failure      404  {object}  api.Error
//	@Router       /notification-channels/{channel_id} [patch]
//	@Security     BearerAuth
func (h *NotificationChannelHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.MustTenantID(w, r)
	if !ok {
		return
	}

	channelID, ok := h.MustUUIDParam(w, r, "channel_id")
	if !ok {
		return
	}

	var req dto.UpdateNotificationChannelRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	result, err := h.svc.Update(r.Context(), tenantID, channelID, &req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, result)
}

// HandleDelete removes a notification channel.
//
//	@Summary      Delete notification channel
//	@Description  Permanently removes the notification channel
//	@Tags         notification-channels
//	@Produce      json
//	@Param        channel_id  path  string  true  "Channel UUID"
//	@Success      204
//	@Failure      401  {object}  api.Error
//	@Failure      404  {object}  api.Error
//	@Router       /notification-channels/{channel_id} [delete]
//	@Security     BearerAuth
func (h *NotificationChannelHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.MustTenantID(w, r)
	if !ok {
		return
	}

	channelID, ok := h.MustUUIDParam(w, r, "channel_id")
	if !ok {
		return
	}

	if err := h.svc.Delete(r.Context(), tenantID, channelID); err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteNoContent(w)
}
