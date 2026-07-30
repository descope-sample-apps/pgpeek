package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/descope-sample-apps/pgpeek/internal/db"
)

func TestMCP_lists_simple_read_only_tools(t *testing.T) {
	// Given: pgpeek is serving its MCP endpoint.
	ts := newRegistryTestServer(t, fakeRegistry{
		defaultID: "primary",
		metadata:  []db.PoolMetadata{{ID: "primary", Name: "Primary"}},
		pools:     map[string]Querier{"primary": &fakeQuerier{}},
	})
	session := connectMCP(t, ts)

	// When: an MCP client discovers the available tools.
	result, err := session.ListTools(context.Background(), nil)
	// Then: pgpeek exposes only the four read-only database tools.
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	want := map[string]bool{
		"list_databases": false,
		"list_tables":    false,
		"describe_table": false,
		"query":          false,
	}
	for _, tool := range result.Tools {
		if _, ok := want[tool.Name]; !ok {
			t.Fatalf("unexpected tool %q", tool.Name)
		}
		want[tool.Name] = true
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || !tool.Annotations.IdempotentHint {
			t.Fatalf("tool %q annotations = %+v", tool.Name, tool.Annotations)
		}
		if tool.OutputSchema == nil {
			t.Fatalf("tool %q has no output schema", tool.Name)
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("tool %q was not advertised", name)
		}
	}
}

func TestMCP_initialize_reportsConfiguredVersion(t *testing.T) {
	// Given: pgpeek is built with a release version.
	ts := newRegistryTestServer(t, fakeRegistry{
		defaultID: "primary",
		metadata:  []db.PoolMetadata{{ID: "primary", Name: "Primary"}},
		pools:     map[string]Querier{"primary": &fakeQuerier{}},
	}, Version("v1.2.3"))

	// When: an MCP client initializes a session.
	session := connectMCP(t, ts)

	// Then: server metadata exposes the build version.
	result := session.InitializeResult()
	if result == nil || result.ServerInfo == nil || result.ServerInfo.Version != "v1.2.3" {
		t.Fatalf("initialize result = %+v", result)
	}
}

func TestMCP_list_databases_returns_safe_registry_metadata(t *testing.T) {
	// Given: pgpeek has two configured databases.
	ts := newRegistryTestServer(t, fakeRegistry{
		defaultID: "primary",
		metadata: []db.PoolMetadata{
			{ID: "primary", Name: "Primary"},
			{ID: "analytics", Name: "Analytics"},
		},
		pools: map[string]Querier{"primary": &fakeQuerier{}, "analytics": &fakeQuerier{}},
	})
	session := connectMCP(t, ts)

	// When: the client calls list_databases.
	result := callMCPTool(t, session, "list_databases", map[string]any{})

	// Then: only safe IDs and display names are returned.
	got := decodeMCPOutput[struct {
		DefaultDatabaseID string            `json:"defaultDatabaseId"`
		Databases         []db.PoolMetadata `json:"databases"`
	}](t, result)
	if got.DefaultDatabaseID != "primary" || len(got.Databases) != 2 || got.Databases[1].Name != "Analytics" {
		t.Fatalf("output = %+v", got)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(encoded), "postgres://") || strings.Contains(strings.ToLower(string(encoded)), "dsn") {
		t.Fatalf("database metadata leaked secret material: %s", encoded)
	}
}

func TestMCP_list_tables_uses_selected_database(t *testing.T) {
	// Given: the selected database has one table.
	primary := &fakeQuerier{}
	analytics := &fakeQuerier{tables: []db.TableInfo{{Schema: "public", Name: "events", Type: "table", EstRows: 42}}}
	ts := newRegistryTestServer(t, fakeRegistry{
		defaultID: "primary",
		metadata:  []db.PoolMetadata{{ID: "primary", Name: "Primary"}, {ID: "analytics", Name: "Analytics"}},
		pools:     map[string]Querier{"primary": primary, "analytics": analytics},
	})
	session := connectMCP(t, ts)

	// When: list_tables selects analytics explicitly.
	result := callMCPTool(t, session, "list_tables", map[string]any{"databaseId": "analytics"})

	// Then: the analytics catalog is returned with the resolved database ID.
	got := decodeMCPOutput[struct {
		DatabaseID string         `json:"databaseId"`
		Tables     []db.TableInfo `json:"tables"`
	}](t, result)
	if got.DatabaseID != "analytics" || len(got.Tables) != 1 || got.Tables[0].Name != "events" {
		t.Fatalf("output = %+v", got)
	}
}

