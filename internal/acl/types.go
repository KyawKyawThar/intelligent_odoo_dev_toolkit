// Package acl implements the 5-stage ACL pipeline that answers
// "why can't user X see record Y?" for Odoo environments.
package acl

// Verdict is the outcome of a single pipeline stage.
type Verdict string

const (
	VerdictOK      Verdict = "OK"      // Stage passed — continue pipeline.
	VerdictDenied  Verdict = "DENIED"  // Stage blocked access — pipeline stops.
	VerdictSkipped Verdict = "SKIPPED" // Stage not applicable (e.g. no rules).
	VerdictError   Verdict = "ERROR"   // Stage could not be evaluated.
)

// StageResult is the common envelope every pipeline stage returns.
type StageResult struct {
	Stage   string  `json:"stage"`
	Verdict Verdict `json:"verdict"`
	Reason  string  `json:"reason,omitempty"`
	Detail  any     `json:"detail,omitempty"`
}

// ResolvedUser is the output of Stage 1 (User Resolver).
// It contains everything the subsequent stages need about the target user.
type ResolvedUser struct {
	UID        int    `json:"uid"`
	Login      string `json:"login"`
	Name       string `json:"name"`
	Active     bool   `json:"active"`
	CompanyID  int    `json:"company_id"`
	CompanyIDs []int  `json:"company_ids,omitempty"` // multi-company
	GroupIDs   []int  `json:"group_ids"`             // res.groups many2many
	Share      bool   `json:"share"`                 // portal/public user
	SuperUser  bool   `json:"super_user"`            // admin bypass
}

// IsSuperUser returns true if the user has the Odoo superuser flag.
// Superusers bypass all ACL checks.
func (u *ResolvedUser) IsSuperUser() bool {
	return u.SuperUser
}

// HasGroup returns true if the user belongs to the given group ID.
func (u *ResolvedUser) HasGroup(groupID int) bool {
	for _, gid := range u.GroupIDs {
		if gid == groupID {
			return true
		}
	}
	return false
}

// GroupRecord represents a single res.groups record as fetched by the agent.
// The agent reads id, name, full_name, implied_ids, category_id from Odoo.
type GroupRecord struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	FullName   string `json:"full_name"` // "module_category / group_name"
	ImpliedIDs []int  `json:"implied_ids"`
	CategoryID int    `json:"category_id,omitempty"`
}

// ResolvedGroups is the output of Stage 2 (Group Resolver).
// It contains the user's direct groups, all transitively implied groups,
// and a fast lookup set of the effective (combined) group IDs.
type ResolvedGroups struct {
	DirectIDs    []int         `json:"direct_ids"`    // from ResolvedUser.GroupIDs
	ImpliedIDs   []int         `json:"implied_ids"`   // added via implied_ids expansion
	EffectiveIDs []int         `json:"effective_ids"` // union of direct + implied (sorted)
	EffectiveSet map[int]bool  `json:"-"`             // fast lookup — not serialized
	Groups       []GroupRecord `json:"groups"`        // full records for all effective groups
}

// HasGroup checks if the effective group set contains the given group ID.
func (rg *ResolvedGroups) HasGroup(groupID int) bool {
	return rg.EffectiveSet[groupID]
}

// Operation is the CRUD operation being checked.
type Operation string

const (
	OpRead   Operation = "read"
	OpWrite  Operation = "write"
	OpCreate Operation = "create"
	OpUnlink Operation = "unlink"
)

// ACLRuleMatch describes a single ir.model.access rule that was evaluated,
// indicating whether it matched the user's groups and whether it grants the
// requested permission.
type ACLRuleMatch struct {
	GroupID    string `json:"group_id"` // "0" = applies to all users
	Grants     bool   `json:"grants"`   // true if this rule grants the permission
	Applies    bool   `json:"applies"`  // true if user belongs to the rule's group
	PermRead   bool   `json:"perm_read"`
	PermWrite  bool   `json:"perm_write"`
	PermCreate bool   `json:"perm_create"`
	PermUnlink bool   `json:"perm_unlink"`
}

// SchemaModels is the deserialized form of the schema snapshot's models JSONB.
// Key = technical model name (e.g. "res.users"), value = model definition.
type SchemaModels map[string]SchemaModel

// SchemaModel mirrors dto.SchemaModel but lives in the acl package to avoid
// a circular dependency. It is populated by unmarshaling the snapshot JSONB.
type SchemaModel struct {
	Model    string                      `json:"model"`
	Name     string                      `json:"name"`
	Fields   map[string]SchemaModelField `json:"fields"`
	Accesses []SchemaAccessRule          `json:"accesses,omitempty"`
	Rules    []SchemaRecordRule          `json:"rules,omitempty"`
}

// SchemaModelField is a single field definition.
type SchemaModelField struct {
	Type     string `json:"type"`
	String   string `json:"string"`
	Required bool   `json:"required,omitempty"`
}

// SchemaAccessRule is a model-level ir.model.access entry.
type SchemaAccessRule struct {
	GroupID    string `json:"group_id"`
	PermRead   bool   `json:"perm_read"`
	PermWrite  bool   `json:"perm_write"`
	PermCreate bool   `json:"perm_create"`
	PermUnlink bool   `json:"perm_unlink"`
}

// SchemaRecordRule is a record-level ir.rule entry.
//
// Odoo ir.rule semantics:
//   - Rules with empty GroupIDs are "global rules" — they apply to ALL users
//     and their domains are ANDed together.
//   - Rules with GroupIDs are "group rules" — they apply only if the user
//     belongs to at least one of the listed groups. Matching group rules'
//     domains are ORed together.
//   - The final effective domain is: global_domains AND (group_domains).
type SchemaRecordRule struct {
	Name       string `json:"name"`
	GroupIDs   []int  `json:"group_ids,omitempty"` // empty = global rule
	Domain     string `json:"domain"`
	PermRead   bool   `json:"perm_read"`
	PermWrite  bool   `json:"perm_write"`
	PermCreate bool   `json:"perm_create"`
	PermUnlink bool   `json:"perm_unlink"`
}

// RecordData holds the field values of the actual Odoo record being checked.
// Keys are field names (e.g. "user_id", "company_id", "active").
// Values are whatever JSON produced: string, float64, bool, nil, []any, etc.
type RecordData map[string]any

// EvalContext provides runtime values for resolving references in domains.
// For example, user.id resolves to UserID, company_ids resolves to CompanyIDs.
type EvalContext struct {
	UserID     int   `json:"user_id"`
	CompanyID  int   `json:"company_id"`
	CompanyIDs []int `json:"company_ids,omitempty"`
}
