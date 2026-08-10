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

type queryCellRequest struct {
	SQL    string `json:"sql"`
	Row    int    `json:"row"`
	Column int    `json:"column"`
	Hash   string `json:"hash"`
}

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	res, ok := s.readOnlyResult(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleQueryCell(w http.ResponseWriter, r *http.Request) {
	var req queryCellRequest
	if !decodeBody(w, r, &req) {
		return
	}
	req.SQL = strings.TrimSpace(req.SQL)
	if req.Row < 0 || req.Column < 0 {
		writeError(w, http.StatusBadRequest, "row and column must be non-negative")
		return
	}
	if err := guard.Validate(req.SQL); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	pool, ok := s.poolForRequest(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.queryWait)
	defer cancel()
	value, err := pool.QueryCell(ctx, req.SQL, req.Row, req.Column)
	if errors.Is(err, db.ErrCellOutOfRange) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		s.log.Error("query cell", "err", err)
		writeError(w, http.StatusBadRequest, queryErrorMessage(err))
		return
	}
	if req.Hash != "" && db.CellHash(value) != req.Hash {
		writeError(w, http.StatusConflict, "query result changed; run the query again")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"value": value})
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
		s.log.Error("query", "err", err)
		writeError(w, http.StatusBadRequest, queryErrorMessage(err))
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="pgpeek-export.csv"`)
	if err := writeCSV(w, res, func(row, column int) (any, error) {
		return pool.QueryCell(ctx, sql, row, column)
	}); err != nil {
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

func writeCSV(w io.Writer, res *db.Result, resolve func(row, column int) (any, error)) error {
	cw := csv.NewWriter(w)
	_ = cw.Write(res.Columns)
	truncated := make(map[db.CellRef]bool, len(res.TruncatedCells))
	for _, cell := range res.TruncatedCells {
		truncated[cell] = true
	}
	row := make([]string, len(res.Columns))
	for rowIndex, rec := range res.Rows {
		for i, cell := range rec {
			if truncated[db.CellRef{Row: rowIndex, Column: i}] && resolve != nil {
				var err error
				cell, err = resolve(rowIndex, i)
				if err != nil {
					return err
				}
			}
			row[i] = db.CellString(cell)
		}
		_ = cw.Write(row)
	}
	cw.Flush()
	return cw.Error()
}
