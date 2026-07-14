package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

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
		rows[i] = []any{strings.Repeat("x", maxMCPQueryResultBytes/4)}
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
	if len(encoded) > maxMCPQueryResultBytes {
		t.Fatalf("encoded size = %d, limit = %d", len(encoded), maxMCPQueryResultBytes)
	}
	if !got.Truncated || got.RowCount != len(got.Rows) || len(got.Rows) >= len(rows) {
		t.Fatalf("capped output = %+v", got)
	}
}

func TestMCP_query_rejects_oversized_metadata(t *testing.T) {
	// Given: query metadata alone cannot fit within the MCP response budget.
	output := mcpQueryOutput{Columns: []string{strings.Repeat("x", maxMCPQueryResultBytes)}}

	// When: the MCP-specific response cap is applied.
	_, err := capMCPQueryOutput(output)

	// Then: the server rejects the result instead of emitting an oversized response.
	if err == nil || !strings.Contains(err.Error(), "metadata exceeds") {
		t.Fatalf("error = %v", err)
	}
}
