package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

var registerMigrateFailDriver sync.Once

type migrateFailDriver struct{}

func (migrateFailDriver) Open(string) (driver.Conn, error) { return migrateFailConn{}, nil }

type migrateFailConn struct{}

func (migrateFailConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("prepare failed") }
func (migrateFailConn) Close() error                        { return nil }
func (migrateFailConn) Begin() (driver.Tx, error)           { return nil, errors.New("begin failed") }

func (migrateFailConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return nil, errors.New("migrate failed")
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	return newTestStore2(t, filepath.Join(t.TempDir(), "test.db"))
}

func newTestStore2(t *testing.T, path string) *Store {
	t.Helper()
	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestCreateGetListUpdateDelete(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	created, err := st.Create(ctx, "q1", "desc", "SELECT 1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == 0 || created.Name != "q1" || created.IsPreset {
		t.Fatalf("unexpected created: %+v", created)
	}
	if created.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set")
	}

	got, err := st.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.SQL != "SELECT 1" {
		t.Errorf("Get SQL = %q", got.SQL)
	}

	list, err := st.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List len = %d, want 1", len(list))
	}

	updated, err := st.Update(ctx, created.ID, "q1b", "d2", "SELECT 2")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "q1b" || updated.SQL != "SELECT 2" {
		t.Errorf("Update result = %+v", updated)
	}

	if err := st.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if list, _ := st.List(ctx); len(list) != 0 {
		t.Errorf("after delete List len = %d, want 0", len(list))
	}
}

func TestNotFound(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	if _, err := st.Get(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get missing: got %v, want ErrNotFound", err)
	}
	if _, err := st.Update(ctx, 999, "x", "", "SELECT 1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Update missing: got %v, want ErrNotFound", err)
	}
	if err := st.Delete(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete missing: got %v, want ErrNotFound", err)
	}
}

func TestSeedPresets(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	presets := []Preset{
		{Name: "p1", Description: "d", SQL: "SELECT 1"},
		{Name: "p2", Description: "d", SQL: "SELECT 2"},
	}
	if err := st.SeedPresets(ctx, presets); err != nil {
		t.Fatalf("SeedPresets: %v", err)
	}
	list, _ := st.List(ctx)
	if len(list) != 2 {
		t.Fatalf("after seed len = %d, want 2", len(list))
	}
	for _, q := range list {
		if !q.IsPreset {
			t.Errorf("%q should be marked preset", q.Name)
		}
	}

	// Idempotent: seeding again into a non-empty store is a no-op.
	if err := st.SeedPresets(ctx, presets); err != nil {
		t.Fatalf("re-seed: %v", err)
	}
	if list, _ := st.List(ctx); len(list) != 2 {
		t.Errorf("after re-seed len = %d, want 2 (no duplicates)", len(list))
	}
}

func TestSeedPresetsSkippedWhenUserDataExists(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if _, err := st.Create(ctx, "mine", "", "SELECT 1"); err != nil {
		t.Fatal(err)
	}
	if err := st.SeedPresets(ctx, DefaultPresets); err != nil {
		t.Fatalf("SeedPresets: %v", err)
	}
	list, _ := st.List(ctx)
	if len(list) != 1 {
		t.Errorf("presets must not seed over existing data: len = %d, want 1", len(list))
	}
}

func TestListOrderingPresetsFirst(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if err := st.SeedPresets(ctx, []Preset{{Name: "zeta-preset", SQL: "SELECT 1"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Create(ctx, "alpha-user", "", "SELECT 1"); err != nil {
		t.Fatal(err)
	}
	list, _ := st.List(ctx)
	if len(list) != 2 || !list[0].IsPreset {
		t.Fatalf("presets should sort first, got %+v", list)
	}
}

func TestOpen_CreatesParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-dir", "x.db")
	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatalf("parent dir: %v", err)
	}
}

func TestOpen_MkdirError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-dir")
	if err := os.WriteFile(path, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(filepath.Join(path, "x.db")); err == nil {
		t.Fatal("expected mkdir error for path below file")
	}
}

func TestOpen_MigrateError(t *testing.T) {
	registerMigrateFailDriver.Do(func() { sql.Register("migratefail", migrateFailDriver{}) })
	orig := sqlOpen
	sqlOpen = func(string, string) (*sql.DB, error) { return sql.Open("migratefail", "") }
	t.Cleanup(func() { sqlOpen = orig })

	if _, err := Open(filepath.Join(t.TempDir(), "x.db")); err == nil {
		t.Fatal("expected migrate error")
	}
}

func TestOperationsOnClosedStore(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "c.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := st.List(ctx); err == nil {
		t.Error("List should error on closed store")
	}
	if _, err := st.Get(ctx, 1); err == nil {
		t.Error("Get should error on closed store")
	}
	if _, err := st.Create(ctx, "n", "d", "SELECT 1"); err == nil {
		t.Error("Create should error on closed store")
	}
	if _, err := st.Update(ctx, 1, "n", "d", "SELECT 1"); err == nil {
		t.Error("Update should error on closed store")
	}
	if err := st.Delete(ctx, 1); err == nil {
		t.Error("Delete should error on closed store")
	}
	if err := st.SeedPresets(ctx, DefaultPresets); err == nil {
		t.Error("SeedPresets should error on closed store")
	}
}

func TestOpen_SQLOpenError(t *testing.T) {
	orig := sqlOpen
	sqlOpen = func(string, string) (*sql.DB, error) { return nil, errors.New("open failed") }
	t.Cleanup(func() { sqlOpen = orig })
	if _, err := Open(filepath.Join(t.TempDir(), "x.db")); err == nil {
		t.Fatal("expected sql.Open error to propagate")
	}
}

func TestList_ScanError(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "scan.db")
	st := newTestStore2(t, path)

	// Inject a row with an unparseable updated_at via a second connection, so
	// List's row scan into time.Time fails.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	if _, err := raw.ExecContext(ctx,
		`INSERT INTO saved_queries (name, description, sql, is_preset, updated_at)
		 VALUES ('n','','SELECT 1',0,'not-a-timestamp')`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.List(ctx); err == nil {
		t.Error("expected scan error from bad updated_at")
	}
}

func TestSeedPresets_InsertError(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "seed.db")
	st := newTestStore2(t, path)

	// Rename a column out from under the insert via a second connection so the
	// COUNT(*) check passes (0 rows) but the INSERT fails.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	if _, err := raw.ExecContext(ctx, `ALTER TABLE saved_queries RENAME COLUMN sql TO sql_old`); err != nil {
		t.Fatal(err)
	}
	if err := st.SeedPresets(ctx, DefaultPresets); err == nil {
		t.Error("expected insert error after column rename")
	}
}

func TestDefaultPresetsValid(t *testing.T) {
	if len(DefaultPresets) == 0 {
		t.Fatal("expected at least one default preset")
	}
	for _, p := range DefaultPresets {
		if p.Name == "" || p.SQL == "" {
			t.Errorf("preset has empty name or sql: %+v", p)
		}
	}
}
