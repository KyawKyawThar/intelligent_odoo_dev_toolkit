// Package migration contains the Odoo deprecation database and scanner.
// It covers breaking changes from v14 through v19, used by the migration
// scan service to warn developers before upgrading.
package migration

// Kind classifies what changed.
const (
	KindModelRemoved     = "model_removed"
	KindModelRenamed     = "model_renamed"
	KindFieldRemoved     = "field_removed"
	KindFieldRenamed     = "field_renamed"
	KindFieldTypeChanged = "field_type_changed"
	KindMethodDeprecated = "method_deprecated"
	KindXMLIDChanged     = "xmlid_changed"
	KindAPIDecorator     = "api_decorator" // @api.multi, @api.one, @api.returns etc.
	KindXMLPattern       = "xml_pattern"   // view-level XML patterns with no Python equivalent
)

// Severity classifies impact on an upgrade.
const (
	SeverityBreaking = "breaking" // upgrade will fail or data is lost
	SeverityWarning  = "warning"  // upgrade succeeds but behavior changes
	SeverityMinor    = "minor"    // soft deprecation, still works this version
)

// Rule describes one deprecation between two consecutive Odoo versions.
type Rule struct {
	// FromVersion / ToVersion are the Odoo series, e.g. "16.0" → "17.0".
	FromVersion string
	ToVersion   string

	// Kind is one of the Kind* constants above.
	Kind string

	// Severity is one of the Severity* constants above.
	Severity string

	// Model is the Odoo technical model name (e.g. "account.move").
	// Empty for method / XML-ID / decorator deprecations that span models.
	Model string

	// Field is the field name within Model.
	// Empty for model-level or method-level rules.
	Field string

	// OldName is the name before the transition (for renames / deprecated symbols).
	OldName string

	// NewName is the replacement name (for renames).
	NewName string

	// Message is a human-readable summary of the change.
	Message string

	// Fix is the recommended action for developers.
	Fix string
}

