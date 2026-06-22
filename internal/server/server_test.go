package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/descope/pgpeek/internal/db"
	"github.com/descope/pgpeek/internal/guard"
	"github.com/descope/pgpeek/internal/store"
)

type fakeQuerier struct {
	result    *db.Result
	err       error
	pingErr   error
	called    bool
	lastSQL   string
	tables    []db.TableInfo
	cols      []db.ColumnInfo
	catErr    error
	lastQuery db.TableQuery
	lastArgs  struct {
		schema, table string
		limit, offset int
	}
}

func (f *fakeQuerier) Query(_ context.Context, sql string) (*db.Result, error) {
	f.called = true
	f.lastSQL = sql
	return f.result, f.err
}

func (f *fakeQuerier) Tables(context.Context) ([]db.TableInfo, error) {
	return f.tables, f.catErr
}

func (f *fakeQuerier) Columns(_ context.Context, schema, table string) ([]db.ColumnInfo, error) {
	f.lastArgs.schema, f.lastArgs.table = schema, table
	return f.cols, f.catErr
}

func (f *fakeQuerier) TableRows(_ context.Context, q db.TableQuery) (*db.Result, error) {
	f.lastQuery = q
	f.lastArgs.schema, f.lastArgs.table = q.Schema, q.Table
	f.lastArgs.limit, f.lastArgs.offset = q.Limit, q.Offset
	return f.result, f.err
}

func (f *fakeQuerier) RowCap() int { return 1000 }

func (f *fakeQuerier) Ping(_ context.Context) error { return f.pingErr }

func newTestServer(t *testing.T, q Querier) (*httptest.Server, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	web := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>pgpeek</html>")},
		"app.js":     &fstest.MapFile{Data: []byte("// js")},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(q, st, web, log, 5*time.Second)
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	return ts, st
}

