// Package errors provides error collection and processing for the agent.
package errors

import (
	"context"
	"encoding/xml"
	"fmt"
	"maps"
	"strconv"
	"time"

	"Intelligent_Dev_ToolKit_Odoo/internal/agent/odoo"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/ringbuf"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/sampler"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Collector polls Odoo's ir.logging model for new error entries.
type Collector struct {
	client    *odoo.Client
	buf       *ringbuf.RingBuffer[ErrorEvent]
	smp       *sampler.Sampler
	logger    zerolog.Logger
	lastLogID int
}

// NewCollector creates a new error collector.
func NewCollector(client *odoo.Client, buf *ringbuf.RingBuffer[ErrorEvent], smp *sampler.Sampler, logger zerolog.Logger) *Collector {
	return &Collector{
		client: client,
		buf:    buf,
		smp:    smp,
		logger: logger,
	}
}

// RunLoop starts the polling loop for the collector.
func (c *Collector) RunLoop(ctx context.Context, interval time.Duration) {
	runPollLoop(ctx, interval, c.logger, "error", c.poll)
}

func (c *Collector) poll(ctx context.Context) error {
	domain := []any{
		[]any{"type", "=", "server"},
		[]any{"level", "in", []any{"ERROR", "CRITICAL"}},
	}
	if c.lastLogID > 0 {
		domain = append(domain, []any{"id", ">", c.lastLogID})
	}

	fields := []string{"id", "name", "message", "level", "create_date", "create_uid"}

	records, err := FetchRecordsWithDomain(ctx, c.client, "ir.logging", fields, domain, map[string]any{"order": "id asc", "limit": 200})
	if err != nil {
		return fmt.Errorf("poll ir.logging: %w", err)
	}

	for _, r := range records {
		if event, ok := recordToErrorEvent(r); ok {
			c.buf.Push(event)
		}
		if id, ok := r["id"].(int); ok && id > c.lastLogID {
			c.lastLogID = id
		}
	}

	if len(records) > 0 {
		c.logger.Debug().Int("count", len(records)).Int("last_id", c.lastLogID).Msg("error poll complete")
	}

	return nil
}

func recordToErrorEvent(r map[string]any) (ErrorEvent, bool) {
	msg, ok := r["message"].(string)
	if !ok || msg == "" {
		return ErrorEvent{}, false
	}
	logID, ok := r["id"].(int)
	if !ok {
		logID = 0
	}

	errorType := ParseErrorType(msg)
	traceback := ExtractTraceback(msg)

	capturedAtStr, ok := r["create_date"].(string)
	if !ok {
		capturedAtStr = ""
	}
	capturedAt, err := time.Parse("2006-01-02 15:04:05", capturedAtStr)
	if err != nil {
		capturedAt = time.Time{}
	}

	userID, ok := r["create_uid"].(int)
	if !ok {
		userID = 0
	}

	return ErrorEvent{
		LogID:      logID,
		Message:    msg,
		ErrorType:  errorType,
		Traceback:  traceback,
		CapturedAt: capturedAt,
		UserID:     userID,
		Signature:  GenerateSignature(errorType, msg, traceback),
	}, true
}

// runPollLoop is a shared polling loop used by collectors. It calls pollFn
// immediately, then on every tick of interval until ctx is canceled.
func runPollLoop(ctx context.Context, interval time.Duration, logger zerolog.Logger, name string, pollFn func(context.Context) error) {
	logger.Info().Dur("interval", interval).Msgf("starting %s collector", name)

	if err := pollFn(ctx); err != nil {
		logger.Error().Err(err).Msgf("initial %s poll failed", name)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info().Msgf("%s collector stopped", name)
			return
		case <-ticker.C:
			if err := pollFn(ctx); err != nil {
				logger.Error().Err(err).Msgf("%s poll failed", name)
			}
		}
	}
}

// ─── XML-RPC Response Parsing Structs ───────────────────────────────────────

// MethodResponse is the top-level structure for an XML-RPC response.
type MethodResponse struct {
	XMLName xml.Name `xml:"methodResponse"`
	Params  []Param  `xml:"params>param"`
	Fault   *Fault   `xml:"fault"`
}

