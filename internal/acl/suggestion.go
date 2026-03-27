package acl

import (
	"fmt"
	"strconv"
	"strings"
)

// Suggestion is a single actionable fix the Odoo admin can apply to resolve
// an access denial.
type Suggestion struct {
	// Stage is the pipeline stage that triggered this suggestion.
	Stage string `json:"stage"`
	// Summary is a short, human-readable description of the fix.
	Summary string `json:"summary"`
	// Detail provides the concrete Odoo action (e.g. which menu to open,
	// which XML ID to add, which field to change).
	Detail string `json:"detail,omitempty"`
	// Severity indicates how disruptive the fix is:
	//   "low"    = safe, targeted change
	//   "medium" = broader impact, review recommended
	//   "high"   = affects many users, proceed with caution
	Severity string `json:"severity"`
}

// SuggestionGenerator inspects a completed pipeline trace and produces
// actionable fix suggestions for any denial.
type SuggestionGenerator struct{}

// NewSuggestionGenerator creates a SuggestionGenerator.
func NewSuggestionGenerator() *SuggestionGenerator {
	return &SuggestionGenerator{}
}

// Generate inspects the pipeline stages and returns suggestions for fixing
// any denied or errored stage. Returns nil if the pipeline was fully allowed.
func (g *SuggestionGenerator) Generate(stages []StageResult) []Suggestion {
	var suggestions []Suggestion

	for _, stage := range stages {
		switch stage.Verdict {
		case VerdictDenied:
			suggestions = append(suggestions, g.suggestForDenied(stage)...)
		case VerdictError:
			suggestions = append(suggestions, g.suggestForError(stage)...)
		case VerdictOK, VerdictSkipped:
			// No suggestions needed for successful or skipped stages.
		}
	}

	return suggestions
}

func (g *SuggestionGenerator) suggestForDenied(stage StageResult) []Suggestion {
	switch stage.Stage {
	case stageUser:
		return g.suggestUserFixes(stage)
	case "groups":
		return g.suggestGroupFixes(stage)
	case "model_acl":
		return g.suggestModelACLFixes(stage)
	case stageRecordRule:
		return g.suggestRecordRuleFixes(stage)
	case stageDomain:
		return g.suggestDomainFixes(stage)
	default:
		return nil
	}
}

func (g *SuggestionGenerator) suggestForError(stage StageResult) []Suggestion {
	return []Suggestion{{
		Stage:    stage.Stage,
		Summary:  fmt.Sprintf("Stage %q returned an error — check schema snapshot", stage.Stage),
		Detail:   fmt.Sprintf("The pipeline could not evaluate stage %q: %s. Ensure the agent has pushed a recent schema snapshot that includes the relevant models.", stage.Stage, stage.Reason),
		Severity: "medium",
	}}
}

// ─── User stage suggestions ────────────────────────────────────────────────

func (g *SuggestionGenerator) suggestUserFixes(stage StageResult) []Suggestion {
	var suggestions []Suggestion

	reason := strings.ToLower(stage.Reason)

	if strings.Contains(reason, "inactive") {
		suggestions = append(suggestions, Suggestion{
			Stage:    "user",
			Summary:  "Re-activate the user account",
			Detail:   "Go to Settings > Users & Companies > Users, find the user, and set 'Active' to True. Inactive users cannot access any Odoo resource.",
			Severity: "low",
		})
	}

	if strings.Contains(reason, "not found") || strings.Contains(reason, "no data") {
		suggestions = append(suggestions, Suggestion{
			Stage:    "user",
			Summary:  "Verify the user exists in this Odoo database",
			Detail:   "The user ID was not found in Odoo. Verify the uid is correct and the user has not been archived or deleted. Check: Settings > Users & Companies > Users.",
			Severity: "low",
		})
	}

	if len(suggestions) == 0 {
		suggestions = append(suggestions, Suggestion{
			Stage:    "user",
			Summary:  "User validation failed",
			Detail:   stage.Reason,
			Severity: "low",
		})
	}

	return suggestions
}

// ─── Group stage suggestions ───────────────────────────────────────────────

