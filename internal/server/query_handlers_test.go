package server

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/descope/pgpeek/internal/db"
)

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

func TestExport_DBError(t *testing.T) {
	// Given: CSV export receives a read-only query but database execution fails.
	q := &fakeQuerier{err: errors.New("boom")}
	ts, _ := newTestServer(t, q)

	// When: export is requested.
	resp := post(t, ts, "/api/export", `{"sql":"SELECT 1"}`)
	defer resp.Body.Close()

	// Then: handler returns the same bad-request contract as query execution.
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}
