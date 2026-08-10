package db

import (
	"context"
	"time"
)

func (p *Pool) TableRows(ctx context.Context, q TableQuery) (*Result, error) {
	sql, args, err := p.tableSQL(ctx, q)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	rows, err := p.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return p.collect(rows, start)
}

func (p *Pool) TableCell(ctx context.Context, q TableQuery, row, column int) (any, error) {
	pageLimit := q.Limit
	if pageLimit <= 0 || pageLimit > p.rowCap {
		pageLimit = p.rowCap
	}
	if row < 0 || row >= pageLimit || column < 0 {
		return nil, ErrCellOutOfRange
	}
	q.Offset = max(q.Offset, 0) + row
	q.Limit = 1
	sql, args, err := p.tableSQL(ctx, q)
	if err != nil {
		return nil, err
	}
	rows, err := p.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return queryCell(rows, 0, column)
}
