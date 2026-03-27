package acl

import (
	"testing"
)

// helper: build a schema with access rules for a given model.
func schemaWithAccesses(model string, accesses []SchemaAccessRule) SchemaModels {
	return SchemaModels{
		"res.users":  SchemaModel{Model: "res.users", Name: "Users"},
		"res.groups": SchemaModel{Model: "res.groups", Name: "Groups"},
		model: SchemaModel{
			Model:    model,
			Name:     model,
			Accesses: accesses,
		},
	}
}

func testUser() *ResolvedUser {
	return &ResolvedUser{
		UID:      42,
		Login:    "demo",
		Name:     "Demo User",
		Active:   true,
		GroupIDs: []int{10, 20},
	}
}

func testGroups(ids ...int) *ResolvedGroups {
	set := make(map[int]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return &ResolvedGroups{
		DirectIDs:    ids,
		EffectiveIDs: ids,
		EffectiveSet: set,
	}
}

func superUser() *ResolvedUser {
	return &ResolvedUser{
		UID:       1,
		Login:     "__system__",
		SuperUser: true,
	}
}

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestACLGrantedByGroupRule(t *testing.T) {
	checker := NewModelACLChecker()
	schema := schemaWithAccesses("sale.order", []SchemaAccessRule{
		{GroupID: "10", PermRead: true, PermWrite: true, PermCreate: true, PermUnlink: false},
	})

	result, err := checker.Check(schema, testUser(), testGroups(10, 20), "sale.order", OpRead)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK (reason: %s)", result.Verdict, result.Reason)
	}
	detail := result.Detail.(*ModelACLDetail)
	if !detail.Granted {
		t.Error("Granted should be true")
	}
}

func TestACLDeniedNoPermission(t *testing.T) {
	// User has group 10, rule exists for group 10 but perm_unlink=false
	checker := NewModelACLChecker()
	schema := schemaWithAccesses("sale.order", []SchemaAccessRule{
		{GroupID: "10", PermRead: true, PermWrite: true, PermCreate: true, PermUnlink: false},
	})

	result, err := checker.Check(schema, testUser(), testGroups(10, 20), "sale.order", OpUnlink)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictDenied {
		t.Fatalf("verdict = %s, want DENIED", result.Verdict)
	}
	detail := result.Detail.(*ModelACLDetail)
	if detail.Granted {
		t.Error("Granted should be false")
	}
}

func TestACLDeniedWrongGroup(t *testing.T) {
	// Rule exists for group 99, but user only has groups 10, 20
	checker := NewModelACLChecker()
	schema := schemaWithAccesses("sale.order", []SchemaAccessRule{
		{GroupID: "99", PermRead: true, PermWrite: true, PermCreate: true, PermUnlink: true},
	})

	result, err := checker.Check(schema, testUser(), testGroups(10, 20), "sale.order", OpRead)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictDenied {
		t.Fatalf("verdict = %s, want DENIED", result.Verdict)
	}
}

func TestACLGlobalRule(t *testing.T) {
	// group_id = "0" applies to all users
	checker := NewModelACLChecker()
	schema := schemaWithAccesses("sale.order", []SchemaAccessRule{
		{GroupID: "0", PermRead: true, PermWrite: false, PermCreate: false, PermUnlink: false},
	})

	result, err := checker.Check(schema, testUser(), testGroups(10), "sale.order", OpRead)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}
}

func TestACLGlobalRuleDeniesWrite(t *testing.T) {
	// Global rule gives read but not write
	checker := NewModelACLChecker()
	schema := schemaWithAccesses("sale.order", []SchemaAccessRule{
		{GroupID: "0", PermRead: true, PermWrite: false, PermCreate: false, PermUnlink: false},
	})

	result, err := checker.Check(schema, testUser(), testGroups(10), "sale.order", OpWrite)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictDenied {
		t.Fatalf("verdict = %s, want DENIED", result.Verdict)
	}
}

