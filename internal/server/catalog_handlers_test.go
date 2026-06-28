package server

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/descope-sample-apps/pgpeek/internal/db"
)

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
	ts, _ := newTestServer(t, &fakeQuerier{catErr: errors.New("postgres://secret-host/hidden: boom")})
	resp := mustGet(t, ts, "/api/tables")
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
	got := decode[map[string]string](t, resp)
	if got["error"] != "failed to list tables" {
		t.Fatalf("error = %q, want sanitized tables error", got["error"])
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
	ts, _ := newTestServer(t, &fakeQuerier{catErr: errors.New("postgres://secret-host/hidden: boom")})
	resp := mustGet(t, ts, "/api/tables/public/users/columns")
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
	got := decode[map[string]string](t, resp)
	if got["error"] != "failed to read columns" {
		t.Fatalf("error = %q, want sanitized columns error", got["error"])
	}
}

func TestForeignKeys_OK(t *testing.T) {
	q := &fakeQuerier{fks: []db.ForeignKey{{Column: "company_id", RefSchema: "public", RefTable: "companies", RefColumn: "id"}}}
	ts, _ := newTestServer(t, q)
	resp := mustGet(t, ts, "/api/tables/public/users/fks")
	got := decode[[]db.ForeignKey](t, resp)
	if len(got) != 1 || got[0].RefTable != "companies" {
		t.Errorf("fks = %+v", got)
	}
	if q.lastArgs.schema != "public" || q.lastArgs.table != "users" {
		t.Errorf("path values not passed: %+v", q.lastArgs)
	}
}

func TestForeignKeys_EmptyReturnsArray(t *testing.T) {
	srv := serverWithStore(t, &fakeQuerier{}, &fakeStore{})
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/tables/public/users/fks", nil))
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("body = %q, want []", rec.Body.String())
	}
}

func TestForeignKeys_Error(t *testing.T) {
	ts, _ := newTestServer(t, &fakeQuerier{catErr: errors.New("postgres://secret-host/hidden: boom")})
	resp := mustGet(t, ts, "/api/tables/public/users/fks")
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
	got := decode[map[string]string](t, resp)
	if got["error"] != "failed to read foreign keys" {
		t.Fatalf("error = %q, want sanitized foreign-key error", got["error"])
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
	ts, _ := newTestServer(t, &fakeQuerier{err: errors.New("postgres://secret-host/hidden: no such table")})
	resp := mustGet(t, ts, "/api/tables/public/nope/data")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	got := decode[map[string]string](t, resp)
	if got["error"] != "failed to read rows" {
		t.Fatalf("error = %q, want sanitized row error", got["error"])
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