func TestMCP_describe_table_returns_columns_and_foreign_keys(t *testing.T) {
	// Given: a table has a column and a foreign key.
	q := &fakeQuerier{
		cols: []db.ColumnInfo{{Name: "account_id", Type: "uuid", Nullable: false}},
		fks:  []db.ForeignKey{{Column: "account_id", RefSchema: "public", RefTable: "accounts", RefColumn: "id"}},
	}
	ts := newRegistryTestServer(t, fakeRegistry{
		defaultID: "primary",
		metadata:  []db.PoolMetadata{{ID: "primary", Name: "Primary"}},
		pools:     map[string]Querier{"primary": q},
	})
	session := connectMCP(t, ts)

	// When: the client describes the table.
	result := callMCPTool(t, session, "describe_table", map[string]any{"schema": "public", "table": "events"})

	// Then: the relation metadata is returned as structured content.
	got := decodeMCPOutput[struct {
		DatabaseID  string          `json:"databaseId"`
		Schema      string          `json:"schema"`
		Table       string          `json:"table"`
		Columns     []db.ColumnInfo `json:"columns"`
		ForeignKeys []db.ForeignKey `json:"foreignKeys"`
	}](t, result)
	if got.DatabaseID != "primary" || got.Schema != "public" || got.Table != "events" {
		t.Fatalf("relation identity = %+v", got)
	}
	if len(got.Columns) != 1 || len(got.ForeignKeys) != 1 || got.ForeignKeys[0].RefTable != "accounts" {
		t.Fatalf("relation metadata = %+v", got)
	}
}

func TestMCP_query_enforces_read_only_SQL_and_returns_row_capped_result(t *testing.T) {
	// Given: the selected database returns one row.
	q := &fakeQuerier{result: okResult()}
	ts := newRegistryTestServer(t, fakeRegistry{
		defaultID: "primary",
		metadata:  []db.PoolMetadata{{ID: "primary", Name: "Primary"}},
		pools:     map[string]Querier{"primary": q},
	})
	session := connectMCP(t, ts)

	// When: the client runs a read-only query.
	result := callMCPTool(t, session, "query", map[string]any{"sql": "  SELECT 1 AS n  "})

	// Then: the trimmed query is executed and its bounded result is structured.
	got := decodeMCPOutput[struct {
		DatabaseID string   `json:"databaseId"`
		Columns    []string `json:"columns"`
		Rows       [][]any  `json:"rows"`
		RowCount   int      `json:"rowCount"`
		Truncated  bool     `json:"truncated"`
		ElapsedMS  int64    `json:"elapsedMs"`
	}](t, result)
	if q.lastSQL != "SELECT 1 AS n" || got.DatabaseID != "primary" || got.RowCount != 1 || len(got.Rows) != 1 {
		t.Fatalf("sql=%q output=%+v", q.lastSQL, got)
	}

	// When: the client attempts a write.
	rejected := callMCPTool(t, session, "query", map[string]any{"sql": "DELETE FROM users"})

	// Then: the tool reports an actionable error without executing it.
	if !rejected.IsError || !strings.Contains(mcpText(t, rejected), "read-only") {
		t.Fatalf("rejected result = %+v", rejected)
	}
	if q.lastSQL != "SELECT 1 AS n" {
		t.Fatalf("write query reached database: %q", q.lastSQL)
	}
}

func TestMCP_tool_errors_are_actionable_and_sanitized(t *testing.T) {
	// Given: registry selection and database access can fail.
	registryErr := errors.New("registry secret")
	ts := newRegistryTestServer(t, failingRegistry{err: registryErr})
	session := connectMCP(t, ts)

	// When: a tool cannot select a database.
	result := callMCPTool(t, session, "list_tables", map[string]any{})

	// Then: the client sees a retryable public error, not the internal failure.
	if !result.IsError || !strings.Contains(mcpText(t, result), "database unavailable") || strings.Contains(mcpText(t, result), registryErr.Error()) {
		t.Fatalf("result = %+v", result)
	}
}

func TestMCP_rejects_cross_origin_requests(t *testing.T) {
	// Given: pgpeek is serving an MCP endpoint without an auth layer.
	ts := newRegistryTestServer(t, fakeRegistry{
		defaultID: "primary",
		metadata:  []db.PoolMetadata{{ID: "primary", Name: "Primary"}},
		pools:     map[string]Querier{"primary": &fakeQuerier{}},
	})
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Origin", "https://attacker.example")

	// When: a browser-originated cross-site request reaches the endpoint.
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	// Then: origin protection rejects it before MCP processes the request.
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestMCP_rejects_oversized_requests(t *testing.T) {
	// Given: pgpeek is serving the unauthenticated MCP endpoint.
	ts := newRegistryTestServer(t, fakeRegistry{
		defaultID: "primary",
		metadata:  []db.PoolMetadata{{ID: "primary", Name: "Primary"}},
		pools:     map[string]Querier{"primary": &fakeQuerier{}},
	})
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader(strings.Repeat("x", maxMCPRequestBytes+1)))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	// When: a client sends more than the endpoint's request-body limit.
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	// Then: MCP rejects it before parsing or executing a tool call.
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}