// Param represents a single <param> in the response.
type Param struct {
	Value Value `xml:"value"`
}

// Value is a generic container for any XML-RPC value type.
type Value struct {
	Array   []Value   `xml:"array>data>value"`
	Struct  []Member  `xml:"struct>member"`
	String  string    `xml:"string"`
	Int     string    `xml:"int"`
	I4      string    `xml:"i4"`
	Boolean string    `xml:"boolean"`
	Double  string    `xml:"double"`
	Nil     *struct{} `xml:"nil"`
}

// Member represents a key-value pair in a struct.
type Member struct {
	Name  string `xml:"name"`
	Value Value  `xml:"value"`
}

// Fault represents an XML-RPC fault response.
type Fault struct {
	Value Value `xml:"value"`
}

// FetchRecordsWithDomain performs a search_read on modelName with the given
// domain, fields, kwargs options (e.g. "order", "limit"). It is exported so
// other agent packages (e.g. errors) can reuse the XML-RPC parsing logic.
func FetchRecordsWithDomain(
	ctx context.Context,
	client *odoo.Client,
	modelName string,
	fields []string,
	domain []any,
	extra map[string]any,
) ([]map[string]any, error) {
	kwargs := map[string]any{"fields": fields}
	maps.Copy(kwargs, extra)

	body, err := client.ExecuteKw(ctx, modelName, "search_read", []any{domain}, kwargs)
	if err != nil {
		return nil, err
	}

	var resp MethodResponse
	if err := unmarshalResponse(body, modelName, &resp); err != nil {
		return nil, err
	}
	return extractRecords(resp, modelName)
}

// unmarshalResponse decodes raw XML-RPC bytes into a MethodResponse.
func unmarshalResponse(body []byte, modelName string, resp *MethodResponse) error {
	if err := xml.Unmarshal(body, resp); err != nil {
		log.Debug().Str("model", modelName).Str("body", string(body)).Msg("raw XML-RPC response")
		return fmt.Errorf("unmarshal %s: %w", modelName, err)
	}
	if resp.Fault != nil {
		return fmt.Errorf("odoo fault on %s: %s", modelName, getFaultString(resp.Fault))
	}
	return nil
}

// extractRecords converts the parsed MethodResponse into a slice of record maps.
func extractRecords(resp MethodResponse, _ string) ([]map[string]any, error) {
	if len(resp.Params) == 0 || len(resp.Params[0].Value.Array) == 0 {
		return []map[string]any{}, nil
	}

	records := make([]map[string]any, 0, len(resp.Params[0].Value.Array))
	for _, recordValue := range resp.Params[0].Value.Array {
		record := make(map[string]any)
		for _, member := range recordValue.Struct {
			record[member.Name] = convertValue(member.Value)
		}
		records = append(records, record)
	}
	return records, nil
}

// convertValue converts an XML-RPC Value to a native Go type.
func convertValue(v Value) any {
	switch {
	case v.Int != "" || v.I4 != "":
		intStr := v.Int
		if v.I4 != "" {
			intStr = v.I4
		}
		i, err := strconv.Atoi(intStr)
		if err != nil {
			return 0
		}
		return i
	case v.String != "":
		return v.String
	case v.Boolean != "":
		return v.Boolean == "1" || v.Boolean == "true"
	case v.Double != "":
		f, err := strconv.ParseFloat(v.Double, 64)
		if err != nil {
			return 0.0
		}
		return f
	case v.Nil != nil:
		return nil
	case len(v.Array) > 0:
		var arr []any
		for _, item := range v.Array {
			arr = append(arr, convertValue(item))
		}
		return arr
	case len(v.Struct) > 0:
		record := make(map[string]any)
		for _, member := range v.Struct {
			record[member.Name] = convertValue(member.Value)
		}
		return record
	default:
		return nil
	}
}

// getFaultString extracts the error message from a fault response.
func getFaultString(fault *Fault) string {
	for _, member := range fault.Value.Struct {
		if member.Name == "faultString" {
			return member.Value.String
		}
	}
	return "unknown fault"
}
