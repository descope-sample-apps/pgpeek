package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/descope-sample-apps/pgpeek/internal/db"
	"github.com/descope-sample-apps/pgpeek/internal/store"
)

type fakeRegistry struct {
	defaultID string
	metadata  []db.PoolMetadata
	pools     map[string]Querier
}

func (f fakeRegistry) List() []db.PoolMetadata { return f.metadata }
func (f fakeRegistry) DefaultID() string       { return f.defaultID }

func (f fakeRegistry) Pool(id string) (Querier, error) {
	if id == "" {
		id = f.defaultID
	}
	pool, ok := f.pools[id]
	if !ok {
		return nil, db.ErrPoolNotFound
	}
	return pool, nil
}

func (f fakeRegistry) Ping(ctx context.Context) error {
	for _, pool := range f.pools {
		if err := pool.Ping(ctx); err != nil {
			return err
		}
	}
	return nil
}

type selectedQuerier struct {
	rowCap   int
	maxConns int32
	used     bool
	pinged   bool
	result   *db.Result
	err      error
}

func (q *selectedQuerier) Query(context.Context, string) (*db.Result, error) {
	q.used = true
	if q.result != nil || q.err != nil {
		return q.result, q.err
	}
	return okResult(), nil
}

func (q *selectedQuerier) Count(context.Context, string) (int64, error) {
	q.used = true
	return 1, q.err
}

func (q *selectedQuerier) ExportCSV(_ context.Context, _ string, dst io.Writer) error {
	q.used = true
	if q.err != nil {
		return q.err
	}
	_, err := io.WriteString(dst, "n\n1\n")
	return err
}

func (q *selectedQuerier) QueryCell(context.Context, string, int, int) (any, error) {
	q.used = true
	return "cell", nil
}

func (q *selectedQuerier) TableCell(context.Context, db.TableQuery, int, int) (any, error) {
	q.used = true
	return "cell", nil
}

func (q *selectedQuerier) Tables(context.Context) ([]db.TableInfo, bool, error) {
	q.used = true
	return []db.TableInfo{}, false, nil
}

func (q *selectedQuerier) Columns(context.Context, string, string) ([]db.ColumnInfo, bool, error) {
	q.used = true
	return []db.ColumnInfo{}, false, nil
}

func (q *selectedQuerier) ForeignKeys(context.Context, string, string) ([]db.ForeignKey, bool, error) {
	q.used = true
	return []db.ForeignKey{}, false, nil
}

func (q *selectedQuerier) TableRows(context.Context, db.TableQuery) (*db.Result, error) {
	q.used = true
	return okResult(), nil
}

func (q *selectedQuerier) RowCap() int {
	q.used = true
	return q.rowCap
}

func (q *selectedQuerier) MaxConns() int32 { return q.maxConns }

func (q *selectedQuerier) Ping(context.Context) error {
	q.pinged = true
	return nil
}

func newRegistryTestServer(t *testing.T, registry DatabaseRegistry, opts ...Option) *httptest.Server {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	web := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("x")}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewWithRegistry(registry, st, web, log, time.Second, opts...)
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	return ts
}

func TestDatabases_lists_safe_metadata(t *testing.T) {
	primary := &selectedQuerier{rowCap: 1000, maxConns: 8, result: &db.Result{Rows: [][]any{{"PostgreSQL 16.4", int64(7200), "24 MB", int64(3), int64(100), int64(90), int64(10), int64(100), int64(900), int64(2), "8 kB", int64(1), int64(12), "pg_stat_statements 1.10, pgcrypto 1.3"}}}}
	analytics := &selectedQuerier{rowCap: 500, maxConns: 4, result: &db.Result{Rows: [][]any{{"PostgreSQL 17.1", int64(1800), "12 MB", int64(1), int64(200), int64(50), int64(5), int64(20), int64(180), int64(0), "0 bytes", int64(0), int64(3), "none"}}}}
	registry := fakeRegistry{
		defaultID: "primary",
		metadata: []db.PoolMetadata{
			{ID: "primary", Name: "Primary"},
			{ID: "analytics", Name: "Analytics"},
		},
		pools: map[string]Querier{"primary": primary, "analytics": analytics},
	}
	ts := newRegistryTestServer(t, registry)

	resp := mustGet(t, ts, "/api/databases")
	got := decode[struct {
		DefaultID string         `json:"defaultId"`
		Databases []databaseInfo `json:"databases"`
	}](t, resp)

	if got.DefaultID != "primary" || len(got.Databases) != 2 || got.Databases[0].Version != "PostgreSQL 16.4" || got.Databases[0].UptimeSeconds != 7200 || got.Databases[0].PoolMaxConnections != 8 || got.Databases[0].CacheHitPercent == nil || *got.Databases[0].CacheHitPercent != 90 || got.Databases[0].TempFiles != 2 || got.Databases[0].Extensions != "pg_stat_statements 1.10, pgcrypto 1.3" || got.Databases[1].DatabaseSize != "12 MB" || got.Databases[1].MaxConnections != 200 || got.Databases[1].PoolMaxConnections != 4 || got.Databases[1].CacheHitPercent == nil {
		t.Fatalf("databases = %+v", got)
	}
	body := marshalString(t, got)
	if strings.Contains(body, "postgres://") || strings.Contains(body, "dsn") {
		t.Fatalf("database metadata leaked secret material: %s", body)
	}
}

