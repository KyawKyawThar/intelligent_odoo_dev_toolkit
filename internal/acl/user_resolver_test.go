package acl

import (
	"testing"
)

// schemaWithUsers returns a minimal SchemaModels that includes res.users.
func schemaWithUsers() SchemaModels {
	return SchemaModels{
		"res.users": SchemaModel{
			Model: "res.users",
			Name:  "Users",
			Fields: map[string]SchemaModelField{
				"login":       {Type: "char", String: "login"},
				"name":        {Type: "char", String: "name"},
				"active":      {Type: "boolean", String: "active"},
				"groups_id":   {Type: "many2many", String: "groups_id"},
				"company_id":  {Type: "many2one", String: "company_id"},
				"company_ids": {Type: "many2many", String: "company_ids"},
				"share":       {Type: "boolean", String: "share"},
			},
		},
		"res.partner": SchemaModel{
			Model: "res.partner",
			Name:  "Contact",
		},
	}
}

// makeUserData builds a typical res.users record as it arrives from the agent
// (JSON-decoded: numbers are float64, arrays are []any).
func makeUserData(uid int, login, name string, active bool, groupIDs []int, companyID int) map[string]any {
	groups := make([]any, len(groupIDs))
	for i, g := range groupIDs {
		groups[i] = float64(g)
	}
	return map[string]any{
		"id":          float64(uid),
		"login":       login,
		"name":        name,
		"active":      active,
		"groups_id":   groups,
		"company_id":  float64(companyID),
		"company_ids": []any{float64(companyID)},
		"share":       false,
	}
}

func TestResolveNormalUser(t *testing.T) {
	resolver := NewUserResolver()
	schema := schemaWithUsers()
	data := makeUserData(42, "demo", "Demo User", true, []int{7, 9, 14}, 1)

	user, result, err := resolver.Resolve(schema, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK (reason: %s)", result.Verdict, result.Reason)
	}
	if user.UID != 42 {
		t.Errorf("UID = %d, want 42", user.UID)
	}
	if user.Login != "demo" {
		t.Errorf("Login = %q, want 'demo'", user.Login)
	}
	if user.Name != "Demo User" {
		t.Errorf("Name = %q, want 'Demo User'", user.Name)
	}
	if !user.Active {
		t.Error("Active = false, want true")
	}
	if user.CompanyID != 1 {
		t.Errorf("CompanyID = %d, want 1", user.CompanyID)
	}
	if len(user.GroupIDs) != 3 {
		t.Fatalf("GroupIDs count = %d, want 3", len(user.GroupIDs))
	}
	if user.GroupIDs[0] != 7 || user.GroupIDs[1] != 9 || user.GroupIDs[2] != 14 {
		t.Errorf("GroupIDs = %v, want [7, 9, 14]", user.GroupIDs)
	}
	if user.SuperUser {
		t.Error("SuperUser should be false for uid 42")
	}
	if user.Share {
		t.Error("Share should be false")
	}
}

func TestResolveSuperUser(t *testing.T) {
	resolver := NewUserResolver()
	schema := schemaWithUsers()
	data := makeUserData(1, "__system__", "OdooBot", true, []int{1, 2, 3}, 1)

	user, result, err := resolver.Resolve(schema, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}
	if !user.SuperUser {
		t.Error("SuperUser should be true for uid 1")
	}
	if !user.IsSuperUser() {
		t.Error("IsSuperUser() should return true")
	}
}

