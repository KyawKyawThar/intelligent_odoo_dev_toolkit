package handler

import (
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"
	"Intelligent_Dev_ToolKit_Odoo/internal/service"
	"net/http"
	"time"
)

// N1Handler handles the N+1 detector endpoints.
type N1Handler struct {
	*BaseHandler
	svc *service.N1Service
}

// NewN1Handler creates an N1Handler.
func NewN1Handler(svc *service.N1Service, base *BaseHandler) *N1Handler {
	return &N1Handler{BaseHandler: base, svc: svc}
}

// HandleDetect runs the full N+1 analysis pipeline.
//
//	@Summary      Detect N+1 patterns
//	@Description  Analyze ORM stats and profiler recordings to detect, score, and suggest fixes for N+1 query patterns
//	@Tags         n1
//	@Produce      json
//	@Param        env_id  path   string  true   "Environment ID"
//	@Param        since   query  string  false  "ISO 8601 timestamp to limit analysis window (default: last 24h)"
//	@Param        limit   query  int     false  "Max patterns to return (default 50)"
//	@Success      200  {object}  dto.N1DetectionResponse
//	@Failure      400  {object}  api.Error
//	@Failure      401  {object}  api.Error
//	@Failure      404  {object}  api.Error
//	@Router       /environments/{env_id}/n1/detect [get]
//	@Security     BearerAuth
func (h *N1Handler) HandleDetect(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.MustTenantID(w, r)
	if !ok {
		return
	}

	envID, ok := h.MustUUIDParam(w, r, "env_id")
	if !ok {
		return
	}

	req := &dto.N1DetectionRequest{
		Since: parseQueryTime(r, "since", time.Now().UTC().Add(-24*time.Hour)),
		Limit: ParseQueryInt32(r, "limit", 50),
	}

	result, err := h.svc.Detect(r.Context(), tenantID, envID, req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, result)
}

// HandleTimeline returns per-period N+1 trend data for charts.
//
//	@Summary      N+1 timeline
//	@Description  Returns per-period N+1 detection counts and durations for trend visualization
//	@Tags         n1
//	@Produce      json
//	@Param        env_id  path   string  true   "Environment ID"
//	@Param        since   query  string  false  "ISO 8601 start time (default: last 24h)"
//	@Param        limit   query  int     false  "Max data points (default 100)"
//	@Success      200  {object}  dto.N1TimelineResponse
//	@Failure      400  {object}  api.Error
//	@Failure      401  {object}  api.Error
//	@Router       /environments/{env_id}/n1/timeline [get]
//	@Security     BearerAuth
func (h *N1Handler) HandleTimeline(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.MustTenantID(w, r)
	if !ok {
		return
	}

	envID, ok := h.MustUUIDParam(w, r, "env_id")
	if !ok {
		return
	}

	since := parseQueryTime(r, "since", time.Now().UTC().Add(-24*time.Hour))
	limit := ParseQueryInt32(r, "limit", 100)

	points, err := h.svc.GetTimeline(r.Context(), tenantID, envID, since, limit)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, dto.N1TimelineResponse{Points: points})
}

// parseQueryTime parses an ISO 8601 time from a query parameter.
func parseQueryTime(r *http.Request, key string, defaultVal time.Time) time.Time {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return defaultVal
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return defaultVal
	}
	return t
}
