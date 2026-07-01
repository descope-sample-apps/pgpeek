package db

import (
	"context"
	"strings"
	"testing"
)

func TestTableRows_Filters(t *testing.T) {
	fp := &fakePool{rows: dataRows(), colRows: colsFor("id", "name", "deleted_at")}
	p := &Pool{pool: fp, rowCap: 100}
	_, err := p.TableRows(context.Background(), TableQuery{
		Schema: "public", Table: "users",
		Filters: []Filter{
			{Column: "id", Op: "gt", Value: "100"},
			{Column: "name", Op: "ilike", Value: "%a%"},
			{Column: "deleted_at", Op: "is_null"},
			{Column: "name", Op: "is_not_null"},
		},
	})
	if err != nil {
		t.Fatalf("TableRows: %v", err)
	}
	sql := fp.lastSQL
	for _, want := range []string{`"id" > $1`, `"name"::text ILIKE $2`, `"deleted_at" IS NULL`, `"name" IS NOT NULL`} {
		if !strings.Contains(sql, want) {
			t.Errorf("missing %q in %q", want, sql)
		}
	}
	if len(fp.lastArgs) != 2 || fp.lastArgs[0] != "100" || fp.lastArgs[1] != "%a%" {
		t.Errorf("args = %v", fp.lastArgs)
	}
}

func TestTableRows_FilterOperators(t *testing.T) {
	cases := []struct {
		name string
		op   string
		val  string
		want string
		arg  []any
	}{
		{name: "eq", op: "eq", val: "7", want: `"id" = $1`, arg: []any{"7"}},
		{name: "ne", op: "ne", val: "7", want: `"id" <> $1`, arg: []any{"7"}},
		{name: "lt", op: "lt", val: "7", want: `"id" < $1`, arg: []any{"7"}},
		{name: "lte", op: "lte", val: "7", want: `"id" <= $1`, arg: []any{"7"}},
		{name: "gt", op: "gt", val: "7", want: `"id" > $1`, arg: []any{"7"}},
		{name: "gte", op: "gte", val: "7", want: `"id" >= $1`, arg: []any{"7"}},
		{name: "like", op: "like", val: "%7%", want: `"id"::text LIKE $1`, arg: []any{"%7%"}},
		{name: "ilike", op: "ilike", val: "%7%", want: `"id"::text ILIKE $1`, arg: []any{"%7%"}},
		{name: "is_null", op: "is_null", want: `"id" IS NULL`},
		{name: "is_not_null", op: "is_not_null", want: `"id" IS NOT NULL`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fp := &fakePool{rows: dataRows(), colRows: colsFor("id")}
			p := &Pool{pool: fp, rowCap: 100}
			_, err := p.TableRows(context.Background(), TableQuery{
				Schema:  "public",
				Table:   "users",
				Filters: []Filter{{Column: "id", Op: tc.op, Value: tc.val}},
			})
			if err != nil {
				t.Fatalf("TableRows: %v", err)
			}
			if !strings.Contains(fp.lastSQL, tc.want) {
				t.Errorf("sql = %q, want %q", fp.lastSQL, tc.want)
			}
			if len(fp.lastArgs) != len(tc.arg) {
				t.Fatalf("args = %v, want %v", fp.lastArgs, tc.arg)
			}
			for i := range tc.arg {
				if fp.lastArgs[i] != tc.arg[i] {
					t.Errorf("arg %d = %v, want %v", i, fp.lastArgs[i], tc.arg[i])
				}
			}
		})
	}
}

func TestTableRows_SearchAndFiltersCombineWithAnd(t *testing.T) {
	fp := &fakePool{rows: dataRows(), colRows: colsFor("id", "name")}
	p := &Pool{pool: fp, rowCap: 100}
	_, err := p.TableRows(context.Background(), TableQuery{
		Schema:  "public",
		Table:   "users",
		Search:  "acme",
		Filters: []Filter{{Column: "id", Op: "gte", Value: "10"}},
	})
	if err != nil {
		t.Fatalf("TableRows: %v", err)
	}
	for _, want := range []string{`("id"::text ILIKE $1 OR "name"::text ILIKE $1)`, `AND "id" >= $2`} {
		if !strings.Contains(fp.lastSQL, want) {
			t.Errorf("missing %q in %q", want, fp.lastSQL)
		}
	}
	if len(fp.lastArgs) != 2 || fp.lastArgs[0] != "%acme%" || fp.lastArgs[1] != "10" {
		t.Errorf("args = %v", fp.lastArgs)
	}
}
