package acl

import "testing"

// helper: build a schema with record rules for a given model.
func schemaWithRules(model string, rules []SchemaRecordRule) SchemaModels {
	return SchemaModels{
		"res.users":  SchemaModel{Model: "res.users", Name: "Users"},
		"res.groups": SchemaModel{Model: "res.groups", Name: "Groups"},
		model: SchemaModel{
			Model: model,
			Name:  model,
			Rules: rules,
		},
	}
}

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestRecordRuleSuperUserBypass(t *testing.T) {
	finder := NewRecordRuleFinder()
	schema := schemaWithRules("sale.order", []SchemaRecordRule{
		{Name: "rule1", GroupIDs: []int{99}, Domain: "[('user_id','=',user.id)]", PermRead: true},
	})

	result, err := finder.Find(schema, superUser(), testGroups(), "sale.order", OpRead)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK (superuser bypass)", result.Verdict)
	}
}

func TestRecordRuleModelNotInSchema(t *testing.T) {
	finder := NewRecordRuleFinder()
	schema := SchemaModels{
		"res.users": SchemaModel{Model: "res.users"},
	}

	result, err := finder.Find(schema, testUser(), testGroups(10), "nonexistent.model", OpRead)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictError {
		t.Fatalf("verdict = %s, want ERROR", result.Verdict)
	}
}

func TestRecordRuleNoRulesSkipped(t *testing.T) {
	finder := NewRecordRuleFinder()
	schema := schemaWithRules("sale.order", nil)

	result, err := finder.Find(schema, testUser(), testGroups(10), "sale.order", OpRead)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictSkipped {
		t.Fatalf("verdict = %s, want SKIPPED (no rules)", result.Verdict)
	}
}

func TestRecordRuleGlobalRuleApplies(t *testing.T) {
	// Global rule (no groups) applies to all users.
	finder := NewRecordRuleFinder()
	schema := schemaWithRules("sale.order", []SchemaRecordRule{
		{Name: "global_rule", GroupIDs: nil, Domain: "[('active','=',True)]", PermRead: true},
	})

	result, err := finder.Find(schema, testUser(), testGroups(10), "sale.order", OpRead)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}
	detail := result.Detail.(*RecordRuleDetail)
	if len(detail.GlobalRules) != 1 {
		t.Fatalf("GlobalRules count = %d, want 1", len(detail.GlobalRules))
	}
	if !detail.GlobalRules[0].Applies {
		t.Error("global rule should apply to all users")
	}
	if !detail.GlobalRules[0].Global {
		t.Error("global rule should be marked as Global")
	}
}

func TestRecordRuleGroupRuleMatchesUser(t *testing.T) {
	// Group rule for group 10 — user has group 10.
	finder := NewRecordRuleFinder()
	schema := schemaWithRules("sale.order", []SchemaRecordRule{
		{Name: "group_rule", GroupIDs: []int{10}, Domain: "[('user_id','=',user.id)]", PermRead: true},
	})

	result, err := finder.Find(schema, testUser(), testGroups(10, 20), "sale.order", OpRead)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}
	detail := result.Detail.(*RecordRuleDetail)
	if len(detail.GroupRules) != 1 {
		t.Fatalf("GroupRules count = %d, want 1", len(detail.GroupRules))
	}
	if !detail.GroupRules[0].Applies {
		t.Error("group rule should apply (user has group 10)")
	}
}

func TestRecordRuleGroupRuleNoMatch(t *testing.T) {
	// Group rule for group 99 — user only has groups 10, 20.
	finder := NewRecordRuleFinder()
	schema := schemaWithRules("sale.order", []SchemaRecordRule{
		{Name: "group_rule_99", GroupIDs: []int{99}, Domain: "[('state','=','draft')]", PermRead: true},
	})

	result, err := finder.Find(schema, testUser(), testGroups(10, 20), "sale.order", OpRead)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictDenied {
		t.Fatalf("verdict = %s, want DENIED (no group rules match)", result.Verdict)
	}
}

