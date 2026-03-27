package acl

import (
	"testing"
)

func TestGenerate_AllowedNoSuggestions(t *testing.T) {
	sg := NewSuggestionGenerator()
	stages := []StageResult{
		{Stage: "user", Verdict: VerdictOK},
		{Stage: "groups", Verdict: VerdictOK},
		{Stage: "model_acl", Verdict: VerdictOK},
		{Stage: "record_rule", Verdict: VerdictSkipped},
	}

	suggestions := sg.Generate(stages)
	if len(suggestions) != 0 {
		t.Errorf("expected 0 suggestions for allowed pipeline, got %d", len(suggestions))
	}
}

func TestGenerate_UserInactive(t *testing.T) {
	sg := NewSuggestionGenerator()
	stages := []StageResult{
		{Stage: "user", Verdict: VerdictDenied, Reason: "user 5 (bob) is inactive"},
	}

	suggestions := sg.Generate(stages)
	if len(suggestions) == 0 {
		t.Fatal("expected at least 1 suggestion for inactive user")
	}
	if suggestions[0].Stage != "user" {
		t.Errorf("expected stage 'user', got %q", suggestions[0].Stage)
	}
	if suggestions[0].Severity != "low" {
		t.Errorf("expected severity 'low', got %q", suggestions[0].Severity)
	}
}

func TestGenerate_UserNotFound(t *testing.T) {
	sg := NewSuggestionGenerator()
	stages := []StageResult{
		{Stage: "user", Verdict: VerdictDenied, Reason: "user not found — no data returned from Odoo"},
	}

	suggestions := sg.Generate(stages)
	if len(suggestions) == 0 {
		t.Fatal("expected at least 1 suggestion for user not found")
	}
	if suggestions[0].Summary != "Verify the user exists in this Odoo database" {
		t.Errorf("unexpected summary: %q", suggestions[0].Summary)
	}
}

func TestGenerate_ModelACLNoRules(t *testing.T) {
	sg := NewSuggestionGenerator()
	stages := []StageResult{
		{Stage: "user", Verdict: VerdictOK},
		{Stage: "groups", Verdict: VerdictOK},
		{
			Stage:   "model_acl",
			Verdict: VerdictDenied,
			Reason:  "no ir.model.access rules defined for model \"sale.order\"",
			Detail: &ModelACLDetail{
				Model:     "sale.order",
				Operation: OpRead,
				Rules:     nil,
				Granted:   false,
			},
		},
	}

	suggestions := sg.Generate(stages)
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}
	s := suggestions[0]
	if s.Stage != "model_acl" {
		t.Errorf("expected stage 'model_acl', got %q", s.Stage)
	}
	if s.Severity != "medium" {
		t.Errorf("expected severity 'medium', got %q", s.Severity)
	}
}

func TestGenerate_ModelACLPermDenied(t *testing.T) {
	sg := NewSuggestionGenerator()
	stages := []StageResult{
		{Stage: "user", Verdict: VerdictOK},
		{Stage: "groups", Verdict: VerdictOK},
		{
			Stage:   "model_acl",
			Verdict: VerdictDenied,
			Reason:  "DENIED — 1 applicable rule(s), none grant write permission",
			Detail: &ModelACLDetail{
				Model:     "sale.order",
				Operation: OpWrite,
				Rules: []ACLRuleMatch{
					{GroupID: "42", Grants: false, Applies: true, PermRead: true, PermWrite: false},
				},
				Granted: false,
			},
		},
	}

	suggestions := sg.Generate(stages)
	if len(suggestions) == 0 {
		t.Fatal("expected suggestion for perm denied")
	}
	found := false
	for _, s := range suggestions {
		if s.Stage == "model_acl" && s.Severity == "low" {
			found = true
		}
	}
	if !found {
		t.Error("expected a low-severity model_acl suggestion for granting the permission")
	}
}

func TestGenerate_ModelACLGroupMismatch(t *testing.T) {
	sg := NewSuggestionGenerator()
	stages := []StageResult{
		{Stage: "user", Verdict: VerdictOK},
		{Stage: "groups", Verdict: VerdictOK},
		{
			Stage:   "model_acl",
			Verdict: VerdictDenied,
			Reason:  "DENIED — 2 rule(s) exist but none apply to user's groups",
			Detail: &ModelACLDetail{
				Model:     "sale.order",
				Operation: OpRead,
				Rules: []ACLRuleMatch{
					{GroupID: "10", Grants: false, Applies: false, PermRead: true},
					{GroupID: "20", Grants: false, Applies: false, PermRead: true},
				},
				Granted: false,
			},
		},
	}

	suggestions := sg.Generate(stages)
	if len(suggestions) == 0 {
		t.Fatal("expected suggestion for group mismatch")
	}
	found := false
	for _, s := range suggestions {
		if s.Stage == "model_acl" && s.Severity == "medium" {
			found = true
		}
	}
	if !found {
		t.Error("expected a medium-severity suggestion to add user to a group")
	}
}

