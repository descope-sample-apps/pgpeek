package db

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestQuery_ReturnsEveryRowWhenCellsAreLarge(t *testing.T) {
	value := strings.Repeat("x", 256<<10)
	rows := &fakeRows{cols: []string{"payload"}, data: [][]any{{value}, {value}, {value}}}
	p := &Pool{pool: &fakePool{rows: rows}, rowCap: 10}
	res, err := p.Query(context.Background(), "SELECT payload")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(res.Rows) != 3 || res.Truncated {
		t.Fatalf("rows=%d truncated=%v", len(res.Rows), res.Truncated)
	}
	if res.Rows[0][0] == value || !res.CellsTruncated || len(res.TruncatedCells) != 3 || res.TruncatedCells[0].Hash == "" {
		t.Fatalf("cell=%T cellsTruncated=%v refs=%v", res.Rows[0][0], res.CellsTruncated, res.TruncatedCells)
	}
}

func TestCellHash(t *testing.T) {
	hash := CellHash("value")
	if hash == "" || hash == CellHash("other") {
		t.Fatal("expected stable hash")
	}
	if CellHash(math.NaN()) != "" {
		t.Fatal("unencodable value should not have a hash")
	}
}

func TestQueryCell_ReturnsFullLargeValue(t *testing.T) {
	value := strings.Repeat("x", 256<<10)
	rows := &fakeRows{cols: []string{"payload"}, data: [][]any{{"skip"}, {value}}}
	p := &Pool{pool: &fakePool{rows: rows}, rowCap: 10}

	got, err := p.QueryCell(context.Background(), "SELECT payload", 1, 0)
	if err != nil {
		t.Fatalf("QueryCell: %v", err)
	}
	if got != value {
		t.Fatal("QueryCell did not return the full value")
	}
}

func TestQueryCell_RejectsInvalidIndexes(t *testing.T) {
	p := &Pool{pool: &fakePool{}, rowCap: 2}
	for _, tc := range []struct{ row, column int }{{-1, 0}, {2, 0}, {0, -1}} {
		if _, err := p.QueryCell(context.Background(), "SELECT payload", tc.row, tc.column); !errors.Is(err, ErrCellOutOfRange) {
			t.Fatalf("QueryCell(%d, %d) error = %v", tc.row, tc.column, err)
		}
	}
}

func TestQueryCell_QueryError(t *testing.T) {
	p := &Pool{pool: &fakePool{queryErr: errors.New("boom")}, rowCap: 2}
	if _, err := p.QueryCell(context.Background(), "SELECT payload", 0, 0); err == nil {
		t.Fatal("expected query error")
	}
}

func TestQueryCell_RowErrors(t *testing.T) {
	tests := []struct {
		name string
		rows *fakeRows
	}{
		{name: "column", rows: &fakeRows{cols: []string{"payload"}}},
		{name: "values", rows: &fakeRows{cols: []string{"payload"}, data: [][]any{{"x"}}, valErr: errors.New("values")}},
		{name: "rows", rows: &fakeRows{cols: []string{"payload"}, errErr: errors.New("rows")}},
		{name: "missing", rows: &fakeRows{cols: []string{"payload"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			column := 0
			if tc.name == "column" {
				column = 1
			}
			_, err := queryCell(tc.rows, 0, column)
			if err == nil {
				t.Fatal("expected row error")
			}
		})
	}
}

func TestTruncateCell_PreservesUTF8(t *testing.T) {
	value := strings.Repeat("x", cellPreviewBytes-1) + string([]byte{0xff}) + strings.Repeat("x", 10)
	cell, truncated := truncateCell(value)
	preview := cell.(string)
	if !truncated || !utf8.ValidString(preview) {
		t.Fatalf("truncated=%v valid=%v", truncated, utf8.ValidString(preview))
	}
	if _, smallTruncated := truncateCell("small"); smallTruncated {
		t.Fatal("small cell was truncated")
	}
}

func TestTruncateCell_LeavesUnencodableValueForQueryError(t *testing.T) {
	cell, truncated := truncateCell(math.NaN())
	if truncated || !math.IsNaN(cell.(float64)) {
		t.Fatalf("cell=%v truncated=%v", cell, truncated)
	}
}

func TestQuery_AggregateByteCapAfterCellTruncation(t *testing.T) {
	cell := strings.Repeat("x", cellPreviewBytes+1)
	row := make([]any, 3000)
	for i := range row {
		row[i] = cell
	}
	rows := &fakeRows{cols: make([]string, len(row)), data: [][]any{row, row}}
	p := &Pool{pool: &fakePool{rows: rows}, rowCap: 10}
	res, err := p.Query(context.Background(), "SELECT wide")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(res.Rows)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > MaxResultBytes || !res.Truncated {
		t.Fatalf("encoded=%d truncated=%v", len(encoded), res.Truncated)
	}
}

func TestQuery_RejectsUnencodableRow(t *testing.T) {
	rows := &fakeRows{cols: []string{"payload"}, data: [][]any{{math.NaN()}}}
	p := &Pool{pool: &fakePool{rows: rows}, rowCap: 10}
	if _, err := p.Query(context.Background(), "SELECT payload"); err == nil {
		t.Fatal("expected JSON encoding error")
	}
}
