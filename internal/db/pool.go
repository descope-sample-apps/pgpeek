// Package db wraps a pgx connection pool configured for safe, read-only
// browsing: bounded pool size, per-session statement timeout, and result-row
// capping so a single query can never buffer an unbounded set into memory.
package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Config holds the tunables for the pool.
type Config struct {
	DSN              string
	MaxConns         int32
	StatementTimeout time.Duration
	IdleTxTimeout    time.Duration
	RowCap           int

	// BeforeConnect, if set, runs before every new physical connection. It is
	// used for RDS/Aurora IAM auth to inject a freshly-minted (short-lived)
	// auth token as the password, so connections never carry a static secret.
	BeforeConnect func(context.Context, *pgx.ConnConfig) error
}

// pgxPool is the subset of *pgxpool.Pool that Pool depends on, so the
// query/health logic can be unit-tested with a fake.
type pgxPool interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Ping(ctx context.Context) error
	Close()
}

// newPool is a seam: production uses pgxpool.NewWithConfig; tests substitute a
// fake so New can be exercised without a live database.
var newPool = func(ctx context.Context, cfg *pgxpool.Config) (pgxPool, error) {
	return pgxpool.NewWithConfig(ctx, cfg)
}

// Pool is a thin wrapper around a pgx pool with a row cap.
type Pool struct {
	pool   pgxPool
	rowCap int
}

// Result is a fully-materialized (but row-capped) query result, ready to be
// serialized to JSON or CSV.
type Result struct {
	Columns   []string `json:"columns"`
	Rows      [][]any  `json:"rows"`
	RowCount  int      `json:"rowCount"`
	Truncated bool     `json:"truncated"`
	ElapsedMS int64    `json:"elapsedMs"`
}

// buildConfig parses the DSN and applies the belt-and-suspenders session
// limits. Split out from New so the parsing and parameter wiring are testable
// without opening a pool.
func buildConfig(c Config) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(c.DSN)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	if c.MaxConns > 0 {
		cfg.MaxConns = c.MaxConns
	}
	// ParseConfig always populates RuntimeParams, so we can assign directly.
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = strconv.FormatInt(c.StatementTimeout.Milliseconds(), 10)
	cfg.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = strconv.FormatInt(c.IdleTxTimeout.Milliseconds(), 10)
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	cfg.ConnConfig.RuntimeParams["application_name"] = "pgpeek"
	cfg.BeforeConnect = c.BeforeConnect
	return cfg, nil
}

// New builds and verifies the pool. The DSN is never logged.
func New(ctx context.Context, c Config) (*Pool, error) {
	cfg, err := buildConfig(c)
	if err != nil {
		return nil, err
	}
	pool, err := newPool(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &Pool{pool: pool, rowCap: c.RowCap}, nil
}

// RowCap is the maximum number of rows any query or page returns.
func (p *Pool) RowCap() int { return p.rowCap }

// Ping checks connectivity (used by /readyz).
func (p *Pool) Ping(ctx context.Context) error { return p.pool.Ping(ctx) }

// Close releases the pool.
func (p *Pool) Close() { p.pool.Close() }

// Query runs sql and returns up to rowCap rows. Reading stops at the cap so an
// enormous result set is never fully buffered; Truncated reports whether more
// rows were available.
func (p *Pool) Query(ctx context.Context, sql string) (*Result, error) {
	start := time.Now()
	rows, err := p.pool.Query(ctx, sql)
	if err != nil {
		return nil, err
	}
	return p.collect(rows, start)
}

// collect reads up to rowCap rows from rows (which it closes), normalizing
// values for JSON, and reports whether more rows were available.
func (p *Pool) collect(rows pgx.Rows, start time.Time) (*Result, error) {
	defer rows.Close()

	fds := rows.FieldDescriptions()
	cols := make([]string, len(fds))
	for i, f := range fds {
		cols[i] = f.Name
	}

	out := make([][]any, 0, 128)
	truncated := false
	for rows.Next() {
		if len(out) >= p.rowCap {
			truncated = true
			break
		}
		vals, err := rows.Values()
		if err != nil {
			return nil, err
		}
		row := make([]any, len(vals))
		for i, v := range vals {
			row[i] = normalize(v)
		}
		out = append(out, row)
	}
	if !truncated {
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return &Result{
		Columns:   cols,
		Rows:      out,
		RowCount:  len(out),
		Truncated: truncated,
		ElapsedMS: time.Since(start).Milliseconds(),
	}, nil
}

// normalize converts pgx-returned values into JSON-friendly forms.
func normalize(v any) any {
	switch t := v.(type) {
	case nil, bool, string,
		float32, float64,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		return t
	case []byte:
		return "\\x" + fmt.Sprintf("%x", t)
	case time.Time:
		return t.Format(time.RFC3339Nano)
	case [16]byte: // uuid returned without type registration
		return fmt.Sprintf("%x-%x-%x-%x-%x", t[0:4], t[4:6], t[6:8], t[8:10], t[10:16])
	default:
		// pgtype values (numeric, arrays, json, ...) implement json.Marshaler;
		// embed their JSON so numbers/objects render natively.
		if b, err := json.Marshal(t); err == nil {
			return json.RawMessage(b)
		}
		return fmt.Sprintf("%v", t)
	}
}

// CellString renders a normalized cell value as a flat string for CSV.
func CellString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.RawMessage:
		return string(t)
	default:
		return fmt.Sprintf("%v", t)
	}
}
