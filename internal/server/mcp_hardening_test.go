package server

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/descope-sample-apps/pgpeek/internal/db"
)

func TestMCP_rejects_JSON_RPC_batches(t *testing.T) {
	// Given: pgpeek is serving its stateless MCP endpoint.
	ts := newRegistryTestServer(t, fakeRegistry{
		defaultID: "primary",
		metadata:  []db.PoolMetadata{{ID: "primary", Name: "Primary"}},
		pools:     map[string]Querier{"primary": &fakeQuerier{}},
	})
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader(" \n\t"+`[{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}]`))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	// When: a client attempts to multiplex calls in one HTTP request.
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	// Then: the request is rejected before protocol dispatch.
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestMCP_query_caps_serialized_output(t *testing.T) {
	// Given: a row-capped database result still contains oversized values.
	rows := make([][]any, 8)
	for i := range rows {
		rows[i] = []any{strings.Repeat("x", maxMCPStructuredContentBytes/4)}
	}
	output := mcpQueryOutput{Columns: []string{"payload"}, Rows: rows, RowCount: len(rows)}

	// When: the MCP-specific response cap is applied.
	got, err := capMCPQueryOutput(output)
	if err != nil {
		t.Fatalf("capMCPQueryOutput: %v", err)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// Then: the response fits the byte budget and reports truncation accurately.
	if len(encoded) > maxMCPStructuredContentBytes {
		t.Fatalf("encoded size = %d, limit = %d", len(encoded), maxMCPStructuredContentBytes)
	}
	if !got.Truncated || got.RowCount != len(got.Rows) || len(got.Rows) >= len(rows) {
		t.Fatalf("capped output = %+v", got)
	}
}

func TestMCP_query_rejects_oversized_metadata(t *testing.T) {
	// Given: query metadata alone cannot fit within the MCP response budget.
	output := mcpQueryOutput{Columns: []string{strings.Repeat("x", maxMCPStructuredContentBytes)}}

	// When: the MCP-specific response cap is applied.
	_, err := capMCPQueryOutput(output)

	// Then: the server rejects the result instead of emitting an oversized response.
	if err == nil || !strings.Contains(err.Error(), "metadata exceeds") {
		t.Fatalf("error = %v", err)
	}
}

func TestMCP_list_databases_caps_serialized_output(t *testing.T) {
	// Given: configured database display names exceed the MCP response budget.
	metadata := make([]db.PoolMetadata, 8)
	for i := range metadata {
		metadata[i] = db.PoolMetadata{ID: "database", Name: strings.Repeat("x", maxMCPStructuredContentBytes/4)}
	}
	session := connectMCP(t, newRegistryTestServer(t, fakeRegistry{
		defaultID: "database",
		metadata:  metadata,
		pools:     map[string]Querier{},
	}))

	// When: the client discovers configured databases.
	result := callMCPTool(t, session, "list_databases", map[string]any{})
	got := decodeMCPOutput[struct {
		Databases []db.PoolMetadata `json:"databases"`
		Truncated bool              `json:"truncated"`
	}](t, result)

	// Then: the structured result is bounded and explicitly reports truncation.
	if !got.Truncated || len(got.Databases) >= len(metadata) {
		t.Fatalf("databases=%d truncated=%v", len(got.Databases), got.Truncated)
	}
	assertMCPResultWithinBudget(t, result)
}

func TestMCP_list_tables_caps_serialized_output(t *testing.T) {
	// Given: a database catalog contains oversized relation names.
	tables := make([]db.TableInfo, 8)
	for i := range tables {
		tables[i] = db.TableInfo{Schema: "public", Name: strings.Repeat("x", maxMCPStructuredContentBytes/4), Type: "table"}
	}
	session := connectMCP(t, newMCPTestServer(t, &fakeQuerier{tables: tables}))

	// When: the client lists tables.
	result := callMCPTool(t, session, "list_tables", map[string]any{})
	got := decodeMCPOutput[struct {
		Tables    []db.TableInfo `json:"tables"`
		Truncated bool           `json:"truncated"`
	}](t, result)

	// Then: the structured result is bounded and explicitly reports truncation.
	if !got.Truncated || len(got.Tables) >= len(tables) {
		t.Fatalf("tables=%d truncated=%v", len(got.Tables), got.Truncated)
	}
	assertMCPResultWithinBudget(t, result)
}

func TestMCP_list_tables_preserves_database_truncation(t *testing.T) {
	session := connectMCP(t, newMCPTestServer(t, &fakeQuerier{catTruncated: true}))
	result := callMCPTool(t, session, "list_tables", map[string]any{})
	got := decodeMCPOutput[struct {
		Truncated bool `json:"truncated"`
	}](t, result)
	if !got.Truncated {
		t.Fatal("truncated = false, want true")
	}
}

func TestMCP_describe_table_caps_serialized_output(t *testing.T) {
	// Given: relation metadata contains more columns and foreign keys than fit.
	largeValue := strings.Repeat("x", maxMCPStructuredContentBytes/4)
	columns := make([]db.ColumnInfo, 8)
	foreignKeys := make([]db.ForeignKey, 8)
	for i := range columns {
		columns[i] = db.ColumnInfo{Name: largeValue, Type: "text", Default: &largeValue}
		foreignKeys[i] = db.ForeignKey{Column: "id", RefSchema: "public", RefTable: largeValue, RefColumn: "id"}
	}
	session := connectMCP(t, newMCPTestServer(t, &fakeQuerier{cols: columns, fks: foreignKeys}))

	// When: the client describes the relation.
	result := callMCPTool(t, session, "describe_table", map[string]any{"schema": "public", "table": "events"})
	got := decodeMCPOutput[struct {
		Columns     []db.ColumnInfo `json:"columns"`
		ForeignKeys []db.ForeignKey `json:"foreignKeys"`
		Truncated   bool            `json:"truncated"`
	}](t, result)

	// Then: metadata is bounded and explicitly reports omitted entries.
	if !got.Truncated || len(got.Columns)+len(got.ForeignKeys) >= len(columns)+len(foreignKeys) {
		t.Fatalf("output columns=%d foreignKeys=%d truncated=%v", len(got.Columns), len(got.ForeignKeys), got.Truncated)
	}
	assertMCPResultWithinBudget(t, result)
}

func TestSingleMCPMessage_reports_body_read_error(t *testing.T) {
	// Given: an MCP POST body fails while the batch guard reads its prefix.
	nextCalled := false
	handler := singleMCPMessage(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		nextCalled = true
	}))
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Body = io.NopCloser(errorReader{err: errors.New("read failed")})
	rec := httptest.NewRecorder()

	// When: the guard inspects the request.
	handler.ServeHTTP(rec, req)

	// Then: it returns a sanitized 400 without dispatching the request.
	if rec.Code != http.StatusBadRequest || nextCalled {
		t.Fatalf("status=%d nextCalled=%v", rec.Code, nextCalled)
	}
}