func TestRecordRuleMixedGlobalAndGroup(t *testing.T) {
	// One global rule + one group rule that matches.
	finder := NewRecordRuleFinder()
	schema := schemaWithRules("sale.order", []SchemaRecordRule{
		{Name: "global_active", GroupIDs: nil, Domain: "[('active','=',True)]", PermRead: true},
		{Name: "group_owner", GroupIDs: []int{10}, Domain: "[('user_id','=',user.id)]", PermRead: true},
	})

	result, err := finder.Find(schema, testUser(), testGroups(10), "sale.order", OpRead)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}
	detail := result.Detail.(*RecordRuleDetail)
	if len(detail.GlobalRules) != 1 {
		t.Errorf("GlobalRules count = %d, want 1", len(detail.GlobalRules))
	}
	if len(detail.GroupRules) != 1 {
		t.Errorf("GroupRules count = %d, want 1", len(detail.GroupRules))
	}
	if detail.AppliedCount != 2 {
		t.Errorf("AppliedCount = %d, want 2", detail.AppliedCount)
	}
}

func TestRecordRuleOperationFiltering(t *testing.T) {
	// Rule only covers perm_read. Checking write should skip it.
	finder := NewRecordRuleFinder()
	schema := schemaWithRules("sale.order", []SchemaRecordRule{
		{Name: "read_only_rule", GroupIDs: nil, Domain: "[('active','=',True)]", PermRead: true, PermWrite: false},
	})

	result, err := finder.Find(schema, testUser(), testGroups(10), "sale.order", OpWrite)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Rule doesn't cover write, so no rules apply → SKIPPED
	if result.Verdict != VerdictSkipped {
		t.Fatalf("verdict = %s, want SKIPPED (rule doesn't cover write)", result.Verdict)
	}
}

func TestRecordRuleAllPermsFalseCoversAllOps(t *testing.T) {
	// When all perm_* are false, rule applies to ALL operations (Odoo behavior).
	finder := NewRecordRuleFinder()
	schema := schemaWithRules("sale.order", []SchemaRecordRule{
		{Name: "universal_rule", GroupIDs: nil, Domain: "[('active','=',True)]"},
	})

	for _, op := range []Operation{OpRead, OpWrite, OpCreate, OpUnlink} {
		result, err := finder.Find(schema, testUser(), testGroups(10), "sale.order", op)
		if err != nil {
			t.Fatalf("op=%s: unexpected error: %v", op, err)
		}
		if result.Verdict != VerdictOK {
			t.Errorf("op=%s: verdict = %s, want OK (all-perms-false = universal)", op, result.Verdict)
		}
	}
}

func TestRecordRuleMultipleGroupsOnRule(t *testing.T) {
	// Rule has groups [30, 10] — user has group 10, should match.
	finder := NewRecordRuleFinder()
	schema := schemaWithRules("sale.order", []SchemaRecordRule{
		{Name: "multi_group_rule", GroupIDs: []int{30, 10}, Domain: "[('user_id','=',user.id)]", PermRead: true},
	})

	result, err := finder.Find(schema, testUser(), testGroups(10), "sale.order", OpRead)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK (user has group 10 which is in rule's groups)", result.Verdict)
	}
}

func TestRecordRuleMultipleGroupRulesOneMatches(t *testing.T) {
	// Two group rules: one for group 99 (no match), one for group 10 (matches).
	// Since at least one group rule matches, verdict should be OK.
	finder := NewRecordRuleFinder()
	schema := schemaWithRules("sale.order", []SchemaRecordRule{
		{Name: "rule_99", GroupIDs: []int{99}, Domain: "[('state','=','done')]", PermRead: true},
		{Name: "rule_10", GroupIDs: []int{10}, Domain: "[('user_id','=',user.id)]", PermRead: true},
	})

	result, err := finder.Find(schema, testUser(), testGroups(10), "sale.order", OpRead)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}
	detail := result.Detail.(*RecordRuleDetail)
	// rule_99 doesn't apply, rule_10 applies
	if detail.AppliedCount != 1 {
		t.Errorf("AppliedCount = %d, want 1", detail.AppliedCount)
	}
}

func TestRecordRuleStageField(t *testing.T) {
	finder := NewRecordRuleFinder()
	schema := schemaWithRules("sale.order", []SchemaRecordRule{
		{Name: "test", GroupIDs: nil, Domain: "[]", PermRead: true},
	})

	result, err := finder.Find(schema, testUser(), testGroups(10), "sale.order", OpRead)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Stage != "record_rule" {
		t.Errorf("Stage = %q, want 'record_rule'", result.Stage)
	}
}

