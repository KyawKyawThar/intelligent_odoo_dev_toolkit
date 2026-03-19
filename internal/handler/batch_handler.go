package handler

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"

	"Intelligent_Dev_ToolKit_Odoo/internal/api"
	"Intelligent_Dev_ToolKit_Odoo/internal/dto"
	"Intelligent_Dev_ToolKit_Odoo/internal/middleware"

	"github.com/redis/go-redis/v9"
)

// BatchHandler receives aggregated batches from agents and pushes them
// onto a Redis stream for asynchronous processing by the ingest worker.
type BatchHandler struct {
	*BaseHandler
	redis      *redis.Client
	streamName string // Redis stream key, e.g. "agent:ingest"
}

// NewBatchHandler creates a BatchHandler.
// streamName is the Redis stream key (e.g. "agent:ingest").
func NewBatchHandler(base *BaseHandler, redisClient *redis.Client, streamName string) *BatchHandler {
	if streamName == "" {
		streamName = "agent:ingest"
	}
	return &BatchHandler{
		BaseHandler: base,
		redis:       redisClient,
		streamName:  streamName,
	}
}

// HandleIngestBatch receives a (possibly gzipped) AggregatedBatch from an
// agent, validates the tenant, and pushes the raw JSON onto the Redis
// "agent:ingest" stream. The ingest worker processes it asynchronously.
//
//	@Summary      Ingest aggregated batch
//	@Description  Agent endpoint: receive an aggregated batch of ORM stats and critical events.
//	@Description  The batch is pushed onto a Redis stream for asynchronous processing by the ingest worker.
//	@Description  Supports gzip-compressed bodies (Content-Encoding: gzip). Max body size: 5 MB.
//	@Tags         agent
//	@Accept       json
//	@Produce      json
//	@Param        body  body      dto.IngestBatchRequest  true  "Aggregated batch payload"
//	@Success      202   {object}  dto.IngestBatchResponse
//	@Failure      400   {object}  api.Error
//	@Failure      401   {object}  api.Error
//	@Failure      500   {object}  api.Error
//	@Router       /agent/batch [post]
//	@Security     ApiKeyAuth
func (h *BatchHandler) HandleIngestBatch(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == "" {
		h.WriteErr(w, r, api.ErrUnauthorized("Tenant ID missing"))
		return
	}

	// Decompress if gzip-encoded (agent sends Content-Encoding: gzip).
	body := r.Body
	if r.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			h.WriteErr(w, r, api.ErrBadRequest("invalid gzip body"))
			return
		}
		defer gz.Close() //nolint:errcheck // gzip reader close; error is harmless
		body = gz
	}

	data, err := io.ReadAll(io.LimitReader(body, 5<<20)) // 5 MB max
	if err != nil {
		h.WriteErr(w, r, api.ErrBadRequest("failed to read body"))
		return
	}

	// Quick structural validation — must be valid JSON with env_id.
	var peek struct {
		EnvID string `json:"env_id"`
	}
	if uerr := json.Unmarshal(data, &peek); uerr != nil || peek.EnvID == "" {
		h.WriteErr(w, r, api.ErrBadRequest("body must be JSON with env_id"))
		return
	}

	// Push to Redis stream for async processing.
	err = h.redis.XAdd(r.Context(), &redis.XAddArgs{
		Stream: h.streamName,
		Values: map[string]any{
			"tenant_id": tenantID,
			"data":      string(data),
		},
	}).Err()
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to push batch to Redis stream")
		h.WriteErr(w, r, api.ErrInternal(err))
		return
	}

	dto.WriteJSON(w, http.StatusAccepted, map[string]string{
		"status": "queued",
	})
}
