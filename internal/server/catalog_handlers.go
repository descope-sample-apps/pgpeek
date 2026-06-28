package server

import (
	"context"
	"net/http"
	"time"

	"github.com/descope-sample-apps/pgpeek/internal/db"
)

func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request) {
	pool, ok := s.poolForRequest(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"rowCap": pool.RowCap()})
}

func (s *Server) handleTables(w http.ResponseWriter, r *http.Request) {
	pool, ok := s.poolForRequest(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.queryWait)
	defer cancel()
	tables, err := pool.Tables(ctx)
	if err != nil {
		s.log.Error("list tables", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list tables")
		return
	}
	if tables == nil {
		tables = []db.TableInfo{}
	}
	writeJSON(w, http.StatusOK, tables)
}

func (s *Server) handleColumns(w http.ResponseWriter, r *http.Request) {
	pool, ok := s.poolForRequest(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.queryWait)
	defer cancel()
	cols, err := pool.Columns(ctx, r.PathValue("schema"), r.PathValue("table"))
	if err != nil {
		s.log.Error("read columns", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to read columns")
		return
	}
	if cols == nil {
		cols = []db.ColumnInfo{}
	}
	writeJSON(w, http.StatusOK, cols)
}

func (s *Server) handleForeignKeys(w http.ResponseWriter, r *http.Request) {
	pool, ok := s.poolForRequest(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.queryWait)
	defer cancel()
	fks, err := pool.ForeignKeys(ctx, r.PathValue("schema"), r.PathValue("table"))
	if err != nil {
		s.log.Error("read foreign keys", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to read foreign keys")
		return
	}
	if fks == nil {
		fks = []db.ForeignKey{}
	}
	writeJSON(w, http.StatusOK, fks)
}

func (s *Server) handleTableData(w http.ResponseWriter, r *http.Request) {
	pool, ok := s.poolForRequest(w, r)
	if !ok {
		return
	}
	q := db.TableQuery{
		Schema:  r.PathValue("schema"),
		Table:   r.PathValue("table"),
		Search:  r.URL.Query().Get("search"),
		Sort:    r.URL.Query().Get("sort"),
		Desc:    r.URL.Query().Get("dir") == "desc",
		Limit:   queryInt(r, "limit", 0),
		Offset:  queryInt(r, "offset", 0),
		Filters: parseFilters(r.URL.Query()["f"]),
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.queryWait)
	defer cancel()
	res, err := pool.TableRows(ctx, q)
	if err != nil {
		s.log.Error("read rows", "err", err)
		writeError(w, http.StatusBadRequest, "failed to read rows")
		return
	}
	if r.URL.Query().Get("format") == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+safeFilename(r.PathValue("table"))+`.csv"`)
		if err := writeCSV(w, res); err != nil {
			s.log.Error("csv export", "err", err)
		}
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	pool, ok := s.poolForRequest(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := pool.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database not ready")
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready"))
}
