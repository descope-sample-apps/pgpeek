package server

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/descope-sample-apps/pgpeek/internal/db"
	"github.com/descope-sample-apps/pgpeek/internal/guard"
)

type queryRequest struct {
	SQL string `json:"sql"`
}

const exportCSRFCookie = "pgpeek_export_csrf"

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

func (s *Server) handleQueryCount(w http.ResponseWriter, r *http.Request) {
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
	started := time.Now()
	count, err := pool.Count(ctx, sql)
	if err != nil {
		s.log.Error("count query", "err", err)
		writeError(w, http.StatusBadRequest, queryErrorMessage(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rowCount": count, "elapsedMs": time.Since(started).Milliseconds()})
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
	if mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mediaType == "application/x-www-form-urlencoded" {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'self'")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	}
	pool, ok := s.poolForRequest(w, r)
	if !ok {
		return
	}
	sql, ok := decodeReadOnlyQuery(w, r)
	if !ok {
		return
	}
	select {
	case s.exportSlots <- struct{}{}:
		defer func() { <-s.exportSlots }()
	default:
		writeError(w, http.StatusTooManyRequests, "another export is already running")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.queryWait)
	defer cancel()
	if err := writeGZIPResponse(w, "pgpeek-export.csv.gz", func(dst io.Writer) error {
		return pool.ExportCSV(ctx, sql, dst)
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
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return "", false
	}
	if mediaType == "application/x-www-form-urlencoded" {
		if r.URL.Path != "/api/export" {
			writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
			return "", false
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		if err := r.ParseForm(); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return "", false
		}
		if crossSiteForm(r) || !validExportCSRF(r) {
			writeError(w, http.StatusForbidden, "cross-site export blocked")
			return "", false
		}
		sql := strings.TrimSpace(r.Form.Get("sql"))
		if err := guard.Validate(sql); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return "", false
		}
		return sql, true
	}
	if mediaType != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return "", false
	}
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

func crossSiteForm(r *http.Request) bool {
	if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
		return true
	}
	origin := r.Header.Get("Origin")
	if origin == "" || origin == "null" {
		return false
	}
	parsed, err := url.Parse(origin)
	return err != nil || !strings.EqualFold(parsed.Host, r.Host)
}

func validExportCSRF(r *http.Request) bool {
	cookie, err := r.Cookie(exportCSRFCookie)
	formToken := r.Form.Get("csrf")
	return err == nil && formToken != "" && subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(formToken)) == 1
}
