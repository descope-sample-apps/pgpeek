package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/descope/pgpeek/internal/db"
)

func TestSingleDatabaseRegistry_methods_select_default_or_reject_unknown(t *testing.T) {
	// Given: a single-database registry wraps one querier.
	q := &fakeQuerier{}
	registry := NewSingleDatabaseRegistry(q)

	// When: callers inspect metadata and select pools.
	metadata := registry.List()
	defaultPool, defaultErr := registry.Pool("")
	namedPool, namedErr := registry.Pool("default")
	_, missingErr := registry.Pool("missing")
	pingErr := registry.Ping(context.Background())

	// Then: only the default id is accepted and delegated.
	if len(metadata) != 1 || metadata[0].ID != "default" || registry.DefaultID() != "default" {
		t.Fatalf("metadata=%+v default=%q", metadata, registry.DefaultID())
	}
	if defaultErr != nil || namedErr != nil || pingErr != nil {
		t.Fatalf("defaultErr=%v namedErr=%v pingErr=%v", defaultErr, namedErr, pingErr)
	}
	if defaultPool != q || namedPool != q {
		t.Fatalf("pool selection returned wrong querier")
	}
	if !errors.Is(missingErr, db.ErrPoolNotFound) {
		t.Fatalf("missingErr=%v, want ErrPoolNotFound", missingErr)
	}
}

func TestPoolForRequest_returns_500_when_registry_selection_fails(t *testing.T) {
	// Given: registry lookup fails with a non-not-found error.
	srv := serverWithStore(t, &fakeQuerier{}, &fakeStore{})
	srv.registry = failingRegistry{err: errors.New("registry unavailable")}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/meta?db=primary", nil)

	// When: a handler selects a pool for the request.
	pool, ok := srv.poolForRequest(rec, req)

	// Then: callers receive a sanitized 500 response and no pool.
	if ok || pool != nil {
		t.Fatalf("poolForRequest ok=%v pool=%v, want no pool", ok, pool)
	}
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "registry unavailable") {
		t.Fatalf("response code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestHandlers_return_not_found_when_selected_database_missing(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "meta", method: http.MethodGet, path: "/api/meta?db=missing"},
		{name: "columns", method: http.MethodGet, path: "/api/tables/public/users/columns?db=missing"},
		{name: "foreign keys", method: http.MethodGet, path: "/api/tables/public/users/fks?db=missing"},
		{name: "table data", method: http.MethodGet, path: "/api/tables/public/users/data?db=missing"},
		{name: "query", method: http.MethodPost, path: "/api/query?db=missing", body: `{"sql":"SELECT 1"}`},
		{name: "export", method: http.MethodPost, path: "/api/export?db=missing", body: `{"sql":"SELECT 1"}`},
		{name: "ready", method: http.MethodGet, path: "/readyz?db=missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: server has no database matching the request id.
			ts := newRegistryTestServer(t, fakeRegistry{
				defaultID: "primary",
				metadata:  []db.PoolMetadata{{ID: "primary", Name: "Primary"}},
				pools:     map[string]Querier{"primary": &selectedQuerier{rowCap: 1000}},
			})
			req, err := http.NewRequest(tt.method, ts.URL+tt.path, strings.NewReader(tt.body))
			if err != nil {
				t.Fatal(err)
			}

			// When: the request is served.
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			// Then: the handler stops at database selection.
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("status=%d, want 404", resp.StatusCode)
			}
		})
	}
}

type failingRegistry struct {
	err error
}

func (f failingRegistry) List() []db.PoolMetadata { return nil }
func (f failingRegistry) DefaultID() string       { return "default" }
func (f failingRegistry) Pool(string) (Querier, error) {
	return nil, f.err
}
func (f failingRegistry) Ping(context.Context) error { return f.err }
