//go:build integration

package dev

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/descope-sample-apps/pgpeek/internal/guard"
)

// wantAuditEvents is how many rows seed.sql's generate_series(0, 359) insert
// puts into public.audit_events. Keep it in step with the seed.
const wantAuditEvents = 360

// seedTx applies seed.sql inside a transaction that is rolled back when the
// test ends, so the sample data never outlives it. tz, when set, is applied
// before the seed runs.
func seedTx(ctx context.Context, t *testing.T, tz string) pgx.Tx {
	t.Helper()
	dsn := os.Getenv("PGPEEK_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PGPEEK_TEST_DATABASE_URL not set")
	}
	seed, err := os.ReadFile("seed.sql")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { conn.Close(ctx) })
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rollback: %v", err)
		}
	})
	if tz != "" {
		if _, err := tx.Exec(ctx, `SET LOCAL TIME ZONE '`+tz+`'`); err != nil {
			t.Fatalf("set timezone: %v", err)
		}
	}
	if _, err := tx.Exec(ctx, string(seed)); err != nil {
		t.Fatalf("execute seed: %v", err)
	}
	return tx
}

func TestSeedSupportsWestOfUTCSession(t *testing.T) {
	ctx := context.Background()
	tx := seedTx(ctx, t, "America/Los_Angeles")

	var rows int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM public.audit_events`).Scan(&rows); err != nil {
		t.Fatalf("count audit events: %v", err)
	}
	if rows != wantAuditEvents {
		t.Fatalf("audit event rows = %d, want %d", rows, wantAuditEvents)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM public.visual_edge_cases`).Scan(&rows); err != nil {
		t.Fatalf("count visual edge cases: %v", err)
	}
	if rows != 11 {
		t.Fatalf("visual edge case rows = %d, want 11", rows)
	}
	var latest time.Time
	if err := tx.QueryRow(ctx, `SELECT max(occurred_at) FROM public.audit_events`).Scan(&latest); err != nil {
		t.Fatalf("latest audit event: %v", err)
	}
	want := time.Date(2026, time.December, 20, 0, 0, 0, 0, time.UTC)
	if !latest.Equal(want) {
		t.Fatalf("latest audit event = %s, want %s", latest, want)
	}
}

// EXPLAIN ANALYZE runs the statement it explains, so with a SELECT target it is
// read-only and the guard has to let it through: it is the only way to see real
// timings and row counts for a query against the sample data.
func TestSeedSupportsExplainAnalyze(t *testing.T) {
	const query = `EXPLAIN ANALYZE SELECT event_type, count(*) FROM public.audit_events GROUP BY event_type ORDER BY 2 DESC`
	if err := guard.Validate(query); err != nil {
		t.Fatalf("guard rejected read-only EXPLAIN ANALYZE: %v", err)
	}

	ctx := context.Background()
	tx := seedTx(ctx, t, "")

	rows, err := tx.Query(ctx, query)
	if err != nil {
		t.Fatalf("explain analyze: %v", err)
	}
	var plan []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan plan line: %v", err)
		}
		plan = append(plan, line)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read plan: %v", err)
	}
	out := strings.Join(plan, "\n")
	if !strings.Contains(out, "audit_events") {
		t.Errorf("plan does not mention the seeded table:\n%s", out)
	}
	// "actual time" only appears when the plan was really executed, which is
	// what separates EXPLAIN ANALYZE from plain EXPLAIN.
	if !strings.Contains(out, "actual time=") {
		t.Errorf("plan has no measured timings:\n%s", out)
	}

	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM public.audit_events`).Scan(&count); err != nil {
		t.Fatalf("count audit events: %v", err)
	}
	if count != wantAuditEvents {
		t.Errorf("audit event rows = %d, want %d (EXPLAIN ANALYZE must not change data)", count, wantAuditEvents)
	}
}