func TestRecordRuleGroupRuleDeniedReasonMentionsGroups(t *testing.T) {
	finder := NewRecordRuleFinder()
	schema := schemaWithRules("sale.order", []SchemaRecordRule{
		{Name: "rule_99", GroupIDs: []int{99}, Domain: "[('state','=','done')]", PermRead: true},
		{Name: "rule_88", GroupIDs: []int{88}, Domain: "[('active','=',True)]", PermRead: true},
	})

	result, err := finder.Find(schema, testUser(), testGroups(10), "sale.order", OpRead)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictDenied {
		t.Fatalf("verdict = %s, want DENIED", result.Verdict)
	}
	detail := result.Detail.(*RecordRuleDetail)
	if len(detail.GroupRules) != 2 {
		t.Errorf("GroupRules = %d, want 2", len(detail.GroupRules))
	}
	for _, gr := range detail.GroupRules {
		if gr.Applies {
			t.Errorf("group rule %q should not apply", gr.Name)
		}
	}
}

func TestRecordRuleOnlyGlobalRulesNoGroupRules(t *testing.T) {
	// Only global rules exist — no group rule check needed. Should be OK.
	finder := NewRecordRuleFinder()
	schema := schemaWithRules("sale.order", []SchemaRecordRule{
		{Name: "global_1", GroupIDs: nil, Domain: "[('active','=',True)]", PermRead: true},
		{Name: "global_2", GroupIDs: nil, Domain: "[('company_id','=',user.company_id)]", PermRead: true},
	})

	result, err := finder.Find(schema, testUser(), testGroups(10), "sale.order", OpRead)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK (only global rules, all apply)", result.Verdict)
	}
	detail := result.Detail.(*RecordRuleDetail)
	if len(detail.GlobalRules) != 2 {
		t.Errorf("GlobalRules = %d, want 2", len(detail.GlobalRules))
	}
	if len(detail.GroupRules) != 0 {
		t.Errorf("GroupRules = %d, want 0", len(detail.GroupRules))
	}
}

func TestRecordRuleEmptyGroupIDsMeansGlobal(t *testing.T) {
	// Explicit empty slice should be treated as global.
	finder := NewRecordRuleFinder()
	schema := schemaWithRules("sale.order", []SchemaRecordRule{
		{Name: "empty_groups", GroupIDs: []int{}, Domain: "[('active','=',True)]", PermRead: true},
	})

	result, err := finder.Find(schema, testUser(), testGroups(10), "sale.order", OpRead)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}
	detail := result.Detail.(*RecordRuleDetail)
	if len(detail.GlobalRules) != 1 {
		t.Errorf("GlobalRules = %d, want 1 (empty GroupIDs = global)", len(detail.GlobalRules))
	}
}

func TestRecordRuleAllOperationsWithSpecificPerms(t *testing.T) {
	// Rule with perm_read=true, perm_write=true, perm_create=false, perm_unlink=false.
	finder := NewRecordRuleFinder()
	schema := schemaWithRules("sale.order", []SchemaRecordRule{
		{Name: "rw_rule", GroupIDs: nil, Domain: "[('active','=',True)]",
			PermRead: true, PermWrite: true, PermCreate: false, PermUnlink: false},
	})

	// Read and write should find the rule
	for _, op := range []Operation{OpRead, OpWrite} {
		result, err := finder.Find(schema, testUser(), testGroups(10), "sale.order", op)
		if err != nil {
			t.Fatalf("op=%s: unexpected error: %v", op, err)
		}
		if result.Verdict != VerdictOK {
			t.Errorf("op=%s: verdict = %s, want OK", op, result.Verdict)
		}
	}

	// Create and unlink should skip (rule doesn't cover them)
	for _, op := range []Operation{OpCreate, OpUnlink} {
		result, err := finder.Find(schema, testUser(), testGroups(10), "sale.order", op)
		if err != nil {
			t.Fatalf("op=%s: unexpected error: %v", op, err)
		}
		if result.Verdict != VerdictSkipped {
			t.Errorf("op=%s: verdict = %s, want SKIPPED", op, result.Verdict)
		}
	}
}
