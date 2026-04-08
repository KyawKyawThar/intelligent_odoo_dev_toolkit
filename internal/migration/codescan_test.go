package migration_test

import (
	"testing"

	"Intelligent_Dev_ToolKit_Odoo/internal/migration"
)

// ─── ScanSource — version path validation ─────────────────────────────────────

func TestScanSource_UnsupportedPath(t *testing.T) {
	_, err := migration.ScanSource(map[string]string{"a.py": ""}, "13.0", "17.0")
	if err == nil {
		t.Fatal("expected error for unsupported version path, got nil")
	}
}

func TestScanSource_EmptyFiles(t *testing.T) {
	result, err := migration.ScanSource(map[string]string{}, "16.0", "17.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("expected 0 findings for empty input, got %d", len(result.Findings))
	}
	if result.FilesScanned != 0 {
		t.Fatalf("expected 0 files scanned, got %d", result.FilesScanned)
	}
}

func TestScanSource_NonCodeFilesSkipped(t *testing.T) {
	files := map[string]string{
		"README.md":       "account.invoice is a model",
		"static/app.js":   "env['account.invoice']",
		"__manifest__.py": `{"name": "My Module"}`,
	}
	// __manifest__.py IS .py so it will be scanned; README and .js should not.
	result, err := migration.ScanSource(files, "14.0", "15.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.FilesScanned != 1 {
		t.Fatalf("expected 1 file scanned (only .py), got %d", result.FilesScanned)
	}
}

// ─── Python: model references ─────────────────────────────────────────────────

func TestScanSource_Python_ModelRemovedViaEnvBracket(t *testing.T) {
	// account.invoice was merged into account.move (v14→v15).
	src := `
class MyModel(models.Model):
    def do_something(self):
        invoices = self.env['account.invoice'].search([])
        return invoices
`
	assertFindings(t, map[string]string{"models/test.py": src}, "14.0", "15.0",
		wantAtLeast{
			count:    1,
			model:    "account.invoice",
			kind:     migration.KindModelRenamed,
			severity: migration.SeverityBreaking,
		})
}

func TestScanSource_Python_InheritDeclaration(t *testing.T) {
	src := `
class CustomInvoice(models.Model):
    _inherit = 'account.invoice'
    extra_field = fields.Char()
`
	assertFindings(t, map[string]string{"models/custom.py": src}, "14.0", "15.0",
		wantAtLeast{
			count:    1,
			model:    "account.invoice",
			kind:     migration.KindModelRenamed,
			severity: migration.SeverityBreaking,
		})
}

func TestScanSource_Python_InheritList(t *testing.T) {
	src := `
class Mixin(models.Model):
    _inherit = ['account.invoice', 'mail.thread']
`
	assertFindings(t, map[string]string{"models/mixin.py": src}, "14.0", "15.0",
		wantAtLeast{
			count:    1,
			model:    "account.invoice",
			kind:     migration.KindModelRenamed,
			severity: migration.SeverityBreaking,
		})
}

func TestScanSource_Python_ModelRenamedPaymentAcquirer(t *testing.T) {
	// payment.acquirer → payment.provider (v15→v16).
	src := `
class PaymentWizard(models.TransientModel):
    acquirer_id = fields.Many2one(comodel_name='payment.acquirer')
`
	assertFindings(t, map[string]string{"models/wizard.py": src}, "15.0", "16.0",
		wantAtLeast{
			count:    1,
			model:    "payment.acquirer",
			kind:     migration.KindModelRenamed,
			severity: migration.SeverityBreaking,
		})
}

// ─── Python: field references ─────────────────────────────────────────────────

func TestScanSource_Python_RemovedFieldDomain(t *testing.T) {
	// account.account.user_type_id removed in v16.
	src := `
def get_accounts(self):
    return self.env['account.account'].search([('user_type_id', '=', self.type_id.id)])
`
	assertFindings(t, map[string]string{"services/account.py": src}, "15.0", "16.0",
		wantAtLeast{
			count:    1,
			field:    "user_type_id",
			kind:     migration.KindFieldRemoved,
			severity: migration.SeverityBreaking,
		})
}

func TestScanSource_Python_RemovedFieldAttributeAccess(t *testing.T) {
	// account.move.invoice_payment_state removed in v17.
	src := `
def check_paid(self):
    if self.invoice_payment_state == 'paid':
        return True
`
	assertFindings(t, map[string]string{"models/move.py": src}, "16.0", "17.0",
		wantAtLeast{
			count:    1,
			field:    "invoice_payment_state",
			kind:     migration.KindFieldRemoved,
			severity: migration.SeverityBreaking,
		})
}

