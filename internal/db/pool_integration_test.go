//go:build integration

// Integration tests for the real pgx pool. Run with:
//
//	PGPEEK_TEST_DATABASE_URL='postgres://...' go test -tags=integration ./internal/db/
//
// CI provides a Postgres service container and sets the env var.
package db

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func testPool(t *testing.T, rowCap int) *Pool {
	t.Helper()
	dsn := os.Getenv("PGPEEK_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PGPEEK_TEST_DATABASE_URL not set")
	}
	p, err := New(context.Background(), Config{
		DSN:              dsn,
		MaxConns:         4,
		StatementTimeout: 5 * time.Second,
		IdleTxTimeout:    5 * time.Second,
		RowCap:           rowCap,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(p.Close)
	return p
}

func TestIntegrationQuery(t *testing.T) {
	p := testPool(t, 1000)
	res, err := p.Query(context.Background(), "SELECT 1 AS n, 'x'::text AS s")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(res.Columns) != 2 || res.Columns[0] != "n" || res.Columns[1] != "s" {
		t.Fatalf("columns = %v", res.Columns)
	}
	if res.RowCount != 1 || res.Truncated {
		t.Fatalf("rowCount=%d truncated=%v", res.RowCount, res.Truncated)
	}
}

func TestIntegrationRowCap(t *testing.T) {
	p := testPool(t, 5)
	res, err := p.Query(context.Background(), "SELECT g FROM generate_series(1, 100) g")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if res.RowCount != 5 {
		t.Errorf("rowCount = %d, want 5 (capped)", res.RowCount)
	}
	if !res.Truncated {
		t.Error("expected Truncated = true")
	}
}

func TestIntegrationStatementTimeout(t *testing.T) {
	p := testPool(t, 1000)
	_, err := p.Query(context.Background(), "SELECT pg_sleep(10)")
	if err == nil {
		t.Fatal("expected statement timeout error, got nil")
	}
}

func TestIntegrationReadOnlyEnforcedBySession(t *testing.T) {
	// Even if the guard were bypassed, the session is read-only.
	p := testPool(t, 1000)
	_, err := p.Query(context.Background(), "CREATE TEMP TABLE pgpeek_x (id int)")
	if err == nil {
		t.Fatal("expected read-only transaction error, got nil")
	}
}

func TestIntegrationPing(t *testing.T) {
	p := testPool(t, 1000)
	if err := p.Ping(context.Background()); err != nil {
		t.Errorf("Ping: %v", err)
	}
}

func TestIntegrationCatalog(t *testing.T) {
	dsn := os.Getenv("PGPEEK_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PGPEEK_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()

	// Seed with a raw (writable) connection — the pgpeek pool is read-only.
	raw, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("raw connect: %v", err)
	}
	defer raw.Close(ctx)
	for _, q := range []string{
		`DROP TABLE IF EXISTS pgpeek_cat`,
		`CREATE TABLE pgpeek_cat (id serial primary key, name text not null, note text)`,
		`INSERT INTO pgpeek_cat (name, note) VALUES ('a', null), ('b', 'x'), ('c', null)`,
	} {
		if _, err := raw.Exec(ctx, q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	t.Cleanup(func() { _, _ = raw.Exec(ctx, `DROP TABLE IF EXISTS pgpeek_cat`) })

	p := testPool(t, 1000)

	tables, err := p.Tables(ctx)
	if err != nil {
		t.Fatalf("Tables: %v", err)
	}
	found := false
	for _, tb := range tables {
		if tb.Name == "pgpeek_cat" && tb.Type == "table" {
			found = true
		}
	}
	if !found {
		t.Error("pgpeek_cat not listed by Tables")
	}

	cols, err := p.Columns(ctx, "public", "pgpeek_cat")
	if err != nil {
		t.Fatalf("Columns: %v", err)
	}
	if len(cols) != 3 || cols[0].Name != "id" || cols[1].Name != "name" || cols[1].Nullable {
		t.Errorf("columns wrong: %+v", cols)
	}

	page, err := p.TableRows(ctx, TableQuery{Schema: "public", Table: "pgpeek_cat", Limit: 2})
	if err != nil {
		t.Fatalf("TableRows: %v", err)
	}
	if page.RowCount != 2 {
		t.Errorf("page rowCount = %d, want 2", page.RowCount)
	}

	// Offset paging returns the remainder.
	page2, err := p.TableRows(ctx, TableQuery{Schema: "public", Table: "pgpeek_cat", Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("TableRows page2: %v", err)
	}
	if page2.RowCount != 1 {
		t.Errorf("page2 rowCount = %d, want 1", page2.RowCount)
	}
}

func TestIntegrationTableRowsFilterSortSearch(t *testing.T) {
	dsn := os.Getenv("PGPEEK_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PGPEEK_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	raw, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("raw connect: %v", err)
	}
	defer raw.Close(ctx)
	for _, q := range []string{
		`DROP TABLE IF EXISTS pgpeek_fs`,
		`CREATE TABLE pgpeek_fs (id int, name text, note text)`,
		`INSERT INTO pgpeek_fs VALUES (1,'acme',null),(2,'beta','x'),(3,'acme2','y')`,
	} {
		if _, err := raw.Exec(ctx, q); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	t.Cleanup(func() { _, _ = raw.Exec(ctx, `DROP TABLE IF EXISTS pgpeek_fs`) })

	p := testPool(t, 1000)

	// global search across all columns
	res, err := p.TableRows(ctx, TableQuery{Schema: "public", Table: "pgpeek_fs", Search: "acme"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if res.RowCount != 2 {
		t.Errorf("search rowCount = %d, want 2", res.RowCount)
	}

	// per-column filter + sort desc
	res, err = p.TableRows(ctx, TableQuery{
		Schema: "public", Table: "pgpeek_fs",
		Filters: []Filter{{Column: "id", Op: "gte", Value: "2"}, {Column: "note", Op: "is_not_null"}},
		Sort:    "id", Desc: true,
	})
	if err != nil {
		t.Fatalf("filter: %v", err)
	}
	if res.RowCount != 2 {
		t.Fatalf("filter rowCount = %d, want 2", res.RowCount)
	}
	if first := res.Rows[0][0]; first != int32(3) && first != int64(3) {
		t.Errorf("sort desc: first id = %v, want 3", first)
	}
}