// Rules is the complete deprecation database, ordered by transition.
// Sources: Odoo upgrade guides, changelogs, and community migration notes.
var Rules = []Rule{

	// ═══════════════════════════════════════════════════════════════
	// v14 → v15
	// ═══════════════════════════════════════════════════════════════

	{
		FromVersion: "14.0", ToVersion: "15.0",
		Kind: KindAPIDecorator, Severity: SeverityBreaking,
		OldName: "api.multi",
		Message: "@api.multi was removed in Odoo 14. All methods are multi-record by default.",
		Fix:     "Delete the @api.multi decorator entirely.",
	},
	{
		FromVersion: "14.0", ToVersion: "15.0",
		Kind: KindAPIDecorator, Severity: SeverityBreaking,
		OldName: "api.one",
		Message: "@api.one was removed in Odoo 14. Rewrite using a for-loop over self.",
		Fix:     "Replace @api.one + method body with: for rec in self: ...",
	},
	{
		FromVersion: "14.0", ToVersion: "15.0",
		Kind: KindMethodDeprecated, Severity: SeverityBreaking,
		OldName: "odoo.exceptions.Warning",
		Message: "odoo.exceptions.Warning is deprecated. Use UserError or ValidationError instead.",
		Fix:     "Replace 'Warning' with 'UserError': from odoo.exceptions import UserError.",
	},
	{
		FromVersion: "14.0", ToVersion: "15.0",
		Kind: KindMethodDeprecated, Severity: SeverityBreaking,
		OldName: "raise Warning",
		Message: "raise Warning(...) is deprecated. Use raise UserError(...) instead.",
		Fix:     "Replace raise Warning(...) with raise UserError(...).",
	},
	{
		FromVersion: "14.0", ToVersion: "15.0",
		Kind: KindMethodDeprecated, Severity: SeverityWarning,
		OldName: "cr_uid_context_signature",
		Message: "Old-API method signature (self, cr, uid, ids, context=None) is the v7/v8 ORM. Use the new ORM (self.env).",
		Fix:     "Remove cr, uid, context parameters; use self.env.cr, self.env.uid, self.env.context.",
	},
	{
		FromVersion: "14.0", ToVersion: "15.0",
		Kind: KindMethodDeprecated, Severity: SeverityWarning,
		OldName: "sudo_with_user_arg",
		Message: ".sudo(user) with a user argument is deprecated. Use .with_user(user) instead.",
		Fix:     "Replace .sudo(user) with .with_user(user).",
	},
	{
		FromVersion: "14.0", ToVersion: "15.0",
		Kind: KindMethodDeprecated, Severity: SeverityWarning,
		OldName: "self.pool",
		Message: "self.pool is the v7 ORM registry. Use self.env['model.name'] instead.",
		Fix:     "Replace self.pool.get('x') with self.env['x'].",
	},
	{
		FromVersion: "14.0", ToVersion: "15.0",
		Kind: KindMethodDeprecated, Severity: SeverityWarning,
		OldName: "ustr",
		Message: "odoo.tools.misc.ustr is deprecated. Use str() instead.",
		Fix:     "Remove the ustr import and replace ustr(x) calls with str(x).",
	},
	{
		FromVersion: "14.0", ToVersion: "15.0",
		Kind: KindFieldRemoved, Severity: SeverityWarning,
		OldName: "digits_tuple",
		Message: "Float field digits=(precision, scale) tuple is deprecated. Use a named precision or plain integer scale.",
		Fix:     "Replace digits=(16, 2) with digits='Product Price' or digits=2.",
	},
	{
		FromVersion: "14.0", ToVersion: "15.0",
		Kind: KindFieldRemoved, Severity: SeverityWarning,
		OldName: "select_true",
		Message: "Field select=True is deprecated. Use index=True instead.",
		Fix:     "Replace select=True with index=True.",
	},
	{
		FromVersion: "14.0", ToVersion: "15.0",
		Kind: KindMethodDeprecated, Severity: SeverityWarning,
		OldName: "_defaults",
		Message: "_defaults dict is the v7 ORM API. Use field default= parameters instead.",
		Fix:     "Move defaults into field definitions: name = fields.Char(default='/').",
	},
	{
		FromVersion: "14.0", ToVersion: "15.0",
		Kind: KindMethodDeprecated, Severity: SeverityWarning,
		OldName: "_columns",
		Message: "_columns dict is the v7 ORM API. Declare fields directly on the model class.",
		Fix:     "Replace _columns entries with direct fields.* declarations.",
	},
	{
		FromVersion: "14.0", ToVersion: "15.0",
		Kind: KindModelRenamed, Severity: SeverityBreaking,
		Model:   "account.invoice",
		OldName: "account.invoice", NewName: "account.move",
		Message: "account.invoice was merged into account.move in v13; any v14 custom modules referencing account.invoice will break.",
		Fix:     "Replace all references to account.invoice with account.move. Use move_type field to filter invoices.",
	},
	{
		FromVersion: "14.0", ToVersion: "15.0",
		Kind: KindFieldRemoved, Severity: SeverityBreaking,
		Model: "product.template", Field: "website_published",
		Message: "product.template.website_published removed; is_published moved to website.published.mixin.",
		Fix:     "Use is_published from the website.published.mixin instead.",
	},
	{
		FromVersion: "14.0", ToVersion: "15.0",
		Kind: KindFieldRemoved, Severity: SeverityBreaking,
		Model: "res.partner", Field: "website_published",
		Message: "res.partner.website_published removed.",
		Fix:     "Use is_published from website.published.mixin.",
	},
	{
		FromVersion: "14.0", ToVersion: "15.0",
		Kind: KindFieldRemoved, Severity: SeverityBreaking,
		Model: "product.template", Field: "website_description",
		Message: "product.template.website_description removed in v15.",
		Fix:     "Use description_sale or the website_description field on website-specific models.",
	},
	{
		FromVersion: "14.0", ToVersion: "15.0",
		Kind: KindFieldRemoved, Severity: SeverityWarning,
		Model: "sale.order.line", Field: "product_updatable",
		Message: "sale.order.line.product_updatable computed field removed.",
		Fix:     "Compute the locked state from order state directly.",
	},
	{
		FromVersion: "14.0", ToVersion: "15.0",
		Kind: KindMethodDeprecated, Severity: SeverityBreaking,
		OldName: "tools.pycompat",
		Message: "tools.pycompat module removed; Python 2 compatibility shims no longer needed.",
		Fix:     "Remove all imports of odoo.tools.pycompat.",
	},
	{
		FromVersion: "14.0", ToVersion: "15.0",
		Kind: KindFieldRemoved, Severity: SeverityBreaking,
		Model: "mail.activity", Field: "feedback",
		Message: "mail.activity.feedback field removed.",
		Fix:     "Store feedback on mail.message instead.",
	},
	{
		FromVersion: "14.0", ToVersion: "15.0",
		Kind: KindFieldRemoved, Severity: SeverityWarning,
		Model: "res.company", Field: "font",
		Message: "res.company.font removed; font configuration moved to report themes.",
		Fix:     "Configure fonts via ir.actions.report or QWeb templates.",
	},
	{
		FromVersion: "14.0", ToVersion: "15.0",
		Kind: KindFieldRenamed, Severity: SeverityBreaking,
		Model: "account.move", Field: "",
		OldName: "invoice_partner_bank_id", NewName: "partner_bank_id",
		Message: "account.move.invoice_partner_bank_id renamed to partner_bank_id.",
		Fix:     "Update all references from invoice_partner_bank_id to partner_bank_id.",
	},
	{
		FromVersion: "14.0", ToVersion: "15.0",
		Kind: KindModelRemoved, Severity: SeverityBreaking,
		Model:   "account.abstract.payment",
		Message: "account.abstract.payment abstract model removed.",
		Fix:     "Inherit from account.payment directly.",
	},
	{
		FromVersion: "14.0", ToVersion: "15.0",
		Kind: KindFieldRemoved, Severity: SeverityBreaking,
		Model: "account.payment", Field: "invoice_ids",
		Message: "account.payment.invoice_ids removed; reconciliation links moved to account.move.line.",
		Fix:     "Query reconciled moves through account.move.line.matched_debit_ids / matched_credit_ids.",
	},
	{
		FromVersion: "14.0", ToVersion: "15.0",
		Kind: KindMethodDeprecated, Severity: SeverityBreaking,
		OldName: "openerp",
		Message: "openerp Python package alias removed; use odoo package name.",
		Fix:     "Replace all `import openerp` with `import odoo` and `from openerp` with `from odoo`.",
	},

	// ═══════════════════════════════════════════════════════════════
	// v15 → v16
	// ═══════════════════════════════════════════════════════════════

	{
		FromVersion: "15.0", ToVersion: "16.0",
		Kind: KindModelRenamed, Severity: SeverityBreaking,
		Model:   "payment.acquirer",
		OldName: "payment.acquirer", NewName: "payment.provider",
		Message: "payment.acquirer renamed to payment.provider. All related models and fields changed too.",
		Fix:     "Rename all references: model payment.acquirer → payment.provider, field acquirer_id → provider_id.",
	},
	{
		FromVersion: "15.0", ToVersion: "16.0",
		Kind: KindFieldRenamed, Severity: SeverityBreaking,
		Model:   "payment.transaction",
		OldName: "acquirer_id", NewName: "provider_id",
		Message: "payment.transaction.acquirer_id renamed to provider_id.",
		Fix:     "Update all ORM reads/writes and domain filters from acquirer_id to provider_id.",
	},
	{
		FromVersion: "15.0", ToVersion: "16.0",
		Kind: KindFieldRenamed, Severity: SeverityBreaking,
		Model:   "payment.token",
		OldName: "acquirer_id", NewName: "provider_id",
		Message: "payment.token.acquirer_id renamed to provider_id.",
		Fix:     "Update all references from acquirer_id to provider_id.",
	},
	{
		FromVersion: "15.0", ToVersion: "16.0",
		Kind: KindFieldRenamed, Severity: SeverityBreaking,
		Model:   "payment.transaction",
		OldName: "acquirer_reference", NewName: "provider_reference",
		Message: "payment.transaction.acquirer_reference renamed to provider_reference.",
		Fix:     "Update all references from acquirer_reference to provider_reference.",
	},
	{
		FromVersion: "15.0", ToVersion: "16.0",
		Kind: KindModelRemoved, Severity: SeverityBreaking,
		Model:   "account.account.type",
		Message: "account.account.type model removed; account types now stored as selection field on account.account.",
		Fix:     "Replace m2o to account.account.type with account_type selection field on account.account.",
	},
	{
		FromVersion: "15.0", ToVersion: "16.0",
		Kind: KindFieldRemoved, Severity: SeverityBreaking,
		Model: "account.account", Field: "user_type_id",
		Message: "account.account.user_type_id (m2o to account.account.type) replaced by account_type selection field.",
		Fix:     "Use account_type selection field: 'asset_receivable', 'liability_payable', 'income', 'expense', etc.",
	},
	{
		FromVersion: "15.0", ToVersion: "16.0",
		Kind: KindFieldRemoved, Severity: SeverityBreaking,
		Model: "account.journal", Field: "default_debit_account_id",
		Message: "account.journal.default_debit_account_id removed in v16.",
		Fix:     "Use default_account_id for journals.",
	},
	{
		FromVersion: "15.0", ToVersion: "16.0",
		Kind: KindFieldRemoved, Severity: SeverityBreaking,
		Model: "account.journal", Field: "default_credit_account_id",
		Message: "account.journal.default_credit_account_id removed in v16.",
		Fix:     "Use default_account_id for journals.",
	},
	{
		FromVersion: "15.0", ToVersion: "16.0",
		Kind: KindFieldRemoved, Severity: SeverityWarning,
		Model: "crm.stage", Field: "probability",
		Message: "crm.stage.probability field removed; probability is now AI-computed per lead.",
		Fix:     "Use crm.lead.probability (per-lead) instead of stage-level probability.",
	},
	{
		FromVersion: "15.0", ToVersion: "16.0",
		Kind: KindFieldRemoved, Severity: SeverityBreaking,
		Model: "account.move.line", Field: "account_internal_type",
		Message: "account.move.line.account_internal_type removed; use account_type from account.account.",
		Fix:     "Join to account.account and use account_type selection field.",
	},
	{
		FromVersion: "15.0", ToVersion: "16.0",
		Kind: KindFieldRemoved, Severity: SeverityBreaking,
		Model: "account.move.line", Field: "account_internal_group",
		Message: "account.move.line.account_internal_group removed.",
		Fix:     "Use account.account.account_type to determine grouping.",
	},
	{
		FromVersion: "15.0", ToVersion: "16.0",
		Kind: KindFieldRemoved, Severity: SeverityWarning,
		Model: "sale.order", Field: "website_order_line",
		Message: "sale.order.website_order_line removed from website_sale in v16.",
		Fix:     "Use order_line with website-specific domain if needed.",
	},
	{
		FromVersion: "15.0", ToVersion: "16.0",
		Kind: KindMethodDeprecated, Severity: SeverityBreaking,
		OldName: "AbstractModel._inherits_check",
		Message: "_inherits_check internal method removed; delegation inheritance works differently in v16.",
		Fix:     "Review _inherits definitions; use _inherit for behavior and _inherits only for delegation.",
	},
	{
		FromVersion: "15.0", ToVersion: "16.0",
		Kind: KindFieldRemoved, Severity: SeverityWarning,
		Model: "res.partner", Field: "signup_token",
		Message: "res.partner.signup_token, signup_type, signup_expiration moved to auth_signup module.",
		Fix:     "Access via auth_signup module; check module dependency.",
	},
	{
		FromVersion: "15.0", ToVersion: "16.0",
		Kind: KindFieldRemoved, Severity: SeverityBreaking,
		Model: "stock.warehouse.orderpoint", Field: "procurement_ids",
		Message: "stock.warehouse.orderpoint.procurement_ids removed in v16.",
		Fix:     "Use stock.move links instead.",
	},
	{
		FromVersion: "15.0", ToVersion: "16.0",
		Kind: KindFieldTypeChanged, Severity: SeverityWarning,
		Model: "account.move", Field: "state",
		Message: "account.move state values changed: 'draft', 'posted', 'cancel' remain but flow constraints tightened.",
		Fix:     "Review state-dependent business logic; posted moves cannot be edited without reset.",
	},

	// ═══════════════════════════════════════════════════════════════
	// v16 → v17
	// ═══════════════════════════════════════════════════════════════

	{
		FromVersion: "16.0", ToVersion: "17.0",
		Kind: KindFieldRemoved, Severity: SeverityBreaking,
		Model: "account.account", Field: "user_type_id",
		Message: "account.account.user_type_id fully removed in v17 (deprecated since v16).",
		Fix:     "Use account_type selection: 'asset_cash', 'asset_receivable', 'liability_payable', 'income', 'expense', etc.",
	},
	{
		FromVersion: "16.0", ToVersion: "17.0",
		Kind: KindFieldRemoved, Severity: SeverityBreaking,
		Model: "account.move", Field: "invoice_payment_state",
		Message: "account.move.invoice_payment_state removed; use payment_state.",
		Fix:     "Replace invoice_payment_state with payment_state everywhere.",
	},
	{
		FromVersion: "16.0", ToVersion: "17.0",
		Kind: KindFieldRemoved, Severity: SeverityBreaking,
		Model: "account.move.line", Field: "account_internal_type",
		Message: "account.move.line.account_internal_type removed in v17.",
		Fix:     "Use account_id.account_type instead.",
	},
	{
		FromVersion: "16.0", ToVersion: "17.0",
		Kind: KindFieldRenamed, Severity: SeverityBreaking,
		Model:   "account.tax.group",
		OldName: "account_id", NewName: "tax_current_account_id",
		Message: "account.tax.group.account_id renamed to tax_current_account_id.",
		Fix:     "Update references from account_id to tax_current_account_id.",
	},
	{
		FromVersion: "16.0", ToVersion: "17.0",
		Kind: KindFieldRenamed, Severity: SeverityBreaking,
		Model:   "account.tax.group",
		OldName: "refund_account_id", NewName: "tax_due_account_id",
		Message: "account.tax.group.refund_account_id renamed to tax_due_account_id.",
		Fix:     "Update references from refund_account_id to tax_due_account_id.",
	},
	{
		FromVersion: "16.0", ToVersion: "17.0",
		Kind: KindFieldRemoved, Severity: SeverityWarning,
		Model: "mail.thread", Field: "message_channel_ids",
		Message: "mail.thread.message_channel_ids removed; channels use discuss.channel in v17.",
		Fix:     "Use discuss.channel model for channel references.",
	},
	{
		FromVersion: "16.0", ToVersion: "17.0",
		Kind: KindModelRenamed, Severity: SeverityBreaking,
		Model:   "mail.channel",
		OldName: "mail.channel", NewName: "discuss.channel",
		Message: "mail.channel renamed to discuss.channel in v17.",
		Fix:     "Replace all references to mail.channel with discuss.channel.",
	},
	{
		FromVersion: "16.0", ToVersion: "17.0",
		Kind: KindModelRemoved, Severity: SeverityBreaking,
		Model:   "mail.channel.member",
		Message: "mail.channel.member renamed to discuss.channel.member.",
		Fix:     "Use discuss.channel.member instead.",
	},
	{
		FromVersion: "16.0", ToVersion: "17.0",
		Kind: KindFieldRemoved, Severity: SeverityBreaking,
		Model: "hr.employee", Field: "hr_presence_state",
		Message: "hr.employee.hr_presence_state removed; presence tracking changed.",
		Fix:     "Use hr.employee.activity_state or attendance records.",
	},
	{
		FromVersion: "16.0", ToVersion: "17.0",
		Kind: KindFieldRemoved, Severity: SeverityWarning,
		Model: "product.product", Field: "website_published",
		Message: "product.product.website_published removed (was already on product.template).",
		Fix:     "Use product.template.is_published.",
	},
	{
		FromVersion: "16.0", ToVersion: "17.0",
		Kind: KindMethodDeprecated, Severity: SeverityBreaking,
		OldName: "BaseModel.read_group (fill_temporal)",
		Message: "read_group fill_temporal parameter removed in v17.",
		Fix:     "Implement time-slot filling in Python/JS code if needed.",
	},
	{
		FromVersion: "16.0", ToVersion: "17.0",
		Kind: KindFieldRemoved, Severity: SeverityWarning,
		Model: "stock.move.line", Field: "product_qty",
		Message: "stock.move.line.product_qty deprecated; use reserved_qty or quantity.",
		Fix:     "Use qty_done for done quantities and reserved_qty for reservations.",
	},
	{
		FromVersion: "16.0", ToVersion: "17.0",
		Kind: KindFieldRenamed, Severity: SeverityBreaking,
		Model:   "stock.move.line",
		OldName: "qty_done", NewName: "quantity",
		Message: "stock.move.line.qty_done renamed to quantity in v17.",
		Fix:     "Replace qty_done with quantity for stock.move.line.",
	},
	{
		FromVersion: "16.0", ToVersion: "17.0",
		Kind: KindFieldRenamed, Severity: SeverityBreaking,
		Model:   "stock.move",
		OldName: "product_qty", NewName: "product_uom_qty",
		Message: "stock.move quantity field consolidation in v17; product_qty now always in UoM.",
		Fix:     "Verify all quantity reads use product_uom_qty for demand.",
	},
	{
		FromVersion: "16.0", ToVersion: "17.0",
		Kind: KindFieldRemoved, Severity: SeverityBreaking,
		Model: "website", Field: "cdn_activated",
		Message: "website.cdn_activated and CDN-related fields removed in v17.",
		Fix:     "Configure CDN at the reverse-proxy / infrastructure level instead.",
	},
	{
		FromVersion: "16.0", ToVersion: "17.0",
		Kind: KindFieldRemoved, Severity: SeverityWarning,
		Model: "res.company", Field: "po_double_validation",
		Message: "res.company.po_double_validation field removed; use purchase order approval flow.",
		Fix:     "Use purchase.order approval settings in res.company.",
	},
	{
		FromVersion: "16.0", ToVersion: "17.0",
		Kind: KindFieldRemoved, Severity: SeverityBreaking,
		Model: "account.move", Field: "sequence_number_reset",
		Message: "account.move.sequence_number_reset field removed in v17.",
		Fix:     "Manage journal entry sequencing through account.journal.",
	},
	{
		FromVersion: "16.0", ToVersion: "17.0",
		Kind: KindMethodDeprecated, Severity: SeverityWarning,
		OldName: "Environment.ref (raise_if_not_found=True default)",
		Message: "env.ref() now returns None by default when not found instead of raising; explicit raise_if_not_found=True needed.",
		Fix:     "Add raise_if_not_found=True where you expect the record to always exist.",
	},
	{
		FromVersion: "16.0", ToVersion: "17.0",
		Kind: KindFieldRemoved, Severity: SeverityWarning,
		Model: "hr.leave", Field: "payslip_state",
		Message: "hr.leave.payslip_state removed in v17.",
		Fix:     "Check payroll integration via hr.payslip instead.",
	},
	{
		FromVersion: "16.0", ToVersion: "17.0",
		Kind: KindFieldRemoved, Severity: SeverityBreaking,
		Model: "mrp.production", Field: "product_qty",
		Message: "mrp.production.product_qty renamed to qty_production in v17.",
		Fix:     "Use qty_production for manufacturing order quantities.",
	},
	{
		FromVersion: "16.0", ToVersion: "17.0",
		Kind: KindFieldRemoved, Severity: SeverityBreaking,
		Model: "account.move", Field: "fattura_pa_attachments_ids",
		Message: "Italian e-invoicing field fattura_pa_attachments_ids removed (moved to l10n_it_edi).",
		Fix:     "Use l10n_it_edi module's dedicated fields.",
	},
	{
		FromVersion: "16.0", ToVersion: "17.0",
		Kind: KindFieldTypeChanged, Severity: SeverityWarning,
		Model: "res.partner", Field: "type",
		Message: "res.partner.type: 'other' value removed; use 'contact' or specific address types.",
		Fix:     "Replace type='other' with type='contact' in existing data and code.",
	},

	// ═══════════════════════════════════════════════════════════════
	// v17 → v18
	// ═══════════════════════════════════════════════════════════════

	{
		FromVersion: "17.0", ToVersion: "18.0",
		Kind: KindMethodDeprecated, Severity: SeverityBreaking,
		OldName: "BaseModel.search (count=True)",
		Message: "search(count=True) removed in v18; use search_count() instead.",
		Fix:     "Replace env['model'].search(domain, count=True) with env['model'].search_count(domain).",
	},
	{
		FromVersion: "17.0", ToVersion: "18.0",
		Kind: KindFieldRemoved, Severity: SeverityBreaking,
		Model: "account.move", Field: "invoice_date_due",
		Message: "account.move.invoice_date_due renamed to invoice_payment_term_id-derived date in v18; use payment_term_id.",
		Fix:     "Compute due dates through payment terms; don't write invoice_date_due directly.",
	},
	{
		FromVersion: "17.0", ToVersion: "18.0",
		Kind: KindFieldRemoved, Severity: SeverityBreaking,
		Model: "sale.order", Field: "amount_undiscounted",
		Message: "sale.order.amount_undiscounted removed in v18.",
		Fix:     "Compute undiscounted amounts from order lines directly.",
	},
	{
		FromVersion: "17.0", ToVersion: "18.0",
		Kind: KindModelRemoved, Severity: SeverityBreaking,
		Model:   "account.move.reversal",
		Message: "account.move.reversal wizard model refactored in v18.",
		Fix:     "Use the updated reversal action on account.move.",
	},
	{
		FromVersion: "17.0", ToVersion: "18.0",
		Kind: KindFieldRemoved, Severity: SeverityWarning,
		Model: "res.partner", Field: "credit_limit",
		Message: "res.partner.credit_limit moved to account_credit_limit module in v18.",
		Fix:     "Install account_credit_limit module or manage limits at application level.",
	},
	{
		FromVersion: "17.0", ToVersion: "18.0",
		Kind: KindFieldRenamed, Severity: SeverityBreaking,
		Model:   "stock.quant",
		OldName: "reserved_quantity", NewName: "reserved_qty",
		Message: "stock.quant.reserved_quantity renamed to reserved_qty in v18.",
		Fix:     "Replace reserved_quantity with reserved_qty on stock.quant.",
	},
	{
		FromVersion: "17.0", ToVersion: "18.0",
		Kind: KindFieldRemoved, Severity: SeverityWarning,
		Model: "hr.attendance", Field: "worked_hours",
		Message: "hr.attendance.worked_hours computation changed; manual entries may differ.",
		Fix:     "Recalculate worked_hours after data migration.",
	},
	{
		FromVersion: "17.0", ToVersion: "18.0",
		Kind: KindFieldRemoved, Severity: SeverityBreaking,
		Model: "website", Field: "menu_id",
		Message: "website.menu_id structure changed in v18; top menu now managed differently.",
		Fix:     "Use website.menu records directly; do not rely on menu_id root.",
	},
	{
		FromVersion: "17.0", ToVersion: "18.0",
		Kind: KindFieldTypeChanged, Severity: SeverityWarning,
		Model: "ir.rule", Field: "domain_force",
		Message: "ir.rule.domain_force evaluation context changed in v18; user variable access may differ.",
		Fix:     "Test all record rules in staging before upgrading production.",
	},
	{
		FromVersion: "17.0", ToVersion: "18.0",
		Kind: KindFieldRemoved, Severity: SeverityWarning,
		Model: "mrp.bom", Field: "sequence",
		Message: "mrp.bom.sequence field semantics changed in v18.",
		Fix:     "Review BoM ordering logic after upgrade.",
	},
	{
		FromVersion: "17.0", ToVersion: "18.0",
		Kind: KindFieldRemoved, Severity: SeverityBreaking,
		Model: "purchase.order", Field: "dest_address_id",
		Message: "purchase.order.dest_address_id consolidated into partner_id flow in v18.",
		Fix:     "Use picking_type_id destination for dropship scenarios.",
	},
	{
		FromVersion: "17.0", ToVersion: "18.0",
		Kind: KindMethodDeprecated, Severity: SeverityBreaking,
		OldName: "report.AbstractReportDownloadMixin",
		Message: "AbstractReportDownloadMixin removed in v18; reports use actions directly.",
		Fix:     "Inherit from report.AbstractReport and use action_report methods.",
	},
	{
		FromVersion: "17.0", ToVersion: "18.0",
		Kind: KindFieldRemoved, Severity: SeverityWarning,
		Model: "product.template", Field: "website_sequence",
		Message: "product.template.website_sequence removed in v18; use website_published_date for ordering.",
		Fix:     "Use website_published_date or custom sequence field.",
	},
	{
		FromVersion: "17.0", ToVersion: "18.0",
		Kind: KindModelRemoved, Severity: SeverityBreaking,
		Model:   "sms.sms",
		Message: "sms.sms model refactored into discuss.channel in v18 for unified messaging.",
		Fix:     "Use discuss.channel for SMS delivery in v18.",
	},

	// ═══════════════════════════════════════════════════════════════
	// v18 → v19
	// ═══════════════════════════════════════════════════════════════

	// ── ORM / Python API ─────────────────────────────────────────

	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindMethodDeprecated, Severity: SeverityBreaking,
		OldName: "record._uid",
		Message: "record._uid is deprecated in v19. Use self.env.uid instead.",
		Fix:     "Replace self._uid with self.env.uid.",
	},
	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindMethodDeprecated, Severity: SeverityBreaking,
		OldName: "record._context",
		Message: "record._context is deprecated in v19. Use self.env.context instead.",
		Fix:     "Replace self._context with self.env.context.",
	},
	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindMethodDeprecated, Severity: SeverityBreaking,
		OldName: "record._cr",
		Message: "record._cr is deprecated in v19. Use self.env.cr instead.",
		Fix:     "Replace self._cr with self.env.cr.",
	},
	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindMethodDeprecated, Severity: SeverityBreaking,
		OldName: "name_get",
		Message: "name_get() is deprecated in v19. Read the display_name field instead.",
		Fix:     "Remove name_get() and override _compute_display_name() if a custom display name is needed.",
	},
	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindAPIDecorator, Severity: SeverityBreaking,
		OldName: "api.returns",
		Message: "@api.returns() decorator is dropped in v19.",
		Fix:     "Remove the @api.returns() decorator entirely.",
	},
	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindMethodDeprecated, Severity: SeverityBreaking,
		OldName: "_apply_ir_rules",
		Message: "_apply_ir_rules() is removed in v19. Security rules are integrated into search methods.",
		Fix:     "Remove the _apply_ir_rules() call; access control is enforced automatically.",
	},
	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindMethodDeprecated, Severity: SeverityWarning,
		OldName: "_flush_search",
		Message: "_flush_search() is deprecated in v19. Flushing is now handled internally by execute_query().",
		Fix:     "Remove the _flush_search() override.",
	},
	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindMethodDeprecated, Severity: SeverityBreaking,
		OldName: "_company_default_get",
		Message: "_company_default_get() is removed in v19. Use self.env.company instead.",
		Fix:     "Replace _company_default_get('model') with self.env.company.",
	},
	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindMethodDeprecated, Severity: SeverityBreaking,
		OldName: "odoo.osv.expression",
		Message: "odoo.osv and odoo.osv.expression are removed in v19. Use odoo.fields.Domain instead.",
		Fix:     "Replace expression.AND/OR with Domain.AND/Domain.OR from odoo.fields.",
	},
	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindMethodDeprecated, Severity: SeverityBreaking,
		OldName: "BaseModel.read_group (lazy=False)",
		Message: "read_group(lazy=False) parameter removed in v19; use _read_group() instead.",
		Fix:     "Replace read_group(..., lazy=False) with _read_group() for full grouping support.",
	},
	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindMethodDeprecated, Severity: SeverityWarning,
		OldName: "get_xml_id",
		Message: "get_xml_id() is deprecated in v19. Use _get_external_ids() instead.",
		Fix:     "Replace .get_xml_id() with ._get_external_ids().",
	},
	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindMethodDeprecated, Severity: SeverityWarning,
		OldName: "fields_get_keys",
		Message: "fields_get_keys() is deprecated in v19. Use self._fields.keys() instead.",
		Fix:     "Replace .fields_get_keys() with list(self._fields.keys()).",
	},
	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindMethodDeprecated, Severity: SeverityBreaking,
		OldName: "get_module_resource",
		Message: "get_module_resource() moved in v19. Import get_resource_from_path from odoo.modules instead.",
		Fix:     "Replace: from odoo.modules.module import get_module_resource\nWith:    from odoo.modules import get_resource_from_path",
	},
	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindMethodDeprecated, Severity: SeverityBreaking,
		OldName: "BaseModel._rec_name_search",
		Message: "_rec_name_search() removed in v19; name_search() logic consolidated into _search().",
		Fix:     "Override _search() or _name_search() instead of _rec_name_search().",
	},
	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindMethodDeprecated, Severity: SeverityWarning,
		OldName: "ir.actions.act_window.read_action",
		Message: "read_action() helper deprecated in v19; retrieve window actions via ir.actions.act_window.read().",
		Fix:     "Use self.env['ir.actions.act_window'].browse(action_id).read() directly.",
	},

	// ── Field renames ────────────────────────────────────────────

	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindFieldRenamed, Severity: SeverityBreaking,
		Model:   "sale.order.line",
		OldName: "product_uom", NewName: "product_uom_id",
		Message: "sale.order.line.product_uom renamed to product_uom_id in v19.",
		Fix:     "Rename product_uom to product_uom_id in field declarations, ORM calls, and XML views.",
	},
	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindFieldRenamed, Severity: SeverityBreaking,
		Model:   "sale.order.line",
		OldName: "tax_id", NewName: "tax_ids",
		Message: "sale.order.line.tax_id renamed to tax_ids in v19.",
		Fix:     "Rename tax_id to tax_ids in all references.",
	},
	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindFieldRenamed, Severity: SeverityBreaking,
		Model:   "product.product",
		OldName: "taxes_id", NewName: "tax_ids",
		Message: "product.product.taxes_id renamed to tax_ids in v19.",
		Fix:     "Rename taxes_id to tax_ids in all references.",
	},
	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindFieldRenamed, Severity: SeverityBreaking,
		Model:   "stock.picking",
		OldName: "move_lines", NewName: "move_ids",
		Message: "stock.picking.move_lines renamed to move_ids in v19 for consistency.",
		Fix:     "Replace all references to picking.move_lines with picking.move_ids.",
	},

	// ── Field removals ───────────────────────────────────────────

	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindFieldRemoved, Severity: SeverityBreaking,
		Model: "res.users", Field: "groups_id",
		Message: "res.users.groups_id access pattern changed in v19; direct many2many assignment deprecated.",
		Fix:     "Use res.users.write({'groups_id': [(4, group_id)]}) or the new role-based assignment API.",
	},
	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindFieldRemoved, Severity: SeverityBreaking,
		Model: "sale.order.line", Field: "product_updatable",
		Message: "sale.order.line.product_updatable removed in v19; editability controlled by order state.",
		Fix:     "Check sale.order.state directly to determine if lines are editable.",
	},
	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindFieldRemoved, Severity: SeverityWarning,
		Model: "account.move", Field: "tax_cash_basis_move_id",
		Message: "account.move.tax_cash_basis_move_id removed in v19; cash basis entries tracked differently.",
		Fix:     "Use account.partial.reconcile to trace cash basis tax entries.",
	},
	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindFieldRemoved, Severity: SeverityWarning,
		Model: "product.template", Field: "sale_line_warn",
		Message: "product.template.sale_line_warn and sale_line_warn_msg removed in v19.",
		Fix:     "Use product notes or custom warning logic via sale order onchange.",
	},
	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindFieldRemoved, Severity: SeverityWarning,
		Model: "product.template", Field: "sale_line_warn_msg",
		Message: "product.template.sale_line_warn_msg removed in v19 alongside sale_line_warn.",
		Fix:     "Use product notes or custom warning logic via sale order onchange.",
	},
	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindFieldRemoved, Severity: SeverityBreaking,
		Model: "hr.employee", Field: "barcode",
		Message: "hr.employee.barcode removed in v19; attendance identification uses badge_id or pin.",
		Fix:     "Use badge_id for badge scanning or pin for keypad attendance identification.",
	},

	// ── Field type changes ────────────────────────────────────────

	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindFieldTypeChanged, Severity: SeverityWarning,
		Model: "res.currency", Field: "rate",
		Message: "res.currency.rate is now computed on-the-fly in v19; writing directly to rate deprecated.",
		Fix:     "Use res.currency.rate records (res.currency.rate model) to set exchange rates.",
	},

	// ── Model renames / removals ──────────────────────────────────

	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindModelRenamed, Severity: SeverityBreaking,
		Model:   "account.analytic.account",
		OldName: "account.analytic.account", NewName: "account.analytic.plan",
		Message: "account.analytic.account merged into account.analytic.plan in v19.",
		Fix:     "Use account.analytic.plan for analytic grouping; migrate existing analytic accounts to plan records.",
	},
	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindModelRemoved, Severity: SeverityBreaking,
		Model:   "mail.alias.mixin",
		Message: "mail.alias.mixin abstract model refactored in v19; alias handling moved to mail.alias.mixin.optional.",
		Fix:     "Inherit from mail.alias.mixin.optional and implement _alias_get_creation_values().",
	},
	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindModelRemoved, Severity: SeverityBreaking,
		Model:   "hr.expense.sheet",
		Message: "hr.expense.sheet model is removed in v19. Expense logic moves to hr.expense directly.",
		Fix:     "Migrate expense sheet logic to hr.expense model.",
	},
	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindModelRemoved, Severity: SeverityBreaking,
		Model:   "hr.candidate",
		Message: "hr.candidate model is removed in v19.",
		Fix:     "Migrate candidate data to hr.applicant.",
	},
	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindModelRemoved, Severity: SeverityBreaking,
		Model:   "res.partner.title",
		Message: "res.partner.title model is removed in v19.",
		Fix:     "Remove all references to res.partner.title.",
	},

	// ── XML ID changes ────────────────────────────────────────────

	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindXMLIDChanged, Severity: SeverityWarning,
		OldName: "base.group_no_one",
		NewName: "base.group_system",
		Message: "base.group_no_one XML ID merged into base.group_system in v19.",
		Fix:     "Replace base.group_no_one with base.group_system in access rules and view groups.",
	},

	// ── XML view patterns ─────────────────────────────────────────

	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindXMLPattern, Severity: SeverityBreaking,
		OldName: "kanban-box",
		Message: "<kanban-box> template removed in v19. Use <card> template instead.",
		Fix:     "Rename t-name=\"kanban-box\" to t-name=\"card\" in all kanban views.",
	},
	{
		FromVersion: "18.0", ToVersion: "19.0",
		Kind: KindXMLPattern, Severity: SeverityWarning,
		OldName: `expand="0"`,
		Message: "Search view <group expand=\"0\"> attribute removed in v19. Use plain <group> instead.",
		Fix:     "Remove expand=\"0\" and string= attributes from <group> tags in search views.",
	},
}

