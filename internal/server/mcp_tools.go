package server

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/descope-sample-apps/pgpeek/internal/db"
	"github.com/descope-sample-apps/pgpeek/internal/guard"
)

var (
	errMCPDatabaseUnavailable = errors.New("database unavailable")
	errMCPRelationRequired    = errors.New("schema and table are required")
)

const maxMCPErrorBytes = 4 << 10

type mcpDatabaseInput struct {
	DatabaseID string `json:"databaseId,omitempty" jsonschema:"configured database ID; omit to use the default database"`
}

type mcpListDatabasesInput struct{}

type mcpListDatabasesOutput struct {
	DefaultDatabaseID string            `json:"defaultDatabaseId" jsonschema:"database ID used when a tool call omits databaseId"`
	Databases         []db.PoolMetadata `json:"databases" jsonschema:"configured database IDs and display names"`
	Truncated         bool              `json:"truncated" jsonschema:"whether configured databases were omitted by the response-size cap"`
}

type mcpListTablesOutput struct {
	DatabaseID string         `json:"databaseId" jsonschema:"database queried"`
	Tables     []db.TableInfo `json:"tables" jsonschema:"user-facing tables and views"`
	Truncated  bool           `json:"truncated" jsonschema:"whether tables were omitted by the response-size cap"`
}

type mcpDescribeTableInput struct {
	DatabaseID string `json:"databaseId,omitempty" jsonschema:"configured database ID; omit to use the default database"`
	Schema     string `json:"schema" jsonschema:"PostgreSQL schema name"`
	Table      string `json:"table" jsonschema:"table or view name"`
}

type mcpDescribeTableOutput struct {
	DatabaseID  string          `json:"databaseId" jsonschema:"database queried"`
	Schema      string          `json:"schema" jsonschema:"PostgreSQL schema name"`
	Table       string          `json:"table" jsonschema:"table or view name"`
	Columns     []db.ColumnInfo `json:"columns" jsonschema:"columns in ordinal order"`
	ForeignKeys []db.ForeignKey `json:"foreignKeys" jsonschema:"single-column foreign keys"`
	Truncated   bool            `json:"truncated" jsonschema:"whether columns or foreign keys were omitted by the response-size cap"`
}

type mcpQueryInput struct {
	DatabaseID string `json:"databaseId,omitempty" jsonschema:"configured database ID; omit to use the default database"`
	SQL        string `json:"sql" jsonschema:"one read-only PostgreSQL statement"`
}

type mcpQueryOutput struct {
	DatabaseID string   `json:"databaseId" jsonschema:"database queried"`
	Columns    []string `json:"columns" jsonschema:"result column names in row order"`
	Rows       [][]any  `json:"rows" jsonschema:"row values aligned with columns"`
	RowCount   int      `json:"rowCount" jsonschema:"number of rows returned"`
	Truncated  bool     `json:"truncated" jsonschema:"whether rows were omitted by the server row or response-size cap"`
	ElapsedMS  int64    `json:"elapsedMs" jsonschema:"database execution and collection time in milliseconds"`
}

func (s *Server) mcpListDatabases(context.Context, *mcp.CallToolRequest, mcpListDatabasesInput) (*mcp.CallToolResult, mcpListDatabasesOutput, error) {
	output, err := capMCPListDatabasesOutput(mcpListDatabasesOutput{
		DefaultDatabaseID: s.registry.DefaultID(),
		Databases:         nonNil(s.registry.List()),
	})
	return mcpStructuredOutput("Database list completed. The result is available in structuredContent.", output, err)
}

func (s *Server) mcpListTables(ctx context.Context, _ *mcp.CallToolRequest, input mcpDatabaseInput) (*mcp.CallToolResult, mcpListTablesOutput, error) {
	databaseID, pool, err := s.mcpPool(input.DatabaseID)
	if err != nil {
		return nil, mcpListTablesOutput{}, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, s.queryWait)
	defer cancel()
	tables, truncated, err := pool.Tables(queryCtx)
	if err != nil {
		s.log.Error("mcp list tables", "databaseID", databaseID, "err", err)
		return nil, mcpListTablesOutput{}, errors.New("failed to list tables")
	}
	output, err := capMCPListTablesOutput(mcpListTablesOutput{DatabaseID: databaseID, Tables: nonNil(tables), Truncated: truncated})
	return mcpStructuredOutput("Table list completed. The result is available in structuredContent.", output, err)
}

