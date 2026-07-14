// Package server wires the HTTP and MCP handlers for read-only PostgreSQL
// browsing, saved-query CRUD, CSV export, the static UI, and k8s probes.
package server

import (
	"io/fs"
	"log/slog"
	"net/http"
	"time"
)

// Server holds the dependencies for the HTTP handlers.
type Server struct {
	registry                DatabaseRegistry
	store                   QueryStore
	web                     fs.FS
	log                     *slog.Logger
	queryWait               time.Duration
	requireCloudflareAccess bool
	version                 string
}

type Option func(*Server)

func RequireCloudflareAccess(require bool) Option {
	return func(s *Server) { s.requireCloudflareAccess = require }
}

func Version(version string) Option {
	return func(s *Server) { s.version = version }
}

// New constructs a Server.
func New(pool Querier, st QueryStore, web fs.FS, log *slog.Logger, queryWait time.Duration, opts ...Option) *Server {
	return NewWithRegistry(NewSingleDatabaseRegistry(pool), st, web, log, queryWait, opts...)
}

func NewWithRegistry(registry DatabaseRegistry, st QueryStore, web fs.FS, log *slog.Logger, queryWait time.Duration, opts ...Option) *Server {
	s := &Server{registry: registry, store: st, web: web, log: log, queryWait: queryWait, version: "dev"}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Routes returns the configured handler.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// Probes
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /readyz", s.handleReady)

	// API
	mux.HandleFunc("GET /api/user", s.handleUser)
	mux.HandleFunc("GET /api/databases", s.handleDatabases)
	mux.HandleFunc("GET /api/meta", s.handleMeta)
	mux.HandleFunc("POST /api/query", s.handleQuery)
	mux.HandleFunc("POST /api/export", s.handleExport)
	mux.HandleFunc("GET /api/tables", s.handleTables)
	mux.HandleFunc("GET /api/tables/{schema}/{table}/columns", s.handleColumns)
	mux.HandleFunc("GET /api/tables/{schema}/{table}/fks", s.handleForeignKeys)
	mux.HandleFunc("GET /api/tables/{schema}/{table}/data", s.handleTableData)
	mux.HandleFunc("GET /api/queries", s.handleListQueries)
	mux.HandleFunc("POST /api/queries", s.handleCreateQuery)
	mux.HandleFunc("PUT /api/queries/{id}", s.handleUpdateQuery)
	mux.HandleFunc("DELETE /api/queries/{id}", s.handleDeleteQuery)

	// MCP (stateless Streamable HTTP, no MCP auth layer)
	mcpHandler := s.mcpHandler()
	mux.Handle("GET /mcp", mcpHandler)
	mux.Handle("POST /mcp", mcpHandler)
	mux.Handle("DELETE /mcp", mcpHandler)

	// Static UI
	mux.Handle("GET /", http.FileServerFS(s.web))

	return securityHeaders(logging(s.log, requireCloudflareAccess(s.requireCloudflareAccess, mux)))
}

func (s *Server) handleUser(w http.ResponseWriter, r *http.Request) {
	u := cloudflareAccessUser(r)
	writeJSON(w, http.StatusOK, map[string]string{"provider": u.Provider, "email": u.Email})
}
