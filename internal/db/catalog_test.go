package db

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func strptr(s string) *string { return &s }

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
	rows := &fakeRows{
		cols: []string{"name", "type", "nullable", "default"},
		data: [][]any{
			{"id", "integer", false, strptr("nextval('s')")},
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

func TestTableRows_BuildsSanitizedSQLAndClamps(t *testing.T) {
	fp := &fakePool{rows: &fakeRows{cols: []string{"n"}, data: [][]any{{int64(1)}}}}
	p := &Pool{pool: fp, rowCap: 100}

	// limit over cap and negative offset get clamped.
	res, err := p.TableRows(context.Background(), "pub lic", "od\"d", 0, -5)
	if err != nil {
		t.Fatalf("TableRows: %v", err)
	}
	if res.RowCount != 1 {
		t.Errorf("rowCount = %d", res.RowCount)
	}
	sql := fp.lastSQL
	if !strings.Contains(sql, `"pub lic"."od""d"`) {
		t.Errorf("identifier not sanitized: %q", sql)
	}
	if !strings.Contains(sql, "LIMIT 100") || !strings.Contains(sql, "OFFSET 0") {
		t.Errorf("clamp wrong: %q", sql)
	}
}

func TestTableRows_HonorsLimitOffset(t *testing.T) {
	fp := &fakePool{rows: &fakeRows{cols: []string{"n"}, data: [][]any{{int64(1)}}}}
	p := &Pool{pool: fp, rowCap: 100}
	if _, err := p.TableRows(context.Background(), "public", "users", 25, 50); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fp.lastSQL, "LIMIT 25") || !strings.Contains(fp.lastSQL, "OFFSET 50") {
		t.Errorf("sql = %q", fp.lastSQL)
	}
}

func TestTableRows_QueryError(t *testing.T) {
	p := &Pool{pool: &fakePool{queryErr: errors.New("boom")}, rowCap: 10}
	if _, err := p.TableRows(context.Background(), "s", "t", 10, 0); err == nil {
		t.Fatal("expected query error")
	}
}