func (g *SuggestionGenerator) suggestGroupFixes(stage StageResult) []Suggestion {
	return []Suggestion{{
		Stage:    "groups",
		Summary:  "Check the user's security groups",
		Detail:   "The group resolution stage failed. Verify the user has the expected groups assigned under Settings > Users & Companies > Users > [user] > Access Rights tab. Also check that implied groups (group inheritance) are correctly configured in the module's security XML.",
		Severity: "medium",
	}}
}

// ─── Model ACL stage suggestions ───────────────────────────────────────────

func (g *SuggestionGenerator) suggestModelACLFixes(stage StageResult) []Suggestion {
	var suggestions []Suggestion

	detail, ok := stage.Detail.(*ModelACLDetail)
	if !ok {
		suggestions = append(suggestions, Suggestion{
			Stage:    "model_acl",
			Summary:  "Add or update ir.model.access rules for this model",
			Detail:   stage.Reason,
			Severity: "medium",
		})
		return suggestions
	}

	// No rules at all for the model.
	if len(detail.Rules) == 0 {
		suggestions = append(suggestions, Suggestion{
			Stage:   "model_acl",
			Summary: fmt.Sprintf("Create an ir.model.access rule for model %q", detail.Model),
			Detail: fmt.Sprintf(
				"No access rules exist for %q. Create a CSV line in your module's security/ir.model.access.csv:\n"+
					"  id,name,model_id:id,group_id:id,perm_read,perm_write,perm_create,perm_unlink\n"+
					"  access_%s_user,%s user,model_%s,base.group_user,1,0,0,0\n"+
					"Then upgrade the module.",
				detail.Model,
				sanitizeModelName(detail.Model),
				detail.Model,
				sanitizeModelName(detail.Model),
			),
			Severity: "medium",
		})
		return suggestions
	}

	// Rules exist but none grant the requested operation.
	// Find which groups DO have rules (but without the needed perm).
	var applicableNoGrant []string
	var notApplicable []string
	for _, rule := range detail.Rules {
		if rule.Applies && !rule.Grants {
			applicableNoGrant = append(applicableNoGrant, rule.GroupID)
		}
		if !rule.Applies {
			notApplicable = append(notApplicable, rule.GroupID)
		}
	}

	if len(applicableNoGrant) > 0 {
		suggestions = append(suggestions, Suggestion{
			Stage:   "model_acl",
			Summary: fmt.Sprintf("Grant %s permission on existing access rule(s)", detail.Operation),
			Detail: fmt.Sprintf(
				"The user's groups match %d rule(s) for %q but none grant perm_%s. "+
					"Edit the ir.model.access.csv entry and set perm_%s=1 for the matching group(s), then upgrade the module.",
				len(applicableNoGrant), detail.Model, detail.Operation, detail.Operation,
			),
			Severity: "low",
		})
	}

	if len(notApplicable) > 0 && len(applicableNoGrant) == 0 {
		suggestions = append(suggestions, Suggestion{
			Stage:   "model_acl",
			Summary: fmt.Sprintf("Add the user to a group that has %s access on %q", detail.Operation, detail.Model),
			Detail: fmt.Sprintf(
				"%d rule(s) grant access to %q but the user doesn't belong to any of their groups (group IDs: %s). "+
					"Either add the user to one of these groups, or create a new ir.model.access rule for the user's group.",
				len(notApplicable), detail.Model, strings.Join(notApplicable, ", "),
			),
			Severity: "medium",
		})
	}

	return suggestions
}

// ─── Record rule stage suggestions ─────────────────────────────────────────

