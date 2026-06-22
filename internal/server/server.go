// Package server wires the HTTP handlers: the SQL query endpoint (guarded +
// capped), saved-query CRUD, CSV export, the static UI, and k8s probes.
package server

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/descope/pgpeek/internal/db"
	"github.com/descope/pgpeek/internal/guard"
	"github.com/descope/pgpeek/internal/store"
)

// maxBodyBytes caps request bodies. Queries are SQL text, not data, so 1 MiB is
// generous while preventing a client from forcing unbounded buffering.
const maxBodyBytes = 1 << 20

// Querier runs read-only queries and reports database health. *db.Pool
// implements it; tests substitute a fake.
type Querier interface {
	Query(ctx context.Context, sql string) (*db.Result, error)
	Ping(ctx context.Context) error
}

// QueryStore persists saved/preset queries. *store.Store implements it.
type QueryStore interface {
	List(ctx context.Context) ([]store.SavedQuery, error)
	Get(ctx context.Context, id int64) (store.SavedQuery, error)
	Create(ctx context.Context, name, desc, sql string) (store.SavedQuery, error)
	Update(ctx context.Context, id int64, name, desc, sql string) (store.SavedQuery, error)
	Delete(ctx context.Context, id int64) error
}

// Server holds the dependencies for the HTTP handlers.
type Server struct {
	pool      Querier
	store     QueryStore
	web       fs.FS
	log       *slog.Logger
	queryWait time.Duration
}

// New constructs a Server.
func New(pool Querier, st QueryStore, web fs.FS, log *slog.Logger, queryWait time.Duration) *Server {
	return &Server{pool: pool, store: st, web: web, log: log, queryWait: queryWait}
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
	mux.HandleFunc("POST /api/query", s.handleQuery)
	mux.HandleFunc("POST /api/export", s.handleExport)
	mux.HandleFunc("GET /api/queries", s.handleListQueries)
	mux.HandleFunc("POST /api/queries", s.handleCreateQuery)
	mux.HandleFunc("PUT /api/queries/{id}", s.handleUpdateQuery)
	mux.HandleFunc("DELETE /api/queries/{id}", s.handleDeleteQuery)

	// Static UI
	mux.Handle("GET /", http.FileServerFS(s.web))

	return securityHeaders(logging(s.log, mux))
}

type queryRequest struct {
	SQL string `json:"sql"`
}

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	var req queryRequest
	if !decodeBody(w, r, &req) {
		return
	}
	sql := strings.TrimSpace(req.SQL)
	if err := guard.Validate(sql); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.queryWait)
	defer cancel()

	res, err := s.pool.Query(ctx, sql)
	if err != nil {
		writeError(w, http.StatusBadRequest, "query failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	var req queryRequest
	if !decodeBody(w, r, &req) {
		return
	}
	sql := strings.TrimSpace(req.SQL)
	if err := guard.Validate(sql); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.queryWait)
	defer cancel()

	res, err := s.pool.Query(ctx, sql)
	if err != nil {
		writeError(w, http.StatusBadRequest, "query failed: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="pgpeek-export.csv"`)
	if err := writeCSV(w, res); err != nil {
		// Headers/partial body may already be sent; just log.
		s.log.Error("csv export", "err", err)
	}
}

// writeCSV streams the result as CSV. encoding/csv accumulates errors stickily,
// so it's sufficient to check Error() once after Flush.
func writeCSV(w io.Writer, res *db.Result) error {
	cw := csv.NewWriter(w)
	_ = cw.Write(res.Columns)
	row := make([]string, len(res.Columns))
	for _, rec := range res.Rows {
		for i, cell := range rec {
			row[i] = db.CellString(cell)
		}
		_ = cw.Write(row)
	}
	cw.Flush()
	return cw.Error()
}

func (s *Server) handleListQueries(w http.ResponseWriter, r *http.Request) {
	qs, err := s.store.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list saved queries")
		s.log.Error("list saved queries", "err", err)
		return
	}
	if qs == nil {
		qs = []store.SavedQuery{}
	}
	writeJSON(w, http.StatusOK, qs)
}

type savedQueryRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	SQL         string `json:"sql"`
}

func (s *Server) handleCreateQuery(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeSavedQuery(w, r)
	if !ok {
		return
	}
	q, err := s.store.Create(r.Context(), req.Name, req.Description, req.SQL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save query")
		s.log.Error("create saved query", "err", err)
		return
	}
	writeJSON(w, http.StatusCreated, q)
}

func (s *Server) handleUpdateQuery(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	req, ok := decodeSavedQuery(w, r)
	if !ok {
		return
	}
	q, err := s.store.Update(r.Context(), id, req.Name, req.Description, req.SQL)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "saved query not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update query")
		s.log.Error("update saved query", "err", err)
		return
	}
	writeJSON(w, http.StatusOK, q)
}

func (s *Server) handleDeleteQuery(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	err := s.store.Delete(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "saved query not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete query")
		s.log.Error("delete saved query", "err", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := s.pool.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database not ready")
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready"))
}

// --- helpers ---

func decodeSavedQuery(w http.ResponseWriter, r *http.Request) (savedQueryRequest, bool) {
	var req savedQueryRequest
	if !decodeBody(w, r, &req) {
		return req, false
	}
	req.Name = strings.TrimSpace(req.Name)
	req.SQL = strings.TrimSpace(req.SQL)
	if req.Name == "" || req.SQL == "" {
		writeError(w, http.StatusBadRequest, "name and sql are required")
		return req, false
	}
	if err := guard.Validate(req.SQL); err != nil {
		writeError(w, http.StatusBadRequest, "saved query must be read-only: "+err.Error())
		return req, false
	}
	return req, true
}

// decodeBody reads a size-capped JSON body into v, rejecting unknown fields. It
// writes the error response itself and returns false on failure.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// logging is a minimal request logger. It never logs request bodies (which
// contain SQL) at info level beyond method/path.
func logging(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			return // don't spam probe logs
		}
		log.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"ms", time.Since(start).Milliseconds(),
		)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// contentSecurityPolicy allows the app's own assets plus the CodeMirror CDN. The
// inline page script lives in /app.js (no 'unsafe-inline' for scripts); styles
// permit 'unsafe-inline' because CodeMirror injects them.
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self' https://cdnjs.cloudflare.com; " +
	"style-src 'self' 'unsafe-inline' https://cdnjs.cloudflare.com; " +
	"img-src 'self' data:; " +
	"connect-src 'self'; " +
	"base-uri 'self'; " +
	"form-action 'self'; " +
	"frame-ancestors 'none'"

// securityHeaders sets conservative defaults on every response.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}
