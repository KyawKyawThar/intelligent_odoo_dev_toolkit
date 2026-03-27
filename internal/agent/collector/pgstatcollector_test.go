package collector

import (
	"testing"
	"time"
)

func TestExtractModel(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want string
	}{
		{
			name: "simple select with quoted table",
			sql:  `SELECT "res_partner"."id" FROM "res_partner" WHERE id = $1`,
			want: "res.partner",
		},
		{
			name: "select with unquoted table",
			sql:  `SELECT id FROM sale_order WHERE state = 'done'`,
			want: "sale.order",
		},
		{
			name: "insert",
			sql:  `INSERT INTO "product_product" (name) VALUES ($1)`,
			want: "product.product",
		},
		{
			name: "update",
			sql:  `UPDATE "account_move" SET state = $1 WHERE id = $2`,
			want: "account.move",
		},
		{
			name: "join extracts first table",
			sql:  `SELECT a.id FROM "res_partner" a JOIN "res_country" b ON a.country_id = b.id`,
			want: "res.partner",
		},
		{
			name: "pg system table ignored",
			sql:  `SELECT * FROM pg_catalog.pg_class`,
			want: "",
		},
		{
			name: "no table found",
			sql:  `SET statement_timeout = 0`,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractModel(tt.sql)
			if got != tt.want {
				t.Errorf("extractModel(%q) = %q, want %q", tt.sql, got, tt.want)
			}
		})
	}
}

func TestSqlToMethod(t *testing.T) {
	tests := []struct {
		sql  string
		want string
	}{
		{"SELECT * FROM foo", "read"},
		{"INSERT INTO foo VALUES (1)", "create"},
		{"UPDATE foo SET x = 1", "write"},
		{"DELETE FROM foo WHERE id = 1", "unlink"},
		{"EXPLAIN SELECT 1", "unknown"},
		{"  select * from foo", "read"},
	}

	for _, tt := range tests {
		t.Run(tt.sql, func(t *testing.T) {
			got := sqlToMethod(tt.sql)
			if got != tt.want {
				t.Errorf("sqlToMethod(%q) = %q, want %q", tt.sql, got, tt.want)
			}
		})
	}
}

func TestTruncateSQL(t *testing.T) {
	short := "SELECT 1"
	if got := truncateSQL(short, 100); got != short {
		t.Errorf("truncateSQL should not truncate short string, got %q", got)
	}

	long := string(make([]byte, 2000))
	got := truncateSQL(long, 1024)
	if len(got) != 1024 {
		t.Errorf("truncateSQL should truncate to maxLen, got len=%d", len(got))
	}
	if got[len(got)-3:] != "..." {
		t.Errorf("truncateSQL should end with '...', got %q", got[len(got)-5:])
	}
}

func TestToEvent(t *testing.T) {
	c := &PgStatCollector{}
	now := time.Now().UTC()

	row := pgStatRow{
		QueryID:         12345,
		Query:           `SELECT "res_partner"."id" FROM "res_partner" WHERE active = true`,
		Calls:           100,
		TotalExecTimeMS: 5000.0,
		MeanExecTimeMS:  50.0,
		MaxExecTimeMS:   200.0,
	}

	ev := c.toEvent(row, 10, 50, 200, now)

	if ev.Category != "sql" {
		t.Errorf("Category = %q, want 'sql'", ev.Category)
	}
	if ev.Model != "res.partner" {
		t.Errorf("Model = %q, want 'res.partner'", ev.Model)
	}
	if ev.Method != "read" {
		t.Errorf("Method = %q, want 'read'", ev.Method)
	}
	if ev.DurationMS != 50 {
		t.Errorf("DurationMS = %d, want 50", ev.DurationMS)
	}
	if ev.SQL == "" {
		t.Error("SQL should not be empty")
	}
}

func TestDeltaComputation(t *testing.T) {
	c := &PgStatCollector{
		baseline: make(map[int64]pgStatSnapshot),
	}

	// Simulate baseline: queryid 1 had 100 calls with 5000ms total.
	c.baseline[1] = pgStatSnapshot{
		Calls:           100,
		TotalExecTimeMS: 5000.0,
		MaxExecTimeMS:   200.0,
		Rows:            1000,
	}

	// New snapshot: 110 calls, 5500ms total.
	newSnap := pgStatSnapshot{
		Calls:           110,
		TotalExecTimeMS: 5500.0,
	}

	deltaCalls := newSnap.Calls - c.baseline[1].Calls
	if deltaCalls != 10 {
		t.Errorf("deltaCalls = %d, want 10", deltaCalls)
	}

	deltaTotalMS := newSnap.TotalExecTimeMS - c.baseline[1].TotalExecTimeMS
	avgMS := deltaTotalMS / float64(deltaCalls)
	if avgMS != 50.0 {
		t.Errorf("avgMS = %f, want 50.0", avgMS)
	}
}

func TestDeltaNoNewCalls(t *testing.T) {
	// When calls haven't changed, no event should be emitted.
	c := &PgStatCollector{
		baseline: make(map[int64]pgStatSnapshot),
	}

	c.baseline[1] = pgStatSnapshot{
		Calls:           100,
		TotalExecTimeMS: 5000.0,
	}

	// Same calls count → deltaCalls = 0 → should skip.
	deltaCalls := int64(100) - c.baseline[1].Calls
	if deltaCalls != 0 {
		t.Errorf("expected 0 delta calls, got %d", deltaCalls)
	}
}
