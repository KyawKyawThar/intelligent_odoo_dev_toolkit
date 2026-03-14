package odoo

// IrModel represents the data structure for an 'ir.model' in Odoo.
type IrModel struct {
	ID         int    `xmlrpc:"id"`
	Model      string `xmlrpc:"model"`
	Name       string `xmlrpc:"name"`
	State      string `xmlrpc:"state"`
	Transient  bool   `xmlrpc:"transient"`
	AccessIDs  []int  `xmlrpc:"access_ids"`
	RuleIDs    []int  `xmlrpc:"rule_ids"`
	FieldIDs   []int  `xmlrpc:"field_id"`
	Count      int    `xmlrpc:"count"`
	LatestDate string `xmlrpc:"__last_update"`
}

// IrModelField represents an 'ir.model.fields' record.
type IrModelField struct {
	ID            int    `xmlrpc:"id"`
	Name          string `xmlrpc:"name"`
	Model         string `xmlrpc:"model"`
	FieldType     string `xmlrpc:"ttype"`
	Relation      string `xmlrpc:"relation"`
	Required      bool   `xmlrpc:"required"`
	Readonly      bool   `xmlrpc:"readonly"`
	Store         bool   `xmlrpc:"store"`
	Index         bool   `xmlrpc:"index"`
	Copy          bool   `xmlrpc:"copy"`
	Related       string `xmlrpc:"related"`
	Help          string `xmlrpc:"help"`
	String        string `xmlrpc:"field_description"`
	Selection     string `xmlrpc:"selection"`
	ComodelName   string `xmlrpc:"comodel_name"`
	RelationField string `xmlrpc:"relation_field"`
}

// IrModelAccess represents an 'ir.model.access' record.
// ModelID and GroupID are many2one fields — Odoo returns [id, "name"] or false.
// GroupID == 0 means the rule applies to all users (no group restriction).
type IrModelAccess struct {
	ID         int    `xmlrpc:"id"`
	Name       string `xmlrpc:"name"`
	ModelID    int    `xmlrpc:"model_id"` // many2one: parsed ID only
	GroupID    int    `xmlrpc:"group_id"` // many2one: 0 = applies to everyone
	PermRead   bool   `xmlrpc:"perm_read"`
	PermWrite  bool   `xmlrpc:"perm_write"`
	PermCreate bool   `xmlrpc:"perm_create"`
	PermUnlink bool   `xmlrpc:"perm_unlink"`
}

// IrRule represents an 'ir.rule' (record rule) in Odoo.
// ModelID is many2one; Groups is many2many (list of group IDs).
// Global == true means the rule applies to all users regardless of group.
type IrRule struct {
	ID         int    `xmlrpc:"id"`
	Name       string `xmlrpc:"name"`
	ModelID    int    `xmlrpc:"model_id"`    // many2one: parsed ID only
	Groups     []int  `xmlrpc:"groups"`      // many2many: list of group IDs
	Domain     string `xmlrpc:"domain_force"`
	PermRead   bool   `xmlrpc:"perm_read"`
	PermWrite  bool   `xmlrpc:"perm_write"`
	PermCreate bool   `xmlrpc:"perm_create"`
	PermUnlink bool   `xmlrpc:"perm_unlink"`
	Global     bool   `xmlrpc:"global"`
	Active     bool   `xmlrpc:"active"`
}
