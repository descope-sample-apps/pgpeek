package server

import (
	"errors"
	"strings"
	"testing"

	"github.com/descope-sample-apps/pgpeek/internal/db"
)

func TestMCP_describe_table_caps_foreign_keys_after_columns(t *testing.T) {
	// Given: columns fit but the relation has oversized foreign-key metadata.
	foreignKeys := make([]db.ForeignKey, 8)
	for i := range foreignKeys {
		foreignKeys[i] = db.ForeignKey{
			Column:    "account_id",
			RefSchema: "public",
			RefTable:  strings.Repeat("x", maxMCPStructuredContentBytes/4),
			RefColumn: "id",
		}
	}
	q := &fakeQuerier{
		cols: []db.ColumnInfo{{Name: "account_id", Type: "uuid"}},
		fks:  foreignKeys,
	}
	session := connectMCP(t, newMCPTestServer(t, q))

	// When: the client describes the relation.
	result := callMCPTool(t, session, "describe_table", map[string]any{"schema": "public", "table": "events"})
	got := decodeMCPOutput[struct {
		Columns     []db.ColumnInfo `json:"columns"`
		ForeignKeys []db.ForeignKey `json:"foreignKeys"`
		Truncated   bool            `json:"truncated"`
	}](t, result)

	// Then: all columns remain available while foreign keys are size-capped.
	if !got.Truncated || len(got.Columns) != 1 || len(got.ForeignKeys) >= len(foreignKeys) {
		t.Fatalf("columns=%d foreignKeys=%d truncated=%v", len(got.Columns), len(got.ForeignKeys), got.Truncated)
	}
	assertMCPResultWithinBudget(t, result)
}

func TestMCP_discovery_caps_reject_oversized_metadata(t *testing.T) {
	oversized := strings.Repeat("x", maxMCPStructuredContentBytes)

	t.Run("databases", func(t *testing.T) {
		// Given: database-list metadata alone exceeds the structured-content budget.
		output := mcpListDatabasesOutput{DefaultDatabaseID: oversized, Databases: []db.PoolMetadata{}}

		// When: the output cap is applied.
		_, err := capMCPListDatabasesOutput(output)

		// Then: the result is rejected rather than emitted oversized.
		if !errors.Is(err, errMCPResultMetadataExceedsLimit) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("tables", func(t *testing.T) {
		// Given: table-list metadata alone exceeds the structured-content budget.
		output := mcpListTablesOutput{DatabaseID: oversized, Tables: []db.TableInfo{}}

		// When: the output cap is applied.
		_, err := capMCPListTablesOutput(output)

		// Then: the result is rejected rather than emitted oversized.
		if !errors.Is(err, errMCPResultMetadataExceedsLimit) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("description", func(t *testing.T) {
		// Given: relation identity metadata alone exceeds the structured-content budget.
		output := mcpDescribeTableOutput{DatabaseID: oversized, Columns: []db.ColumnInfo{}, ForeignKeys: []db.ForeignKey{}}

		// When: the output cap is applied.
		_, err := capMCPDescribeTableOutput(output)

		// Then: the result is rejected rather than emitted oversized.
		if !errors.Is(err, errMCPResultMetadataExceedsLimit) {
			t.Fatalf("error = %v", err)
		}
	})
}