// ByTransition returns only rules for a specific version transition.
func ByTransition(from, to string) []Rule {
	var result []Rule
	for _, r := range Rules {
		if r.FromVersion == from && r.ToVersion == to {
			result = append(result, r)
		}
	}
	return result
}

// ByModel returns all rules that affect a specific Odoo model across all versions.
func ByModel(model string) []Rule {
	var result []Rule
	for _, r := range Rules {
		if r.Model == model || r.OldName == model {
			result = append(result, r)
		}
	}
	return result
}

// Transitions returns the ordered list of supported version transitions.
func Transitions() [][2]string {
	return [][2]string{
		{"14.0", "15.0"},
		{"15.0", "16.0"},
		{"16.0", "17.0"},
		{"17.0", "18.0"},
		{"18.0", "19.0"},
	}
}

// PathBetween returns all rules needed to upgrade from `from` to `to`, crossing
// intermediate versions. For example, "15.0" → "17.0" returns v15→v16 + v16→v17.
// Returns nil if either version is unknown or from >= to.
func PathBetween(from, to string) []Rule {
	order := []string{"14.0", "15.0", "16.0", "17.0", "18.0", "19.0"}

	fromIdx, toIdx := -1, -1
	for i, v := range order {
		if v == from {
			fromIdx = i
		}
		if v == to {
			toIdx = i
		}
	}
	if fromIdx < 0 || toIdx < 0 || fromIdx >= toIdx {
		return nil
	}

	var result []Rule
	for i := fromIdx; i < toIdx; i++ {
		result = append(result, ByTransition(order[i], order[i+1])...)
	}
	return result
}