func TestDatabases_reports_unavailable_details(t *testing.T) {
	registry := fakeRegistry{
		defaultID: "primary",
		metadata:  []db.PoolMetadata{{ID: "primary", Name: "Primary"}, {ID: "missing", Name: "Missing"}},
		pools:     map[string]Querier{"primary": &selectedQuerier{err: errors.New("down")}},
	}
	ts := newRegistryTestServer(t, registry)

	resp := mustGet(t, ts, "/api/databases")
	got := decode[databasesResponse](t, resp)

	if got.Databases[0].Error != "details unavailable" || got.Databases[1].Error != "unavailable" {
		t.Fatalf("databases = %+v", got.Databases)
	}
}

func TestDatabaseSelection_uses_selected_pool_for_db_bound_endpoints(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "readyz", method: http.MethodGet, path: "/readyz?db=analytics"},
		{name: "meta", method: http.MethodGet, path: "/api/meta?db=analytics"},
		{name: "query", method: http.MethodPost, path: "/api/query?db=analytics", body: `{"sql":"SELECT 1"}`},
		{name: "query cell", method: http.MethodPost, path: "/api/query/cell?db=analytics", body: `{"sql":"SELECT 1","row":0,"column":0}`},
		{name: "export", method: http.MethodPost, path: "/api/export?db=analytics", body: `{"sql":"SELECT 1"}`},
		{name: "tables", method: http.MethodGet, path: "/api/tables?db=analytics"},
		{name: "columns", method: http.MethodGet, path: "/api/tables/public/users/columns?db=analytics"},
		{name: "fks", method: http.MethodGet, path: "/api/tables/public/users/fks?db=analytics"},
		{name: "data", method: http.MethodGet, path: "/api/tables/public/users/data?db=analytics"},
		{name: "data cell", method: http.MethodGet, path: "/api/tables/public/users/data/cell?row=0&column=0&db=analytics"},
		{name: "data csv", method: http.MethodGet, path: "/api/tables/public/users/data?format=csv&db=analytics"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			primary := &selectedQuerier{rowCap: 1000}
			analytics := &selectedQuerier{rowCap: 2000}
			ts := newRegistryTestServer(t, fakeRegistry{
				defaultID: "primary",
				metadata:  []db.PoolMetadata{{ID: "primary", Name: "Primary"}, {ID: "analytics", Name: "Analytics"}},
				pools:     map[string]Querier{"primary": primary, "analytics": analytics},
			})
			req, err := http.NewRequest(tt.method, ts.URL+tt.path, strings.NewReader(tt.body))
			if err != nil {
				t.Fatal(err)
			}
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}
			if tt.name == "readyz" {
				if !analytics.pinged || primary.pinged {
					t.Fatalf("pinged primary=%v analytics=%v", primary.pinged, analytics.pinged)
				}
				return
			}
			if !analytics.used || primary.used {
				t.Fatalf("used primary=%v analytics=%v", primary.used, analytics.used)
			}
		})
	}
}

func TestDatabaseSelection_missing_or_empty_db_uses_default(t *testing.T) {
	primary := &selectedQuerier{rowCap: 1000}
	analytics := &selectedQuerier{rowCap: 2000}
	ts := newRegistryTestServer(t, fakeRegistry{
		defaultID: "primary",
		metadata:  []db.PoolMetadata{{ID: "primary", Name: "Primary"}, {ID: "analytics", Name: "Analytics"}},
		pools:     map[string]Querier{"primary": primary, "analytics": analytics},
	})

	resp := mustGet(t, ts, "/api/meta?db=")
	got := decode[map[string]int](t, resp)

	if got["rowCap"] != 1000 || !primary.used || analytics.used {
		t.Fatalf("default selection failed: body=%+v primary=%v analytics=%v", got, primary.used, analytics.used)
	}
}

func TestDatabaseSelection_unknown_db_returns_404(t *testing.T) {
	primary := &selectedQuerier{rowCap: 1000}
	ts := newRegistryTestServer(t, fakeRegistry{
		defaultID: "primary",
		metadata:  []db.PoolMetadata{{ID: "primary", Name: "Primary"}},
		pools:     map[string]Querier{"primary": primary},
	})

	resp := mustGet(t, ts, "/api/tables?db=missing")
	got := decode[map[string]string](t, resp)

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	if got["error"] == "" || strings.Contains(got["error"], "postgres://") {
		t.Fatalf("bad error body: %+v", got)
	}
	if primary.used {
		t.Fatal("unknown db should not use default pool")
	}
}

func TestDatabaseSelection_saved_query_endpoints_ignore_db(t *testing.T) {
	primary := &selectedQuerier{rowCap: 1000}
	ts := newRegistryTestServer(t, fakeRegistry{
		defaultID: "primary",
		metadata:  []db.PoolMetadata{{ID: "primary", Name: "Primary"}},
		pools:     map[string]Querier{"primary": primary},
	})

	resp := post(t, ts, "/api/queries?db=missing", `{"name":"q","sql":"SELECT 1"}`)
	created := decode[store.SavedQuery](t, resp)

	if resp.StatusCode != http.StatusCreated || created.ID == 0 {
		t.Fatalf("create saved query status=%d query=%+v", resp.StatusCode, created)
	}
	if primary.used || primary.pinged {
		t.Fatal("saved query endpoint should not select a database pool")
	}
}

func marshalString(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}
