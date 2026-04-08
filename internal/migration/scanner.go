package migration

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Issue is one finding from a migration scan.
type Issue struct {
	// Severity is "breaking", "warning", or "minor".
	Severity string `json:"severity"`
	// Kind is one of the Kind* constants.
	Kind string `json:"kind"`
	// Model is the affected Odoo model (empty for method/API deprecations).
	Model string `json:"model,omitempty"`
	// Field is the affected field (empty for model-level findings).
	Field string `json:"field,omitempty"`
	// OldName / NewName for renames.
	OldName string `json:"old_name,omitempty"`
	NewName string `json:"new_name,omitempty"`
	// Message is a human-readable description.
	Message string `json:"message"`
	// Fix is the recommended remediation.
	Fix string `json:"fix,omitempty"`
	// FromVersion / ToVersion identifies the transition.
	FromVersion string `json:"from_version"`
	ToVersion   string `json:"to_version"`
}

// ScanResult holds the output of one scan run.
type ScanResult struct {
	Issues        []Issue `json:"issues"`
	BreakingCount int     `json:"breaking_count"`
	WarningCount  int     `json:"warning_count"`
	MinorCount    int     `json:"minor_count"`
}

// modelFieldSet is an in-memory index built from a schema snapshot.
// models["sale.order"]["name"] = true  →  field "name" exists on "sale.order"
type modelFieldSet map[string]map[string]bool // model → set of field names

// Scan checks modelsJSON (the JSONB from schema_snapshots.models) against the
// deprecation rules for the given version transition.
//
// modelsJSON shape (produced by the syncer):
//
//	{
//	  "sale.order": { "model": "sale.order", "name": "Sales Order",
//	                  "fields": { "name": {...}, "partner_id": {...} } },
//	  ...
//	}
//
// The function is deliberately lenient: if the JSON cannot be parsed or a model
// is absent it simply skips that rule rather than returning an error, so a
// partial schema still produces useful results.
func Scan(modelsJSON json.RawMessage, fromVersion, toVersion string) (*ScanResult, error) {
	rules := PathBetween(fromVersion, toVersion)
	if rules == nil {
		return nil, fmt.Errorf("unsupported version path: %s → %s", fromVersion, toVersion)
	}

	idx, err := buildIndex(modelsJSON)
	if err != nil {
		// Non-fatal: return an empty result with the error surfaced.
		return &ScanResult{Issues: []Issue{}}, fmt.Errorf("parse schema: %w", err)
	}

	result := &ScanResult{Issues: []Issue{}}

	for _, rule := range rules {
		issue, found := evaluate(rule, idx)
		if !found {
			continue
		}
		result.Issues = append(result.Issues, issue)
		switch issue.Severity {
		case SeverityBreaking:
			result.BreakingCount++
		case SeverityWarning:
			result.WarningCount++
		case SeverityMinor:
			result.MinorCount++
		}
	}

	return result, nil
}

func evalModelExists(rule Rule, idx modelFieldSet) (Issue, bool) {
	target := coalesce(rule.OldName, rule.Model)

	if !modelExists(idx, target) {
		return Issue{}, false
	}
	return makeIssue(rule), true
}

func evalFieldExists(rule Rule, idx modelFieldSet, field string) (Issue, bool) {
	fields, ok := idx[rule.Model]
	if !ok {
		return Issue{}, false
	}

	if !fields[field] {
		return Issue{}, false
	}

	return makeIssue(rule), true
}

func modelExists(idx modelFieldSet, model string) bool {
	_, exists := idx[model]
	return exists
}

func coalesce(primary, fallback string) string {
	if primary != "" {
		return primary
	}
	return fallback
}

// evaluate checks whether a single rule fires against the schema index.
// Returns (issue, true) if the rule fires, or (zero, false) if the schema
// does not contain the deprecated artifact (i.e. the customer is already clean).

