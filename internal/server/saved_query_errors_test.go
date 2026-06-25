package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/descope/pgpeek/internal/db"
)

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
