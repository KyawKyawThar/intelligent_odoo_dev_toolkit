package acl

import "fmt"

const stageRecordRule = "record_rule"

// RecordRuleFinder is Stage 4 of the ACL pipeline.
// It finds all ir.rule entries for the target model, filters by the requested
// operation, and matches each rule against the user's effective groups.
//
// Odoo ir.rule semantics:
//   - Global rules (GroupIDs is empty): apply to ALL users. Their domains are
//     ANDed together — every global rule must be satisfied.
//   - Group rules (GroupIDs is non-empty): apply only if the user belongs to
//     at least one of the rule's groups. Matching group rules' domains are
//     ORed together — satisfying any one is enough.
//   - The final effective domain is:
//     (global_1 AND global_2 AND …) AND (group_1 OR group_2 OR …)
//   - If no rules exist for the model, access is unrestricted (SKIPPED).
//   - Superusers bypass all record rules.
type RecordRuleFinder struct{}

// NewRecordRuleFinder creates a RecordRuleFinder.
func NewRecordRuleFinder() *RecordRuleFinder {
	return &RecordRuleFinder{}
}

// RecordRuleMatch describes a single ir.rule that was evaluated.
type RecordRuleMatch struct {
	Name     string `json:"name"`
	Domain   string `json:"domain"`
	Global   bool   `json:"global"`  // true if rule has no groups (global rule)
	Applies  bool   `json:"applies"` // true if the rule applies to this user
	GroupIDs []int  `json:"group_ids,omitempty"`
}

// RecordRuleDetail is the stage detail showing which rules were found,
// how they were categorized, and which apply to the user.
type RecordRuleDetail struct {
	Model        string            `json:"model"`
	Operation    Operation         `json:"operation"`
	GlobalRules  []RecordRuleMatch `json:"global_rules"`
	GroupRules   []RecordRuleMatch `json:"group_rules"`
	TotalRules   int               `json:"total_rules"`
	AppliedCount int               `json:"applied_count"`
}

// Find evaluates ir.rule entries for the given model and operation.
//
// Parameters:
//   - schema: the deserialized models JSONB from the schema snapshot
//   - user: the ResolvedUser from Stage 1
//   - groups: the ResolvedGroups from Stage 2
//   - model: the Odoo model technical name (e.g. "sale.order")
//   - op: the CRUD operation to check
func (f *RecordRuleFinder) Find(
	schema SchemaModels,
	user *ResolvedUser,
	groups *ResolvedGroups,
	model string,
	op Operation,
) (*StageResult, error) {
	// ── Superuser bypass ─────────────────────────────────────────────────
	if user.IsSuperUser() {
		return &StageResult{
			Stage:   stageRecordRule,
			Verdict: VerdictOK,
			Reason:  fmt.Sprintf("user %d is SUPERUSER — ir.rule check skipped for %s.%s", user.UID, model, op),
			Detail: &RecordRuleDetail{
				Model:     model,
				Operation: op,
			},
		}, nil
	}

	// ── Look up the model in schema ──────────────────────────────────────
	schemaModel, ok := schema[model]
	if !ok {
		return &StageResult{
			Stage:   stageRecordRule,
			Verdict: VerdictError,
			Reason:  fmt.Sprintf("model %q not found in schema snapshot", model),
		}, nil
	}

	// ── No record rules → access unrestricted at this stage ─────────────
	if len(schemaModel.Rules) == 0 {
		return &StageResult{
			Stage:   stageRecordRule,
			Verdict: VerdictSkipped,
			Reason:  fmt.Sprintf("no ir.rule entries for model %q — record-level access unrestricted", model),
			Detail: &RecordRuleDetail{
				Model:     model,
				Operation: op,
			},
		}, nil
	}

	// ── Filter rules by operation and categorize ────────────────────────
	var globalRules []RecordRuleMatch
	var groupRules []RecordRuleMatch
	appliedCount := 0

	for _, rule := range schemaModel.Rules {
		if !recordRulePermForOp(rule, op) {
			continue // rule doesn't cover this operation
		}

		isGlobal := len(rule.GroupIDs) == 0
		applies := isGlobal || userInRuleGroups(rule.GroupIDs, groups)

		match := RecordRuleMatch{
			Name:     rule.Name,
			Domain:   rule.Domain,
			Global:   isGlobal,
			Applies:  applies,
			GroupIDs: rule.GroupIDs,
		}

		if isGlobal {
			globalRules = append(globalRules, match)
		} else {
			groupRules = append(groupRules, match)
		}

		if applies {
			appliedCount++
		}
	}

	totalRules := len(globalRules) + len(groupRules)

	detail := &RecordRuleDetail{
		Model:        model,
		Operation:    op,
		GlobalRules:  globalRules,
		GroupRules:   groupRules,
		TotalRules:   totalRules,
		AppliedCount: appliedCount,
	}

	// ── No rules match this operation → unrestricted ────────────────────
	if totalRules == 0 {
		return &StageResult{
			Stage:   stageRecordRule,
			Verdict: VerdictSkipped,
			Reason:  fmt.Sprintf("no ir.rule entries for model %q cover operation %s — record-level access unrestricted", model, op),
			Detail:  detail,
		}, nil
	}

	// ── Check if any group rules apply ──────────────────────────────────
	// If group rules exist but NONE apply to the user, access is denied
	// because the user doesn't match any group rule (the OR set is empty).
	hasGroupRules := len(groupRules) > 0
	anyGroupRuleApplies := false
	for _, gr := range groupRules {
		if gr.Applies {
			anyGroupRuleApplies = true
			break
		}
	}

	if hasGroupRules && !anyGroupRuleApplies {
		return &StageResult{
			Stage:   stageRecordRule,
			Verdict: VerdictDenied,
			Reason: fmt.Sprintf(
				"model %q %s: DENIED — %d group rule(s) exist but none match user's groups; "+
					"user must belong to at least one rule's groups",
				model, op, len(groupRules),
			),
			Detail: detail,
		}, nil
	}

	// ── Rules found and applicable → pass to domain evaluator ───────────
	reason := fmt.Sprintf(
		"model %q %s: %d rule(s) apply (%d global, %d group) — domains must be evaluated",
		model, op, appliedCount, len(globalRules), countApplied(groupRules),
	)

	return &StageResult{
		Stage:   stageRecordRule,
		Verdict: VerdictOK,
		Reason:  reason,
		Detail:  detail,
	}, nil
}

// recordRulePermForOp returns true if the rule covers the given operation.
// In Odoo, if all perm_* flags are false, the rule applies to all operations.
func recordRulePermForOp(rule SchemaRecordRule, op Operation) bool {
	// If all permissions are false, the rule applies to all operations.
	allFalse := !rule.PermRead && !rule.PermWrite && !rule.PermCreate && !rule.PermUnlink
	if allFalse {
		return true
	}

	switch op {
	case OpRead:
		return rule.PermRead
	case OpWrite:
		return rule.PermWrite
	case OpCreate:
		return rule.PermCreate
	case OpUnlink:
		return rule.PermUnlink
	default:
		return false
	}
}

// userInRuleGroups returns true if the user's effective group set overlaps
// with any of the rule's group IDs.
func userInRuleGroups(ruleGroupIDs []int, groups *ResolvedGroups) bool {
	for _, gid := range ruleGroupIDs {
		if groups.HasGroup(gid) {
			return true
		}
	}
	return false
}

// countApplied counts how many matches have Applies == true.
func countApplied(matches []RecordRuleMatch) int {
	n := 0
	for _, m := range matches {
		if m.Applies {
			n++
		}
	}
	return n
}
