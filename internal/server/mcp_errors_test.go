package server

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/descope-sample-apps/pgpeek/internal/db"
)

func TestMCP_list_tables_reports_catalog_failure(t *testing.T) {
	// Given: the selected database cannot read its catalog.
	q := &fakeQuerier{catErr: errors.New("catalog secret")}
	session := connectMCP(t, newMCPTestServer(t, q))

	// When: the client lists tables.
	result := callMCPTool(t, session, "list_tables", map[string]any{})

	// Then: the tool reports a stable public error.
	if !result.IsError || mcpText(t, result) != "failed to list tables" {
		t.Fatalf("result = %+v", result)
	}
}

func TestMCP_describe_table_reports_column_failure(t *testing.T) {
	// Given: reading relation columns fails.
	q := &describeErrorQuerier{fakeQuerier: &fakeQuerier{}, columnsErr: errors.New("column secret")}
	session := connectMCP(t, newMCPTestServer(t, q))

	// When: the client describes a table.
	result := callMCPTool(t, session, "describe_table", map[string]any{"schema": "public", "table": "events"})

	// Then: the internal error is sanitized.
	if !result.IsError || mcpText(t, result) != "failed to read columns" {
		t.Fatalf("result = %+v", result)
	}
}

func TestMCP_describe_table_reports_foreign_key_failure(t *testing.T) {
	// Given: columns load but foreign-key inspection fails.
	q := &describeErrorQuerier{fakeQuerier: &fakeQuerier{cols: []db.ColumnInfo{}}, foreignKeysErr: errors.New("foreign key secret")}
	session := connectMCP(t, newMCPTestServer(t, q))

	// When: the client describes a table.
	result := callMCPTool(t, session, "describe_table", map[string]any{"schema": "public", "table": "events"})

	// Then: the internal error is sanitized.
	if !result.IsError || mcpText(t, result) != "failed to read foreign keys" {
		t.Fatalf("result = %+v", result)
	}
}

func TestMCP_describe_table_reports_unknown_database(t *testing.T) {
	// Given: no configured database matches the requested ID.
	session := connectMCP(t, newMCPTestServer(t, &fakeQuerier{}))

	// When: describe_table targets the unknown database.
	result := callMCPTool(t, session, "describe_table", map[string]any{
		"databaseId": "missing",
		"schema":     "public",
		"table":      "events",
	})

	// Then: the model receives an actionable selection error.
	if !result.IsError || !strings.Contains(mcpText(t, result), `database "missing" not found`) {
		t.Fatalf("result = %+v", result)
	}
}

func TestMCP_describe_table_rejects_empty_relation_name(t *testing.T) {
	// Given: pgpeek has a healthy default database.
	session := connectMCP(t, newMCPTestServer(t, &fakeQuerier{}))

	// When: describe_table receives an empty schema name.
	result := callMCPTool(t, session, "describe_table", map[string]any{"schema": "  ", "table": "events"})

	// Then: input is rejected before the catalog is queried.
	if !result.IsError || mcpText(t, result) != "schema and table are required" {
		t.Fatalf("result = %+v", result)
	}
}

func TestMCP_query_reports_unknown_database(t *testing.T) {
	// Given: no configured database matches the requested ID.
	session := connectMCP(t, newMCPTestServer(t, &fakeQuerier{}))

	// When: query targets the unknown database.
	result := callMCPTool(t, session, "query", map[string]any{"databaseId": "missing", "sql": "SELECT 1"})

	// Then: the query is not executed.
	if !result.IsError || !strings.Contains(mcpText(t, result), `database "missing" not found`) {
		t.Fatalf("result = %+v", result)
	}
}

func TestMCP_query_reports_sanitized_database_failure(t *testing.T) {
	// Given: executing SQL fails with an internal error.
	q := &fakeQuerier{err: errors.New("query secret")}
	session := connectMCP(t, newMCPTestServer(t, q))

	// When: the client runs a valid read-only query.
	result := callMCPTool(t, session, "query", map[string]any{"sql": "SELECT 1"})

	// Then: the internal error is not exposed.
	if !result.IsError || mcpText(t, result) != "query failed" {
		t.Fatalf("result = %+v", result)
	}
}

func TestMCP_query_normalizes_nil_result_slices(t *testing.T) {
	// Given: a query result contains no rows or columns.
	q := &fakeQuerier{result: &db.Result{}}
	session := connectMCP(t, newMCPTestServer(t, q))

	// When: the result is returned through MCP structured content.
	result := callMCPTool(t, session, "query", map[string]any{"sql": "SELECT 1 WHERE false"})
	got := decodeMCPOutput[struct {
		Columns []string `json:"columns"`
		Rows    [][]any  `json:"rows"`
	}](t, result)

	// Then: arrays are empty rather than null and satisfy the output schema.
	if got.Columns == nil || got.Rows == nil || len(got.Columns) != 0 || len(got.Rows) != 0 {
		t.Fatalf("output = %+v", got)
	}
}

func newMCPTestServer(t *testing.T, q Querier) *httptest.Server {
	t.Helper()
	return newRegistryTestServer(t, fakeRegistry{
		defaultID: "primary",
		metadata:  []db.PoolMetadata{{ID: "primary", Name: "Primary"}},
		pools:     map[string]Querier{"primary": q},
	})
}

type describeErrorQuerier struct {
	*fakeQuerier
	columnsErr     error
	foreignKeysErr error
}

func (q *describeErrorQuerier) Columns(_ context.Context, _, _ string) ([]db.ColumnInfo, error) {
	if q.columnsErr != nil {
		return nil, q.columnsErr
	}
	return q.cols, nil
}

func (q *describeErrorQuerier) ForeignKeys(_ context.Context, _, _ string) ([]db.ForeignKey, error) {
	if q.foreignKeysErr != nil {
		return nil, q.foreignKeysErr
	}
	return q.fks, nil
}