func TestFirstJSONByte_accepts_empty_body(t *testing.T) {
	// Given: an empty request body.
	body := bufio.NewReader(strings.NewReader(""))

	// When: the batch guard searches for the first JSON byte.
	prefix, first, err := firstJSONByte(body)

	// Then: EOF is treated as an empty payload for the MCP handler to reject.
	if err != nil || len(prefix) != 0 || first != 0 {
		t.Fatalf("prefix=%q first=%q err=%v", prefix, first, err)
	}
}

func TestMCP_query_reports_response_limit_error(t *testing.T) {
	// Given: query metadata alone exceeds the MCP response budget.
	q := &fakeQuerier{result: &db.Result{Columns: []string{strings.Repeat("x", maxMCPStructuredContentBytes)}}}
	session := connectMCP(t, newMCPTestServer(t, q))

	// When: the client executes a valid query.
	result := callMCPTool(t, session, "query", map[string]any{"sql": "SELECT 1"})

	// Then: the handler returns a tool error instead of an oversized result.
	if !result.IsError || !strings.Contains(mcpText(t, result), "metadata exceeds") {
		t.Fatalf("result = %+v", result)
	}
}

func TestMCP_query_caps_database_error_on_wire(t *testing.T) {
	// Given: PostgreSQL returns an attacker-controlled oversized error message.
	q := &fakeQuerier{err: &pgconn.PgError{Message: strings.Repeat("x", maxMCPStructuredContentBytes)}}
	session := connectMCP(t, newMCPTestServer(t, q))

	// When: the client executes a valid query.
	result := callMCPTool(t, session, "query", map[string]any{"sql": "SELECT fail()"})

	// Then: the tool error remains within the MCP wire budget.
	if !result.IsError {
		t.Fatalf("result = %+v", result)
	}
	assertMCPResultWithinBudget(t, result)
}

func assertMCPResultWithinBudget(t *testing.T, result any) {
	t.Helper()
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal result: %v", err)
	}
	if len(encoded) > maxMCPResponseBytes-maxMCPRequestBytes {
		t.Fatalf("encoded result size = %d, result budget = %d", len(encoded), maxMCPResponseBytes-maxMCPRequestBytes)
	}
}

type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) {
	return 0, r.err
}
