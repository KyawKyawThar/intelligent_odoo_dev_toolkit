// Package collector contains the code for collecting data from Odoo.
package collector

import (
	"context"
	"fmt"

	"Intelligent_Dev_ToolKit_Odoo/internal/agent/odoo"

	"github.com/rs/zerolog/log"
)

// CollectACLAndRules fetches ir.model.access and ir.rule records from Odoo.
//
// ir.model.access — model-level permissions (read/write/create/unlink per group).
// ir.rule         — record-level rules (domain-filtered access restrictions).
//
// Both are required by the ACL Debugger to answer "why can't user X see record Y?".
func CollectACLAndRules(ctx context.Context, client *odoo.Client) ([]odoo.IrModelAccess, []odoo.IrRule, error) {
	accessList, err := collectModelAccess(ctx, client)
	if err != nil {
		return nil, nil, err
	}

	ruleList, err := collectRecordRules(ctx, client)
	if err != nil {
		return nil, nil, err
	}

	return accessList, ruleList, nil
}

// ─── ir.model.access ─────────────────────────────────────────────────────────

func collectModelAccess(ctx context.Context, client *odoo.Client) ([]odoo.IrModelAccess, error) {
	raw, err := fetchRecords(ctx, client, "ir.model.access", []string{
		"id", "name", "model_id", "group_id",
		"perm_read", "perm_write", "perm_create", "perm_unlink",
	})
	if err != nil {
		return nil, fmt.Errorf("fetch ir.model.access: %w", err)
	}

	list := make([]odoo.IrModelAccess, 0, len(raw))
	for _, r := range raw {
		list = append(list, parseIrModelAccess(r))
	}

	log.Info().Int("count", len(list)).Msg("collected ir.model.access records")
	if len(list) > 0 {
		a := list[0]
		log.Debug().
			Int("id", a.ID).
			Str("name", a.Name).
			Int("model_id", a.ModelID).
			Int("group_id", a.GroupID).
			Bool("perm_read", a.PermRead).
			Bool("perm_write", a.PermWrite).
			Msg("example ir.model.access record")
	}

	return list, nil
}

// parseIrModelAccess converts a raw fetchRecords map to a typed IrModelAccess.
//
// Odoo many2one fields (model_id, group_id) come back as [id, "display_name"]
// or false (→ nil after convertValue). We extract just the integer ID.
// GroupID == 0 means the ACL rule applies to all users (no group restriction).
func parseIrModelAccess(r map[string]interface{}) odoo.IrModelAccess {
	return odoo.IrModelAccess{
		ID:         intVal(r["id"]),
		Name:       stringVal(r["name"]),
		ModelID:    many2oneID(r["model_id"]),
		GroupID:    many2oneID(r["group_id"]),
		PermRead:   boolVal(r["perm_read"]),
		PermWrite:  boolVal(r["perm_write"]),
		PermCreate: boolVal(r["perm_create"]),
		PermUnlink: boolVal(r["perm_unlink"]),
	}
}

// ─── ir.rule ─────────────────────────────────────────────────────────────────

func collectRecordRules(ctx context.Context, client *odoo.Client) ([]odoo.IrRule, error) {
	// Include active=false so the ACL debugger can explain why a rule
	// exists but is currently disabled.
	domain := []any{
		[]any{"active", "in", []any{true, false}},
	}

	raw, err := fetchRecordsWithDomain(ctx, client, "ir.rule", []string{
		"id", "name", "model_id", "groups",
		"domain_force", "perm_read", "perm_write", "perm_create", "perm_unlink",
		"global", "active",
	}, domain)
	if err != nil {
		return nil, fmt.Errorf("fetch ir.rule: %w", err)
	}

	list := make([]odoo.IrRule, 0, len(raw))
	for _, r := range raw {
		list = append(list, parseIrRule(r))
	}

	log.Info().Int("count", len(list)).Msg("collected ir.rule records")
	if len(list) > 0 {
		r := list[0]
		log.Debug().
			Int("id", r.ID).
			Str("name", r.Name).
			Int("model_id", r.ModelID).
			Bool("global", r.Global).
			Bool("active", r.Active).
			Str("domain", r.Domain).
			Msg("example ir.rule record")
	}

	return list, nil
}

// parseIrRule converts a raw map to a typed IrRule.
//
// model_id is many2one  → extract single int ID.
// groups   is many2many → list of int group IDs; empty slice = global rule.
func parseIrRule(r map[string]interface{}) odoo.IrRule {
	return odoo.IrRule{
		ID:         intVal(r["id"]),
		Name:       stringVal(r["name"]),
		ModelID:    many2oneID(r["model_id"]),
		Groups:     intSliceVal(r["groups"]),
		Domain:     stringVal(r["domain_force"]),
		PermRead:   boolVal(r["perm_read"]),
		PermWrite:  boolVal(r["perm_write"]),
		PermCreate: boolVal(r["perm_create"]),
		PermUnlink: boolVal(r["perm_unlink"]),
		Global:     boolVal(r["global"]),
		Active:     boolVal(r["active"]),
	}
}

// ─── Typed value helpers ─────────────────────────────────────────────────────

// intVal extracts an int from a convertValue result.
func intVal(v interface{}) int {
	if v == nil {
		return 0
	}
	switch t := v.(type) {
	case int:
		return t
	case float64:
		return int(t)
	}
	return 0
}

// stringVal returns the string value or "" for nil/non-string.
func stringVal(v interface{}) string {
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

// boolVal returns the bool value or false for nil.
func boolVal(v interface{}) bool {
	if v == nil {
		return false
	}
	b, ok := v.(bool)
	if !ok {
		return false
	}
	return b
}

// many2oneID extracts the integer ID from an Odoo many2one field.
//
// Odoo XML-RPC returns many2one as [id, "display_name"] ([]interface{}) when set,
// or false → nil after convertValue when the field is empty.
func many2oneID(v interface{}) int {
	if v == nil {
		return 0
	}
	arr, ok := v.([]interface{})
	if !ok || len(arr) == 0 {
		return 0
	}
	return intVal(arr[0])
}

// intSliceVal converts an Odoo many2many result ([]interface{} of ints) to []int.
func intSliceVal(v interface{}) []int {
	if v == nil {
		return nil
	}
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	result := make([]int, 0, len(arr))
	for _, item := range arr {
		if id := intVal(item); id != 0 {
			result = append(result, id)
		}
	}
	return result
}
