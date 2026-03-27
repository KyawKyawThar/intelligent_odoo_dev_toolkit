package acl

import (
	"testing"
)

// schemaWithGroups returns a SchemaModels with both res.users and res.groups.
func schemaWithGroups() SchemaModels {
	return SchemaModels{
		"res.users": SchemaModel{
			Model: "res.users",
			Name:  "Users",
		},
		"res.groups": SchemaModel{
			Model: "res.groups",
			Name:  "Groups",
			Fields: map[string]SchemaModelField{
				"name":        {Type: "char", String: "name"},
				"full_name":   {Type: "char", String: "full_name"},
				"implied_ids": {Type: "many2many", String: "implied_ids"},
				"category_id": {Type: "many2one", String: "category_id"},
			},
		},
	}
}

// makeGroupData builds a res.groups record as it arrives from the agent.
func makeGroupData(id int, name string, impliedIDs []int) map[string]any {
	implied := make([]any, len(impliedIDs))
	for i, gid := range impliedIDs {
		implied[i] = float64(gid)
	}
	return map[string]any{
		"id":          float64(id),
		"name":        name,
		"full_name":   "Administration / " + name,
		"implied_ids": implied,
		"category_id": float64(1),
	}
}

func TestResolveDirectGroupsOnly(t *testing.T) {
	resolver := NewGroupResolver()
	schema := schemaWithGroups()
	user := &ResolvedUser{
		UID:      42,
		Login:    "demo",
		GroupIDs: []int{10, 20},
	}
	groups := []map[string]any{
		makeGroupData(10, "Sales / User", nil),
		makeGroupData(20, "Inventory / User", nil),
	}

	resolved, result, err := resolver.Resolve(schema, user, groups)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK (reason: %s)", result.Verdict, result.Reason)
	}
	if len(resolved.DirectIDs) != 2 {
		t.Errorf("DirectIDs = %v, want 2 entries", resolved.DirectIDs)
	}
	if len(resolved.ImpliedIDs) != 0 {
		t.Errorf("ImpliedIDs = %v, want empty", resolved.ImpliedIDs)
	}
	if len(resolved.EffectiveIDs) != 2 {
		t.Errorf("EffectiveIDs = %v, want 2 entries", resolved.EffectiveIDs)
	}
}

func TestResolveWithImpliedGroups(t *testing.T) {
	// Group 10 (Sales Manager) implies Group 11 (Sales User)
	// Group 11 (Sales User) implies Group 1 (Internal User)
	resolver := NewGroupResolver()
	schema := schemaWithGroups()
	user := &ResolvedUser{
		UID:      42,
		Login:    "demo",
		GroupIDs: []int{10},
	}
	groups := []map[string]any{
		makeGroupData(10, "Sales Manager", []int{11}),
		makeGroupData(11, "Sales User", []int{1}),
		makeGroupData(1, "Internal User", nil),
	}

	resolved, result, err := resolver.Resolve(schema, user, groups)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}

	// Effective: {10, 11, 1}
	if len(resolved.EffectiveIDs) != 3 {
		t.Fatalf("EffectiveIDs = %v, want 3 entries", resolved.EffectiveIDs)
	}
	if !resolved.HasGroup(10) || !resolved.HasGroup(11) || !resolved.HasGroup(1) {
		t.Errorf("EffectiveSet missing expected groups: %v", resolved.EffectiveSet)
	}

	// Implied should be {11, 1} (not the direct group 10)
	if len(resolved.ImpliedIDs) != 2 {
		t.Errorf("ImpliedIDs = %v, want [1, 11]", resolved.ImpliedIDs)
	}
}

func TestResolveDeepImpliedChain(t *testing.T) {
	// Chain: 100 → 101 → 102 → 103
	resolver := NewGroupResolver()
	schema := schemaWithGroups()
	user := &ResolvedUser{
		UID:      42,
		Login:    "demo",
		GroupIDs: []int{100},
	}
	groups := []map[string]any{
		makeGroupData(100, "Level 0", []int{101}),
		makeGroupData(101, "Level 1", []int{102}),
		makeGroupData(102, "Level 2", []int{103}),
		makeGroupData(103, "Level 3", nil),
	}

	resolved, _, err := resolver.Resolve(schema, user, groups)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved.EffectiveIDs) != 4 {
		t.Fatalf("EffectiveIDs = %v, want 4 entries", resolved.EffectiveIDs)
	}
	for _, gid := range []int{100, 101, 102, 103} {
		if !resolved.HasGroup(gid) {
			t.Errorf("missing group %d in effective set", gid)
		}
	}
}

