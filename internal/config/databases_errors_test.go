package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestLoadDatabases_returns_required_error_when_no_sources_configured(t *testing.T) {
	// Given: no database URL source is configured.
	clearEnv(t)

	// When: database entries are loaded.
	_, _, err := loadDatabases(false, "")

	// Then: configuration fails with the public missing-URL message.
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("loadDatabases error = %v, want DATABASE_URL requirement", err)
	}
}

func TestAppendDatabaseFileEntries_uses_default_database_id_when_present(t *testing.T) {
	// Given: a mounted database config uses the explicit defaultDatabaseID field.
	clearEnv(t)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "databases.json")
	body := `{"default":"legacy","defaultDatabaseID":"primary","databases":[{"id":"primary","url":"postgres://u:p@h/db"}]}`
	if err := os.WriteFile(configPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PGPEEK_DATABASES_FILE", configPath)
	entries := []DatabaseEntry{}

	// When: database entries are loaded from the file.
	defaultID, err := appendDatabaseFileEntries(&entries, false, "us-east-1")

	// Then: defaultDatabaseID takes precedence over legacy default.
	if err != nil {
		t.Fatalf("appendDatabaseFileEntries: %v", err)
	}
	if defaultID != "primary" || len(entries) != 1 || entries[0].Region != "us-east-1" {
		t.Fatalf("default=%q entries=%+v", defaultID, entries)
	}
}

func TestAppendDatabaseFileEntries_returns_parse_error_when_file_invalid(t *testing.T) {
	// Given: the mounted database config is not valid JSON.
	clearEnv(t)
	configPath := filepath.Join(t.TempDir(), "databases.json")
	if err := os.WriteFile(configPath, []byte(`{"databases":`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PGPEEK_DATABASES_FILE", configPath)

	// When: databases are loaded.
	_, _, err := loadDatabases(false, "")

	// Then: the parse error names the operator-provided file variable.
	if err == nil || !strings.Contains(err.Error(), "parse PGPEEK_DATABASES_FILE") {
		t.Fatalf("appendDatabaseFileEntries error = %v", err)
	}
}

func TestAppendDatabaseFileEntries_returns_read_error_when_file_missing(t *testing.T) {
	// Given: the mounted database config file is missing.
	clearEnv(t)
	t.Setenv("PGPEEK_DATABASES_FILE", filepath.Join(t.TempDir(), "missing.json"))

	// When: databases are loaded.
	_, _, err := loadDatabases(false, "")

	// Then: the operator file read error is returned.
	if err == nil || !strings.Contains(err.Error(), "read PGPEEK_DATABASES_FILE") {
		t.Fatalf("loadDatabases error = %v", err)
	}
}

func TestDatabaseFileDSN_returns_read_error_when_url_file_missing(t *testing.T) {
	// Given: an entry references a missing urlFile.
	clearEnv(t)
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing-url")
	configPath := filepath.Join(dir, "databases.json")
	body := `{"databases":[{"id":"primary","urlFile":` + strconv.Quote(missing) + `}]}`
	if err := os.WriteFile(configPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PGPEEK_DATABASES_FILE", configPath)

	// When: databases are loaded.
	_, _, err := loadDatabases(false, "")

	// Then: the read error is returned without exposing a DSN.
	if err == nil || !strings.Contains(err.Error(), "read database urlFile") {
		t.Fatalf("databaseFileDSN error = %v", err)
	}
}

func TestAppendListDatabaseEntries_returns_parse_error_when_csv_invalid(t *testing.T) {
	// Given: list-form URLs contain malformed CSV quoting.
	clearEnv(t)
	t.Setenv("PGPEEK_DATABASE_URLS", `"postgres://u:p@h/db`)

	// When: databases are loaded.
	_, _, err := loadDatabases(false, "")

	// Then: parsing fails before any entry is appended.
	if err == nil || !strings.Contains(err.Error(), "parse database list") {
		t.Fatalf("appendListDatabaseEntries error = %v", err)
	}
}

func TestAppendListDatabaseEntries_returns_parse_error_when_ids_or_names_invalid(t *testing.T) {
	tests := []struct {
		name string
		env  string
	}{
		{name: "ids", env: "PGPEEK_DATABASE_IDS"},
		{name: "names", env: "PGPEEK_DATABASE_NAMES"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: list-form URLs are valid but a companion list has malformed CSV quoting.
			clearEnv(t)
			t.Setenv("PGPEEK_DATABASE_URLS", "postgres://u:p@h/db")
			t.Setenv(tt.env, `"unterminated`)

			// When: databases are loaded.
			_, _, err := loadDatabases(false, "")

			// Then: companion-list parsing fails.
			if err == nil || !strings.Contains(err.Error(), "parse database list") {
				t.Fatalf("loadDatabases error = %v", err)
			}
		})
	}
}

func TestAppendNumberedDatabaseEntries_returns_file_error_when_url_file_missing(t *testing.T) {
	// Given: numbered env configuration points at a missing mounted secret.
	clearEnv(t)
	t.Setenv("PGPEEK_DATABASE_URL_1_FILE", filepath.Join(t.TempDir(), "missing-url"))

	// When: databases are loaded.
	_, _, err := loadDatabases(false, "")

	// Then: the read error is returned.
	if err == nil || !strings.Contains(err.Error(), "read PGPEEK_DATABASE_URL_1") {
		t.Fatalf("appendNumberedDatabaseEntries error = %v", err)
	}
}

func TestLoadDatabasesRejectsMultipleMultiDatabaseSources(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T)
	}{
		{name: "file and list", setup: func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "databases.json")
			if err := os.WriteFile(configPath, []byte(`{"databases":[]}`), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PGPEEK_DATABASES_FILE", configPath)
			t.Setenv("PGPEEK_DATABASE_URLS", "postgres://u:p@h/db")
		}},
		{name: "list and numbered", setup: func(t *testing.T) {
			t.Setenv("PGPEEK_DATABASE_URLS", "postgres://u:p@h/db")
			t.Setenv("PGPEEK_DATABASE_URL_1", "postgres://u:p@h/other")
		}},
		{name: "file and numbered file", setup: func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "databases.json")
			if err := os.WriteFile(configPath, []byte(`{"databases":[]}`), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PGPEEK_DATABASES_FILE", configPath)
			t.Setenv("PGPEEK_DATABASE_URL_1_FILE", filepath.Join(t.TempDir(), "db-url"))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			tt.setup(t)

			_, _, err := loadDatabases(false, "")

			if err == nil || !strings.Contains(err.Error(), "configure only one multi-database source") {
				t.Fatalf("loadDatabases error = %v", err)
			}
		})
	}
}

func TestValidateDatabases_rejects_empty_or_incomplete_entries(t *testing.T) {
	tests := []struct {
		name string
		dbs  []DatabaseEntry
	}{
		{name: "empty registry", dbs: nil},
		{name: "empty id", dbs: []DatabaseEntry{{DSN: "postgres://u:p@h/db"}}},
		{name: "empty dsn", dbs: []DatabaseEntry{{ID: "primary"}}},
		{name: "iam missing region", dbs: []DatabaseEntry{{ID: "primary", DSN: "postgres://u:p@h/db", IAMAuth: true}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When: database entries are validated.
			err := validateDatabases(tt.dbs)

			// Then: invalid entries are rejected.
			if err == nil {
				t.Fatal("validateDatabases expected error")
			}
		})
	}
}
