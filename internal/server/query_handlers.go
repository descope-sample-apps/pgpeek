package server

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/descope-sample-apps/pgpeek/internal/db"
	"github.com/descope-sample-apps/pgpeek/internal/guard"
)

type queryRequest struct {
	SQL string `json:"sql"`
}

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	res, ok := s.readOnlyResult(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	res, ok := s.readOnlyResult(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="pgpeek-export.csv"`)
	if err := writeCSV(w, res); err != nil {
		s.log.Error("csv export", "err", err)
	}
}

func (s *Server) readOnlyResult(w http.ResponseWriter, r *http.Request) (*db.Result, bool) {
	pool, ok := s.poolForRequest(w, r)
	if !ok {
		return nil, false
	}
	sql, ok := decodeReadOnlyQuery(w, r)
	if !ok {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.queryWait)
	defer cancel()
	res, err := pool.Query(ctx, sql)
	if err != nil {
		s.log.Error("query", "err", err)
		writeError(w, http.StatusBadRequest, queryErrorMessage(err))
		return nil, false
	}
	return res, true
}

func queryErrorMessage(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Message != "" {
		return fmt.Sprintf("%s (SQLSTATE %s)", pgErr.Message, pgErr.Code)
	}
	return "query failed"
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
