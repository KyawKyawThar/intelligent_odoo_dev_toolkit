package handler

import (
	"net/http"
	"strings"
	"time"

	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/storage"
)

// AgentDistributionHandler serves agent binary downloads, version info,
// and the install script — all from S3/R2 so the source repo can stay private.
//
// S3 layout expected:
//
//	agents/latest                                     → plain-text "v0.1.0"
//	agents/install.sh                                 → install script
//	agents/<version>/odoodevtools-agent-<platform>    → binary
//	agents/<version>/checksums.txt                    → SHA-256 checksums
type AgentDistributionHandler struct {
	*BaseHandler
	s3 *storage.S3Client
}

func NewAgentDistributionHandler(s3 *storage.S3Client, base *BaseHandler) *AgentDistributionHandler {
	return &AgentDistributionHandler{BaseHandler: base, s3: s3}
}

var validPlatforms = map[string]bool{
	"linux-amd64":  true,
	"linux-arm64":  true,
	"linux-armv7":  true,
	"darwin-amd64": true,
	"darwin-arm64": true,
}

// resolveVersion returns the concrete version string: if v is empty or "latest"
// it reads agents/latest from S3; otherwise it returns v unchanged.
func (h *AgentDistributionHandler) resolveVersion(w http.ResponseWriter, r *http.Request, v string) (string, bool) {
	if v != "" && v != "latest" {
		return v, true
	}
	data, err := h.s3.Get(r.Context(), "agents/latest")
	if err != nil {
		h.WriteErr(w, r, api.ErrInternal(err))
		return "", false
	}
	return strings.TrimSpace(string(data)), true
}

// HandleAgentVersion godoc
// @Summary      Get latest agent version
// @Description  Returns the latest published agent version tag (e.g. "v0.1.0").
// @Tags         agent-distribution
// @Produce      json
// @Success      200  {object}  map[string]string
// @Router       /api/v1/agent/version [get]
func (h *AgentDistributionHandler) HandleAgentVersion(w http.ResponseWriter, r *http.Request) {
	data, err := h.s3.Get(r.Context(), "agents/latest")
	if err != nil {
		h.WriteErr(w, r, api.ErrInternal(err))
		return
	}
	version := strings.TrimSpace(string(data))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=60")
	writeJSONMap(w, http.StatusOK, map[string]string{"latest": version})
}

// HandleAgentDownload godoc
// @Summary      Download agent binary
// @Description  Redirects to a pre-signed S3 URL for the requested platform binary.
// @Tags         agent-distribution
// @Param        version   query  string  false  "Version tag (default: latest)"
// @Param        platform  query  string  true   "Target platform (linux-amd64 | linux-arm64 | linux-armv7 | darwin-amd64 | darwin-arm64)"
// @Success      302
// @Failure      400  {object}  api.Error
// @Router       /api/v1/agent/download [get]
func (h *AgentDistributionHandler) HandleAgentDownload(w http.ResponseWriter, r *http.Request) {
	platform := r.URL.Query().Get("platform")
	if platform == "" {
		h.WriteErr(w, r, api.ErrBadRequest("platform is required (e.g. linux-amd64)"))
		return
	}
	if !validPlatforms[platform] {
		h.WriteErr(w, r, api.ErrBadRequest("unsupported platform: "+platform+". Valid: linux-amd64, linux-arm64, linux-armv7, darwin-amd64, darwin-arm64"))
		return
	}

	version, ok := h.resolveVersion(w, r, r.URL.Query().Get("version"))
	if !ok {
		return
	}

	key := "agents/" + version + "/odoodevtools-agent-" + platform
	presignedURL, err := h.s3.PresignGetURL(r.Context(), key, 15*time.Minute)
	if err != nil {
		h.WriteErr(w, r, api.ErrInternal(err))
		return
	}

	http.Redirect(w, r, presignedURL, http.StatusFound)
}

// HandleAgentChecksums godoc
// @Summary      Get agent checksums
// @Description  Returns the checksums.txt for a given (or latest) agent version.
// @Tags         agent-distribution
// @Param        version  query  string  false  "Version tag (default: latest)"
// @Produce      text/plain
// @Success      200  {string}  string
// @Router       /api/v1/agent/checksums [get]
func (h *AgentDistributionHandler) HandleAgentChecksums(w http.ResponseWriter, r *http.Request) {
	version, ok := h.resolveVersion(w, r, r.URL.Query().Get("version"))
	if !ok {
		return
	}

	data, err := h.s3.Get(r.Context(), "agents/"+version+"/checksums.txt")
	if err != nil {
		h.WriteErr(w, r, api.ErrNotFound("checksums not found for version "+version))
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// HandleInstallScript godoc
// @Summary      Get agent install script
// @Description  Returns the install-agent.sh bash script. Pipe directly into bash.
// @Tags         agent-distribution
// @Produce      text/plain
// @Success      200  {string}  string
// @Router       /install [get]
func (h *AgentDistributionHandler) HandleInstallScript(w http.ResponseWriter, r *http.Request) {
	data, err := h.s3.Get(r.Context(), "agents/install.sh")
	if err != nil {
		h.WriteErr(w, r, api.ErrInternal(err))
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// writeJSONMap is a minimal JSON writer to avoid importing dto here.
func writeJSONMap(w http.ResponseWriter, status int, m map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	b := []byte(`{`)
	first := true
	for k, v := range m {
		if !first {
			b = append(b, ',')
		}
		b = append(b, []byte(`"`+k+`":"`+v+`"`)...)
		first = false
	}
	b = append(b, '}')
	_, _ = w.Write(b)
}