func TestScanSource_Python_RenamedField_AcquirerID(t *testing.T) {
	// payment.transaction.acquirer_id → provider_id (v15→v16).
	src := `
tx = self.env['payment.transaction'].browse(tx_id)
provider = tx.acquirer_id
`
	assertFindings(t, map[string]string{"hooks/payment.py": src}, "15.0", "16.0",
		wantAtLeast{
			count:    1,
			oldName:  "acquirer_id",
			kind:     migration.KindFieldRenamed,
			severity: migration.SeverityBreaking,
		})
}

func TestScanSource_Python_RenamedField_QtyDone(t *testing.T) {
	// stock.move.line.qty_done → quantity (v16→v17).
	src := `
for line in move.move_line_ids:
    line.qty_done = line.product_uom_qty
`
	assertFindings(t, map[string]string{"models/stock.py": src}, "16.0", "17.0",
		wantAtLeast{
			count:    1,
			oldName:  "qty_done",
			kind:     migration.KindFieldRenamed,
			severity: migration.SeverityBreaking,
		})
}

// ─── Python: method deprecations ──────────────────────────────────────────────

func TestScanSource_Python_PycompatImport(t *testing.T) {
	src := `
from odoo.tools import pycompat
import odoo.tools.pycompat
`
	result, err := migration.ScanSource(map[string]string{"utils/compat.py": src}, "14.0", "15.0")
	if err != nil {
		t.Fatal(err)
	}
	if result.BreakingCount == 0 {
		t.Fatal("expected at least one breaking finding for pycompat import")
	}
}

func TestScanSource_Python_OpenERPImport(t *testing.T) {
	src := `
import openerp
from openerp import models
`
	result, err := migration.ScanSource(map[string]string{"__init__.py": src}, "14.0", "15.0")
	if err != nil {
		t.Fatal(err)
	}
	if result.BreakingCount == 0 {
		t.Fatal("expected breaking finding for openerp import")
	}
}

func TestScanSource_Python_SearchCountTrue(t *testing.T) {
	// search(count=True) removed in v18.
	src := `
n = self.env['sale.order'].search([('state', '=', 'draft')], count=True)
`
	result, err := migration.ScanSource(map[string]string{"services/count.py": src}, "17.0", "18.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) == 0 {
		t.Fatal("expected finding for search(count=True)")
	}
}

func TestScanSource_Python_FillTemporal(t *testing.T) {
	// read_group fill_temporal removed in v17.
	src := `
data = Model.read_group(domain, fields, groupby, fill_temporal=True)
`
	result, err := migration.ScanSource(map[string]string{"reports/trend.py": src}, "16.0", "17.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) == 0 {
		t.Fatal("expected finding for fill_temporal")
	}
}

// ─── XML: model references ────────────────────────────────────────────────────

func TestScanSource_XML_ModelAttribute(t *testing.T) {
	// mail.channel → discuss.channel (v16→v17).
	src := `<odoo>
  <record id="mail_channel_form" model="mail.channel">
    <field name="name">General</field>
  </record>
</odoo>`
	assertFindings(t, map[string]string{"data/channels.xml": src}, "16.0", "17.0",
		wantAtLeast{
			count:    1,
			model:    "mail.channel",
			kind:     migration.KindModelRenamed,
			severity: migration.SeverityBreaking,
		})
}

func TestScanSource_XML_ResModel(t *testing.T) {
	// payment.acquirer → payment.provider (v15→v16).
	src := `<odoo>
  <record id="act_payment" model="ir.actions.act_window">
    <field name="res_model">payment.acquirer</field>
  </record>
</odoo>`
	assertFindings(t, map[string]string{"views/actions.xml": src}, "15.0", "16.0",
		wantAtLeast{
			count:    1,
			model:    "payment.acquirer",
			kind:     migration.KindModelRenamed,
			severity: migration.SeverityBreaking,
		})
}

// ─── XML: field references ────────────────────────────────────────────────────

func TestScanSource_XML_FieldName(t *testing.T) {
	// account.account.user_type_id removed in v16.
	src := `<odoo>
  <record id="view_account_form" model="ir.ui.view">
    <field name="arch" type="xml">
      <form>
        <field name="user_type_id" widget="selection"/>
      </form>
    </field>
  </record>
</odoo>`
	assertFindings(t, map[string]string{"views/account.xml": src}, "15.0", "16.0",
		wantAtLeast{
			count:    1,
			field:    "user_type_id",
			kind:     migration.KindFieldRemoved,
			severity: migration.SeverityBreaking,
		})
}