func TestResolveCyclicImplied(t *testing.T) {
	// Cycle: 10 → 11 → 12 → 10
	resolver := NewGroupResolver()
	schema := schemaWithGroups()
	user := &ResolvedUser{
		UID:      42,
		Login:    "demo",
		GroupIDs: []int{10},
	}
	groups := []map[string]any{
		makeGroupData(10, "Group A", []int{11}),
		makeGroupData(11, "Group B", []int{12}),
		makeGroupData(12, "Group C", []int{10}),
	}

	resolved, result, err := resolver.Resolve(schema, user, groups)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK (cycle should not cause error)", result.Verdict)
	}
	if len(resolved.EffectiveIDs) != 3 {
		t.Fatalf("EffectiveIDs = %v, want 3 entries (cycle handled)", resolved.EffectiveIDs)
	}
}

func TestResolveDiamondImplied(t *testing.T) {
	// Diamond: 10 → {11, 12}, 11 → 13, 12 → 13
	resolver := NewGroupResolver()
	schema := schemaWithGroups()
	user := &ResolvedUser{
		UID:      42,
		Login:    "demo",
		GroupIDs: []int{10},
	}
	groups := []map[string]any{
		makeGroupData(10, "Top", []int{11, 12}),
		makeGroupData(11, "Left", []int{13}),
		makeGroupData(12, "Right", []int{13}),
		makeGroupData(13, "Bottom", nil),
	}

	resolved, _, err := resolver.Resolve(schema, user, groups)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should be {10, 11, 12, 13} — no duplicates
	if len(resolved.EffectiveIDs) != 4 {
		t.Fatalf("EffectiveIDs = %v, want 4 entries (diamond deduped)", resolved.EffectiveIDs)
	}
}

func TestResolveMultipleDirectWithOverlap(t *testing.T) {
	// User has groups 10 and 11. Group 10 implies 11.
	// Effective should still be {10, 11} with no duplicates.
	resolver := NewGroupResolver()
	schema := schemaWithGroups()
	user := &ResolvedUser{
		UID:      42,
		Login:    "demo",
		GroupIDs: []int{10, 11},
	}
	groups := []map[string]any{
		makeGroupData(10, "Manager", []int{11}),
		makeGroupData(11, "User", nil),
	}

	resolved, _, err := resolver.Resolve(schema, user, groups)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved.EffectiveIDs) != 2 {
		t.Fatalf("EffectiveIDs = %v, want 2 entries", resolved.EffectiveIDs)
	}
	// ImpliedIDs should be empty since 11 is already a direct group
	if len(resolved.ImpliedIDs) != 0 {
		t.Errorf("ImpliedIDs = %v, want empty (11 is already direct)", resolved.ImpliedIDs)
	}
}

func TestResolveSuperUserBypass(t *testing.T) {
	resolver := NewGroupResolver()
	schema := schemaWithGroups()
	user := &ResolvedUser{
		UID:       1,
		Login:     "__system__",
		GroupIDs:  []int{1, 2, 3},
		SuperUser: true,
	}

	resolved, result, err := resolver.Resolve(schema, user, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}
	if len(resolved.DirectIDs) != 3 {
		t.Errorf("DirectIDs = %v, want 3", resolved.DirectIDs)
	}
}

