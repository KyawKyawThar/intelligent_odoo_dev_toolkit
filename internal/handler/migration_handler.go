package handler

import (
	"net/http"

	"Intelligent_Dev_ToolKit_Odoo/internal/dto"
	"Intelligent_Dev_ToolKit_Odoo/internal/service"
)

// MigrationHandler handles deprecation scan endpoints.
type MigrationHandler struct {
	*BaseHandler
	svc *service.MigrationService
}

// NewMigrationHandler creates a MigrationHandler.
func NewMigrationHandler(svc *service.MigrationService, base *BaseHandler) *MigrationHandler {
	return &MigrationHandler{BaseHandler: base, svc: svc}
}

// HandleRunScan triggers a deprecation scan on the environment's latest schema.
//
//	@Summary      Run migration scan
//	@Description  Scan the environment's schema snapshot against the Odoo deprecation database for the given version transition
//	@Tags         migration
//	@Accept       json
//	@Produce      json
//	@Param        env_id  path   string                     true  "Environment ID"
//	@Param        body    body   dto.RunMigrationScanRequest true  "Scan parameters"
//	@Success      201  {object}  dto.MigrationScanResponse
//	@Failure      400  {object}  api.Error
//	@Failure      401  {object}  api.Error
//	@Failure      404  {object}  api.Error
//	@Router       /environments/{env_id}/migration/scan [post]
//	@Security     BearerAuth
func (h *MigrationHandler) HandleRunScan(w http.ResponseWriter, r *http.Request) {
	tenantID, envID, ok := h.MustTenantAndEnvID(w, r)
	if !ok {
		return
	}

	userID, ok := h.MustUserID(w, r)
	if !ok {
		return
	}

	var req dto.RunMigrationScanRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	result, err := h.svc.RunScan(r.Context(), tenantID, envID, &userID, &req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteCreated(w, r, result)
}

// HandleGetScan returns one migration scan by ID.
//
//	@Summary      Get migration scan
//	@Description  Retrieve a specific migration scan result
//	@Tags         migration
//	@Produce      json
//	@Param        env_id   path  string  true  "Environment ID"
//	@Param        scan_id  path  string  true  "Scan ID"
//	@Success      200  {object}  dto.MigrationScanResponse
//	@Failure      401  {object}  api.Error
//	@Failure      404  {object}  api.Error
//	@Router       /environments/{env_id}/migration/scans/{scan_id} [get]
//	@Security     BearerAuth
func (h *MigrationHandler) HandleGetScan(w http.ResponseWriter, r *http.Request) {
	tenantID, envID, scanID, ok := h.MustTenantEnvAndExtraID(w, r, "scan_id")
	if !ok {
		return
	}

	result, err := h.svc.GetScan(r.Context(), tenantID, envID, scanID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, result)
}

// HandleGetLatestScan returns the most recent scan for the environment.
//
//	@Summary      Get latest migration scan
//	@Description  Retrieve the most recent migration scan result for the environment
//	@Tags         migration
//	@Produce      json
//	@Param        env_id  path  string  true  "Environment ID"
//	@Success      200  {object}  dto.MigrationScanResponse
//	@Failure      401  {object}  api.Error
//	@Failure      404  {object}  api.Error
//	@Router       /environments/{env_id}/migration/scans/latest [get]
//	@Security     BearerAuth
func (h *MigrationHandler) HandleGetLatestScan(w http.ResponseWriter, r *http.Request) {
	tenantID, envID, ok := h.MustTenantAndEnvID(w, r)
	if !ok {
		return
	}

	result, err := h.svc.GetLatestScan(r.Context(), tenantID, envID)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, result)
}

// HandleListScans returns a paginated list of migration scans.
//
//	@Summary      List migration scans
//	@Description  List migration scan results for an environment
//	@Tags         migration
//	@Produce      json
//	@Param        env_id  path   string  true   "Environment ID"
//	@Param        limit   query  int     false  "Page size (default 20)"
//	@Param        offset  query  int     false  "Offset (default 0)"
//	@Success      200  {object}  dto.MigrationScanListResponse
//	@Failure      401  {object}  api.Error
//	@Router       /environments/{env_id}/migration/scans [get]
//	@Security     BearerAuth
func (h *MigrationHandler) HandleListScans(w http.ResponseWriter, r *http.Request) {
	tenantID, envID, ok := h.MustTenantAndEnvID(w, r)
	if !ok {
		return
	}

	req := &dto.ListMigrationScansRequest{
		Limit:  ParseQueryInt32(r, "limit", 20),
		Offset: ParseQueryInt32(r, "offset", 0),
	}

	result, err := h.svc.ListScans(r.Context(), tenantID, envID, req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, result)
}

// HandleDeleteScan removes a migration scan record.
//
//	@Summary      Delete migration scan
//	@Description  Remove a migration scan result
//	@Tags         migration
//	@Produce      json
//	@Param        env_id   path  string  true  "Environment ID"
//	@Param        scan_id  path  string  true  "Scan ID"
//	@Success      204  "No Content"
//	@Failure      401  {object}  api.Error
//	@Failure      404  {object}  api.Error
//	@Router       /environments/{env_id}/migration/scans/{scan_id} [delete]
//	@Security     BearerAuth
func (h *MigrationHandler) HandleDeleteScan(w http.ResponseWriter, r *http.Request) {
	tenantID, envID, scanID, ok := h.MustTenantEnvAndExtraID(w, r, "scan_id")
	if !ok {
		return
	}

	if err := h.svc.DeleteScan(r.Context(), tenantID, envID, scanID); err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteNoContent(w)
}

// HandleSupportedTransitions lists the version pairs the deprecation DB covers.
//
//	@Summary      Supported migration transitions
//	@Description  List the Odoo version transitions covered by the deprecation database
//	@Tags         migration
//	@Produce      json
//	@Success      200  {object}  dto.SupportedTransitionsResponse
//	@Router       /migration/transitions [get]
//	@Security     BearerAuth
func (h *MigrationHandler) HandleSupportedTransitions(w http.ResponseWriter, r *http.Request) {
	h.WriteSuccess(w, r, h.svc.SupportedTransitions())
}

// HandleScanSource scans uploaded Odoo module files for deprecated Python/XML patterns.
//
//	@Summary      Scan module source code
//	@Description  Analyze uploaded Python and XML source files for deprecated patterns in the given version transition path. Returns per-file, per-line findings with remediation hints.
//	@Tags         migration
//	@Accept       json
//	@Produce      json
//	@Param        body  body   dto.ScanSourceRequest  true  "Files and version transition"
//	@Success      200   {object}  dto.ScanSourceResponse
//	@Failure      400   {object}  api.Error
//	@Failure      401   {object}  api.Error
//	@Router       /migration/scan/source [post]
//	@Security     BearerAuth
func (h *MigrationHandler) HandleScanSource(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.MustTenantID(w, r)
	if !ok {
		return
	}

	var req dto.ScanSourceRequest
	if !h.DecodeAndValidate(w, r, &req) {
		return
	}

	result, err := h.svc.ScanSourceCode(r.Context(), tenantID, &req)
	if err != nil {
		h.HandleErr(w, r, err)
		return
	}

	h.WriteSuccess(w, r, result)
}