func (s *Server) mcpDescribeTable(ctx context.Context, _ *mcp.CallToolRequest, input mcpDescribeTableInput) (*mcp.CallToolResult, mcpDescribeTableOutput, error) {
	databaseID, pool, err := s.mcpPool(input.DatabaseID)
	if err != nil {
		return nil, mcpDescribeTableOutput{}, err
	}
	schema := strings.TrimSpace(input.Schema)
	table := strings.TrimSpace(input.Table)
	if schema == "" || table == "" {
		return nil, mcpDescribeTableOutput{}, errMCPRelationRequired
	}
	queryCtx, cancel := context.WithTimeout(ctx, s.queryWait)
	defer cancel()
	columns, columnsTruncated, err := pool.Columns(queryCtx, schema, table)
	if err != nil {
		s.log.Error("mcp describe table columns", "databaseID", databaseID, "err", err)
		return nil, mcpDescribeTableOutput{}, errors.New("failed to read columns")
	}
	foreignKeys, foreignKeysTruncated, err := pool.ForeignKeys(queryCtx, schema, table)
	if err != nil {
		s.log.Error("mcp describe table foreign keys", "databaseID", databaseID, "err", err)
		return nil, mcpDescribeTableOutput{}, errors.New("failed to read foreign keys")
	}
	output, err := capMCPDescribeTableOutput(mcpDescribeTableOutput{
		DatabaseID:  databaseID,
		Schema:      schema,
		Table:       table,
		Columns:     nonNil(columns),
		ForeignKeys: nonNil(foreignKeys),
		Truncated:   columnsTruncated || foreignKeysTruncated,
	})
	return mcpStructuredOutput("Table description completed. The result is available in structuredContent.", output, err)
}

func (s *Server) mcpQuery(ctx context.Context, _ *mcp.CallToolRequest, input mcpQueryInput) (*mcp.CallToolResult, mcpQueryOutput, error) {
	databaseID, pool, err := s.mcpPool(input.DatabaseID)
	if err != nil {
		return nil, mcpQueryOutput{}, err
	}
	sql := strings.TrimSpace(input.SQL)
	if err := guard.Validate(sql); err != nil {
		return nil, mcpQueryOutput{}, fmt.Errorf("validate query: %w", err)
	}
	queryCtx, cancel := context.WithTimeout(ctx, s.queryWait)
	defer cancel()
	result, err := pool.Query(queryCtx, sql)
	if err != nil {
		s.log.Error("mcp query", "databaseID", databaseID, "err", err)
		message := queryErrorMessage(err)
		if len(message) > maxMCPErrorBytes {
			message = "database query failed: error detail exceeds limit"
		}
		return nil, mcpQueryOutput{}, errors.New(message)
	}
	output := mcpQueryOutput{
		DatabaseID: databaseID,
		Columns:    nonNil(result.Columns),
		Rows:       nonNil(result.Rows),
		RowCount:   result.RowCount,
		Truncated:  result.Truncated,
		ElapsedMS:  result.ElapsedMS,
	}
	output, err = capMCPQueryOutput(output)
	return mcpStructuredOutput("Query completed. The result is available in structuredContent.", output, err)
}

func (s *Server) mcpPool(id string) (string, Querier, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		id = s.registry.DefaultID()
	}
	pool, err := s.registry.Pool(id)
	if errors.Is(err, db.ErrPoolNotFound) {
		return "", nil, fmt.Errorf("database %q not found", id)
	}
	if err != nil {
		s.log.Error("mcp select database", "databaseID", id, "err", err)
		return "", nil, errMCPDatabaseUnavailable
	}
	return id, pool, nil
}

func nonNil[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
}
