package db

import (
	"context"
	"math"
	"strings"
	"testing"
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
	if _, ok := res.Rows[0][0].(TruncatedCell); !ok || !res.CellsTruncated {
		t.Fatalf("cell=%T cellsTruncated=%v", res.Rows[0][0], res.CellsTruncated)
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

func TestQuery_RejectsUnencodableRow(t *testing.T) {
	rows := &fakeRows{cols: []string{"payload"}, data: [][]any{{math.NaN()}}}
	p := &Pool{pool: &fakePool{rows: rows}, rowCap: 10}
	if _, err := p.Query(context.Background(), "SELECT payload"); err == nil {
		t.Fatal("expected JSON encoding error")
	}
}
