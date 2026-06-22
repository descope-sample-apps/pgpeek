package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// TableInfo describes a browsable relation (table or view).
type TableInfo struct {
	Schema  string `json:"schema"`
	Name    string `json:"name"`
	Type    string `json:"type"`    // "table" | "view"
	EstRows int64  `json:"estRows"` // planner estimate (reltuples), -1 if unknown
}

// ColumnInfo describes one column of a relation.
type ColumnInfo struct {
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Nullable bool    `json:"nullable"`
	Default  *string `json:"default"`
}

// User-facing relations only: skip system + TOAST schemas. relkind r/p = table,
// v/m = view/materialized view. reltuples is the planner's row estimate (fast).
const tablesSQL = `
SELECT n.nspname,
       c.relname,
       CASE WHEN c.relkind IN ('v','m') THEN 'view' ELSE 'table' END,
       c.reltuples::bigint
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind IN ('r','p','v','m')
  AND n.nspname NOT IN ('pg_catalog','information_schema')
  AND n.nspname NOT LIKE 'pg_toast%'
ORDER BY n.nspname, c.relname`

const columnsSQL = `
SELECT column_name, data_type, (is_nullable = 'YES'), column_default
FROM information_schema.columns
WHERE table_schema = $1 AND table_name = $2
ORDER BY ordinal_position`

// Tables lists all browsable tables and views.
func (p *Pool) Tables(ctx context.Context) ([]TableInfo, error) {
	rows, err := p.pool.Query(ctx, tablesSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]TableInfo, 0, 64)
	for rows.Next() {
		var t TableInfo
		if err := rows.Scan(&t.Schema, &t.Name, &t.Type, &t.EstRows); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Columns returns the structure of a single relation.
func (p *Pool) Columns(ctx context.Context, schema, table string) ([]ColumnInfo, error) {
	rows, err := p.pool.Query(ctx, columnsSQL, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ColumnInfo, 0, 16)
	for rows.Next() {
		var c ColumnInfo
		if err := rows.Scan(&c.Name, &c.Type, &c.Nullable, &c.Default); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// TableRows returns a page of rows from schema.table. Identifiers are sanitized
// via pgx.Identifier (the read-only role + statement timeout + row cap remain
// the real safeguards); limit is clamped to the row cap and offset to >= 0.
func (p *Pool) TableRows(ctx context.Context, schema, table string, limit, offset int) (*Result, error) {
	if limit <= 0 || limit > p.rowCap {
		limit = p.rowCap
	}
	if offset < 0 {
		offset = 0
	}
	ident := pgx.Identifier{schema, table}.Sanitize()
	sql := fmt.Sprintf("SELECT * FROM %s LIMIT %d OFFSET %d", ident, limit, offset)

	start := time.Now()
	rows, err := p.pool.Query(ctx, sql)
	if err != nil {
		return nil, err
	}
	return p.collect(rows, start)
}
