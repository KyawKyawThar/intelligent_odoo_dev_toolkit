package acl

import (
	"fmt"
	"strconv"
)

const stageModelACL = "model_acl"

// ModelACLChecker is Stage 3 of the ACL pipeline.
// It evaluates ir.model.access rules for the target model to determine
// whether the user's groups grant the requested CRUD operation.
//
// Odoo ir.model.access semantics:
//   - Each rule is scoped to a group (group_id). "0" means all users.
//   - A rule "applies" if group_id is "0" OR the user belongs to that group.
//   - A rule "grants" if it applies AND the relevant perm_* flag is true.
//   - Access is ALLOWED if at least one applicable rule grants the permission.
//   - If NO rules exist for the model at all, access is DENIED (fail-closed).
//   - Superusers bypass all checks.
type ModelACLChecker struct{}

// NewModelACLChecker creates a ModelACLChecker.
func NewModelACLChecker() *ModelACLChecker {
	return &ModelACLChecker{}
}

// ModelACLDetail is the stage detail showing every rule evaluated and why
// access was granted or denied.
type ModelACLDetail struct {
	Model     string         `json:"model"`
	Operation Operation      `json:"operation"`
	Rules     []ACLRuleMatch `json:"rules"`
	Granted   bool           `json:"granted"`
}

// Check evaluates ir.model.access rules for the given model and operation.
//
// Parameters:
//   - schema: the deserialized models JSONB from the schema snapshot
//   - user: the ResolvedUser from Stage 1
//   - groups: the ResolvedGroups from Stage 2
//   - model: the Odoo model technical name (e.g. "sale.order")
//   - op: the CRUD operation to check
func (c *ModelACLChecker) Check(
	schema SchemaModels,
	user *ResolvedUser,
	groups *ResolvedGroups,
	model string,
	op Operation,
) (*StageResult, error) {
	// ── Superuser bypass ─────────────────────────────────────────────────
	if user.IsSuperUser() {
		return &StageResult{
			Stage:   stageModelACL,
			Verdict: VerdictOK,
			Reason:  fmt.Sprintf("user %d is SUPERUSER — ir.model.access check skipped for %s.%s", user.UID, model, op),
			Detail: &ModelACLDetail{
				Model:     model,
				Operation: op,
				Granted:   true,
			},
		}, nil
	}

	// ── Look up the model in schema ──────────────────────────────────────
	schemaModel, ok := schema[model]
	if !ok {
		return &StageResult{
			Stage:   stageModelACL,
			Verdict: VerdictError,
			Reason:  fmt.Sprintf("model %q not found in schema snapshot", model),
		}, nil
	}

	// ── No access rules → fail-closed ────────────────────────────────────
	if len(schemaModel.Accesses) == 0 {
		return &StageResult{
			Stage:   stageModelACL,
			Verdict: VerdictDenied,
			Reason:  fmt.Sprintf("no ir.model.access rules defined for model %q — access denied by default", model),
			Detail: &ModelACLDetail{
				Model:     model,
				Operation: op,
				Granted:   false,
			},
		}, nil
	}

	// ── Evaluate each rule ───────────────────────────────────────────────
	matches := make([]ACLRuleMatch, 0, len(schemaModel.Accesses))
	granted := false

	for _, rule := range schemaModel.Accesses {
		applies := ruleApplies(rule.GroupID, groups)
		grants := applies && permForOp(rule, op)

		matches = append(matches, ACLRuleMatch{
			GroupID:    rule.GroupID,
			Grants:     grants,
			Applies:    applies,
			PermRead:   rule.PermRead,
			PermWrite:  rule.PermWrite,
			PermCreate: rule.PermCreate,
			PermUnlink: rule.PermUnlink,
		})

		if grants {
			granted = true
		}
	}

	detail := &ModelACLDetail{
		Model:     model,
		Operation: op,
		Rules:     matches,
		Granted:   granted,
	}

	if granted {
		// Count how many rules grant access for the reason string.
		grantCount := 0
		for _, m := range matches {
			if m.Grants {
				grantCount++
			}
		}
		return &StageResult{
			Stage:   stageModelACL,
			Verdict: VerdictOK,
			Reason: fmt.Sprintf(
				"model %q %s: ALLOWED — %d of %d rule(s) grant access",
				model, op, grantCount, len(matches),
			),
			Detail: detail,
		}, nil
	}

	// ── Build denial reason ──────────────────────────────────────────────
	applicableCount := 0
	for _, m := range matches {
		if m.Applies {
			applicableCount++
		}
	}

	var reason string
	if applicableCount == 0 {
		reason = fmt.Sprintf(
			"model %q %s: DENIED — %d rule(s) exist but none apply to user's groups",
			model, op, len(matches),
		)
	} else {
		reason = fmt.Sprintf(
			"model %q %s: DENIED — %d applicable rule(s), none grant %s permission",
			model, op, applicableCount, op,
		)
	}

	return &StageResult{
		Stage:   stageModelACL,
		Verdict: VerdictDenied,
		Reason:  reason,
		Detail:  detail,
	}, nil
}

// ruleApplies returns true if the rule's group_id is "0" (all users) or
// the user's effective group set contains the group.
func ruleApplies(groupID string, groups *ResolvedGroups) bool {
	if groupID == "0" || groupID == "" {
		return true
	}
	gid, err := strconv.Atoi(groupID)
	if err != nil {
		return false
	}
	return groups.HasGroup(gid)
}

// permForOp returns the permission flag for the given operation.
func permForOp(rule SchemaAccessRule, op Operation) bool {
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
