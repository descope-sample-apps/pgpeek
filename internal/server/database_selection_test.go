package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/descope/pgpeek/internal/db"
	"github.com/descope/pgpeek/internal/store"
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
	rowCap int
	used   bool
	pinged bool
}

func (q *selectedQuerier) Query(context.Context, string) (*db.Result, error) {
	q.used = true
	return okResult(), nil
}

func (q *selectedQuerier) Tables(context.Context) ([]db.TableInfo, error) {
	q.used = true
	return []db.TableInfo{}, nil
}

func (q *selectedQuerier) Columns(context.Context, string, string) ([]db.ColumnInfo, error) {
	q.used = true
	return []db.ColumnInfo{}, nil
}

func (q *selectedQuerier) ForeignKeys(context.Context, string, string) ([]db.ForeignKey, error) {
	q.used = true
	return []db.ForeignKey{}, nil
}

func (q *selectedQuerier) TableRows(context.Context, db.TableQuery) (*db.Result, error) {
	q.used = true
	return okResult(), nil
}

func (q *selectedQuerier) RowCap() int {
	q.used = true
	return q.rowCap
}

func (q *selectedQuerier) Ping(context.Context) error {
	q.pinged = true
	return nil
}

func newRegistryTestServer(t *testing.T, registry DatabaseRegistry) *httptest.Server {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	web := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("x")}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewWithRegistry(registry, st, web, log, time.Second)
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	return ts
}

func TestDatabases_lists_safe_metadata(t *testing.T) {
	primary := &selectedQuerier{rowCap: 1000}
	registry := fakeRegistry{
		defaultID: "primary",
		metadata: []db.PoolMetadata{
			{ID: "primary", Name: "Primary"},
			{ID: "analytics", Name: "Analytics"},
		},
		pools: map[string]Querier{"primary": primary, "analytics": &selectedQuerier{}},
	}
	ts := newRegistryTestServer(t, registry)

	resp := mustGet(t, ts, "/api/databases")
	got := decode[struct {
		DefaultID string            `json:"defaultId"`
		Databases []db.PoolMetadata `json:"databases"`
	}](t, resp)

	if got.DefaultID != "primary" || len(got.Databases) != 2 || got.Databases[0].ID != "primary" || got.Databases[1].Name != "Analytics" {
		t.Fatalf("databases = %+v", got)
	}
	body := marshalString(t, got)
	if strings.Contains(body, "postgres://") || strings.Contains(body, "dsn") {
		t.Fatalf("database metadata leaked secret material: %s", body)
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
		{name: "export", method: http.MethodPost, path: "/api/export?db=analytics", body: `{"sql":"SELECT 1"}`},
		{name: "tables", method: http.MethodGet, path: "/api/tables?db=analytics"},
		{name: "columns", method: http.MethodGet, path: "/api/tables/public/users/columns?db=analytics"},
		{name: "fks", method: http.MethodGet, path: "/api/tables/public/users/fks?db=analytics"},
		{name: "data", method: http.MethodGet, path: "/api/tables/public/users/data?db=analytics"},
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