func TestScanSource_XML_InvoicePaymentState(t *testing.T) {
	// account.move.invoice_payment_state removed in v17.
	src := `<odoo>
  <record id="view_move_tree" model="ir.ui.view">
    <field name="arch" type="xml">
      <tree>
        <field name="invoice_payment_state"/>
      </tree>
    </field>
  </record>
</odoo>`
	assertFindings(t, map[string]string{"views/move.xml": src}, "16.0", "17.0",
		wantAtLeast{
			count:    1,
			field:    "invoice_payment_state",
			kind:     migration.KindFieldRemoved,
			severity: migration.SeverityBreaking,
		})
}

// ─── Multi-file scan ──────────────────────────────────────────────────────────

func TestScanSource_MultiFile_CombinedFindings(t *testing.T) {
	files := map[string]string{
		"models/invoice.py": `
class CustomInvoice(models.Model):
    _inherit = 'account.invoice'
    def get_bank(self):
        return self.invoice_partner_bank_id
`,
		"views/invoice.xml": `<odoo>
  <record id="view_invoice" model="account.invoice">
    <field name="arch" type="xml">
      <form><field name="invoice_partner_bank_id"/></form>
    </field>
  </record>
</odoo>`,
		"static/style.css": "/* account.invoice {color: red} */",
	}

	result, err := migration.ScanSource(files, "14.0", "15.0")
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesScanned != 2 {
		t.Fatalf("expected 2 files scanned (.py + .xml), got %d", result.FilesScanned)
	}
	if len(result.Findings) == 0 {
		t.Fatal("expected findings across both files")
	}

	// Check that findings come from both files.
	seenPy, seenXML := false, false
	for _, f := range result.Findings {
		if f.File == "models/invoice.py" {
			seenPy = true
		}
		if f.File == "views/invoice.xml" {
			seenXML = true
		}
	}
	if !seenPy {
		t.Error("expected findings in models/invoice.py")
	}
	if !seenXML {
		t.Error("expected findings in views/invoice.xml")
	}
}

// ─── Comment lines are skipped ────────────────────────────────────────────────

func TestScanSource_Python_CommentLinesSkipped(t *testing.T) {
	src := `
# self.env['account.invoice'].search([])
# _inherit = 'account.invoice'
# import openerp
actual_code = "no deprecated patterns here"
`
	result, err := migration.ScanSource(map[string]string{"models/clean.py": src}, "14.0", "15.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("expected 0 findings (all matches are in comments), got %d: %+v", len(result.Findings), result.Findings)
	}
}

// ─── Cross-version path (14→17) ───────────────────────────────────────────────

func TestScanSource_CrossVersion_14to17(t *testing.T) {
	// A file that mixes patterns from multiple transitions.
	src := `
from openerp import models                        # v14→v15 (openerp alias)
from odoo.tools import pycompat                   # v14→v15

class BadModule(models.Model):
    _inherit = 'account.invoice'                  # v14→v15 (model renamed)

    def act(self):
        self.env['account.account'].search([
            ('user_type_id', '=', 1),             # v15→v16 (field removed)
        ])
        tx = self.env['payment.transaction'].browse(1)
        return tx.acquirer_id                     # v15→v16 (field renamed)
`
	result, err := migration.ScanSource(map[string]string{"models/bad.py": src}, "14.0", "17.0")
	if err != nil {
		t.Fatal(err)
	}
	if result.BreakingCount < 4 {
		t.Fatalf("expected ≥4 breaking findings for 14→17 path, got %d", result.BreakingCount)
	}
}

// ─── Line number accuracy ─────────────────────────────────────────────────────

func TestScanSource_Python_LineNumber(t *testing.T) {
	src := "line1\nline2\nself.env['account.invoice'].search([])\nline4\n"
	result, err := migration.ScanSource(map[string]string{"m.py": src}, "14.0", "15.0")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range result.Findings {
		if f.Model == "account.invoice" && f.Line == 3 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected finding on line 3 for account.invoice; got findings: %+v", result.Findings)
	}
}

// ─── v18 → v19 ───────────────────────────────────────────────────────────────

func TestScanSource_V18V19_TransitionRegistered(t *testing.T) {
	// Ensures the 18.0→19.0 hop is in Transitions() so version dropdowns show it.
	found := false
	for _, tr := range migration.Transitions() {
		if tr[0] == "18.0" && tr[1] == "19.0" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 18.0→19.0 in Transitions(), not found")
	}
}

