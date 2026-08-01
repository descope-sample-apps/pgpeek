//go:build integration

package dev

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestSeedSupportsWestOfUTCSession(t *testing.T) {
	dsn := os.Getenv("PGPEEK_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PGPEEK_TEST_DATABASE_URL not set")
	}
	seed, err := os.ReadFile("seed.sql")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rollback: %v", err)
		}
	}()

	if _, err := tx.Exec(ctx, `SET LOCAL TIME ZONE 'America/Los_Angeles'`); err != nil {
		t.Fatalf("set timezone: %v", err)
	}
	if _, err := tx.Exec(ctx, string(seed)); err != nil {
		t.Fatalf("execute seed: %v", err)
	}
	var rows int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM public.audit_events`).Scan(&rows); err != nil {
		t.Fatalf("count audit events: %v", err)
	}
	if rows != 360 {
		t.Fatalf("audit event rows = %d, want 360", rows)
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
