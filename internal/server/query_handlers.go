package server

import (
	"context"
	"encoding/csv"
	"io"
	"net/http"
	"strings"

	"github.com/descope-sample-apps/pgpeek/internal/db"
	"github.com/descope-sample-apps/pgpeek/internal/guard"
)

type queryRequest struct {
	SQL string `json:"sql"`
}

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	pool, ok := s.poolForRequest(w, r)
	if !ok {
		return
	}
	sql, ok := decodeReadOnlyQuery(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.queryWait)
	defer cancel()
	res, err := pool.Query(ctx, sql)
	if err != nil {
		writeError(w, http.StatusBadRequest, "query failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	pool, ok := s.poolForRequest(w, r)
	if !ok {
		return
	}
	sql, ok := decodeReadOnlyQuery(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.queryWait)
	defer cancel()
	res, err := pool.Query(ctx, sql)
	if err != nil {
		writeError(w, http.StatusBadRequest, "query failed: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="pgpeek-export.csv"`)
	if err := writeCSV(w, res); err != nil {
		s.log.Error("csv export", "err", err)
	}
}

func decodeReadOnlyQuery(w http.ResponseWriter, r *http.Request) (string, bool) {
	var req queryRequest
	if !decodeBody(w, r, &req) {
		return "", false
	}
	sql := strings.TrimSpace(req.SQL)
	if err := guard.Validate(sql); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return "", false
	}
	return sql, true
}

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
