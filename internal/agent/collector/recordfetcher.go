package collector

import (
	"context"
	"fmt"

	"Intelligent_Dev_ToolKit_Odoo/internal/acl"
	"Intelligent_Dev_ToolKit_Odoo/internal/acl/domain"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/odoo"

	"github.com/rs/zerolog/log"
)

// RecordFetcher fetches actual record field values from Odoo via XML-RPC.
// It is used by the ACL pipeline's domain evaluator (Stage 5) to get the
// concrete field values needed to evaluate ir.rule domain conditions.
type RecordFetcher struct {
	client *odoo.Client
}

// NewRecordFetcher creates a RecordFetcher backed by the given Odoo client.
func NewRecordFetcher(client *odoo.Client) *RecordFetcher {
	return &RecordFetcher{client: client}
}

// FetchRecord retrieves a single record's field values from Odoo.
//
// Parameters:
//   - modelName: the Odoo model technical name (e.g. "sale.order")
//   - recordID: the database ID of the target record
//   - fields: the field names to fetch (e.g. ["active", "user_id", "company_id"])
//
// Returns an acl.RecordData (map[string]any) with the field values, or an
// error if the record is not found or the RPC call fails.
func (f *RecordFetcher) FetchRecord(
	ctx context.Context,
	modelName string,
	recordID int,
	fields []string,
) (acl.RecordData, error) {
	if len(fields) == 0 {
		return acl.RecordData{}, nil
	}

	dom := []any{
		[]any{"id", "=", recordID},
	}

	raw, err := FetchRecordsWithDomain(ctx, f.client, modelName, fields, dom, map[string]any{
		"limit": 1,
	})
	if err != nil {
		return nil, fmt.Errorf("fetch %s(id=%d): %w", modelName, recordID, err)
	}

	if len(raw) == 0 {
		return nil, fmt.Errorf("record not found: %s(id=%d)", modelName, recordID)
	}

	record := acl.RecordData(raw[0])

	log.Debug().
		Str("model", modelName).
		Int("record_id", recordID).
		Int("fields_fetched", len(fields)).
		Msg("fetched record for ACL domain evaluation")

	return record, nil
}

// FetchRecordForRules is a convenience method that extracts the required
// fields from the applicable record rules' domains, fetches the record,
// and returns the data ready for domain evaluation.
//
// Parameters:
//   - modelName: the Odoo model technical name
//   - recordID: the database ID of the target record
//   - ruleDetail: the RecordRuleDetail from the record rule finder (Stage 4)
func (f *RecordFetcher) FetchRecordForRules(
	ctx context.Context,
	modelName string,
	recordID int,
	ruleDetail *acl.RecordRuleDetail,
) (acl.RecordData, error) {
	// Collect all domain strings from applicable rules.
	var domainStrs []string
	for _, r := range ruleDetail.GlobalRules {
		if r.Applies {
			domainStrs = append(domainStrs, r.Domain)
		}
	}
	for _, r := range ruleDetail.GroupRules {
		if r.Applies {
			domainStrs = append(domainStrs, r.Domain)
		}
	}

	fields := ExtractFieldsFromDomains(domainStrs)
	if len(fields) == 0 {
		// No fields needed (all domains are empty or unparseable).
		return acl.RecordData{}, nil
	}

	return f.FetchRecord(ctx, modelName, recordID, fields)
}

// ExtractFieldsFromDomains parses multiple Odoo domain strings and returns
// a deduplicated list of field names referenced in their conditions.
func ExtractFieldsFromDomains(domainStrs []string) []string {
	seen := make(map[string]bool)
	for _, ds := range domainStrs {
		ast, err := domain.Parse(ds)
		if err != nil || ast == nil {
			continue
		}
		extractFieldsFromNode(ast, seen)
	}

	fields := make([]string, 0, len(seen))
	for f := range seen {
		fields = append(fields, f)
	}
	return fields
}

// extractFieldsFromNode recursively walks the AST and collects field names.
func extractFieldsFromNode(node domain.Node, seen map[string]bool) {
	switch n := node.(type) {
	case *domain.Condition:
		seen[n.Field] = true
	case *domain.BoolOp:
		for _, child := range n.Children {
			extractFieldsFromNode(child, seen)
		}
	}
}