func TestGenerate_RecordRuleDenied(t *testing.T) {
	sg := NewSuggestionGenerator()
	stages := []StageResult{
		{Stage: "user", Verdict: VerdictOK},
		{Stage: "groups", Verdict: VerdictOK},
		{Stage: "model_acl", Verdict: VerdictOK},
		{
			Stage:   "record_rule",
			Verdict: VerdictDenied,
			Reason:  "DENIED — group rules exist but none match",
			Detail: &RecordRuleDetail{
				Model:     "sale.order",
				Operation: OpRead,
				GroupRules: []RecordRuleMatch{
					{Name: "sale_order_rule", Applies: false, GroupIDs: []int{7, 9}},
				},
				TotalRules:   1,
				AppliedCount: 0,
			},
		},
	}

	suggestions := sg.Generate(stages)
	if len(suggestions) == 0 {
		t.Fatal("expected suggestion for record rule denial")
	}
	if suggestions[0].Stage != "record_rule" {
		t.Errorf("expected stage 'record_rule', got %q", suggestions[0].Stage)
	}
}

func TestGenerate_DomainGlobalFailed(t *testing.T) {
	sg := NewSuggestionGenerator()
	stages := []StageResult{
		{Stage: "user", Verdict: VerdictOK},
		{Stage: "groups", Verdict: VerdictOK},
		{Stage: "model_acl", Verdict: VerdictOK},
		{Stage: "record_rule", Verdict: VerdictOK},
		{
			Stage:   "domain",
			Verdict: VerdictDenied,
			Reason:  "DENIED — 1 global rule(s) failed",
			Detail: &DomainEvalDetail{
				Model:      "sale.order",
				Operation:  OpRead,
				GlobalPass: false,
				GroupPass:  true,
				RuleResults: []RuleDomainResult{
					{
						RuleName: "global_company_rule",
						Domain:   "[('company_id','in',company_ids)]",
						Global:   true,
						Pass:     false,
						Conditions: []ConditionResult{
							{Field: "company_id", Operator: "in", Expected: []any{1}, Actual: float64(2), Pass: false},
						},
					},
				},
			},
		},
	}

	suggestions := sg.Generate(stages)
	if len(suggestions) == 0 {
		t.Fatal("expected suggestion for global domain failure")
	}
	s := suggestions[0]
	if s.Stage != "domain" {
		t.Errorf("expected stage 'domain', got %q", s.Stage)
	}
	if s.Severity != "high" {
		t.Errorf("expected severity 'high' for global rule fix, got %q", s.Severity)
	}
}

func TestGenerate_DomainGroupFailed(t *testing.T) {
	sg := NewSuggestionGenerator()
	stages := []StageResult{
		{Stage: "user", Verdict: VerdictOK},
		{Stage: "groups", Verdict: VerdictOK},
		{Stage: "model_acl", Verdict: VerdictOK},
		{Stage: "record_rule", Verdict: VerdictOK},
		{
			Stage:   "domain",
			Verdict: VerdictDenied,
			Reason:  "DENIED — no group rule domain passed",
			Detail: &DomainEvalDetail{
				Model:      "sale.order",
				Operation:  OpRead,
				GlobalPass: true,
				GroupPass:  false,
				RuleResults: []RuleDomainResult{
					{
						RuleName: "sale_order_see_own",
						Domain:   "[('user_id','=',user.id)]",
						Global:   false,
						Pass:     false,
						Conditions: []ConditionResult{
							{Field: "user_id", Operator: "=", Expected: int64(5), Actual: float64(12), Pass: false},
						},
					},
				},
			},
		},
	}

	suggestions := sg.Generate(stages)
	if len(suggestions) == 0 {
		t.Fatal("expected suggestion for group domain failure")
	}
	found := false
	for _, s := range suggestions {
		if s.Stage == "domain" && s.Severity == "medium" {
			found = true
		}
	}
	if !found {
		t.Error("expected a medium-severity domain suggestion")
	}
}

func TestGenerate_ErrorStage(t *testing.T) {
	sg := NewSuggestionGenerator()
	stages := []StageResult{
		{Stage: "user", Verdict: VerdictError, Reason: "model 'res.users' not found in schema snapshot"},
	}

	suggestions := sg.Generate(stages)
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}
	if suggestions[0].Severity != "medium" {
		t.Errorf("expected severity 'medium', got %q", suggestions[0].Severity)
	}
}

func TestCollectFailedConditions(t *testing.T) {
	rr := RuleDomainResult{
		Conditions: []ConditionResult{
			{Field: "company_id", Operator: "in", Expected: []any{1}, Actual: float64(2), Pass: false},
			{Field: "active", Operator: "=", Expected: true, Actual: true, Pass: true},
		},
	}

	result := collectFailedConditions(rr)
	if result == "" {
		t.Error("expected non-empty result")
	}
	if result == "unknown" {
		t.Error("should have found the failing condition")
	}
}

func TestSanitizeModelName(t *testing.T) {
	if got := sanitizeModelName("sale.order.line"); got != "sale_order_line" {
		t.Errorf("expected 'sale_order_line', got %q", got)
	}
}

func TestDedupStrings(t *testing.T) {
	got := dedupStrings([]string{"a", "b", "a", "c", "b"})
	if len(got) != 3 {
		t.Errorf("expected 3 unique strings, got %d", len(got))
	}
}
