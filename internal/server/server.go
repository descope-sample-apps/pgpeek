// Package server wires the HTTP handlers: the SQL query endpoint (guarded +
// capped), saved-query CRUD, CSV export, the static UI, and k8s probes.
package server

import (
	"io/fs"
	"log/slog"
	"net/http"
	"time"
)

// Server holds the dependencies for the HTTP handlers.
type Server struct {
	registry  DatabaseRegistry
	store     QueryStore
	web       fs.FS
	log       *slog.Logger
	queryWait time.Duration
}

// New constructs a Server.
func New(pool Querier, st QueryStore, web fs.FS, log *slog.Logger, queryWait time.Duration) *Server {
	return NewWithRegistry(NewSingleDatabaseRegistry(pool), st, web, log, queryWait)
}

func NewWithRegistry(registry DatabaseRegistry, st QueryStore, web fs.FS, log *slog.Logger, queryWait time.Duration) *Server {
	return &Server{registry: registry, store: st, web: web, log: log, queryWait: queryWait}
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

	// Static UI
	mux.Handle("GET /", http.FileServerFS(s.web))

	return securityHeaders(logging(s.log, mux))
}