func TestACLEmptyGroupIDMeansGlobal(t *testing.T) {
	// Empty string group_id should also mean "all users"
	checker := NewModelACLChecker()
	schema := schemaWithAccesses("sale.order", []SchemaAccessRule{
		{GroupID: "", PermRead: true, PermWrite: true, PermCreate: true, PermUnlink: true},
	})

	result, err := checker.Check(schema, testUser(), testGroups(10), "sale.order", OpRead)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}
}

func TestACLNoRulesFailClosed(t *testing.T) {
	// Model exists in schema but has no access rules → denied
	checker := NewModelACLChecker()
	schema := schemaWithAccesses("custom.model", nil)

	result, err := checker.Check(schema, testUser(), testGroups(10), "custom.model", OpRead)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictDenied {
		t.Fatalf("verdict = %s, want DENIED (no rules = fail-closed)", result.Verdict)
	}
}

func TestACLModelNotInSchema(t *testing.T) {
	checker := NewModelACLChecker()
	schema := SchemaModels{
		"res.users": SchemaModel{Model: "res.users"},
	}

	result, err := checker.Check(schema, testUser(), testGroups(10), "nonexistent.model", OpRead)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictError {
		t.Fatalf("verdict = %s, want ERROR", result.Verdict)
	}
}

func TestACLSuperUserBypass(t *testing.T) {
	checker := NewModelACLChecker()
	// Even with no rules, superuser should pass
	schema := schemaWithAccesses("sale.order", nil)

	result, err := checker.Check(schema, superUser(), testGroups(), "sale.order", OpRead)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK (superuser bypass)", result.Verdict)
	}
}

func TestACLMultipleRulesOneGrants(t *testing.T) {
	// Two rules: group 99 grants read, group 10 denies read.
	// User has group 10 only → group 10 rule applies but doesn't grant.
	// Group 99 doesn't apply. Result: DENIED.
	checker := NewModelACLChecker()
	schema := schemaWithAccesses("sale.order", []SchemaAccessRule{
		{GroupID: "99", PermRead: true, PermWrite: true, PermCreate: true, PermUnlink: true},
		{GroupID: "10", PermRead: false, PermWrite: false, PermCreate: false, PermUnlink: false},
	})

	result, err := checker.Check(schema, testUser(), testGroups(10), "sale.order", OpRead)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictDenied {
		t.Fatalf("verdict = %s, want DENIED", result.Verdict)
	}
}

func TestACLMultipleRulesAdditive(t *testing.T) {
	// Rule for group 10: read only. Rule for group 20: write only.
	// User has both groups → read is granted (by group 10 rule).
	checker := NewModelACLChecker()
	schema := schemaWithAccesses("sale.order", []SchemaAccessRule{
		{GroupID: "10", PermRead: true, PermWrite: false, PermCreate: false, PermUnlink: false},
		{GroupID: "20", PermRead: false, PermWrite: true, PermCreate: false, PermUnlink: false},
	})

	// Check read → granted by group 10
	result, err := checker.Check(schema, testUser(), testGroups(10, 20), "sale.order", OpRead)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("read verdict = %s, want OK", result.Verdict)
	}

	// Check write → granted by group 20
	result, err = checker.Check(schema, testUser(), testGroups(10, 20), "sale.order", OpWrite)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("write verdict = %s, want OK", result.Verdict)
	}

	// Check create → not granted by any
	result, err = checker.Check(schema, testUser(), testGroups(10, 20), "sale.order", OpCreate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictDenied {
		t.Fatalf("create verdict = %s, want DENIED", result.Verdict)
	}
}

func TestACLGlobalPlusGroupRules(t *testing.T) {
	// Global rule gives read. Group 10 rule gives write.
	// User has group 10 → both read and write allowed.
	checker := NewModelACLChecker()
	schema := schemaWithAccesses("sale.order", []SchemaAccessRule{
		{GroupID: "0", PermRead: true, PermWrite: false, PermCreate: false, PermUnlink: false},
		{GroupID: "10", PermRead: false, PermWrite: true, PermCreate: false, PermUnlink: false},
	})

	// Read → granted by global
	result, err := checker.Check(schema, testUser(), testGroups(10), "sale.order", OpRead)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}

	// Write → granted by group 10
	result, err = checker.Check(schema, testUser(), testGroups(10), "sale.order", OpWrite)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}
}

