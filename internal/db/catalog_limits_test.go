package db

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCatalogQueries_ReportEncodingError(t *testing.T) {
	original := marshalCatalogItem
	marshalCatalogItem = func(any) ([]byte, error) { return nil, errors.New("encode") }
	t.Cleanup(func() { marshalCatalogItem = original })

	tests := []struct {
		name string
		run  func() error
	}{
		{"schema", func() error {
			p := &Pool{pool: &fakePool{rows: &fakeRows{data: [][]any{{"public", "users", "id"}}}}, rowCap: 10}
			_, _, err := p.SchemaCatalog(context.Background())
			return err
		}},
		{"tables", func() error {
			p := &Pool{pool: &fakePool{rows: &fakeRows{data: [][]any{{"public", "users", "table", int64(1), false, "", ""}}}}, rowCap: 10}
			_, _, err := p.Tables(context.Background())
			return err
		}},
		{"columns", func() error {
			p := &Pool{pool: &fakePool{rows: &fakeRows{data: [][]any{{"id", "integer", false, (*string)(nil)}}}}, rowCap: 10}
			_, _, err := p.Columns(context.Background(), "public", "users")
			return err
		}},
		{"foreign keys", func() error {
			p := &Pool{pool: &fakePool{rows: &fakeRows{data: [][]any{{"account_id", "public", "accounts", "id"}}}}, rowCap: 10}
			_, _, err := p.ForeignKeys(context.Background(), "public", "users")
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err == nil {
				t.Fatal("expected encoding error")
			}
		})
	}
}

func TestTables_ByteCap(t *testing.T) {
	large := strings.Repeat("x", MaxCatalogBytes/2)
	rows := &fakeRows{data: [][]any{{"public", large, "table", int64(1), false, "", ""}, {"public", large, "table", int64(2), false, "", ""}, {"public", large, "table", int64(3), false, "", ""}}}
	p := &Pool{pool: &fakePool{rows: rows}, rowCap: 10}
	got, truncated, err := p.Tables(context.Background())
	if err != nil {
		t.Fatalf("Tables: %v", err)
	}
	if !truncated || len(got) >= 3 {
		t.Fatalf("tables=%d truncated=%v", len(got), truncated)
	}
}

func TestColumns_ByteCap(t *testing.T) {
	large := strings.Repeat("x", MaxCatalogBytes/2)
	rows := &fakeRows{data: [][]any{{large, "text", true, (*string)(nil)}, {large, "text", true, (*string)(nil)}, {large, "text", true, (*string)(nil)}}}
	p := &Pool{pool: &fakePool{rows: rows}, rowCap: 10}
	got, truncated, err := p.Columns(context.Background(), "public", "users")
	if err != nil {
		t.Fatalf("Columns: %v", err)
	}
	if !truncated || len(got) >= 3 {
		t.Fatalf("columns=%d truncated=%v", len(got), truncated)
	}
}

func TestForeignKeys_ByteCap(t *testing.T) {
	large := strings.Repeat("x", MaxCatalogBytes/2)
	rows := &fakeRows{data: [][]any{{large, "public", "users", "id"}, {large, "public", "users", "id"}, {large, "public", "users", "id"}}}
	p := &Pool{pool: &fakePool{rows: rows}, rowCap: 10}
	got, truncated, err := p.ForeignKeys(context.Background(), "public", "users")
	if err != nil {
		t.Fatalf("ForeignKeys: %v", err)
	}
	if !truncated || len(got) >= 3 {
		t.Fatalf("foreignKeys=%d truncated=%v", len(got), truncated)
	}
}

func TestTableRows_RejectsTruncatedColumnCatalog(t *testing.T) {
	large := strings.Repeat("x", MaxCatalogBytes/2)
	cols := &fakeRows{data: [][]any{{large, "text", true, (*string)(nil)}, {large, "text", true, (*string)(nil)}, {large, "text", true, (*string)(nil)}}}
	p := &Pool{pool: &fakePool{rows: dataRows(), colRows: cols}, rowCap: 10}
	_, err := p.TableRows(context.Background(), TableQuery{Schema: "public", Table: "users", Search: "x"})
	if err == nil || !strings.Contains(err.Error(), "column catalog exceeds") {
		t.Fatalf("error = %v", err)
	}
}