func post(t *testing.T, ts *httptest.Server, path, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(ts.URL+path, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

func decode[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	defer resp.Body.Close()
	var v T
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return v
}

func okResult() *db.Result {
	return &db.Result{
		Columns:   []string{"n"},
		Rows:      [][]any{{int64(1)}},
		RowCount:  1,
		ElapsedMS: 1,
	}
}

func TestQuery_OK(t *testing.T) {
	q := &fakeQuerier{result: okResult()}
	ts, _ := newTestServer(t, q)

	resp := post(t, ts, "/api/query", `{"sql":"  SELECT 1  "}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	res := decode[db.Result](t, resp)
	if res.RowCount != 1 {
		t.Errorf("rowCount = %d", res.RowCount)
	}
	if q.lastSQL != "SELECT 1" {
		t.Errorf("SQL not trimmed before exec: %q", q.lastSQL)
	}
}

func TestQuery_GuardRejectsDML(t *testing.T) {
	q := &fakeQuerier{result: okResult()}
	ts, _ := newTestServer(t, q)

	resp := post(t, ts, "/api/query", `{"sql":"DELETE FROM users"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if q.called {
		t.Error("guard should block the query before it reaches the database")
	}
}

func TestQuery_InvalidJSON(t *testing.T) {
	ts, _ := newTestServer(t, &fakeQuerier{})
	resp := post(t, ts, "/api/query", `{not json`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestQuery_UnknownField(t *testing.T) {
	ts, _ := newTestServer(t, &fakeQuerier{})
	resp := post(t, ts, "/api/query", `{"sql":"SELECT 1","evil":true}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (DisallowUnknownFields)", resp.StatusCode)
	}
}

func TestQuery_DBError(t *testing.T) {
	q := &fakeQuerier{err: errors.New("boom")}
	ts, _ := newTestServer(t, q)
	resp := post(t, ts, "/api/query", `{"sql":"SELECT 1"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestQuery_BodyTooLarge(t *testing.T) {
	ts, _ := newTestServer(t, &fakeQuerier{result: okResult()})
	huge := strings.Repeat("a", (1<<20)+10)
	resp := post(t, ts, "/api/query", `{"sql":"`+huge+`"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for oversized body", resp.StatusCode)
	}
}

func TestExport_CSV(t *testing.T) {
	q := &fakeQuerier{result: &db.Result{
		Columns:  []string{"name", "n"},
		Rows:     [][]any{{"Acme", int64(2)}, {"Globex,Inc", int64(1)}},
		RowCount: 2,
	}}
	ts, _ := newTestServer(t, q)

	resp := post(t, ts, "/api/export", `{"sql":"SELECT 1"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("content-type = %q", ct)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "pgpeek-export.csv") {
		t.Errorf("content-disposition = %q", cd)
	}
	body, _ := io.ReadAll(resp.Body)
	got := string(body)
	if !strings.Contains(got, "name,n") || !strings.Contains(got, "Acme,2") {
		t.Errorf("csv body = %q", got)
	}
	// Field with a comma must be quoted by encoding/csv.
	if !strings.Contains(got, `"Globex,Inc"`) {
		t.Errorf("comma field not quoted: %q", got)
	}
}

func TestExport_GuardRejects(t *testing.T) {
	q := &fakeQuerier{result: okResult()}
	ts, _ := newTestServer(t, q)
	resp := post(t, ts, "/api/export", `{"sql":"DROP TABLE x"}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestSavedQueries_CRUD(t *testing.T) {
	ts, _ := newTestServer(t, &fakeQuerier{result: okResult()})

	// Create
	resp := post(t, ts, "/api/queries", `{"name":"q","description":"d","sql":"SELECT 1"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d", resp.StatusCode)
	}
	created := decode[store.SavedQuery](t, resp)
	if created.ID == 0 {
		t.Fatal("no id returned")
	}

	// List
	resp = mustGet(t, ts, "/api/queries")
	list := decode[[]store.SavedQuery](t, resp)
	if len(list) != 1 {
		t.Fatalf("list len = %d", len(list))
	}

	// Update
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/queries/"+itoa(created.ID),
		strings.NewReader(`{"name":"q2","sql":"SELECT 2"}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Delete
	req, _ = http.NewRequest(http.MethodDelete, ts.URL+"/api/queries/"+itoa(created.ID), nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestSavedQueries_Validation(t *testing.T) {
	ts, _ := newTestServer(t, &fakeQuerier{})

	cases := []struct {
		name, body string
	}{
		{"missing name", `{"sql":"SELECT 1"}`},
		{"missing sql", `{"name":"x"}`},
		{"non-readonly sql", `{"name":"x","sql":"DELETE FROM t"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := post(t, ts, "/api/queries", c.body)
			resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", resp.StatusCode)
			}
		})
	}
}

func TestSavedQueries_NotFoundAndBadID(t *testing.T) {
	ts, _ := newTestServer(t, &fakeQuerier{})

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/queries/999", nil)
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("delete missing status = %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()

	req, _ = http.NewRequest(http.MethodPut, ts.URL+"/api/queries/abc",
		strings.NewReader(`{"name":"x","sql":"SELECT 1"}`))
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("bad id status = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestHealthz(t *testing.T) {
	ts, _ := newTestServer(t, &fakeQuerier{})
	resp := mustGet(t, ts, "/healthz")
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("healthz = %d", resp.StatusCode)
	}
}

func TestReadyz(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		ts, _ := newTestServer(t, &fakeQuerier{})
		resp := mustGet(t, ts, "/readyz")
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("readyz = %d, want 200", resp.StatusCode)
		}
	})
	t.Run("db down", func(t *testing.T) {
		ts, _ := newTestServer(t, &fakeQuerier{pingErr: errors.New("down")})
		resp := mustGet(t, ts, "/readyz")
		resp.Body.Close()
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("readyz = %d, want 503", resp.StatusCode)
		}
	})
}

func TestSecurityHeaders(t *testing.T) {
	ts, _ := newTestServer(t, &fakeQuerier{})
	resp := mustGet(t, ts, "/healthz")
	resp.Body.Close()
	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	}
	for k, v := range want {
		if got := resp.Header.Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("missing CSP: %q", csp)
	}
}

func TestUIServed(t *testing.T) {
	ts, _ := newTestServer(t, &fakeQuerier{})
	resp := mustGet(t, ts, "/")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(body, []byte("pgpeek")) {
		t.Errorf("index not served: %q", body)
	}
}

// DefaultPresets must all survive the read-only guard, otherwise the saved-query
// validation would reject them if a user re-saved one.
func TestDefaultPresetsPassGuard(t *testing.T) {
	for _, p := range store.DefaultPresets {
		if err := guard.Validate(p.SQL); err != nil {
			t.Errorf("preset %q fails guard: %v", p.Name, err)
		}
	}
}

// --- error-path coverage with a fake store and failing writer ------------

type fakeStore struct {
	listErr, getErr, createErr, updateErr, deleteErr error
}

func (f *fakeStore) List(context.Context) ([]store.SavedQuery, error) {
	return nil, f.listErr
}
func (f *fakeStore) Get(context.Context, int64) (store.SavedQuery, error) {
	return store.SavedQuery{}, f.getErr
}
func (f *fakeStore) Create(context.Context, string, string, string) (store.SavedQuery, error) {
	return store.SavedQuery{}, f.createErr
}
func (f *fakeStore) Update(context.Context, int64, string, string, string) (store.SavedQuery, error) {
	return store.SavedQuery{}, f.updateErr
}
func (f *fakeStore) Delete(context.Context, int64) error { return f.deleteErr }

func serverWithStore(t *testing.T, q Querier, st QueryStore) *Server {
	t.Helper()
	web := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("x")}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(q, st, web, log, time.Second)
}

func TestStoreErrorPaths(t *testing.T) {
	boom := errors.New("db down")
	srv := serverWithStore(t, &fakeQuerier{}, &fakeStore{
		listErr: boom, createErr: boom, updateErr: boom, deleteErr: boom,
	})
	mux := srv.Routes()

	do := func(method, path, body string) int {
		var rdr io.Reader
		if body != "" {
			rdr = strings.NewReader(body)
		}
		req := httptest.NewRequest(method, path, rdr)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := do(http.MethodGet, "/api/queries", ""); code != http.StatusInternalServerError {
		t.Errorf("list error code = %d, want 500", code)
	}
	if code := do(http.MethodPost, "/api/queries", `{"name":"x","sql":"SELECT 1"}`); code != http.StatusInternalServerError {
		t.Errorf("create error code = %d, want 500", code)
	}
	if code := do(http.MethodPut, "/api/queries/1", `{"name":"x","sql":"SELECT 1"}`); code != http.StatusInternalServerError {
		t.Errorf("update error code = %d, want 500", code)
	}
	if code := do(http.MethodDelete, "/api/queries/1", ""); code != http.StatusInternalServerError {
		t.Errorf("delete error code = %d, want 500", code)
	}
}

// --- catalog / browse endpoints -----------------------------------------

func TestMeta(t *testing.T) {
	ts, _ := newTestServer(t, &fakeQuerier{})
	resp := mustGet(t, ts, "/api/meta")
	got := decode[map[string]int](t, resp)
	if got["rowCap"] != 1000 {
		t.Errorf("rowCap = %d, want 1000", got["rowCap"])
	}
}

func TestSafeFilename(t *testing.T) {
	cases := map[string]string{
		"users":          "users",
		`a"; x`:          "a___x",
		"sch.tbl":        "sch.tbl",
		"évil/../x":      "_vil_.._x",
		"":               "table",
		"weird name!@#$": "weird_name____",
	}
	for in, want := range cases {
		if got := safeFilename(in); got != want {
			t.Errorf("safeFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTables_OK(t *testing.T) {
	q := &fakeQuerier{tables: []db.TableInfo{{Schema: "public", Name: "users", Type: "table", EstRows: 5}}}
	ts, _ := newTestServer(t, q)
	resp := mustGet(t, ts, "/api/tables")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	got := decode[[]db.TableInfo](t, resp)
	if len(got) != 1 || got[0].Name != "users" {
		t.Errorf("tables = %+v", got)
	}
}

func TestTables_EmptyReturnsArray(t *testing.T) {
	srv := serverWithStore(t, &fakeQuerier{}, &fakeStore{})
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/tables", nil))
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("body = %q, want []", rec.Body.String())
	}
}

func TestTables_Error(t *testing.T) {
	ts, _ := newTestServer(t, &fakeQuerier{catErr: errors.New("boom")})
	resp := mustGet(t, ts, "/api/tables")
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
}

func TestColumns_OK(t *testing.T) {
	q := &fakeQuerier{cols: []db.ColumnInfo{{Name: "id", Type: "integer"}}}
	ts, _ := newTestServer(t, q)
	resp := mustGet(t, ts, "/api/tables/public/users/columns")
	got := decode[[]db.ColumnInfo](t, resp)
	if len(got) != 1 || got[0].Name != "id" {
		t.Errorf("columns = %+v", got)
	}
	if q.lastArgs.schema != "public" || q.lastArgs.table != "users" {
		t.Errorf("path values not passed: %+v", q.lastArgs)
	}
}

func TestColumns_EmptyReturnsArray(t *testing.T) {
	srv := serverWithStore(t, &fakeQuerier{}, &fakeStore{})
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/tables/public/users/columns", nil))
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("body = %q, want []", rec.Body.String())
	}
}

func TestColumns_Error(t *testing.T) {
	ts, _ := newTestServer(t, &fakeQuerier{catErr: errors.New("boom")})
	resp := mustGet(t, ts, "/api/tables/public/users/columns")
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
}

func TestTableData_OK(t *testing.T) {
	q := &fakeQuerier{result: okResult()}
	ts, _ := newTestServer(t, q)
	resp := mustGet(t, ts, "/api/tables/public/users/data?limit=25&offset=50")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	decode[db.Result](t, resp)
	if q.lastArgs.limit != 25 || q.lastArgs.offset != 50 {
		t.Errorf("limit/offset = %d/%d", q.lastArgs.limit, q.lastArgs.offset)
	}
}

func TestTableData_ParsesSearchSortFilters(t *testing.T) {
	q := &fakeQuerier{result: okResult()}
	ts, _ := newTestServer(t, q)
	resp := mustGet(t, ts, "/api/tables/public/users/data?search=acme&sort=id&dir=desc&f=id:gt:100&f=name:ilike:%25a%25&f=deleted_at:is_null")
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	lq := q.lastQuery
	if lq.Search != "acme" || lq.Sort != "id" || !lq.Desc {
		t.Errorf("search/sort/desc = %+v", lq)
	}
	if len(lq.Filters) != 3 {
		t.Fatalf("filters = %+v", lq.Filters)
	}
	if lq.Filters[0] != (db.Filter{Column: "id", Op: "gt", Value: "100"}) {
		t.Errorf("filter0 = %+v", lq.Filters[0])
	}
	if lq.Filters[1] != (db.Filter{Column: "name", Op: "ilike", Value: "%a%"}) {
		t.Errorf("filter1 = %+v", lq.Filters[1])
	}
	if lq.Filters[2] != (db.Filter{Column: "deleted_at", Op: "is_null"}) {
		t.Errorf("filter2 (no value) = %+v", lq.Filters[2])
	}
}

func TestParseFilters(t *testing.T) {
	if parseFilters(nil) != nil {
		t.Error("nil input should yield nil")
	}
	got := parseFilters([]string{"id:gt:100", "name:is_null", "bare"})
	want := []db.Filter{
		{Column: "id", Op: "gt", Value: "100"},
		{Column: "name", Op: "is_null"},
		{Column: "bare"},
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d", len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("filter %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestTableData_DefaultsAndBadParams(t *testing.T) {
	q := &fakeQuerier{result: okResult()}
	ts, _ := newTestServer(t, q)
	// Non-numeric params fall back to defaults (0 -> pool clamps).
	resp := mustGet(t, ts, "/api/tables/public/users/data?limit=abc")
	resp.Body.Close()
	if q.lastArgs.limit != 0 || q.lastArgs.offset != 0 {
		t.Errorf("expected defaults, got %d/%d", q.lastArgs.limit, q.lastArgs.offset)
	}
}

func TestTableData_Error(t *testing.T) {
	ts, _ := newTestServer(t, &fakeQuerier{err: errors.New("no such table")})
	resp := mustGet(t, ts, "/api/tables/public/nope/data")
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestTableData_CSV(t *testing.T) {
	q := &fakeQuerier{result: &db.Result{Columns: []string{"a"}, Rows: [][]any{{"1"}}, RowCount: 1}}
	ts, _ := newTestServer(t, q)
	resp := mustGet(t, ts, "/api/tables/public/users/data?format=csv")
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("content-type = %q", ct)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "users.csv") {
		t.Errorf("disposition = %q", cd)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "a\n1\n") {
		t.Errorf("csv body = %q", body)
	}
}

func TestTableData_CSVWriteFailure(t *testing.T) {
	srv := serverWithStore(t, &fakeQuerier{result: okResult()}, &fakeStore{})
	req := httptest.NewRequest(http.MethodGet, "/api/tables/public/users/data?format=csv", nil)
	fw := &failingWriter{}
	srv.handleTableData(fw, req)
	if ct := fw.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("content-type = %q", ct)
	}
}

func TestListEmptyReturnsArray(t *testing.T) {
	// A successful List of an empty store returns nil; the handler must emit [].
	srv := serverWithStore(t, &fakeQuerier{}, &fakeStore{})
	req := httptest.NewRequest(http.MethodGet, "/api/queries", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
		t.Errorf("body = %q, want []", body)
	}
}

func TestUpdate_InvalidJSON(t *testing.T) {
	ts, _ := newTestServer(t, &fakeQuerier{})
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/queries/1", strings.NewReader(`{bad`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestUpdate_NotFound(t *testing.T) {
	ts, _ := newTestServer(t, &fakeQuerier{})
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/queries/999", strings.NewReader(`{"name":"x","sql":"SELECT 1"}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestDelete_BadID(t *testing.T) {
	ts, _ := newTestServer(t, &fakeQuerier{})
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/queries/abc", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestExport_QueryError(t *testing.T) {
	ts, _ := newTestServer(t, &fakeQuerier{err: errors.New("boom")})
	resp := post(t, ts, "/api/export", `{"sql":"SELECT 1"}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestExport_InvalidJSON(t *testing.T) {
	ts, _ := newTestServer(t, &fakeQuerier{})
	resp := post(t, ts, "/api/export", `{bad`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestSavedQuery_InvalidJSON(t *testing.T) {
	ts, _ := newTestServer(t, &fakeQuerier{})
	resp := post(t, ts, "/api/queries", `{bad`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// failingWriter is an http.ResponseWriter whose body writes always fail.
type failingWriter struct {
	header http.Header
	code   int
}

func (f *failingWriter) Header() http.Header {
	if f.header == nil {
		f.header = http.Header{}
	}
	return f.header
}
func (f *failingWriter) Write([]byte) (int, error) { return 0, errors.New("connection reset") }
func (f *failingWriter) WriteHeader(code int)      { f.code = code }

func TestExport_WriteFailureLogged(t *testing.T) {
	// Exercise the handler's csv-error branch directly with a writer that fails.
	srv := serverWithStore(t, &fakeQuerier{result: okResult()}, &fakeStore{})
	req := httptest.NewRequest(http.MethodPost, "/api/export", strings.NewReader(`{"sql":"SELECT 1"}`))
	fw := &failingWriter{}
	srv.handleExport(fw, req)
	// Header is set before the failing body write.
	if ct := fw.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("content-type = %q", ct)
	}
}

func TestWriteCSV(t *testing.T) {
	res := &db.Result{Columns: []string{"a"}, Rows: [][]any{{"1"}}}
	var buf strings.Builder
	if err := writeCSV(&buf, res); err != nil {
		t.Fatalf("writeCSV: %v", err)
	}
	if !strings.Contains(buf.String(), "a\n1\n") {
		t.Errorf("csv = %q", buf.String())
	}
	// Failing writer -> error surfaces via cw.Error() after Flush.
	if err := writeCSV(failWriter{}, res); err == nil {
		t.Error("expected error from failing writer")
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("boom") }

func mustGet(t *testing.T, ts *httptest.Server, path string) *http.Response {
	t.Helper()
	resp, err := http.Get(ts.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

func itoa(i int64) string { return strconv.FormatInt(i, 10) }
