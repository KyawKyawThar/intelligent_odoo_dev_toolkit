package dto

import (
	"Intelligent_Dev_ToolKit_Odoo/internal/config"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Response is the standard JSON envelope for all API responses.
type Response struct {
	Success   bool   `json:"success"`
	Data      any    `json:"data,omitempty"`
	Meta      *Meta  `json:"meta,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// Meta contains pagination and timing metadata.
type Meta struct {
	Page       int    `json:"page,omitempty"`
	PerPage    int    `json:"per_page,omitempty"`
	Total      int64  `json:"total,omitempty"`
	TotalPages int    `json:"total_pages,omitempty"`
	HasNext    bool   `json:"has_next,omitempty"`
	HasPrev    bool   `json:"has_prev,omitempty"`
	Took       string `json:"took,omitempty"`
}

// ListResponse represents a paginated list response.
type ListResponse struct {
	Data       any   `json:"data"`
	Pagination *Meta `json:"pagination,omitempty"`
}

// =============================================================================
// Pagination
// =============================================================================

// Pagination holds page, per-page, and total counts for paginated queries.
type Pagination struct {
	Page    int
	PerPage int
	Total   int64
}

// NewPagination returns a Pagination clamped to sane page/perPage bounds.
func NewPagination(page, perPage int) *Pagination {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}
	return &Pagination{
		Page:    page,
		PerPage: perPage,
	}
}

// Offset returns the SQL OFFSET derived from the current page and per-page size.
func (p *Pagination) Offset() int {
	return (p.Page - 1) * p.PerPage
}

// TotalPages returns the number of pages needed to cover all records.
func (p *Pagination) TotalPages() int {
	if p.Total == 0 {
		return 0
	}
	pages := int(p.Total) / p.PerPage
	if int(p.Total)%p.PerPage > 0 {
		pages++
	}
	return pages
}

// ToMeta converts the Pagination state into a Meta response struct.
func (p *Pagination) ToMeta() *Meta {
	totalPages := p.TotalPages()
	return &Meta{
		Page:       p.Page,
		PerPage:    p.PerPage,
		Total:      p.Total,
		TotalPages: totalPages,
		HasNext:    p.Page < totalPages,
		HasPrev:    p.Page > 1,
	}
}

// =============================================================================
// JSON Response Helpers
// =============================================================================
func setCommonHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

// WriteJSON encodes data as JSON and writes it with the given HTTP status code.
func WriteJSON(w http.ResponseWriter, status int, data any) {
	setCommonHeaders(w)
	w.WriteHeader(status)

	if data != nil {
		_ = json.NewEncoder(w).Encode(data) //nolint:errcheck // best-effort write; caller cannot recover
	}
}

// WriteList writes a paginated list response with metadata headers.
func WriteList(w http.ResponseWriter, r *http.Request, data any, pagination *Pagination) {
	response := ListResponse{
		Data:       data,
		Pagination: pagination.ToMeta(),
	}

	setCommonHeaders(w)
	w.Header().Set("X-Request-ID", getOrCreateRequestID(r))
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response) //nolint:errcheck // best-effort write; caller cannot recover
}

// WriteSuccess writes a 200 OK response wrapping data in the standard envelope.
func WriteSuccess(w http.ResponseWriter, r *http.Request, data any) {
	response := Response{
		Success:   true,
		Data:      data,
		RequestID: getOrCreateRequestID(r),
	}
	WriteJSON(w, http.StatusOK, response)
}

// WriteSuccessWithMeta writes a 200 OK response with pagination metadata.
func WriteSuccessWithMeta(w http.ResponseWriter, r *http.Request, data any, pagination *Pagination) {
	response := Response{
		Success:   true,
		Data:      data,
		Meta:      pagination.ToMeta(),
		RequestID: getOrCreateRequestID(r),
	}
	WriteJSON(w, http.StatusOK, response)
}

// WriteCreated writes a 201 Created response wrapping data in the standard envelope.
func WriteCreated(w http.ResponseWriter, r *http.Request, data any) {
	response := Response{
		Success:   true,
		Data:      data,
		RequestID: getOrCreateRequestID(r),
	}
	WriteJSON(w, http.StatusCreated, response)
}

// WriteAccepted writes a 202 Accepted response wrapping data in the standard envelope.
func WriteAccepted(w http.ResponseWriter, r *http.Request, data any) {
	response := Response{
		Success:   true,
		Data:      data,
		RequestID: getOrCreateRequestID(r),
	}
	WriteJSON(w, http.StatusAccepted, response)
}

// WriteNoContent writes a 204 No Content response with no body.
func WriteNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// =============================================================================
// Specialized Responses
// =============================================================================

// HealthResponse is returned by the health-check endpoint.
type HealthResponse struct {
	Status    string            `json:"status"`
	Timestamp time.Time         `json:"timestamp"`
	Checks    map[string]string `json:"checks,omitempty"`
	Version   string            `json:"version,omitempty"`
}

// WriteHealth writes a health-check response, returning 503 when unhealthy.
func WriteHealth(w http.ResponseWriter, healthy bool, checks map[string]string, version string) {
	status := config.StatusHealthy
	httpStatus := http.StatusOK

	if !healthy {
		status = config.StatusUnhealthy
		httpStatus = http.StatusServiceUnavailable
	}

	response := HealthResponse{
		Status:    status,
		Timestamp: time.Now().UTC(),
		Checks:    checks,
		Version:   version,
	}

	WriteJSON(w, httpStatus, response)
}

// ReadyResponse is returned by the readiness probe endpoint.
type ReadyResponse struct {
	Ready  bool              `json:"ready"`
	Checks map[string]string `json:"checks,omitempty"`
}

// WriteReady writes a readiness response, returning 503 when not ready.
func WriteReady(w http.ResponseWriter, ready bool, checks map[string]string) {
	httpStatus := http.StatusOK
	if !ready {
		httpStatus = http.StatusServiceUnavailable
	}

	response := ReadyResponse{
		Ready:  ready,
		Checks: checks,
	}

	WriteJSON(w, httpStatus, response)
}

// =============================================================================
// Request Helpers
// =============================================================================

func getOrCreateRequestID(r *http.Request) string {
	if id := r.Header.Get("X-Request-ID"); id != "" {
		return id
	}
	return uuid.New().String()
}

// GetPaginationFromRequest extracts page and per_page from the query string.
func GetPaginationFromRequest(r *http.Request) *Pagination {
	page := 1
	perPage := 25

	q := r.URL.RawQuery
	if q == "" {
		return NewPagination(page, perPage)
	}

	pairs := strings.Split(q, "&")
	for _, pair := range pairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := parts[0]
		value := parts[1]
		switch key {
		case "page":
			if parsed := parseInt(value); parsed > 0 {
				page = parsed
			}
		case "per_page":
			if parsed := parseInt(value); parsed > 0 {
				perPage = parsed
			}
		}
	}

	return NewPagination(page, perPage)
}

func parseInt(s string) int {
	var result int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		result = result*10 + int(c-'0')
	}
	return result
}

// =============================================================================
// Rate Limit Headers
// =============================================================================

// WriteRateLimitHeaders sets the standard X-RateLimit-* response headers.
func WriteRateLimitHeaders(w http.ResponseWriter, limit, remaining int, resetUnix int64) {
	w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
	w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
	w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetUnix))
}
