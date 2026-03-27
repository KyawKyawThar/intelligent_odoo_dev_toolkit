package handler

import (
	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"
	"Intelligent_Dev_ToolKit_Odoo/internal/service"
	"net/http"
)

// ProfilerHandler handles the profiler recording endpoints.
type ProfilerHandler struct {
	*BaseHandler
	svc *service.ProfilerService
}

// NewProfilerHandler creates a ProfilerHandler.
func NewProfilerHandler(svc *service.ProfilerService, base *BaseHandler) *ProfilerHandler {
	return &ProfilerHandler{BaseHandler: base, svc: svc}
}

// HandleGetRecording returns a single profiler recording with its built waterfall.
//
//	@Summary      Get profiler recording
//	@Description  Retrieve a profiler recording with a fully-built waterfall timeline
//	@Tags         profiler
//	@Produce      json
//	@Param        env_id        path  string  true  "Environment ID"
//	@Param        recording_id  path  string  true  "Recording ID"
//	@Success      200  {object}  dto.ProfilerRecordingResponse
//	@Failure      400  {object}  api.Error
//	@Failure      401  {object}  api.Error
//	@Failure      404  {object}  api.Error
//	@Router       /environments/{env_id}/profiler/recordings/{recording_id} [get]
//	@Security     BearerAuth
func (h *ProfilerHandler) HandleGetRecording(w http.ResponseWriter, r *http.Request) {
	tenantID, envID, recordingID, ok := h.MustTenantEnvAndExtraID(w, r, "recording_id")
	if !ok {
		return
	}

	result, err := h.svc.GetRecording(r.Context(), tenantID, envID, recordingID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, result)
}

// HandleListRecordings returns a paginated list of profiler recordings.
//
//	@Summary      List profiler recordings
//	@Description  List profiler recordings for an environment (lightweight, no waterfall)
//	@Tags         profiler
//	@Produce      json
//	@Param        env_id  path   string  true   "Environment ID"
//	@Param        limit   query  int     false  "Page size (default 20)"
//	@Param        offset  query  int     false  "Offset (default 0)"
//	@Success      200  {object}  dto.ProfilerRecordingListResponse
//	@Failure      400  {object}  api.Error
//	@Failure      401  {object}  api.Error
//	@Router       /environments/{env_id}/profiler/recordings [get]
//	@Security     BearerAuth
func (h *ProfilerHandler) HandleListRecordings(w http.ResponseWriter, r *http.Request) {
	tenantID, envID, ok := h.MustTenantAndEnvID(w, r)
	if !ok {
		return
	}

	req := &dto.ListProfilerRecordingsRequest{
		Limit:  ParseQueryInt32(r, "limit", 20),
		Offset: ParseQueryInt32(r, "offset", 0),
	}

	result, err := h.svc.ListRecordings(r.Context(), tenantID, envID, req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, result)
}

// HandleListSlowRecordings returns recordings exceeding a duration threshold.
//
//	@Summary      List slow profiler recordings
//	@Description  List profiler recordings that exceed the given ms threshold
//	@Tags         profiler
//	@Produce      json
//	@Param        env_id        path   string  true   "Environment ID"
//	@Param        threshold_ms  query  int     true   "Duration threshold in milliseconds"
//	@Param        limit         query  int     false  "Max results (default 20)"
//	@Success      200  {object}  dto.ProfilerRecordingListResponse
//	@Failure      400  {object}  api.Error
//	@Failure      401  {object}  api.Error
//	@Router       /environments/{env_id}/profiler/recordings/slow [get]
//	@Security     BearerAuth
func (h *ProfilerHandler) HandleListSlowRecordings(w http.ResponseWriter, r *http.Request) {
	tenantID, envID, ok := h.MustTenantAndEnvID(w, r)
	if !ok {
		return
	}

	thresholdMS := ParseQueryInt32(r, "threshold_ms", 0)
	if thresholdMS <= 0 {
		h.WriteErr(w, r, api.ErrBadRequest("threshold_ms query parameter is required and must be > 0"))
		return
	}

	req := &dto.ListSlowRecordingsRequest{
		ThresholdMS: thresholdMS,
		Limit:       ParseQueryInt32(r, "limit", 20),
	}

	result, err := h.svc.ListSlowRecordings(r.Context(), tenantID, envID, req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, result)
}
