package server

import (
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/descope-sample-apps/pgpeek/internal/db"
	"github.com/descope-sample-apps/pgpeek/internal/guard"
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

func TestQueryCount_OK(t *testing.T) {
	q := &fakeQuerier{count: 1_000_000}
	ts, _ := newTestServer(t, q)
	resp := post(t, ts, "/api/query/count", `{"sql":" SELECT * FROM events; "}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	got := decode[struct {
		RowCount string `json:"rowCount"`
	}](t, resp)
	if got.RowCount != "1000000" || q.lastSQL != "SELECT * FROM events;" || !q.countCalled {
		t.Fatalf("result=%+v SQL=%q counted=%v", got, q.lastSQL, q.countCalled)
	}
}

func TestQueryCount_Error(t *testing.T) {
	ts, _ := newTestServer(t, &fakeQuerier{countErr: errors.New("secret")})
	resp := post(t, ts, "/api/query/count", `{"sql":"SELECT 1"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got := decode[map[string]string](t, resp)["error"]; got != "query failed" {
		t.Fatalf("error = %q", got)
	}
}

func TestQueryCount_RejectsExplainWithActionableError(t *testing.T) {
	q := &fakeQuerier{countErr: guard.ErrCountExplain}
	ts, _ := newTestServer(t, q)
	resp := post(t, ts, "/api/query/count", `{"sql":"EXPLAIN SELECT 1"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got := decode[map[string]string](t, resp)["error"]; got != guard.ErrCountExplain.Error() {
		t.Fatalf("error = %q", got)
	}
}

func TestQueryCount_RejectsWrite(t *testing.T) {
	q := &fakeQuerier{}
	ts, _ := newTestServer(t, q)
	resp := post(t, ts, "/api/query/count", `{"sql":"DELETE FROM events"}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest || q.countCalled {
		t.Fatalf("status=%d counted=%v", resp.StatusCode, q.countCalled)
	}
}

func TestQuery_TruncatesLargeCellsWithoutDroppingRows(t *testing.T) {
	value := strings.Repeat("x", 32<<10)
	q := &fakeQuerier{result: &db.Result{
		Columns:        []string{"payload"},
		Rows:           [][]any{{value[:100] + "…"}, {"small"}},
		RowCount:       2,
		CellsTruncated: true,
		TruncatedCells: []db.CellRef{{Row: 0, Column: 0}},
	}}
	ts, _ := newTestServer(t, q)

	resp := post(t, ts, "/api/query", `{"sql":"SELECT payload"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	res := decode[db.Result](t, resp)
	if len(res.Rows) != 2 || !res.CellsTruncated {
		t.Fatalf("rows=%d cellsTruncated=%v", len(res.Rows), res.CellsTruncated)
	}
	if res.Rows[0][0] == value || len(res.TruncatedCells) != 1 {
		t.Fatalf("large cell was not replaced with a preview: %#v", res.Rows[0][0])
	}
}

func TestQueryCell_ReturnsFullSelectedValue(t *testing.T) {
	value := strings.Repeat("x", 32<<10)
	q := &fakeQuerier{result: &db.Result{
		Columns:  []string{"payload"},
		Rows:     [][]any{{value}},
		RowCount: 1,
	}}
	ts, _ := newTestServer(t, q)

	resp := post(t, ts, "/api/query/cell", `{"sql":"SELECT payload","row":0,"column":0}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	got := decode[map[string]any](t, resp)
	if got["value"] != value {
		t.Fatal("cell endpoint did not return the full value")
	}
}

func TestQueryCell_RejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "invalid json", body: `{`},
		{name: "negative row", body: `{"sql":"SELECT 1","row":-1,"column":0}`},
		{name: "negative column", body: `{"sql":"SELECT 1","row":0,"column":-1}`},
		{name: "write query", body: `{"sql":"DELETE FROM users","row":0,"column":0}`},
		{name: "missing cell", body: `{"sql":"SELECT 1","row":99,"column":0}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ts, _ := newTestServer(t, &fakeQuerier{result: okResult()})
			resp := post(t, ts, "/api/query/cell", tc.body)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d", resp.StatusCode)
			}
		})
	}
}

func TestQueryCell_DBError(t *testing.T) {
	ts, _ := newTestServer(t, &fakeQuerier{err: errors.New("boom")})
	resp := post(t, ts, "/api/query/cell", `{"sql":"SELECT 1","row":0,"column":0}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestQueryCell_RejectsChangedResult(t *testing.T) {
	ts, _ := newTestServer(t, &fakeQuerier{result: &db.Result{Rows: [][]any{{"new value"}}}})
	resp := post(t, ts, "/api/query/cell", `{"sql":"SELECT 1","row":0,"column":0,"hash":"stale"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestQueryCell_UnknownDatabase(t *testing.T) {
	ts, _ := newTestServer(t, &fakeQuerier{result: okResult()})
	resp := post(t, ts, "/api/query/cell?db=missing", `{"sql":"SELECT 1","row":0,"column":0}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d", resp.StatusCode)
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
	got := decode[map[string]string](t, resp)
	if got["error"] != "invalid request body" {
		t.Fatalf("error = %q, want invalid request body", got["error"])
	}
}

func TestQuery_UnknownField(t *testing.T) {
	ts, _ := newTestServer(t, &fakeQuerier{})
	resp := post(t, ts, "/api/query", `{"sql":"SELECT 1","evil":true}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (DisallowUnknownFields)", resp.StatusCode)
	}
}

func TestQuery_RejectsUnsupportedContentType(t *testing.T) {
	for _, contentType := range []string{"text/plain", "application/x-www-form-urlencoded", "application/json; charset"} {
		q := &fakeQuerier{result: okResult()}
		ts, _ := newTestServer(t, q)
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/query", strings.NewReader(`{"sql":"SELECT pg_sleep(30) --="}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", contentType)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnsupportedMediaType || q.called {
			t.Fatalf("content-type=%q status=%d called=%v", contentType, resp.StatusCode, q.called)
		}
	}
}

func TestQuery_DBError(t *testing.T) {
	q := &fakeQuerier{err: errors.New("postgres://secret-host/hidden: boom")}
	ts, _ := newTestServer(t, q)
	resp := post(t, ts, "/api/query", `{"sql":"SELECT 1"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	got := decode[map[string]string](t, resp)
	if got["error"] != "query failed" {
		t.Fatalf("error = %q, want sanitized query failed", got["error"])
	}
}

func TestQuery_PostgresError(t *testing.T) {
	q := &fakeQuerier{err: &pgconn.PgError{Message: `syntax error at or near "IS"`, Code: "42601"}}
	ts, _ := newTestServer(t, q)
	resp := post(t, ts, "/api/query", `{"sql":"SELECT * FROM access_keys tenants IS NULL"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	got := decode[map[string]string](t, resp)
	want := `syntax error at or near "IS" (SQLSTATE 42601)`
	if got["error"] != want {
		t.Fatalf("error = %q, want %q", got["error"], want)
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
	if ct := resp.Header.Get("Content-Type"); ct != "application/gzip" {
		t.Errorf("content-type = %q", ct)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "pgpeek-export.csv.gz") {
		t.Errorf("content-disposition = %q", cd)
	}
	body := readGZIP(t, resp.Body)
	got := string(body)
	if !strings.Contains(got, "name,n") || !strings.Contains(got, "Acme,2") {
		t.Errorf("csv body = %q", got)
	}
	// Field with a comma must be quoted by encoding/csv.
	if !strings.Contains(got, `"Globex,Inc"`) {
		t.Errorf("comma field not quoted: %q", got)
	}
	if !q.exportCalled {
		t.Fatal("export did not use the uncapped CSV path")
	}
}

func TestExport_FormPost(t *testing.T) {
	q := &fakeQuerier{result: okResult()}
	ts, _ := newTestServer(t, q)
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/export", strings.NewReader("sql=++SELECT+1++&csrf=token"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.AddCookie(&http.Cookie{Name: exportCSRFCookie, Value: "token"})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || q.lastSQL != "SELECT 1" || !q.exportCalled {
		t.Fatalf("status=%d SQL=%q exported=%v", resp.StatusCode, q.lastSQL, q.exportCalled)
	}
	if resp.Header.Get("X-Frame-Options") != "SAMEORIGIN" || !strings.Contains(resp.Header.Get("Content-Security-Policy"), "frame-ancestors 'self'") {
		t.Fatalf("frame headers=%q %q", resp.Header.Get("X-Frame-Options"), resp.Header.Get("Content-Security-Policy"))
	}
	if cookie := resp.Cookies(); len(cookie) == 0 || cookie[0].Name != exportDoneCookiePrefix+"token" || cookie[0].Value != "1" || !cookie[0].Secure {
		t.Fatalf("completion cookie = %#v", cookie)
	}
}

func TestExport_DoesNotSignalCompletionOnFailure(t *testing.T) {
	q := &fakeQuerier{err: errors.New("export failed")}
	ts, _ := newTestServer(t, q)
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/export", strings.NewReader("sql=SELECT+1&csrf=token"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: exportCSRFCookie, Value: "token"})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	for _, cookie := range resp.Cookies() {
		if cookie.Name == exportDoneCookiePrefix+"token" {
			t.Fatal("failed export signaled completion")
		}
	}
}

func TestExport_InvalidFormPost(t *testing.T) {
	for _, body := range []string{"sql=%&csrf=token", "sql=DROP+TABLE+x&csrf=token"} {
		q := &fakeQuerier{result: okResult()}
		ts, _ := newTestServer(t, q)
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/export", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: exportCSRFCookie, Value: "token"})
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest || q.exportCalled {
			t.Fatalf("body=%q status=%d exported=%v", body, resp.StatusCode, q.exportCalled)
		}
	}
}

func TestExport_FormOrigin(t *testing.T) {
	q := &fakeQuerier{result: okResult()}
	ts, _ := newTestServer(t, q)
	request := func(origin, fetchSite, token string) *http.Response {
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/export", strings.NewReader("sql=SELECT+1&csrf="+token))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "Application/X-Www-Form-Urlencoded; Charset=UTF-8")
		req.Header.Set("Origin", origin)
		req.Header.Set("Sec-Fetch-Site", fetchSite)
		if token != "" {
			req.AddCookie(&http.Cookie{Name: exportCSRFCookie, Value: token})
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	resp := request(ts.URL, "same-origin", "token")
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("same origin status=%d", resp.StatusCode)
	}
	resp = request("null", "", "token")
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("opaque same-site status=%d", resp.StatusCode)
	}
	for _, requestData := range [][3]string{{"https://attacker.example", "", "token"}, {"", "cross-site", "token"}, {"%", "", "token"}, {"", "", ""}} {
		q.exportCalled = false
		resp = request(requestData[0], requestData[1], requestData[2])
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden || q.exportCalled {
			t.Fatalf("origin=%q fetch-site=%q token=%q status=%d exported=%v", requestData[0], requestData[1], requestData[2], resp.StatusCode, q.exportCalled)
		}
	}
}

func TestExport_RejectsConcurrentRequest(t *testing.T) {
	q := &blockingExportQuerier{
		fakeQuerier: &fakeQuerier{result: okResult()},
		started:     make(chan struct{}),
		release:     make(chan struct{}),
	}
	ts, _ := newTestServer(t, q)
	type responseResult struct {
		resp *http.Response
		err  error
	}
	first := make(chan responseResult, 1)
	go func() {
		resp, err := http.Post(ts.URL+"/api/export", "application/json", strings.NewReader(`{"sql":"SELECT 1"}`))
		first <- responseResult{resp: resp, err: err}
	}()
	<-q.started

	second := post(t, ts, "/api/export", `{"sql":"SELECT 2"}`)
	second.Body.Close()
	if second.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("second status=%d, want 429", second.StatusCode)
	}
	close(q.release)
	result := <-first
	if result.err != nil {
		t.Fatal(result.err)
	}
	result.resp.Body.Close()
	if result.resp.StatusCode != http.StatusOK {
		t.Fatalf("first status=%d", result.resp.StatusCode)
	}
}

func TestExport_CSVResolvesTruncatedCells(t *testing.T) {
	q := &fakeQuerier{
		result: &db.Result{
			Columns:        []string{"payload"},
			Rows:           [][]any{{"preview…"}},
			RowCount:       1,
			CellsTruncated: true,
			TruncatedCells: []db.CellRef{{Row: 0, Column: 0, Hash: db.CellHash("full value")}},
		},
		cellValue: "full value",
	}
	ts, _ := newTestServer(t, q)
	resp := post(t, ts, "/api/export", `{"sql":"SELECT payload"}`)
	defer resp.Body.Close()
	body := readGZIP(t, resp.Body)
	if !strings.Contains(string(body), "full value") || strings.Contains(string(body), "preview") {
		t.Fatalf("csv = %q", body)
	}
}

func readGZIP(t *testing.T, src io.Reader) []byte {
	t.Helper()
	reader, err := gzip.NewReader(src)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

type blockingExportQuerier struct {
	*fakeQuerier
	started chan struct{}
	release chan struct{}
}

func (q *blockingExportQuerier) ExportCSV(ctx context.Context, sql string, dst io.Writer) error {
	close(q.started)
	select {
	case <-q.release:
		return q.fakeQuerier.ExportCSV(ctx, sql, dst)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestExport_CSVRejectsChangedCellBeforeDownload(t *testing.T) {
	q := &fakeQuerier{
		result: &db.Result{
			Columns:        []string{"payload"},
			Rows:           [][]any{{"preview…"}},
			RowCount:       1,
			CellsTruncated: true,
			TruncatedCells: []db.CellRef{{Row: 0, Column: 0, Hash: db.CellHash("original")}},
		},
		cellValue: "changed",
	}
	ts, _ := newTestServer(t, q)
	resp := post(t, ts, "/api/export", `{"sql":"SELECT payload"}`)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
	got := decode[map[string]string](t, resp)
	if got["error"] != "The data changed. Reload and export again." {
		t.Fatalf("error = %q", got["error"])
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
	q := &fakeQuerier{err: errors.New("postgres://secret-host/hidden: boom")}
	ts, _ := newTestServer(t, q)

	// When: export is requested.
	resp := post(t, ts, "/api/export", `{"sql":"SELECT 1"}`)
	defer resp.Body.Close()

	// Then: handler returns the same bad-request contract as query execution.
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	got := decode[map[string]string](t, resp)
	if got["error"] != "Failed to export the data" {
		t.Fatalf("error = %q, want sanitized export failure", got["error"])
	}
}
