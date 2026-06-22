package db

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// --- fakes ---------------------------------------------------------------

type fakeRows struct {
	pgx.Rows
	cols    []string
	data    [][]any
	idx     int
	valErr  error
	errErr  error
	scanErr error
	closed  bool
}

// Scan copies the current row's values into dest (used by catalog queries).
func (r *fakeRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	row := r.data[r.idx-1]
	for i := range dest {
		dv := reflect.ValueOf(dest[i]).Elem()
		if row[i] == nil {
			dv.Set(reflect.Zero(dv.Type()))
			continue
		}
		dv.Set(reflect.ValueOf(row[i]))
	}
	return nil
}

func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription {
	fds := make([]pgconn.FieldDescription, len(r.cols))
	for i, c := range r.cols {
		fds[i] = pgconn.FieldDescription{Name: c}
	}
	return fds
}

func (r *fakeRows) Next() bool {
	if r.idx >= len(r.data) {
		return false
	}
	r.idx++
	return true
}

func (r *fakeRows) Values() ([]any, error) {
	if r.valErr != nil {
		return nil, r.valErr
	}
	return r.data[r.idx-1], nil
}

func (r *fakeRows) Err() error { return r.errErr }
func (r *fakeRows) Close()     { r.closed = true }

type fakePool struct {
	rows     pgx.Rows // data rows
	colRows  pgx.Rows // rows for the information_schema.columns validation query
	fkRows   pgx.Rows // rows for the foreign-key introspection query
	queryErr error
	pingErr  error
	closed   bool
	lastSQL  string
	lastArgs []any
}

func (p *fakePool) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	if p.queryErr != nil {
		return nil, p.queryErr
	}
	// catalog.Columns runs the information_schema query first (for validation).
	if p.colRows != nil && strings.Contains(sql, "information_schema.columns") {
		return p.colRows, nil
	}
	if p.fkRows != nil && strings.Contains(sql, "table_constraints") {
		return p.fkRows, nil
	}
	p.lastSQL = sql
	p.lastArgs = args
	return p.rows, nil
}
func (p *fakePool) Ping(context.Context) error { return p.pingErr }
func (p *fakePool) Close()                     { p.closed = true }

// --- normalize / CellString ---------------------------------------------

func TestNormalize(t *testing.T) {
	ts := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	uuid := [16]byte{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88}

	tests := []struct {
		name string
		in   any
		want any
	}{
		{"nil", nil, nil},
		{"string", "hello", "hello"},
		{"int64", int64(42), int64(42)},
		{"bool", true, true},
		{"float64", 3.5, 3.5},
		{"bytes", []byte{0xde, 0xad}, "\\xdead"},
		{"time", ts, "2026-06-22T12:00:00Z"},
		{"uuid", uuid, "12345678-9abc-def0-1122-334455667788"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalize(tc.in)
			if got != tc.want {
				t.Errorf("normalize(%v) = %v (%T), want %v (%T)", tc.in, got, got, tc.want, tc.want)
			}
		})
	}
}

func TestNormalizeJSONMarshalable(t *testing.T) {
	type custom struct {
		A int `json:"a"`
	}
	got := normalize(custom{A: 1})
	raw, ok := got.(json.RawMessage)
	if !ok {
		t.Fatalf("expected json.RawMessage, got %T", got)
	}
	if string(raw) != `{"a":1}` {
		t.Errorf("got %s", raw)
	}
}

// unmarshalable is a type whose MarshalJSON fails, forcing the final fallback.
type unmarshalable struct{}

func (unmarshalable) MarshalJSON() ([]byte, error) { return nil, errors.New("nope") }

func TestNormalizeFallback(t *testing.T) {
	got := normalize(unmarshalable{})
	if got != "{}" {
		t.Errorf("fallback = %v, want %q", got, "{}")
	}
}

