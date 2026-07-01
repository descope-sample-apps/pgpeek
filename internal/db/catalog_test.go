package db

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestTables_Success(t *testing.T) {
	rows := &fakeRows{
		cols: []string{"schema", "name", "type", "est"},
		data: [][]any{
			{"public", "users", "table", int64(42)},
			{"public", "v_active", "view", int64(0)},
		},
	}
	p := &Pool{pool: &fakePool{rows: rows}, rowCap: 10}
	got, err := p.Tables(context.Background())
	if err != nil {
		t.Fatalf("Tables: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0] != (TableInfo{Schema: "public", Name: "users", Type: "table", EstRows: 42}) {
		t.Errorf("row0 = %+v", got[0])
	}
	if got[1].Type != "view" {
		t.Errorf("row1 type = %q", got[1].Type)
	}
}

func TestTables_QueryError(t *testing.T) {
	p := &Pool{pool: &fakePool{queryErr: errors.New("boom")}, rowCap: 10}
	if _, err := p.Tables(context.Background()); err == nil {
		t.Fatal("expected query error")
	}
}

func TestTables_ScanError(t *testing.T) {
	rows := &fakeRows{cols: []string{"a"}, data: [][]any{{int64(1)}}, scanErr: errors.New("scan")}
	p := &Pool{pool: &fakePool{rows: rows}, rowCap: 10}
	if _, err := p.Tables(context.Background()); err == nil {
		t.Fatal("expected scan error")
	}
}

func TestTables_RowsErr(t *testing.T) {
	rows := &fakeRows{cols: []string{"a"}, errErr: errors.New("cursor")}
	p := &Pool{pool: &fakePool{rows: rows}, rowCap: 10}
	if _, err := p.Tables(context.Background()); err == nil {
		t.Fatal("expected rows.Err")
	}
}

func TestColumns_Success(t *testing.T) {
	defaultValue := "nextval('s')"
	rows := &fakeRows{
		cols: []string{"name", "type", "nullable", "default"},
		data: [][]any{
			{"id", "integer", false, &defaultValue},
			{"email", "text", true, (*string)(nil)},
		},
	}
	p := &Pool{pool: &fakePool{rows: rows}, rowCap: 10}
	got, err := p.Columns(context.Background(), "public", "users")
	if err != nil {
		t.Fatalf("Columns: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Name != "id" || got[0].Nullable || got[0].Default == nil {
		t.Errorf("col0 = %+v", got[0])
	}
	if !got[1].Nullable || got[1].Default != nil {
		t.Errorf("col1 = %+v (default should be nil)", got[1])
	}
}

func TestColumns_QueryError(t *testing.T) {
	p := &Pool{pool: &fakePool{queryErr: errors.New("boom")}, rowCap: 10}
	if _, err := p.Columns(context.Background(), "s", "t"); err == nil {
		t.Fatal("expected query error")
	}
}

func TestColumns_ScanError(t *testing.T) {
	rows := &fakeRows{cols: []string{"a"}, data: [][]any{{"x"}}, scanErr: errors.New("scan")}
	p := &Pool{pool: &fakePool{rows: rows}, rowCap: 10}
	if _, err := p.Columns(context.Background(), "s", "t"); err == nil {
		t.Fatal("expected scan error")
	}
}

func TestColumns_RowsErr(t *testing.T) {
	rows := &fakeRows{cols: []string{"a"}, errErr: errors.New("cursor")}
	p := &Pool{pool: &fakePool{rows: rows}, rowCap: 10}
	if _, err := p.Columns(context.Background(), "s", "t"); err == nil {
		t.Fatal("expected rows.Err")
	}
}

// colsFor builds a fakeRows usable as the information_schema.columns validation
// result for the given column names.
func colsFor(cols ...string) *fakeRows {
	data := make([][]any, len(cols))
	for i, c := range cols {
		data[i] = []any{c, "text", true, (*string)(nil)}
	}
	return &fakeRows{cols: []string{"name", "type", "nullable", "default"}, data: data}
}

func dataRows() *fakeRows {
	return &fakeRows{cols: []string{"id"}, data: [][]any{{int64(1)}}}
}

func TestForeignKeys_Success(t *testing.T) {
	rows := &fakeRows{
		cols: []string{"col", "rs", "rt", "rc"},
		data: [][]any{
			{"company_id", "public", "companies", "id"},
			{"manager_id", "public", "users", "id"},
		},
	}
	p := &Pool{pool: &fakePool{rows: rows}, rowCap: 10}
	got, err := p.ForeignKeys(context.Background(), "public", "users")
	if err != nil {
		t.Fatalf("ForeignKeys: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0] != (ForeignKey{Column: "company_id", RefSchema: "public", RefTable: "companies", RefColumn: "id"}) {
		t.Errorf("fk0 = %+v", got[0])
	}
}

func TestForeignKeys_QueryError(t *testing.T) {
	p := &Pool{pool: &fakePool{queryErr: errors.New("boom")}, rowCap: 10}
	if _, err := p.ForeignKeys(context.Background(), "s", "t"); err == nil {
		t.Fatal("expected query error")
	}
}

func TestForeignKeys_ScanError(t *testing.T) {
	rows := &fakeRows{cols: []string{"col"}, data: [][]any{{"x"}}, scanErr: errors.New("scan")}
	p := &Pool{pool: &fakePool{rows: rows}, rowCap: 10}
	if _, err := p.ForeignKeys(context.Background(), "s", "t"); err == nil {
		t.Fatal("expected scan error")
	}
}

func TestForeignKeys_RowsErr(t *testing.T) {
	rows := &fakeRows{cols: []string{"col"}, errErr: errors.New("cursor")}
	p := &Pool{pool: &fakePool{rows: rows}, rowCap: 10}
	if _, err := p.ForeignKeys(context.Background(), "s", "t"); err == nil {
		t.Fatal("expected rows.Err")
	}
}

func TestTableRows_BuildsSanitizedSQLAndClamps(t *testing.T) {
	fp := &fakePool{rows: dataRows()}
	p := &Pool{pool: fp, rowCap: 100}

	// limit over cap and negative offset get clamped; no filters -> no col query.
	res, err := p.TableRows(context.Background(), TableQuery{Schema: "pub lic", Table: `od"d`, Limit: 0, Offset: -5})
	if err != nil {
		t.Fatalf("TableRows: %v", err)
	}
	if res.RowCount != 1 {
		t.Errorf("rowCount = %d", res.RowCount)
	}
	if !strings.Contains(fp.lastSQL, `"pub lic"."od""d"`) {
		t.Errorf("identifier not sanitized: %q", fp.lastSQL)
	}
	if !strings.Contains(fp.lastSQL, "LIMIT 100") || !strings.Contains(fp.lastSQL, "OFFSET 0") {
		t.Errorf("clamp wrong: %q", fp.lastSQL)
	}
}

func TestTableRows_HonorsLimitOffset(t *testing.T) {
	fp := &fakePool{rows: dataRows()}
	p := &Pool{pool: fp, rowCap: 100}
	if _, err := p.TableRows(context.Background(), TableQuery{Schema: "public", Table: "users", Limit: 25, Offset: 50}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fp.lastSQL, "LIMIT 25") || !strings.Contains(fp.lastSQL, "OFFSET 50") {
		t.Errorf("sql = %q", fp.lastSQL)
	}
}

func TestTableRows_QueryError(t *testing.T) {
	p := &Pool{pool: &fakePool{queryErr: errors.New("boom")}, rowCap: 10}
	if _, err := p.TableRows(context.Background(), TableQuery{Schema: "s", Table: "t", Limit: 10}); err == nil {
		t.Fatal("expected query error")
	}
}

func TestTableRows_Search(t *testing.T) {
	fp := &fakePool{rows: dataRows(), colRows: colsFor("id", "email")}
	p := &Pool{pool: fp, rowCap: 100}
	if _, err := p.TableRows(context.Background(), TableQuery{Schema: "public", Table: "users", Search: "acme"}); err != nil {
		t.Fatal(err)
	}
	sql := fp.lastSQL
	if !strings.Contains(sql, `"id"::text ILIKE $1`) || !strings.Contains(sql, `"email"::text ILIKE $1`) {
		t.Errorf("search SQL wrong: %q", sql)
	}
	if !strings.Contains(sql, " OR ") || !strings.Contains(sql, "WHERE (") {
		t.Errorf("search not OR-combined: %q", sql)
	}
	if len(fp.lastArgs) != 1 || fp.lastArgs[0] != "%acme%" {
		t.Errorf("search arg = %v", fp.lastArgs)
	}
}

func TestTableRows_SortAscDesc(t *testing.T) {
	for _, tc := range []struct {
		desc bool
		want string
	}{{false, `ORDER BY "id" ASC`}, {true, `ORDER BY "id" DESC`}} {
		fp := &fakePool{rows: dataRows(), colRows: colsFor("id")}
		p := &Pool{pool: fp, rowCap: 100}
		if _, err := p.TableRows(context.Background(), TableQuery{Schema: "s", Table: "t", Sort: "id", Desc: tc.desc}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(fp.lastSQL, tc.want) {
			t.Errorf("sql = %q, want %q", fp.lastSQL, tc.want)
		}
	}
}

func TestTableRows_RejectsUnknownColumnsAndOps(t *testing.T) {
	mk := func() *Pool { return &Pool{pool: &fakePool{rows: dataRows(), colRows: colsFor("id")}, rowCap: 100} }
	base := TableQuery{Schema: "s", Table: "t"}

	cases := map[string]TableQuery{
		"unknown filter column": {Schema: base.Schema, Table: base.Table, Filters: []Filter{{Column: "nope", Op: "eq", Value: "1"}}},
		"unknown operator":      {Schema: base.Schema, Table: base.Table, Filters: []Filter{{Column: "id", Op: "evil", Value: "1"}}},
		"unknown sort column":   {Schema: base.Schema, Table: base.Table, Sort: "nope"},
	}
	for name, q := range cases {
		if _, err := mk().TableRows(context.Background(), q); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestTableRows_ColumnsLookupError(t *testing.T) {
	// queryErr makes the validation (Columns) query fail when filters are present.
	p := &Pool{pool: &fakePool{queryErr: errors.New("boom")}, rowCap: 100}
	if _, err := p.TableRows(context.Background(), TableQuery{Schema: "s", Table: "t", Sort: "id"}); err == nil {
		t.Fatal("expected columns lookup error")
	}
}

func TestTableRows_NoColumns(t *testing.T) {
	// Validation query returns zero columns -> error.
	fp := &fakePool{rows: dataRows(), colRows: &fakeRows{cols: []string{"name", "type", "nullable", "default"}}}
	p := &Pool{pool: fp, rowCap: 100}
	if _, err := p.TableRows(context.Background(), TableQuery{Schema: "s", Table: "t", Search: "x"}); err == nil {
		t.Fatal("expected no-columns error")
	}
}