func TestResolveNoGroupsInSchema(t *testing.T) {
	resolver := NewGroupResolver()
	schema := SchemaModels{
		"res.users": SchemaModel{Model: "res.users"},
	}
	user := &ResolvedUser{
		UID:      42,
		Login:    "demo",
		GroupIDs: []int{10},
	}

	_, result, err := resolver.Resolve(schema, user, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictError {
		t.Fatalf("verdict = %s, want ERROR", result.Verdict)
	}
}

func TestResolveEmptyGroupData(t *testing.T) {
	resolver := NewGroupResolver()
	schema := schemaWithGroups()
	user := &ResolvedUser{
		UID:      42,
		Login:    "demo",
		GroupIDs: []int{10},
	}

	_, result, err := resolver.Resolve(schema, user, []map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictError {
		t.Fatalf("verdict = %s, want ERROR", result.Verdict)
	}
}

func TestResolveUserWithNoGroups(t *testing.T) {
	resolver := NewGroupResolver()
	schema := schemaWithGroups()
	user := &ResolvedUser{
		UID:      42,
		Login:    "demo",
		GroupIDs: nil,
	}
	groups := []map[string]any{
		makeGroupData(10, "Some Group", nil),
	}

	resolved, result, err := resolver.Resolve(schema, user, groups)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}
	if len(resolved.EffectiveIDs) != 0 {
		t.Errorf("EffectiveIDs = %v, want empty", resolved.EffectiveIDs)
	}
}

func TestResolveGroupNotInData(t *testing.T) {
	// User has group 99 which is not in the group data (maybe from
	// an uninstalled module). Should not error, just skip.
	resolver := NewGroupResolver()
	schema := schemaWithGroups()
	user := &ResolvedUser{
		UID:      42,
		Login:    "demo",
		GroupIDs: []int{10, 99},
	}
	groups := []map[string]any{
		makeGroupData(10, "Known Group", nil),
	}

	resolved, result, err := resolver.Resolve(schema, user, groups)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}
	// Both 10 and 99 should be in effective set (99 just has no implied expansion)
	if len(resolved.EffectiveIDs) != 2 {
		t.Fatalf("EffectiveIDs = %v, want [10, 99]", resolved.EffectiveIDs)
	}
}

func TestResolveEffectiveIDsSorted(t *testing.T) {
	resolver := NewGroupResolver()
	schema := schemaWithGroups()
	user := &ResolvedUser{
		UID:      42,
		Login:    "demo",
		GroupIDs: []int{30, 10, 20},
	}
	groups := []map[string]any{
		makeGroupData(10, "A", nil),
		makeGroupData(20, "B", nil),
		makeGroupData(30, "C", nil),
	}

	resolved, _, err := resolver.Resolve(schema, user, groups)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i := 1; i < len(resolved.EffectiveIDs); i++ {
		if resolved.EffectiveIDs[i] < resolved.EffectiveIDs[i-1] {
			t.Fatalf("EffectiveIDs not sorted: %v", resolved.EffectiveIDs)
		}
	}
}

func TestResolveImpliedToUnknownGroup(t *testing.T) {
	// Group 10 implies group 50, but group 50 is not in data.
	// 50 should still appear in effective set, just not expandable.
	resolver := NewGroupResolver()
	schema := schemaWithGroups()
	user := &ResolvedUser{
		UID:      42,
		Login:    "demo",
		GroupIDs: []int{10},
	}
	groups := []map[string]any{
		makeGroupData(10, "Manager", []int{50}),
	}

	resolved, _, err := resolver.Resolve(schema, user, groups)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resolved.HasGroup(50) {
		t.Error("group 50 should be in effective set even if not in group data")
	}
	if len(resolved.EffectiveIDs) != 2 {
		t.Fatalf("EffectiveIDs = %v, want [10, 50]", resolved.EffectiveIDs)
	}
}

func TestResolveGroupRecordsInResult(t *testing.T) {
	resolver := NewGroupResolver()
	schema := schemaWithGroups()
	user := &ResolvedUser{
		UID:      42,
		Login:    "demo",
		GroupIDs: []int{10},
	}
	groups := []map[string]any{
		makeGroupData(10, "Sales Manager", []int{11}),
		makeGroupData(11, "Sales User", nil),
	}

	resolved, _, err := resolver.Resolve(schema, user, groups)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved.Groups) != 2 {
		t.Fatalf("Groups = %d, want 2 records", len(resolved.Groups))
	}
	// Verify the records have proper fields
	found := false
	for _, g := range resolved.Groups {
		if g.ID == 10 && g.Name == "Sales Manager" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find 'Sales Manager' in resolved group records")
	}
}

func TestResolveStageFieldGroups(t *testing.T) {
	resolver := NewGroupResolver()
	schema := schemaWithGroups()
	user := &ResolvedUser{UID: 42, Login: "demo", GroupIDs: []int{10}}
	groups := []map[string]any{makeGroupData(10, "G", nil)}

	_, result, err := resolver.Resolve(schema, user, groups)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Stage != "groups" {
		t.Errorf("Stage = %q, want 'groups'", result.Stage)
	}
}