func TestScanSource_V18V19_RulesNonEmpty(t *testing.T) {
	rules := migration.ByTransition("18.0", "19.0")
	if len(rules) == 0 {
		t.Fatal("expected at least one rule for 18.0→19.0, got none")
	}
}

func TestScanSource_V18V19_AnalyticAccountModelRenamed(t *testing.T) {
	// account.analytic.account → account.analytic.plan (v18→v19).
	src := `
class AnalyticLine(models.Model):
    _inherit = 'account.analytic.account'
    def get_plans(self):
        return self.env['account.analytic.account'].search([])
`
	assertFindings(t, map[string]string{"models/analytic.py": src}, "18.0", "19.0",
		wantAtLeast{
			count:    1,
			model:    "account.analytic.account",
			kind:     migration.KindModelRenamed,
			severity: migration.SeverityBreaking,
		})
}

func TestScanSource_V18V19_AnalyticAccountXML(t *testing.T) {
	src := `<odoo>
  <record id="act_analytic" model="ir.actions.act_window">
    <field name="res_model">account.analytic.account</field>
  </record>
</odoo>`
	assertFindings(t, map[string]string{"views/analytic.xml": src}, "18.0", "19.0",
		wantAtLeast{
			count:    1,
			model:    "account.analytic.account",
			kind:     migration.KindModelRenamed,
			severity: migration.SeverityBreaking,
		})
}

func TestScanSource_V18V19_StockPickingMoveLinesRenamed(t *testing.T) {
	// stock.picking.move_lines → move_ids (v18→v19).
	src := `
def validate(self):
    for move in self.picking_id.move_lines:
        move.write({'state': 'done'})
`
	assertFindings(t, map[string]string{"models/stock.py": src}, "18.0", "19.0",
		wantAtLeast{
			count:    1,
			oldName:  "move_lines",
			kind:     migration.KindFieldRenamed,
			severity: migration.SeverityBreaking,
		})
}

func TestScanSource_V18V19_StockPickingMoveLinesXML(t *testing.T) {
	src := `<odoo>
  <record id="view_picking_form" model="ir.ui.view">
    <field name="arch" type="xml">
      <form><field name="move_lines"/></form>
    </field>
  </record>
</odoo>`
	assertFindings(t, map[string]string{"views/stock.xml": src}, "18.0", "19.0",
		wantAtLeast{
			count:    1,
			oldName:  "move_lines",
			kind:     migration.KindFieldRenamed,
			severity: migration.SeverityBreaking,
		})
}

func TestScanSource_V18V19_MailAliasMixinRemoved(t *testing.T) {
	// mail.alias.mixin abstract model removed (v18→v19).
	src := `
class ProjectTask(models.Model):
    _inherit = ['project.task', 'mail.alias.mixin']
`
	assertFindings(t, map[string]string{"models/task.py": src}, "18.0", "19.0",
		wantAtLeast{
			count:    1,
			model:    "mail.alias.mixin",
			kind:     migration.KindModelRemoved,
			severity: migration.SeverityBreaking,
		})
}

func TestScanSource_V18V19_SaleOrderLineProductUpdatable(t *testing.T) {
	// sale.order.line.product_updatable removed (v18→v19).
	src := `
def can_edit(self):
    return self.order_line_id.product_updatable
`
	assertFindings(t, map[string]string{"models/sale.py": src}, "18.0", "19.0",
		wantAtLeast{
			count:    1,
			field:    "product_updatable",
			kind:     migration.KindFieldRemoved,
			severity: migration.SeverityBreaking,
		})
}

func TestScanSource_V18V19_RecNameSearchDeprecated(t *testing.T) {
	// BaseModel._rec_name_search removed (v18→v19).
	src := `
class ResPartner(models.Model):
    _inherit = 'res.partner'

    def _rec_name_search(self, name, args=None, operator='ilike', limit=100):
        return super()._rec_name_search(name, args, operator, limit)
`
	result, err := migration.ScanSource(map[string]string{"models/partner.py": src}, "18.0", "19.0")
	if err != nil {
		t.Fatal(err)
	}
	if result.BreakingCount == 0 {
		t.Fatal("expected breaking finding for _rec_name_search usage")
	}
}

func TestScanSource_V18V19_ReadGroupLazyFalse(t *testing.T) {
	// read_group(lazy=False) removed (v18→v19).
	src := `
data = self.env['sale.order'].read_group(
    domain=[('state', '=', 'sale')],
    fields=['amount_total:sum'],
    groupby=['partner_id', 'date_order:month'],
    lazy=False,
)
`
	result, err := migration.ScanSource(map[string]string{"reports/summary.py": src}, "18.0", "19.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) == 0 {
		t.Fatal("expected finding for read_group(lazy=False)")
	}
}