func TestResolveInactiveUser(t *testing.T) {
	resolver := NewUserResolver()
	schema := schemaWithUsers()
	data := makeUserData(99, "blocked", "Blocked User", false, []int{7}, 1)

	user, result, err := resolver.Resolve(schema, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictDenied {
		t.Fatalf("verdict = %s, want DENIED", result.Verdict)
	}
	if user == nil {
		t.Fatal("user should still be returned for inactive users")
	}
	if user.Active {
		t.Error("Active should be false")
	}
}

func TestResolveUserNotFound(t *testing.T) {
	resolver := NewUserResolver()
	schema := schemaWithUsers()

	// nil data = user not found in Odoo
	_, result, err := resolver.Resolve(schema, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictDenied {
		t.Fatalf("verdict = %s, want DENIED", result.Verdict)
	}
}

func TestResolveEmptyUserData(t *testing.T) {
	resolver := NewUserResolver()
	schema := schemaWithUsers()

	_, result, err := resolver.Resolve(schema, map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictDenied {
		t.Fatalf("verdict = %s, want DENIED", result.Verdict)
	}
}

func TestResolveNoResUsersInSchema(t *testing.T) {
	resolver := NewUserResolver()
	// Schema without res.users model
	schema := SchemaModels{
		"res.partner": SchemaModel{Model: "res.partner", Name: "Contact"},
	}
	data := makeUserData(42, "demo", "Demo", true, nil, 1)

	_, result, err := resolver.Resolve(schema, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictError {
		t.Fatalf("verdict = %s, want ERROR", result.Verdict)
	}
}

func TestResolveMissingIDField(t *testing.T) {
	resolver := NewUserResolver()
	schema := schemaWithUsers()
	// User data without "id" key
	data := map[string]any{
		"login":  "noid",
		"name":   "No ID",
		"active": true,
	}

	_, _, err := resolver.Resolve(schema, data)
	if err == nil {
		t.Fatal("expected error for missing 'id' field")
	}
}

func TestResolvePortalUser(t *testing.T) {
	resolver := NewUserResolver()
	schema := schemaWithUsers()
	data := makeUserData(55, "portal_user", "Portal User", true, []int{18}, 1)
	data["share"] = true

	user, result, err := resolver.Resolve(schema, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}
	if !user.Share {
		t.Error("Share should be true for portal user")
	}
}

func TestResolveNoGroups(t *testing.T) {
	resolver := NewUserResolver()
	schema := schemaWithUsers()
	data := makeUserData(50, "nogroups", "No Groups User", true, nil, 1)

	user, result, err := resolver.Resolve(schema, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}
	if len(user.GroupIDs) != 0 {
		t.Errorf("GroupIDs = %v, want empty", user.GroupIDs)
	}
}

func TestResolveMany2OneAsArray(t *testing.T) {
	// Odoo XML-RPC returns many2one as [id, "name"]
	resolver := NewUserResolver()
	schema := schemaWithUsers()
	data := map[string]any{
		"id":          float64(10),
		"login":       "xmlrpc_user",
		"name":        "XML-RPC User",
		"active":      true,
		"groups_id":   []any{float64(1)},
		"company_id":  []any{float64(5), "My Company"},
		"company_ids": []any{float64(5)},
		"share":       false,
	}

	user, result, err := resolver.Resolve(schema, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}
	if user.CompanyID != 5 {
		t.Errorf("CompanyID = %d, want 5 (from many2one array)", user.CompanyID)
	}
}

func TestResolveMultiCompany(t *testing.T) {
	resolver := NewUserResolver()
	schema := schemaWithUsers()
	data := makeUserData(30, "multi", "Multi Company", true, []int{7}, 1)
	data["company_ids"] = []any{float64(1), float64(2), float64(3)}

	user, result, err := resolver.Resolve(schema, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}
	if len(user.CompanyIDs) != 3 {
		t.Fatalf("CompanyIDs count = %d, want 3", len(user.CompanyIDs))
	}
}

func TestHasGroup(t *testing.T) {
	user := &ResolvedUser{
		GroupIDs: []int{7, 9, 14, 22},
	}
	if !user.HasGroup(9) {
		t.Error("HasGroup(9) should be true")
	}
	if user.HasGroup(99) {
		t.Error("HasGroup(99) should be false")
	}
}

func TestResolveIntTypeID(t *testing.T) {
	// When data comes from internal Go code, id might be int not float64
	resolver := NewUserResolver()
	schema := schemaWithUsers()
	data := map[string]any{
		"id":         int(25),
		"login":      "intid",
		"name":       "Int ID User",
		"active":     true,
		"groups_id":  []any{float64(7)},
		"company_id": int(1),
	}

	user, result, err := resolver.Resolve(schema, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", result.Verdict)
	}
	if user.UID != 25 {
		t.Errorf("UID = %d, want 25", user.UID)
	}
	if user.CompanyID != 1 {
		t.Errorf("CompanyID = %d, want 1", user.CompanyID)
	}
}

func TestResolveStageField(t *testing.T) {
	resolver := NewUserResolver()
	schema := schemaWithUsers()
	data := makeUserData(42, "demo", "Demo User", true, []int{7}, 1)

	_, result, err := resolver.Resolve(schema, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Stage != "user" {
		t.Errorf("Stage = %q, want 'user'", result.Stage)
	}
}
