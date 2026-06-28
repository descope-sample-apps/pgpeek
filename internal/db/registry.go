package db

import (
	"context"
	"errors"
	"fmt"
)

var ErrPoolNotFound = errors.New("database pool not found")

type PoolMetadata struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type RegistryEntry struct {
	ID      string
	Name    string
	Pool    *Pool
	Default bool
}

type Registry struct {
	entries   []registeredPool
	byID      map[string]*Pool
	defaultID string
}

type registeredPool struct {
	metadata PoolMetadata
	pool     *Pool
}

func NewRegistry(entries []RegistryEntry) (*Registry, error) {
	if len(entries) == 0 {
		return nil, errors.New("database registry requires at least one pool")
	}

	registered := make([]registeredPool, 0, len(entries))
	byID := make(map[string]*Pool, len(entries))
	defaultID := ""
	for _, entry := range entries {
		if entry.ID == "" {
			return nil, errors.New("database registry entry id is required")
		}
		if entry.Name == "" {
			return nil, fmt.Errorf("database registry entry %q name is required", entry.ID)
		}
		if entry.Pool == nil {
			return nil, fmt.Errorf("database registry entry %q pool is required", entry.ID)
		}
		if _, exists := byID[entry.ID]; exists {
			return nil, fmt.Errorf("database registry entry %q is duplicated", entry.ID)
		}
		if entry.Default {
			if defaultID != "" {
				return nil, errors.New("database registry has multiple defaults")
			}
			defaultID = entry.ID
		}

		byID[entry.ID] = entry.Pool
		registered = append(registered, registeredPool{
			metadata: PoolMetadata{ID: entry.ID, Name: entry.Name},
			pool:     entry.Pool,
		})
	}
	if defaultID == "" {
		defaultID = entries[0].ID
	}

	return &Registry{entries: registered, byID: byID, defaultID: defaultID}, nil
}

func (r *Registry) List() []PoolMetadata {
	metadata := make([]PoolMetadata, 0, len(r.entries))
	for _, entry := range r.entries {
		metadata = append(metadata, entry.metadata)
	}
	return metadata
}

func (r *Registry) DefaultID() string { return r.defaultID }

func (r *Registry) Pool(id string) (*Pool, error) {
	if id == "" {
		id = r.defaultID
	}
	pool, ok := r.byID[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrPoolNotFound, id)
	}
	return pool, nil
}

func (r *Registry) Ping(ctx context.Context) error {
	for _, entry := range r.entries {
		if err := entry.pool.Ping(ctx); err != nil {
			return fmt.Errorf("ping database pool %q: %w", entry.metadata.ID, err)
		}
	}
	return nil
}

func (r *Registry) Close() {
	for i := len(r.entries) - 1; i >= 0; i-- {
		r.entries[i].pool.Close()
	}
}