func TestScanSource_V18V19_HrEmployeeBarcode(t *testing.T) {
	// hr.employee.barcode removed (v18→v19).
	src := `
def get_employee_barcode(self):
    return self.env['hr.employee'].search([('barcode', '=', self.code)])
`
	assertFindings(t, map[string]string{"models/attendance.py": src}, "18.0", "19.0",
		wantAtLeast{
			count:    1,
			field:    "barcode",
			kind:     migration.KindFieldRemoved,
			severity: migration.SeverityBreaking,
		})
}

func TestScanSource_V18V19_ProductSaleLineWarn(t *testing.T) {
	// product.template.sale_line_warn removed (v18→v19).
	src := `
class SaleOrderLine(models.Model):
    _inherit = 'sale.order.line'

    def _check_warning(self):
        warn = self.product_id.sale_line_warn
        msg = self.product_id.sale_line_warn_msg
        return warn, msg
`
	result, err := migration.ScanSource(map[string]string{"models/sale_line.py": src}, "18.0", "19.0")
	if err != nil {
		t.Fatal(err)
	}
	if result.WarningCount == 0 {
		t.Fatal("expected warning findings for sale_line_warn / sale_line_warn_msg")
	}
}

func TestScanSource_V18V19_CrossVersion_16to19(t *testing.T) {
	// A file mixing patterns from v16→v17, v17→v18, and v18→v19 transitions.
	src := `
class BadModule(models.Model):
    _inherit = 'mail.channel'                            # v16→v17 (model renamed)

    def process(self):
        self.env['sale.order'].search([('state', '=', 'draft')], count=True)  # v17→v18 (search count=True)
        for move in self.picking_id.move_lines:          # v18→v19 (field renamed)
            pass
        analytic = self.env['account.analytic.account'].browse(1)  # v18→v19 (model renamed)
`
	result, err := migration.ScanSource(map[string]string{"models/mixed.py": src}, "16.0", "19.0")
	if err != nil {
		t.Fatal(err)
	}
	if result.BreakingCount < 3 {
		t.Fatalf("expected ≥3 breaking findings for 16→19 cross-version scan, got %d", result.BreakingCount)
	}
}

func TestScanSource_V18V19_PathBetween(t *testing.T) {
	// PathBetween must include 18.0→19.0 rules when destination is 19.0.
	rules := migration.PathBetween("18.0", "19.0")
	if len(rules) == 0 {
		t.Fatal("PathBetween(18.0, 19.0) returned no rules")
	}
	found := false
	for _, r := range rules {
		if r.FromVersion == "18.0" && r.ToVersion == "19.0" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("PathBetween(18.0, 19.0) did not include any 18.0→19.0 rules")
	}
}

// ─── Summary counters ─────────────────────────────────────────────────────────

func TestScanSource_SummaryCounters(t *testing.T) {
	src := `
self.env['account.invoice'].search([])    # breaking (model renamed)
`
	result, err := migration.ScanSource(map[string]string{"m.py": src}, "14.0", "15.0")
	if err != nil {
		t.Fatal(err)
	}
	total := result.BreakingCount + result.WarningCount + result.MinorCount
	if total != len(result.Findings) {
		t.Fatalf("summary counts (%d) do not match findings length (%d)", total, len(result.Findings))
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// wantAtLeast describes the minimum expectations for a scan result.
type wantAtLeast struct {
	count    int
	model    string
	field    string
	oldName  string
	kind     string
	severity string
}

func assertFindings(t *testing.T, files map[string]string, from, to string, want wantAtLeast) {
	t.Helper()
	result, err := migration.ScanSource(files, from, to)
	if err != nil {
		t.Fatalf("ScanSource error: %v", err)
	}
	if len(result.Findings) < want.count {
		t.Fatalf("expected ≥%d findings, got %d: %+v", want.count, len(result.Findings), result.Findings)
	}
	for _, f := range result.Findings {
		if want.model != "" && f.Model != want.model {
			continue
		}
		if want.field != "" && f.Field != want.field {
			continue
		}
		if want.oldName != "" && f.OldName != want.oldName {
			continue
		}
		if want.kind != "" && f.Kind != want.kind {
			continue
		}
		if want.severity != "" && f.Severity != want.severity {
			continue
		}
		return // found a matching finding
	}
	t.Fatalf("no finding matched criteria model=%q field=%q oldName=%q kind=%q severity=%q in %+v",
		want.model, want.field, want.oldName, want.kind, want.severity, result.Findings)
}
