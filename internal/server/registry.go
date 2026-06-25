package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/descope/pgpeek/internal/db"
	"github.com/descope/pgpeek/internal/store"
)

type Querier interface {
	Query(ctx context.Context, sql string) (*db.Result, error)
	Tables(ctx context.Context) ([]db.TableInfo, error)
	Columns(ctx context.Context, schema, table string) ([]db.ColumnInfo, error)
	ForeignKeys(ctx context.Context, schema, table string) ([]db.ForeignKey, error)
	TableRows(ctx context.Context, q db.TableQuery) (*db.Result, error)
	RowCap() int
	Ping(ctx context.Context) error
}

type QueryStore interface {
	List(ctx context.Context) ([]store.SavedQuery, error)
	Get(ctx context.Context, id int64) (store.SavedQuery, error)
	Create(ctx context.Context, name, desc, sql string) (store.SavedQuery, error)
	Update(ctx context.Context, id int64, name, desc, sql string) (store.SavedQuery, error)
	Delete(ctx context.Context, id int64) error
}

type DatabaseRegistry interface {
	List() []db.PoolMetadata
	DefaultID() string
	Pool(id string) (Querier, error)
	Ping(ctx context.Context) error
}

type singleDatabaseRegistry struct {
	pool Querier
}

func NewSingleDatabaseRegistry(pool Querier) DatabaseRegistry {
	return singleDatabaseRegistry{pool: pool}
}

func NewDatabaseRegistry(registry *db.Registry) DatabaseRegistry {
	return dbRegistryAdapter{registry: registry}
}

func (r singleDatabaseRegistry) List() []db.PoolMetadata {
	return []db.PoolMetadata{{ID: "default", Name: "Default"}}
}

func (r singleDatabaseRegistry) DefaultID() string { return "default" }

func (r singleDatabaseRegistry) Pool(id string) (Querier, error) {
	if id == "" || id == r.DefaultID() {
		return r.pool, nil
	}
	return nil, db.ErrPoolNotFound
}

func (r singleDatabaseRegistry) Ping(ctx context.Context) error {
	return r.pool.Ping(ctx)
}

type dbRegistryAdapter struct {
	registry *db.Registry
}

type databasesResponse struct {
	DefaultID string            `json:"defaultId"`
	Databases []db.PoolMetadata `json:"databases"`
}

func (r dbRegistryAdapter) List() []db.PoolMetadata { return r.registry.List() }

func (r dbRegistryAdapter) DefaultID() string { return r.registry.DefaultID() }

func (r dbRegistryAdapter) Pool(id string) (Querier, error) {
	return r.registry.Pool(id)
}

func (r dbRegistryAdapter) Ping(ctx context.Context) error { return r.registry.Ping(ctx) }

func (s *Server) handleDatabases(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, databasesResponse{DefaultID: s.registry.DefaultID(), Databases: s.registry.List()})
}

func (s *Server) poolForRequest(w http.ResponseWriter, r *http.Request) (Querier, bool) {
	pool, err := s.registry.Pool(strings.TrimSpace(r.URL.Query().Get("db")))
	if errors.Is(err, db.ErrPoolNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return nil, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		s.log.Error("select database", "err", err)
		return nil, false
	}
	return pool, true
}
