package hook

import (
	"testing"
	"time"
)

func TestIsORMQueryEntry(t *testing.T) {
	tests := []struct {
		name   string
		logger string
		want   bool
	}{
		{"models.query", "odoo.models.query", true},
		{"sql_db", "odoo.sql_db", true},
		{"error logger", "odoo.addons.sale.models.sale_order", false},
		{"http logger", "odoo.http", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &LogEntry{Logger: tt.logger}
			if got := IsORMQueryEntry(entry); got != tt.want {
				t.Errorf("IsORMQueryEntry(%q) = %v, want %v", tt.logger, got, tt.want)
			}
		})
	}
}

func TestToORMEvent_ModelsQuery(t *testing.T) {
	entry := &LogEntry{
		Timestamp: time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC),
		Logger:    "odoo.models.query",
		Message:   `4.2ms SELECT "res_partner"."id" FROM "res_partner" WHERE "res_partner"."active" = true`,
	}

	ev, ok := ToORMEvent(entry)
	if !ok {
		t.Fatal("expected ok=true")
	}

	if ev.Category != "orm" {
		t.Errorf("Category = %q, want %q", ev.Category, "orm")
	}
	if ev.DurationMS != 4 {
		t.Errorf("DurationMS = %d, want 4", ev.DurationMS)
	}
	if ev.Model != "res.partner" {
		t.Errorf("Model = %q, want %q", ev.Model, "res.partner")
	}
	if ev.Method != "read" {
		t.Errorf("Method = %q, want %q", ev.Method, "read")
	}
	if ev.SQL == "" {
		t.Error("SQL should not be empty")
	}
}

func TestToORMEvent_SqlDb(t *testing.T) {
	entry := &LogEntry{
		Timestamp: time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC),
		Logger:    "odoo.sql_db",
		Message:   `query: INSERT INTO "sale_order" ("name","partner_id") VALUES ('SO001',1)`,
	}

	ev, ok := ToORMEvent(entry)
	if !ok {
		t.Fatal("expected ok=true")
	}

	if ev.DurationMS != 0 {
		t.Errorf("DurationMS = %d, want 0 (sql_db has no duration)", ev.DurationMS)
	}
	if ev.Model != "sale.order" {
		t.Errorf("Model = %q, want %q", ev.Model, "sale.order")
	}
	if ev.Method != "create" {
		t.Errorf("Method = %q, want %q", ev.Method, "create")
	}
}

func TestToORMEvent_MultiLineSQL(t *testing.T) {
	entry := &LogEntry{
		Timestamp:  time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC),
		Logger:     "odoo.models.query",
		Message:    `12ms SELECT "product_product"."id"`,
		Contiguous: `FROM "product_product"` + "\n" + `WHERE "product_product"."active" = true`,
	}

	ev, ok := ToORMEvent(entry)
	if !ok {
		t.Fatal("expected ok=true")
	}

	if ev.Model != "product.product" {
		t.Errorf("Model = %q, want %q", ev.Model, "product.product")
	}
	if ev.DurationMS != 12 {
		t.Errorf("DurationMS = %d, want 12", ev.DurationMS)
	}
}

func TestToORMEvent_NotORM(t *testing.T) {
	entry := &LogEntry{
		Logger:  "odoo.http",
		Message: "some http log",
	}

	_, ok := ToORMEvent(entry)
	if ok {
		t.Error("expected ok=false for non-ORM entry")
	}
}

func TestToORMEvent_UnparseableMessage(t *testing.T) {
	entry := &LogEntry{
		Logger:  "odoo.models.query",
		Message: "not a query line at all",
	}

	_, ok := ToORMEvent(entry)
	if ok {
		t.Error("expected ok=false for unparseable message")
	}
}

func TestTableToModel(t *testing.T) {
	tests := []struct {
		table string
		want  string
	}{
		{"res_partner", "res.partner"},
		{"sale_order_line", "sale.order.line"},
		{"ir_model_fields", "ir.model.fields"},
		{"product_product", "product.product"},
		{"account_move", "account.move"},
		{"mail_message", "mail.message"},
	}

	for _, tt := range tests {
		t.Run(tt.table, func(t *testing.T) {
			if got := tableToModel(tt.table); got != tt.want {
				t.Errorf("tableToModel(%q) = %q, want %q", tt.table, got, tt.want)
			}
		})
	}
}

func TestSqlVerbToMethod(t *testing.T) {
	tests := []struct {
		sql  string
		want string
	}{
		{`SELECT "id" FROM "res_partner"`, "read"},
		{`select "id" from "res_partner"`, "read"},
		{`INSERT INTO "sale_order" ("name") VALUES ('SO001')`, "create"},
		{`UPDATE "res_partner" SET "name" = 'Test'`, "write"},
		{`DELETE FROM "res_partner" WHERE id = 1`, "unlink"},
		{`SAVEPOINT`, "unknown"},
		{``, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := sqlVerbToMethod(tt.sql); got != tt.want {
				t.Errorf("sqlVerbToMethod(%q) = %q, want %q", tt.sql, got, tt.want)
			}
		})
	}
}

func TestModelFromSQL(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want string
	}{
		{"select", `SELECT "res_partner"."id" FROM "res_partner" WHERE id = 1`, "res.partner"},
		{"insert", `INSERT INTO "sale_order" ("name") VALUES ('SO001')`, "sale.order"},
		{"update", `UPDATE "product_product" SET "active" = false`, "product.product"},
		{"delete", `DELETE FROM "account_move" WHERE id = 1`, "account.move"},
		{"no table", `SAVEPOINT sp1`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := modelFromSQL(tt.sql); got != tt.want {
				t.Errorf("modelFromSQL(%q) = %q, want %q", tt.sql, got, tt.want)
			}
		})
	}
}
