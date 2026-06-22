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
