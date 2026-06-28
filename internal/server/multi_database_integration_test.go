//go:build integration

package server

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/descope-sample-apps/pgpeek/internal/db"
	"github.com/descope-sample-apps/pgpeek/internal/store"
)

func TestIntegrationMultiDatabaseHTTPSelection(t *testing.T) {
	baseDSN := os.Getenv("PGPEEK_TEST_DATABASE_URL")
	if baseDSN == "" {
		t.Skip("PGPEEK_TEST_DATABASE_URL not set")
	}

	ctx := context.Background()
	maintenance, err := pgx.Connect(ctx, baseDSN)
	if err != nil {
		t.Fatalf("maintenance connect: %v", err)
	}
	defer maintenance.Close(ctx)

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	leftName := "pgpeek_multi_left_" + suffix
	rightName := "pgpeek_multi_right_" + suffix
	createDatabase(t, maintenance, leftName)
	createDatabase(t, maintenance, rightName)
	t.Cleanup(func() {
		dropDatabase(t, maintenance, leftName)
		dropDatabase(t, maintenance, rightName)
	})

	leftDSN := databaseDSN(t, baseDSN, leftName)
	rightDSN := databaseDSN(t, baseDSN, rightName)
	seedMarker(t, leftDSN, "left")
	seedMarker(t, rightDSN, "right")

	leftPool := newIntegrationPool(t, leftDSN)
	rightPool := newIntegrationPool(t, rightDSN)
	registry, err := db.NewRegistry([]db.RegistryEntry{
		{ID: "left", Name: "Left", Pool: leftPool, Default: true},
		{ID: "right", Name: "Right", Pool: rightPool},
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	t.Cleanup(registry.Close)

	st, err := store.Open(t.TempDir() + "/queries.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	srv := NewWithRegistry(
		NewDatabaseRegistry(registry),
		st,
		fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("pgpeek")}},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		5*time.Second,
	)
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	left := runMarkerQuery(t, ts, "left")
	right := runMarkerQuery(t, ts, "right")

	if left != "left" || right != "right" {
		t.Fatalf("markers = %q/%q, want left/right", left, right)
	}
}

func createDatabase(t *testing.T, conn *pgx.Conn, name string) {
	t.Helper()
	if _, err := conn.Exec(context.Background(), "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create database %s: %v", name, err)
	}
}

func dropDatabase(t *testing.T, conn *pgx.Conn, name string) {
	t.Helper()
	_, _ = conn.Exec(context.Background(), "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
}

func databaseDSN(t *testing.T, baseDSN, dbName string) string {
	t.Helper()
	u, err := url.Parse(baseDSN)
	if err != nil || u.Scheme == "" || u.Host == "" {
		t.Skip("PGPEEK_TEST_DATABASE_URL must be a postgres URL for multi-database integration")
	}
	u.Path = "/" + dbName
	return u.String()
}

func seedMarker(t *testing.T, dsn, marker string) {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("seed connect: %v", err)
	}
	defer conn.Close(context.Background())
	if _, err := conn.Exec(context.Background(), `CREATE TABLE pgpeek_marker (value text not null)`); err != nil {
		t.Fatalf("create marker table: %v", err)
	}
	if _, err := conn.Exec(context.Background(), `INSERT INTO pgpeek_marker VALUES ($1)`, marker); err != nil {
		t.Fatalf("insert marker: %v", err)
	}
}

func newIntegrationPool(t *testing.T, dsn string) *db.Pool {
	t.Helper()
	pool, err := db.New(context.Background(), db.Config{
		DSN:              dsn,
		MaxConns:         2,
		StatementTimeout: 5 * time.Second,
		IdleTxTimeout:    5 * time.Second,
		RowCap:           10,
	})
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	return pool
}

func runMarkerQuery(t *testing.T, ts *httptest.Server, id string) string {
	t.Helper()
	resp := post(t, ts, "/api/query?db="+id, `{"sql":"SELECT value FROM pgpeek_marker"}`)
	if resp.StatusCode != 200 {
		t.Fatalf("query %s status = %d", id, resp.StatusCode)
	}
	result := decode[db.Result](t, resp)
	if result.RowCount != 1 {
		t.Fatalf("query %s rowCount = %d, want 1", id, result.RowCount)
	}
	marker, ok := result.Rows[0][0].(string)
	if !ok {
		t.Fatalf("query %s marker type = %T", id, result.Rows[0][0])
	}
	return marker
}
