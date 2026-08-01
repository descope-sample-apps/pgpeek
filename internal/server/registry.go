package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/descope-sample-apps/pgpeek/internal/db"
	"github.com/descope-sample-apps/pgpeek/internal/store"
)

type Querier interface {
	Query(ctx context.Context, sql string) (*db.Result, error)
	Tables(ctx context.Context) ([]db.TableInfo, bool, error)
	Columns(ctx context.Context, schema, table string) ([]db.ColumnInfo, bool, error)
	ForeignKeys(ctx context.Context, schema, table string) ([]db.ForeignKey, bool, error)
	TableRows(ctx context.Context, q db.TableQuery) (*db.Result, error)
	RowCap() int
	MaxConns() int32
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
	DefaultID string         `json:"defaultId"`
	Databases []databaseInfo `json:"databases"`
}

type databaseInfo struct {
	db.PoolMetadata
	Version            string `json:"version,omitempty"`
	UptimeSeconds      int64  `json:"uptimeSeconds,omitempty"`
	DatabaseSize       string `json:"databaseSize,omitempty"`
	ActiveConnections  int64  `json:"activeConnections,omitempty"`
	MaxConnections     int64  `json:"maxConnections,omitempty"`
	PoolMaxConnections int32  `json:"poolMaxConnections"`
	Error              string `json:"error,omitempty"`
}

func (r dbRegistryAdapter) List() []db.PoolMetadata { return r.registry.List() }

func (r dbRegistryAdapter) DefaultID() string { return r.registry.DefaultID() }

func (r dbRegistryAdapter) Pool(id string) (Querier, error) {
	return r.registry.Pool(id)
}

func (r dbRegistryAdapter) Ping(ctx context.Context) error { return r.registry.Ping(ctx) }

func (s *Server) handleDatabases(w http.ResponseWriter, r *http.Request) {
	metadata := s.registry.List()
	databases := make([]databaseInfo, 0, len(metadata))
	for _, item := range metadata {
		info := databaseInfo{PoolMetadata: item}
		pool, err := s.registry.Pool(item.ID)
		if err == nil {
			info.PoolMaxConnections = pool.MaxConns()
			queryCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			result, queryErr := pool.Query(queryCtx, `SELECT current_setting('server_version'), EXTRACT(EPOCH FROM (clock_timestamp() - pg_postmaster_start_time()))::bigint, pg_size_pretty(pg_database_size(current_database())), (SELECT count(*) FROM pg_stat_activity WHERE datname = current_database()), current_setting('max_connections')::bigint`)
			cancel()
			if queryErr == nil && result != nil && len(result.Rows) == 1 && len(result.Rows[0]) == 5 {
				info.Version, _ = result.Rows[0][0].(string)
				info.UptimeSeconds, _ = result.Rows[0][1].(int64)
				info.DatabaseSize, _ = result.Rows[0][2].(string)
				info.ActiveConnections, _ = result.Rows[0][3].(int64)
				info.MaxConnections, _ = result.Rows[0][4].(int64)
			} else {
				info.Error = "details unavailable"
			}
		} else {
			info.Error = "unavailable"
		}
		databases = append(databases, info)
	}
	writeJSON(w, http.StatusOK, databasesResponse{DefaultID: s.registry.DefaultID(), Databases: databases})
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