func evaluate(rule Rule, idx modelFieldSet) (Issue, bool) {
	switch rule.Kind {

	case KindModelRemoved, KindModelRenamed:
		return evalModelExists(rule, idx)

	case KindFieldRemoved:
		return evalFieldExists(rule, idx, rule.Field)

	case KindFieldRenamed:
		old := coalesce(rule.Field, rule.OldName)
		return evalFieldExists(rule, idx, old)

	case KindFieldTypeChanged:
		field := coalesce(rule.OldName, rule.Field)
		return evalFieldExists(rule, idx, field)

	case KindMethodDeprecated, KindXMLIDChanged:
		return makeIssue(rule), true
	}

	return Issue{}, false
}

// func evaluate(rule Rule, idx modelFieldSet) (Issue, bool) {
// 	switch rule.Kind {

// 	case KindModelRemoved, KindModelRenamed:
// 		// Fire if the old model still exists in the schema.
// 		target := rule.Model
// 		if rule.OldName != "" {
// 			target = rule.OldName
// 		}
// 		if _, exists := idx[target]; !exists {
// 			return Issue{}, false // model already gone → no issue
// 		}
// 		return makeIssue(rule), true

// 	case KindFieldRemoved:
// 		// Fire if the field still exists on the model.
// 		fields, ok := idx[rule.Model]
// 		if !ok {
// 			return Issue{}, false // model doesn't exist → no issue
// 		}
// 		fieldName := rule.Field
// 		if !fields[fieldName] {
// 			return Issue{}, false // field already gone → no issue
// 		}
// 		return makeIssue(rule), true

// 	case KindFieldRenamed:
// 		// Fire if the old field name still exists (meaning the rename hasn't happened).
// 		fields, ok := idx[rule.Model]
// 		if !ok {
// 			return Issue{}, false
// 		}
// 		old := rule.OldName
// 		if rule.Field != "" {
// 			old = rule.Field
// 		}
// 		if !fields[old] {
// 			return Issue{}, false // old name gone → already migrated
// 		}
// 		return makeIssue(rule), true

// 	case KindFieldTypeChanged:
// 		// Fire if the field is present (type change is invisible in schema text).
// 		fields, ok := idx[rule.Model]
// 		if !ok {
// 			return Issue{}, false
// 		}
// 		fieldName := rule.Field
// 		if rule.OldName != "" {
// 			fieldName = rule.OldName
// 		}
// 		if !fields[fieldName] {
// 			return Issue{}, false
// 		}
// 		return makeIssue(rule), true

// 	case KindMethodDeprecated, KindXMLIDChanged:
// 		// These cannot be detected from a schema snapshot — always emit as a
// 		// "known risk" warning so developers are aware.
// 		return makeIssue(rule), true
// 	}

// 	return Issue{}, false
// }

func makeIssue(rule Rule) Issue {
	return Issue{
		Severity:    rule.Severity,
		Kind:        rule.Kind,
		Model:       rule.Model,
		Field:       rule.Field,
		OldName:     rule.OldName,
		NewName:     rule.NewName,
		Message:     rule.Message,
		Fix:         rule.Fix,
		FromVersion: rule.FromVersion,
		ToVersion:   rule.ToVersion,
	}
}

// buildIndex parses the schema snapshot JSON into a fast lookup structure.
// It handles both the flat {"model": {..., "fields": {...}}} shape and
// a slice format, so it is robust against minor syncer format drift.
func buildIndex(raw json.RawMessage) (modelFieldSet, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return modelFieldSet{}, nil
	}

	// Try map[modelName]modelObject first (syncer output).
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &asMap); err != nil {
		return nil, err
	}

	idx := make(modelFieldSet, len(asMap))

	for modelName, modelRaw := range asMap {
		// Normalise key (Odoo technical names are lowercase but be safe).
		key := strings.ToLower(modelName)

		var obj struct {
			Fields map[string]json.RawMessage `json:"fields"`
		}
		if err := json.Unmarshal(modelRaw, &obj); err != nil {
			// Model entry is malformed — create empty field set so model presence
			// is still detected.
			idx[key] = map[string]bool{}
			continue
		}

		fieldSet := make(map[string]bool, len(obj.Fields))
		for fname := range obj.Fields {
			fieldSet[strings.ToLower(fname)] = true
		}
		idx[key] = fieldSet
	}

	return idx, nil
}
