// Package store persists saved/preset queries in a local SQLite file, kept
// completely independent of the Postgres database being browsed. modernc.org/sqlite
// is a pure-Go driver, so the binary stays static (no cgo) and the final image
// can be scratch/distroless.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned when a saved query id does not exist.
var ErrNotFound = errors.New("saved query not found")

// SavedQuery is a named, reusable query.
type SavedQuery struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	SQL         string    `json:"sql"`
	IsPreset    bool      `json:"isPreset"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// Store is the saved-query repository.
type Store struct {
	db *sql.DB
}

// Open opens (and migrates) the SQLite store at path.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite: serialize writers, avoids "database is locked"
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS saved_queries (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			name        TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			sql         TEXT NOT NULL,
			is_preset   INTEGER NOT NULL DEFAULT 0,
			updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the underlying database.
func (s *Store) Close() error { return s.db.Close() }

// List returns all saved queries, presets first, then alphabetical.
func (s *Store) List(ctx context.Context) ([]SavedQuery, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, description, sql, is_preset, updated_at
		FROM saved_queries
		ORDER BY is_preset DESC, name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SavedQuery
	for rows.Next() {
		q, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// Get returns a single saved query by id.
func (s *Store) Get(ctx context.Context, id int64) (SavedQuery, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, description, sql, is_preset, updated_at
		FROM saved_queries WHERE id = ?`, id)
	q, err := scan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return SavedQuery{}, ErrNotFound
	}
	return q, err
}

// Create inserts a new (non-preset) saved query and returns it.
func (s *Store) Create(ctx context.Context, name, desc, query string) (SavedQuery, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO saved_queries (name, description, sql, is_preset, updated_at)
		VALUES (?, ?, ?, 0, ?)`, name, desc, query, time.Now().UTC())
	if err != nil {
		return SavedQuery{}, err
	}
	id, _ := res.LastInsertId()
	return s.Get(ctx, id)
}

// Update modifies an existing saved query.
func (s *Store) Update(ctx context.Context, id int64, name, desc, query string) (SavedQuery, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE saved_queries
		SET name = ?, description = ?, sql = ?, updated_at = ?
		WHERE id = ?`, name, desc, query, time.Now().UTC(), id)
	if err != nil {
		return SavedQuery{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return SavedQuery{}, ErrNotFound
	}
	return s.Get(ctx, id)
}

// Delete removes a saved query.
func (s *Store) Delete(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM saved_queries WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Preset is a seed query loaded once on first run.
type Preset struct {
	Name        string
	Description string
	SQL         string
}

// SeedPresets inserts presets only if the store is empty, so it never clobbers
// edits the team has made. Presets are idempotent on first boot only.
func (s *Store) SeedPresets(ctx context.Context, presets []Preset) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM saved_queries`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	now := time.Now().UTC()
	for _, p := range presets {
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO saved_queries (name, description, sql, is_preset, updated_at)
			VALUES (?, ?, ?, 1, ?)`, p.Name, p.Description, p.SQL, now); err != nil {
			return err
		}
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scan(r scanner) (SavedQuery, error) {
	var q SavedQuery
	var preset int
	err := r.Scan(&q.ID, &q.Name, &q.Description, &q.SQL, &preset, &q.UpdatedAt)
	q.IsPreset = preset != 0
	return q, err
}
