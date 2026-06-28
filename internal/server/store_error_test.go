package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/descope-sample-apps/pgpeek/internal/store"
)

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
