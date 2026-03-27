package acl

import (
	"fmt"
	"strconv"
)

const (
	stageUser     = "user"
	modelResUsers = "res.users"
)

// UserResolver is Stage 1 of the ACL pipeline.
// It takes raw user record data (fetched from Odoo by the agent) and the
// schema snapshot, then validates and extracts a ResolvedUser.
type UserResolver struct{}

// NewUserResolver creates a UserResolver.
func NewUserResolver() *UserResolver {
	return &UserResolver{}
}

// Resolve validates the user record data against the schema and produces
// a ResolvedUser plus a StageResult.
//
// userData is a map[string]any as returned by the agent's XML-RPC search_read
// on res.users for the target UID. It must contain at least: id, login, name,
// active, groups_id, company_id.
//
// schema is the deserialized models JSONB from the latest schema snapshot.
// It is used to verify that res.users exists in the environment.
func (r *UserResolver) Resolve(schema SchemaModels, userData map[string]any) (*ResolvedUser, *StageResult, error) {
	// ── Validate res.users exists in schema ──────────────────────────────
	if _, ok := schema[modelResUsers]; !ok {
		return nil, &StageResult{
			Stage:   stageUser,
			Verdict: VerdictError,
			Reason:  "model 'res.users' not found in schema snapshot",
		}, nil
	}

	// ── Validate user data is present ────────────────────────────────────
	if len(userData) == 0 {
		return nil, &StageResult{
			Stage:   stageUser,
			Verdict: VerdictDenied,
			Reason:  "user not found — no data returned from Odoo",
		}, nil
	}

	// ── Extract fields ───────────────────────────────────────────────────
	uid, err := extractInt(userData, "id")
	if err != nil {
		return nil, nil, fmt.Errorf("user resolver: %w", err)
	}

	login := extractString(userData, "login")
	name := extractString(userData, "name")
	active := extractBool(userData, "active")
	share := extractBool(userData, "share")

	companyID := extractMany2OneID(userData, "company_id")
	companyIDs := extractIntSlice(userData, "company_ids")
	groupIDs := extractIntSlice(userData, "groups_id")

	// Odoo doesn't expose a "superuser" field via search_read. UID 1 is the
	// built-in superuser (__system__) in Odoo, and uid 2 is the default admin.
	// The SUPERUSER_ID constant in Odoo is 1.
	superUser := uid == 1

	user := &ResolvedUser{
		UID:        uid,
		Login:      login,
		Name:       name,
		Active:     active,
		CompanyID:  companyID,
		CompanyIDs: companyIDs,
		GroupIDs:   groupIDs,
		Share:      share,
		SuperUser:  superUser,
	}

	// ── Check if user is inactive ────────────────────────────────────────
	if !active {
		return user, &StageResult{
			Stage:   stageUser,
			Verdict: VerdictDenied,
			Reason:  fmt.Sprintf("user %d (%s) is inactive", uid, login),
			Detail:  user,
		}, nil
	}

	// ── Check if superuser (bypass) ──────────────────────────────────────
	reason := fmt.Sprintf("user %d (%s) resolved — %d groups", uid, login, len(groupIDs))
	if superUser {
		reason = fmt.Sprintf("user %d (%s) is SUPERUSER — all checks bypassed", uid, login)
	}

	return user, &StageResult{
		Stage:   stageUser,
		Verdict: VerdictOK,
		Reason:  reason,
		Detail:  user,
	}, nil
}

// ─── Field extraction helpers ──────────────────────────────────────────────
// These mirror the patterns in internal/agent/collector/acl.go but are
// tailored for the server side where data arrives as JSON-decoded maps
// (float64 for numbers, []interface{} for arrays, etc.).

func extractInt(m map[string]any, key string) (int, error) {
	v, ok := m[key]
	if !ok {
		return 0, fmt.Errorf("missing required field %q", key)
	}
	switch t := v.(type) {
	case int:
		return t, nil
	case float64:
		return int(t), nil
	case int64:
		return int(t), nil
	case string:
		i, err := strconv.Atoi(t)
		if err != nil {
			return 0, fmt.Errorf("field %q is a string but could not be converted to an integer: %w", key, err)
		}
		return i, nil
	}
	return 0, fmt.Errorf("field %q has unexpected type %T", key, v)
}

func extractString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func extractBool(m map[string]any, key string) bool {
	v, ok := m[key]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	if !ok {
		return false
	}
	return b
}

// extractMany2OneID handles Odoo many2one fields that arrive as either:
//   - [id, "display_name"] (from XML-RPC via agent)
//   - float64 (from JSON-decoded data)
//   - nil/false (unset)
func extractMany2OneID(m map[string]any, key string) int {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case []any:
		if len(t) > 0 {
			return anyToInt(t[0])
		}
	}
	return 0
}

// extractIntSlice handles Odoo many2many/one2many fields that arrive as
// []interface{} of numbers.
func extractIntSlice(m map[string]any, key string) []int {
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	result := make([]int, 0, len(arr))
	for _, item := range arr {
		if id := anyToInt(item); id != 0 {
			result = append(result, id)
		}
	}
	return result
}

func anyToInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	}
	return 0
}
