package collector

import (
	"context"
	"encoding/xml"
	"fmt"
	"maps"
	"strconv"

	"Intelligent_Dev_ToolKit_Odoo/internal/agent/odoo"

	"github.com/rs/zerolog/log"
)

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

// ─── Public Functions ────────────────────────────────────────────────────────

// CollectModels fetches ir.model and ir.model.fields and returns them as a
// combined slice of raw record maps ready for JSON serialization.
// Each model entry includes a "fields" key containing its field definitions.
func CollectModels(ctx context.Context, client *odoo.Client) ([]map[string]any, error) {
	models, err := fetchRecords(ctx, client, "ir.model", []string{"id", "model", "name"})
	if err != nil {
		return nil, fmt.Errorf("fetch ir.model: %w", err)
	}

	fields, err := fetchRecords(ctx, client, "ir.model.fields", []string{
		"id", "name", "model", "ttype", "relation",
		"required", "readonly", "store", "index",
	})
	if err != nil {
		return nil, fmt.Errorf("fetch ir.model.fields: %w", err)
	}

	// Index fields by model name for O(1) lookup.
	byModel := make(map[string][]map[string]any, len(models))
	for _, f := range fields {
		modelName, ok := f["model"].(string)
		if !ok {
			continue
		}
		byModel[modelName] = append(byModel[modelName], f)
	}

	for i, m := range models {
		modelName, ok := m["model"].(string)
		if !ok {
			continue
		}
		models[i]["fields"] = byModel[modelName]
	}

	log.Info().
		Int("models", len(models)).
		Int("fields", len(fields)).
		Msg("schema collection complete")

	return models, nil
}

// CollectModelsAndFields fetches all ir.model and ir.model.fields records.
func CollectModelsAndFields(ctx context.Context, client *odoo.Client) error {
	log.Info().Msg("starting schema collection for ir.model and ir.model.fields")

	// 1. Fetch all ir.model records
	models, err := fetchRecords(ctx, client, "ir.model", []string{"id", "model", "name"})
	if err != nil {
		return fmt.Errorf("failed to fetch ir.model: %w", err)
	}
	log.Info().Int("count", len(models)).Msg("collected ir.model records")
	if len(models) > 0 {
		log.Debug().Interface("first_model", models[0]).Msg("example ir.model record")
	}

	// 2. Fetch all ir.model.fields records
	fields, err := fetchRecords(ctx, client, "ir.model.fields", []string{"id", "name", "model", "ttype"})
	if err != nil {
		return fmt.Errorf("failed to fetch ir.model.fields: %w", err)
	}
	log.Info().Int("count", len(fields)).Msg("collected ir.model.fields records")
	if len(fields) > 0 {
		log.Debug().Interface("first_field", fields[0]).Msg("example ir.model.fields record")
	}

	return nil
}

// ─── Exported Helpers ────────────────────────────────────────────────────────

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

// ─── Internal Helpers ────────────────────────────────────────────────────────

// fetchRecords performs a search_read on the given model with an empty domain.
func fetchRecords(ctx context.Context, client *odoo.Client, modelName string, fields []string) ([]map[string]any, error) {
	return fetchRecordsWithDomain(ctx, client, modelName, fields, []any{})
}

// fetchRecordsWithDomain performs a search_read with a caller-supplied domain.
// domain follows the Odoo domain expression format, e.g.:
//
//	[]any{[]any{"active", "in", []any{true, false}}}
func fetchRecordsWithDomain(
	ctx context.Context,
	client *odoo.Client,
	modelName string,
	fields []string,
	domain []any,
) ([]map[string]any, error) {
	kwargs := map[string]any{
		"fields": fields,
	}

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
