package server

import (
	"context"
	"io"
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

type schemaResponse struct {
	Schemas db.SchemaCatalog `json:"schemas"`
}

func (s *Server) handleSchema(w http.ResponseWriter, r *http.Request) {
	pool, ok := s.poolForRequest(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.queryWait)
	defer cancel()
	catalog, truncated, err := pool.SchemaCatalog(ctx)
	if err != nil {
		s.log.Error("read schema catalog", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to read schema catalog")
		return
	}
	if truncated {
		writeError(w, http.StatusInternalServerError, "schema catalog exceeds response limit")
		return
	}
	if catalog == nil {
		catalog = make(db.SchemaCatalog)
	}
	writeJSON(w, http.StatusOK, schemaResponse{Schemas: catalog})
}

func (s *Server) handleTables(w http.ResponseWriter, r *http.Request) {
	pool, ok := s.poolForRequest(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.queryWait)
	defer cancel()
	tables, truncated, err := pool.Tables(ctx)
	if err != nil {
		s.log.Error("list tables", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list tables")
		return
	}
	if truncated {
		writeError(w, http.StatusInternalServerError, "table catalog exceeds response limit")
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
	cols, truncated, err := pool.Columns(ctx, r.PathValue("schema"), r.PathValue("table"))
	if err != nil {
		s.log.Error("read columns", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to read columns")
		return
	}
	if truncated {
		writeError(w, http.StatusInternalServerError, "column catalog exceeds response limit")
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
	fks, truncated, err := pool.ForeignKeys(ctx, r.PathValue("schema"), r.PathValue("table"))
	if err != nil {
		s.log.Error("read foreign keys", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to read foreign keys")
		return
	}
	if truncated {
		writeError(w, http.StatusInternalServerError, "foreign-key catalog exceeds response limit")
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
	q := tableQuery(r)
	ctx, cancel := context.WithTimeout(r.Context(), s.queryWait)
	defer cancel()
	res, err := pool.TableRows(ctx, q)
	if err != nil {
		s.log.Error("read rows")
		writeError(w, http.StatusBadRequest, "failed to read rows")
		return
	}
	if r.URL.Query().Get("format") == "csv" {
		if err := writeCSVResponse(w, safeFilename(r.PathValue("table"))+".csv", func(dst io.Writer) error {
			return writeCSV(dst, res, func(row, column int) (any, error) {
				return pool.TableCell(ctx, q, row, column)
			})
		}); err != nil {
			s.log.Error("csv export", "err", err)
		}
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleTableCell(w http.ResponseWriter, r *http.Request) {
	pool, ok := s.poolForRequest(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.queryWait)
	defer cancel()
	value, err := pool.TableCell(ctx, tableQuery(r), queryInt(r, "row", -1), queryInt(r, "column", -1))
	if err != nil {
		s.log.Error("read cell")
		writeError(w, http.StatusBadRequest, "failed to read cell")
		return
	}
	if expected := r.URL.Query().Get("hash"); expected != "" && db.CellHash(value) != expected {
		writeError(w, http.StatusConflict, "table result changed; reload the page")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"value": value})
}

func tableQuery(r *http.Request) db.TableQuery {
	return db.TableQuery{
		Schema:  r.PathValue("schema"),
		Table:   r.PathValue("table"),
		Search:  r.URL.Query().Get("search"),
		Sort:    r.URL.Query().Get("sort"),
		Desc:    r.URL.Query().Get("dir") == "desc",
		Limit:   queryInt(r, "limit", 0),
		Offset:  queryInt(r, "offset", 0),
		Filters: parseFilters(r.URL.Query()["f"]),
	}
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