func TestCellString(t *testing.T) {
	tests := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{"abc", "abc"},
		{int64(7), "7"},
		{true, "true"},
		{json.RawMessage(`{"a":1}`), `{"a":1}`},
		{"\\xdead", "\\xdead"},
	}
	for _, tc := range tests {
		if got := CellString(tc.in); got != tc.want {
			t.Errorf("CellString(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// --- Query ---------------------------------------------------------------

func TestQuery_Success(t *testing.T) {
	rows := &fakeRows{cols: []string{"a", "b"}, data: [][]any{{int64(1), "x"}, {int64(2), "y"}}}
	p := &Pool{pool: &fakePool{rows: rows}, rowCap: 10}
	res, err := p.Query(context.Background(), "SELECT a,b")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if res.RowCount != 2 || res.Truncated {
		t.Fatalf("rowCount=%d truncated=%v", res.RowCount, res.Truncated)
	}
	if len(res.Columns) != 2 || res.Columns[0] != "a" {
		t.Errorf("columns = %v", res.Columns)
	}
	if !rows.closed {
		t.Error("rows not closed")
	}
}

func TestQuery_RowCap(t *testing.T) {
	rows := &fakeRows{cols: []string{"n"}, data: [][]any{{int64(1)}, {int64(2)}, {int64(3)}}}
	p := &Pool{pool: &fakePool{rows: rows}, rowCap: 2}
	res, err := p.Query(context.Background(), "SELECT n")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if res.RowCount != 2 || !res.Truncated {
		t.Fatalf("rowCount=%d truncated=%v, want 2/true", res.RowCount, res.Truncated)
	}
}

func TestQuery_QueryError(t *testing.T) {
	p := &Pool{pool: &fakePool{queryErr: errors.New("boom")}, rowCap: 10}
	if _, err := p.Query(context.Background(), "SELECT 1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestQuery_ValuesError(t *testing.T) {
	rows := &fakeRows{cols: []string{"n"}, data: [][]any{{int64(1)}}, valErr: errors.New("scan fail")}
	p := &Pool{pool: &fakePool{rows: rows}, rowCap: 10}
	if _, err := p.Query(context.Background(), "SELECT n"); err == nil {
		t.Fatal("expected values error")
	}
}

func TestQuery_RowsErr(t *testing.T) {
	rows := &fakeRows{cols: []string{"n"}, errErr: errors.New("cursor fail")}
	p := &Pool{pool: &fakePool{rows: rows}, rowCap: 10}
	if _, err := p.Query(context.Background(), "SELECT n"); err == nil {
		t.Fatal("expected rows.Err error")
	}
}

func TestRowCap(t *testing.T) {
	if got := (&Pool{rowCap: 250}).RowCap(); got != 250 {
		t.Errorf("RowCap = %d, want 250", got)
	}
}

func TestPingAndClose(t *testing.T) {
	fp := &fakePool{}
	p := &Pool{pool: fp, rowCap: 1}
	if err := p.Ping(context.Background()); err != nil {
		t.Errorf("Ping: %v", err)
	}
	fp.pingErr = errors.New("down")
	if err := p.Ping(context.Background()); err == nil {
		t.Error("expected ping error")
	}
	p.Close()
	if !fp.closed {
		t.Error("Close did not propagate")
	}
}

// --- buildConfig / New ---------------------------------------------------

func TestBuildConfig_Success(t *testing.T) {
	cfg, err := buildConfig(Config{
		DSN:              "postgres://u:p@localhost:5432/db",
		MaxConns:         5,
		StatementTimeout: 7 * time.Second,
		IdleTxTimeout:    8 * time.Second,
	})
	if err != nil {
		t.Fatalf("buildConfig: %v", err)
	}
	if cfg.MaxConns != 5 {
		t.Errorf("MaxConns = %d", cfg.MaxConns)
	}
	rp := cfg.ConnConfig.RuntimeParams
	if rp["statement_timeout"] != "7000" {
		t.Errorf("statement_timeout = %q", rp["statement_timeout"])
	}
	if rp["default_transaction_read_only"] != "on" {
		t.Errorf("read_only = %q", rp["default_transaction_read_only"])
	}
	if rp["application_name"] != "pgpeek" {
		t.Errorf("application_name = %q", rp["application_name"])
	}
}

func TestBuildConfig_BadDSN(t *testing.T) {
	if _, err := buildConfig(Config{DSN: "://not a dsn"}); err == nil {
		t.Fatal("expected parse error")
	}
}

func withFakeNewPool(t *testing.T, fn func(context.Context, *pgxpool.Config) (pgxPool, error)) {
	t.Helper()
	orig := newPool
	newPool = fn
	t.Cleanup(func() { newPool = orig })
}

func TestNew_BadDSN(t *testing.T) {
	if _, err := New(context.Background(), Config{DSN: "://bad"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestNew_PoolError(t *testing.T) {
	withFakeNewPool(t, func(context.Context, *pgxpool.Config) (pgxPool, error) {
		return nil, errors.New("create failed")
	})
	if _, err := New(context.Background(), Config{DSN: "postgres://u:p@h:5432/db"}); err == nil {
		t.Fatal("expected create error")
	}
}

func TestNew_PingError(t *testing.T) {
	fp := &fakePool{pingErr: errors.New("unreachable")}
	withFakeNewPool(t, func(context.Context, *pgxpool.Config) (pgxPool, error) {
		return fp, nil
	})
	if _, err := New(context.Background(), Config{DSN: "postgres://u:p@h:5432/db"}); err == nil {
		t.Fatal("expected ping error")
	}
	if !fp.closed {
		t.Error("pool should be closed when ping fails")
	}
}

func TestNew_Success(t *testing.T) {
	withFakeNewPool(t, func(context.Context, *pgxpool.Config) (pgxPool, error) {
		return &fakePool{}, nil
	})
	p, err := New(context.Background(), Config{DSN: "postgres://u:p@h:5432/db", RowCap: 50})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if p.rowCap != 50 {
		t.Errorf("rowCap = %d, want 50", p.rowCap)
	}
}
