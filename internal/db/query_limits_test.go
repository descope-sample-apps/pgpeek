package db

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestQuery_ByteCap(t *testing.T) {
	value := strings.Repeat("x", MaxResultBytes/2)
	rows := &fakeRows{cols: []string{"payload"}, data: [][]any{{value}, {value}, {value}}}
	p := &Pool{pool: &fakePool{rows: rows}, rowCap: 10}
	res, err := p.Query(context.Background(), "SELECT payload")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	encoded, err := json.Marshal(res.Rows)
	if err != nil {
		t.Fatalf("Marshal rows: %v", err)
	}
	if len(encoded) > MaxResultBytes || !res.Truncated || len(res.Rows) >= 3 {
		t.Fatalf("encoded=%d rows=%d truncated=%v", len(encoded), len(res.Rows), res.Truncated)
	}
}

func TestQuery_RejectsUnencodableRow(t *testing.T) {
	rows := &fakeRows{cols: []string{"payload"}, data: [][]any{{math.NaN()}}}
	p := &Pool{pool: &fakePool{rows: rows}, rowCap: 10}
	if _, err := p.Query(context.Background(), "SELECT payload"); err == nil {
		t.Fatal("expected JSON encoding error")
	}
}
