package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/descope-sample-apps/pgpeek/internal/db"
	"github.com/descope-sample-apps/pgpeek/internal/store"
)

type fakeQuerier struct {
	result    *db.Result
	err       error
	pingErr   error
	called    bool
	lastSQL   string
	tables    []db.TableInfo
	cols      []db.ColumnInfo
	fks       []db.ForeignKey
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

func (f *fakeQuerier) ForeignKeys(_ context.Context, _, _ string) ([]db.ForeignKey, error) {
	return f.fks, f.catErr
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
