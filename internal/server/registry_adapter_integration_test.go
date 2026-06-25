//go:build integration

package server

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/descope-sample-apps/pgpeek/internal/db"
)

func TestDatabaseRegistryAdapter_delegates_to_db_registry(t *testing.T) {
	// Given: a real database registry backs the server adapter.
	dsn := os.Getenv("PGPEEK_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PGPEEK_TEST_DATABASE_URL not set")
	}
	pool, err := db.New(context.Background(), db.Config{
		DSN:              dsn,
		MaxConns:         1,
		StatementTimeout: 5 * time.Second,
		IdleTxTimeout:    5 * time.Second,
		RowCap:           10,
	})
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(pool.Close)
	registry, err := db.NewRegistry([]db.RegistryEntry{{ID: "primary", Name: "Primary", Pool: pool, Default: true}})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	adapter := NewDatabaseRegistry(registry)

	// When: server code calls adapter methods.
	metadata := adapter.List()
	querier, poolErr := adapter.Pool("primary")
	pingErr := adapter.Ping(context.Background())

	// Then: calls return the underlying registry data and pool.
	if adapter.DefaultID() != "primary" || len(metadata) != 1 || metadata[0].Name != "Primary" {
		t.Fatalf("default=%q metadata=%+v", adapter.DefaultID(), metadata)
	}
	if poolErr != nil || querier == nil {
		t.Fatalf("Pool error=%v querier=%v", poolErr, querier)
	}
	if pingErr != nil {
		t.Fatalf("Ping: %v", pingErr)
	}
}
