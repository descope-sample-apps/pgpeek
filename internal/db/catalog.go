package db

import (
	"context"
	"fmt"
	"strconv"
	"strings"
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

// Single-column foreign keys for a relation, with their referenced target.
const fksSQL = `
SELECT kcu.column_name, ccu.table_schema, ccu.table_name, ccu.column_name
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
  ON tc.constraint_name = kcu.constraint_name AND tc.constraint_schema = kcu.constraint_schema
JOIN information_schema.constraint_column_usage ccu
  ON tc.constraint_name = ccu.constraint_name AND tc.constraint_schema = ccu.constraint_schema
WHERE tc.constraint_type = 'FOREIGN KEY' AND tc.table_schema = $1 AND tc.table_name = $2
ORDER BY kcu.column_name`

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

// ForeignKey describes a single-column foreign key and the row it points to.
type ForeignKey struct {
	Column    string `json:"column"`
	RefSchema string `json:"refSchema"`
	RefTable  string `json:"refTable"`
	RefColumn string `json:"refColumn"`
}

// ForeignKeys returns the single-column foreign keys of a relation, so the UI
// can turn FK cells into click-through links.
func (p *Pool) ForeignKeys(ctx context.Context, schema, table string) ([]ForeignKey, error) {
	rows, err := p.pool.Query(ctx, fksSQL, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ForeignKey, 0, 8)
	for rows.Next() {
		var fk ForeignKey
		if err := rows.Scan(&fk.Column, &fk.RefSchema, &fk.RefTable, &fk.RefColumn); err != nil {
			return nil, err
		}
		out = append(out, fk)
	}
	return out, rows.Err()
}

// Filter is one column predicate. Op is a key from filterOps, or "is_null" /
// "is_not_null" (which ignore Value).
type Filter struct {
	Column string
	Op     string
	Value  string
}

// TableQuery describes a (possibly filtered/sorted) page request.
type TableQuery struct {
	Schema, Table string
	Search        string // case-insensitive substring matched across all columns
	Filters       []Filter
	Sort          string // column to ORDER BY
	Desc          bool
	Limit, Offset int
}

// filterOps maps allowlisted operator keys to SQL. IS NULL / IS NOT NULL are
// handled separately because they take no value.
var filterOps = map[string]string{
	"eq":    "=",
	"ne":    "<>",
	"lt":    "<",
	"lte":   "<=",
	"gt":    ">",
	"gte":   ">=",
	"like":  "LIKE",
	"ilike": "ILIKE",
}

// TableRows returns a page of rows from schema.table, optionally filtered and
// sorted. Safety: every column name is validated against the relation's real
// columns and emitted via pgx.Identifier; operators come from an allowlist;
// values are bound as query parameters; sort direction is ASC/DESC only. The
// read-only role + statement timeout + row cap remain the real safeguards.
func (p *Pool) TableRows(ctx context.Context, q TableQuery) (*Result, error) {
	limit := q.Limit
	if limit <= 0 || limit > p.rowCap {
		limit = p.rowCap
	}
	offset := max(q.Offset, 0)
	ident := pgx.Identifier{q.Schema, q.Table}.Sanitize()

	var (
		where []string
		args  []any
		valid map[string]bool
		names []string
	)
	if q.Search != "" || len(q.Filters) > 0 || q.Sort != "" {
		cols, err := p.Columns(ctx, q.Schema, q.Table)
		if err != nil {
			return nil, err
		}
		valid = make(map[string]bool, len(cols))
		for _, c := range cols {
			valid[c.Name] = true
			names = append(names, c.Name)
		}
		if len(names) == 0 {
			return nil, fmt.Errorf("relation %s has no columns", ident)
		}
	}

	if q.Search != "" {
		args = append(args, "%"+q.Search+"%")
		ph := "$" + strconv.Itoa(len(args))
		ors := make([]string, len(names))
		for i, c := range names {
			ors[i] = pgx.Identifier{c}.Sanitize() + "::text ILIKE " + ph
		}
		where = append(where, "("+strings.Join(ors, " OR ")+")")
	}

	for _, f := range q.Filters {
		if !valid[f.Column] {
			return nil, fmt.Errorf("unknown column %q", f.Column)
		}
		id := pgx.Identifier{f.Column}.Sanitize()
		switch f.Op {
		case "is_null":
			where = append(where, id+" IS NULL")
		case "is_not_null":
			where = append(where, id+" IS NOT NULL")
		default:
			op, ok := filterOps[f.Op]
			if !ok {
				return nil, fmt.Errorf("unknown operator %q", f.Op)
			}
			args = append(args, f.Value)
			if f.Op == "like" || f.Op == "ilike" {
				id += "::text"
			}
			where = append(where, id+" "+op+" $"+strconv.Itoa(len(args)))
		}
	}

	sql := "SELECT * FROM " + ident
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	if q.Sort != "" {
		if !valid[q.Sort] {
			return nil, fmt.Errorf("unknown sort column %q", q.Sort)
		}
		sql += " ORDER BY " + pgx.Identifier{q.Sort}.Sanitize()
		if q.Desc {
			sql += " DESC"
		} else {
			sql += " ASC"
		}
	}
	sql += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)

	start := time.Now()
	rows, err := p.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return p.collect(rows, start)
}