func TestACLDetailRuleMatches(t *testing.T) {
	checker := NewModelACLChecker()
	schema := schemaWithAccesses("sale.order", []SchemaAccessRule{
		{GroupID: "10", PermRead: true, PermWrite: false, PermCreate: false, PermUnlink: false},
		{GroupID: "99", PermRead: true, PermWrite: true, PermCreate: true, PermUnlink: true},
	})

	result, err := checker.Check(schema, testUser(), testGroups(10), "sale.order", OpRead)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	detail := result.Detail.(*ModelACLDetail)
	if len(detail.Rules) != 2 {
		t.Fatalf("Rules count = %d, want 2", len(detail.Rules))
	}

	// Rule 0: group 10 → applies=true, grants=true (perm_read=true)
	r0 := detail.Rules[0]
	if !r0.Applies {
		t.Error("rule[0] Applies should be true")
	}
	if !r0.Grants {
		t.Error("rule[0] Grants should be true")
	}

	// Rule 1: group 99 → applies=false, grants=false
	r1 := detail.Rules[1]
	if r1.Applies {
		t.Error("rule[1] Applies should be false")
	}
	if r1.Grants {
		t.Error("rule[1] Grants should be false")
	}
}

func TestACLAllOperations(t *testing.T) {
	// Single rule granting all permissions
	checker := NewModelACLChecker()
	schema := schemaWithAccesses("sale.order", []SchemaAccessRule{
		{GroupID: "10", PermRead: true, PermWrite: true, PermCreate: true, PermUnlink: true},
	})

	for _, op := range []Operation{OpRead, OpWrite, OpCreate, OpUnlink} {
		result, err := checker.Check(schema, testUser(), testGroups(10), "sale.order", op)
		if err != nil {
			t.Fatalf("op=%s: unexpected error: %v", op, err)
		}
		if result.Verdict != VerdictOK {
			t.Errorf("op=%s: verdict = %s, want OK", op, result.Verdict)
		}
	}
}

func TestACLStageField(t *testing.T) {
	checker := NewModelACLChecker()
	schema := schemaWithAccesses("sale.order", []SchemaAccessRule{
		{GroupID: "10", PermRead: true},
	})

	result, err := checker.Check(schema, testUser(), testGroups(10), "sale.order", OpRead)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Stage != "model_acl" {
		t.Errorf("Stage = %q, want 'model_acl'", result.Stage)
	}
}

func TestACLDeniedReasonNoApplicableRules(t *testing.T) {
	// User has no matching groups → "none apply to user's groups"
	checker := NewModelACLChecker()
	schema := schemaWithAccesses("sale.order", []SchemaAccessRule{
		{GroupID: "99", PermRead: true},
		{GroupID: "88", PermRead: true},
	})

	result, err := checker.Check(schema, testUser(), testGroups(10), "sale.order", OpRead)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictDenied {
		t.Fatalf("verdict = %s, want DENIED", result.Verdict)
	}
	// Verify the reason mentions "none apply"
	detail := result.Detail.(*ModelACLDetail)
	applicableCount := 0
	for _, r := range detail.Rules {
		if r.Applies {
			applicableCount++
		}
	}
	if applicableCount != 0 {
		t.Errorf("expected 0 applicable rules, got %d", applicableCount)
	}
}

func TestACLDeniedReasonApplicableButNoPermission(t *testing.T) {
	// User's group matches but the permission flag is false
	checker := NewModelACLChecker()
	schema := schemaWithAccesses("sale.order", []SchemaAccessRule{
		{GroupID: "10", PermRead: true, PermWrite: false},
	})

	result, err := checker.Check(schema, testUser(), testGroups(10), "sale.order", OpWrite)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictDenied {
		t.Fatalf("verdict = %s, want DENIED", result.Verdict)
	}
	detail := result.Detail.(*ModelACLDetail)
	applicableCount := 0
	for _, r := range detail.Rules {
		if r.Applies {
			applicableCount++
		}
	}
	if applicableCount != 1 {
		t.Errorf("expected 1 applicable rule, got %d", applicableCount)
	}
}
