package db

import (
	"context"
	"encoding/csv"
	"errors"
	"io"
	"strings"
)

var errCountUnavailable = errors.New("count query returned no rows")

func (p *Pool) Count(ctx context.Context, sql string) (int64, error) {
	sql = strings.TrimSuffix(strings.TrimSpace(sql), ";")
	rows, err := p.pool.Query(ctx, "SELECT count(*) FROM (\n"+sql+"\n) AS pgpeek_count")
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, err
		}
		return 0, errCountUnavailable
	}
	var count int64
	if err := rows.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (p *Pool) ExportCSV(ctx context.Context, sql string, dst io.Writer) error {
	rows, err := p.pool.Query(ctx, sql)
	if err != nil {
		return err
	}
	defer rows.Close()

	cw := csv.NewWriter(dst)
	fields := rows.FieldDescriptions()
	record := make([]string, len(fields))
	for i, field := range fields {
		record[i] = field.Name
	}
	if err := cw.Write(record); err != nil {
		return err
	}
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return err
		}
		for i, value := range values {
			record[i] = CellString(normalize(value))
		}
		if err := cw.Write(record); err != nil {
			return err
		}
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return err
	}
	return rows.Err()
}
