package server

import (
	"bufio"
	"bytes"
	"io"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	mcpInstructions    = "pgpeek provides read-only access to configured PostgreSQL databases. List databases and tables before querying unfamiliar schemas. Queries must be a single read-only statement and results are capped by the server's row limit."
	maxMCPRequestBytes = 32 << 10
)

func (s *Server) mcpHandler() http.Handler {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "pgpeek",
		Title:   "pgpeek PostgreSQL browser",
		Version: s.version,
	}, &mcp.ServerOptions{
		Instructions: mcpInstructions,
		Capabilities: &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}},
	})

	annotations := &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_databases",
		Title:       "List databases",
		Description: "List the configured pgpeek database IDs and display names. No credentials or connection strings are returned.",
		Annotations: annotations,
	}, s.mcpListDatabases)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_tables",
		Title:       "List tables",
		Description: "List user-facing tables and views in a configured database. Omit databaseId to use the default database.",
		Annotations: annotations,
	}, s.mcpListTables)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "describe_table",
		Title:       "Describe table",
		Description: "Return columns and single-column foreign keys for a table or view. Omit databaseId to use the default database.",
		Annotations: annotations,
	}, s.mcpDescribeTable)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "query",
		Title:       "Run read-only SQL",
		Description: "Run one read-only SELECT, WITH, VALUES, TABLE, or EXPLAIN statement. Writes and multiple statements are rejected; results are row-capped.",
		Annotations: annotations,
	}, s.mcpQuery)

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{
		Stateless:    true,
		JSONResponse: true,
		Logger:       s.log,
	})
	return http.NewCrossOriginProtection().Handler(http.MaxBytesHandler(singleMCPMessage(handler), maxMCPRequestBytes))
}

func singleMCPMessage(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.Body != nil {
			body := bufio.NewReader(r.Body)
			prefix, first, err := firstJSONByte(body)
			if err != nil {
				http.Error(w, "failed to read request", http.StatusBadRequest)
				return
			}
			if first == '[' {
				http.Error(w, "JSON-RPC batch requests are not supported", http.StatusBadRequest)
				return
			}
			r.Body = struct {
				io.Reader
				io.Closer
			}{Reader: io.MultiReader(bytes.NewReader(prefix), body), Closer: r.Body}
		}
		next.ServeHTTP(w, r)
	})
}

func firstJSONByte(body *bufio.Reader) ([]byte, byte, error) {
	prefix := make([]byte, 0, 8)
	for {
		value, err := body.ReadByte()
		if err != nil {
			if err == io.EOF {
				return prefix, 0, nil
			}
			return nil, 0, err
		}
		prefix = append(prefix, value)
		if value != ' ' && value != '\t' && value != '\r' && value != '\n' {
			return prefix, value, nil
		}
	}
}
