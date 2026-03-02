package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// Response Types
// =============================================================================

// Response represents a standard API response
type Response struct {
	Success   bool   `json:"success"`
	Data      any    `json:"data,omitempty"`
	Meta      *Meta  `json:"meta,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// Meta contains pagination and timing metadata
type Meta struct {
	Page       int    `json:"page,omitempty"`
	PerPage    int    `json:"per_page,omitempty"`
	Total      int64  `json:"total,omitempty"`
	TotalPages int    `json:"total_pages,omitempty"`
	HasNext    bool   `json:"has_next,omitempty"`
	HasPrev    bool   `json:"has_prev,omitempty"`
	Took       string `json:"took,omitempty"`
}

// ListResponse represents a paginated list response
type ListResponse struct {
	Data       any   `json:"data"`
	Pagination *Meta `json:"pagination,omitempty"`
}

// =============================================================================
// Pagination
// =============================================================================

type Pagination struct {
	Page    int
	PerPage int
	Total   int64
}

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

func (p *Pagination) Offset() int {
	return (p.Page - 1) * p.PerPage
}

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

func WriteJSON(w http.ResponseWriter, status int, data any) {
	setCommonHeaders(w)
	w.WriteHeader(status)

	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

func WriteSuccess(w http.ResponseWriter, r *http.Request, data any) {
	response := Response{
		Success:   true,
		Data:      data,
		RequestID: getOrCreateRequestID(r),
	}
	WriteJSON(w, http.StatusOK, response)
}

func WriteSuccessWithMeta(w http.ResponseWriter, r *http.Request, data any, pagination *Pagination) {
	response := Response{
		Success:   true,
		Data:      data,
		Meta:      pagination.ToMeta(),
		RequestID: getOrCreateRequestID(r),
	}
	WriteJSON(w, http.StatusOK, response)
}

func WriteCreated(w http.ResponseWriter, r *http.Request, data any) {
	response := Response{
		Success:   true,
		Data:      data,
		RequestID: getOrCreateRequestID(r),
	}
	WriteJSON(w, http.StatusCreated, response)
}

func WriteAccepted(w http.ResponseWriter, r *http.Request, data any) {
	response := Response{
		Success:   true,
		Data:      data,
		RequestID: getOrCreateRequestID(r),
	}
	WriteJSON(w, http.StatusAccepted, response)
}

func WriteNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

func WriteList(w http.ResponseWriter, r *http.Request, data any, pagination *Pagination) {
	response := ListResponse{
		Data:       data,
		Pagination: pagination.ToMeta(),
	}

	setCommonHeaders(w)
	w.Header().Set("X-Request-ID", getOrCreateRequestID(r))
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// =============================================================================
// Specialized Responses
// =============================================================================

type HealthResponse struct {
	Status    string            `json:"status"`
	Timestamp time.Time         `json:"timestamp"`
	Checks    map[string]string `json:"checks,omitempty"`
	Version   string            `json:"version,omitempty"`
}

func WriteHealth(w http.ResponseWriter, healthy bool, checks map[string]string, version string) {
	status := "healthy"
	httpStatus := http.StatusOK

	if !healthy {
		status = "unhealthy"
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

type ReadyResponse struct {
	Ready  bool              `json:"ready"`
	Checks map[string]string `json:"checks,omitempty"`
}

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

func GetPaginationFromRequest(r *http.Request) *Pagination {
	page := 1
	perPage := 25

	if p := r.URL.Query().Get("page"); p != "" {
		if parsed := parseInt(p); parsed > 0 {
			page = parsed
		}
	}

	if pp := r.URL.Query().Get("per_page"); pp != "" {
		if parsed := parseInt(pp); parsed > 0 {
			perPage = parsed
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

func WriteRateLimitHeaders(w http.ResponseWriter, limit, remaining int, resetUnix int64) {
	w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
	w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
	w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetUnix))
}