func (g *SuggestionGenerator) suggestRecordRuleFixes(stage StageResult) []Suggestion {
	var suggestions []Suggestion

	detail, ok := stage.Detail.(*RecordRuleDetail)
	if !ok {
		suggestions = append(suggestions, Suggestion{
			Stage:    "record_rule",
			Summary:  "Review ir.rule entries for this model",
			Detail:   stage.Reason,
			Severity: "medium",
		})
		return suggestions
	}

	// Group rules exist but none apply to the user.
	if len(detail.GroupRules) > 0 {
		var ruleNames []string
		var ruleGroupIDs []string
		for _, r := range detail.GroupRules {
			if !r.Applies {
				ruleNames = append(ruleNames, r.Name)
				for _, gid := range r.GroupIDs {
					ruleGroupIDs = append(ruleGroupIDs, strconv.Itoa(gid))
				}
			}
		}

		if len(ruleNames) > 0 {
			suggestions = append(suggestions, Suggestion{
				Stage:   "record_rule",
				Summary: fmt.Sprintf("Add the user to a group that matches a record rule for %q", detail.Model),
				Detail: fmt.Sprintf(
					"%d group-based ir.rule(s) exist [%s] but the user doesn't belong to their groups (group IDs: %s). "+
						"Add the user to one of these groups under Settings > Users & Companies > Users.",
					len(ruleNames),
					strings.Join(ruleNames, ", "),
					strings.Join(dedupStrings(ruleGroupIDs), ", "),
				),
				Severity: "medium",
			})
		}
	}

	return suggestions
}

// ─── Domain stage suggestions ──────────────────────────────────────────────

func (g *SuggestionGenerator) suggestDomainFixes(stage StageResult) []Suggestion {
	var suggestions []Suggestion

	detail, ok := stage.Detail.(*DomainEvalDetail)
	if !ok {
		suggestions = append(suggestions, Suggestion{
			Stage:    "domain",
			Summary:  "A record rule domain blocked access",
			Detail:   stage.Reason,
			Severity: "medium",
		})
		return suggestions
	}

	// Find failing global rules.
	for _, rr := range detail.RuleResults {
		if rr.Global && !rr.Pass {
			failedConds := collectFailedConditions(rr)
			suggestions = append(suggestions, Suggestion{
				Stage:   "domain",
				Summary: fmt.Sprintf("Global rule %q domain failed", rr.RuleName),
				Detail: fmt.Sprintf(
					"Global ir.rule %q applies to ALL users and its domain %s evaluated to false. "+
						"Failed condition(s): %s. "+
						"Either update the record's field values to satisfy the domain, or modify the rule's domain in Settings > Technical > Security > Record Rules.",
					rr.RuleName, rr.Domain, failedConds,
				),
				Severity: "high",
			})
		}
	}

	// Find failing group rules (all applicable group rules failed).
	if !detail.GroupPass {
		var failedGroupRules []string
		for _, rr := range detail.RuleResults {
			if !rr.Global && !rr.Pass {
				conds := collectFailedConditions(rr)
				failedGroupRules = append(failedGroupRules, fmt.Sprintf(
					"%s (domain: %s, failed: %s)",
					rr.RuleName, rr.Domain, conds,
				))
			}
		}
		if len(failedGroupRules) > 0 {
			suggestions = append(suggestions, Suggestion{
				Stage:   "domain",
				Summary: "No group record rule domain matched the record",
				Detail: fmt.Sprintf(
					"All applicable group-based ir.rule domains evaluated to false. "+
						"At least one must pass (they are ORed). Failed rules:\n  - %s\n"+
						"Fix options: (1) change the record's data to match a rule, "+
						"(2) relax a rule's domain, or (3) add a new rule with a matching domain for the user's group.",
					strings.Join(failedGroupRules, "\n  - "),
				),
				Severity: "medium",
			})
		}
	}

	return suggestions
}

// ─── Helpers ───────────────────────────────────────────────────────────────

// collectFailedConditions returns a summary string of failing conditions.
func collectFailedConditions(rr RuleDomainResult) string {
	var parts []string
	for _, c := range rr.Conditions {
		if c.Pass {
			continue
		}
		if c.Error != "" {
			parts = append(parts, fmt.Sprintf("%s: %s", c.Field, c.Error))
		} else {
			parts = append(parts, fmt.Sprintf("%s %s %v (actual: %v)", c.Field, c.Operator, c.Expected, c.Actual))
		}
	}
	if len(parts) == 0 {
		if rr.ParseError != "" {
			return "parse error: " + rr.ParseError
		}
		return "unknown"
	}
	return strings.Join(parts, "; ")
}

// sanitizeModelName converts "sale.order" to "sale_order" for use in XML IDs.
func sanitizeModelName(model string) string {
	return strings.ReplaceAll(model, ".", "_")
}

// dedupStrings returns unique strings preserving order.
func dedupStrings(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	result := make([]string, 0, len(ss))
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}
