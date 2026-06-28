package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestLoad_DatabaseURLsList(t *testing.T) {
	clearEnv(t)
	t.Setenv("PGPEEK_DATABASE_URLS", "postgres://u:secret@h/one, postgres://u:secret@h/two;postgres://u:secret@h/three")
	t.Setenv("PGPEEK_DATABASE_IDS", "analytics, billing")
	t.Setenv("PGPEEK_DATABASE_NAMES", "Analytics, Billing")
	t.Setenv("PGPEEK_DEFAULT_DATABASE", "billing")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.DefaultDatabaseID != "billing" || c.DB.DSN != "postgres://u:secret@h/two" {
		t.Fatalf("default database selection failed; id = %q", c.DefaultDatabaseID)
	}
	entries := c.Databases
	if len(entries) != 3 {
		t.Fatalf("databases = %d", len(entries))
	}
	if entries[0].ID != "analytics" || entries[0].Name != "Analytics" {
		t.Errorf("first entry id/name = %q/%q", entries[0].ID, entries[0].Name)
	}
	if entries[1].ID != "billing" || entries[1].Name != "Billing" {
		t.Errorf("second entry id/name = %q/%q", entries[1].ID, entries[1].Name)
	}
	if entries[2].ID != "db3" || entries[2].Name != "Database 3" {
		t.Errorf("derived entry id/name = %q/%q", entries[2].ID, entries[2].Name)
	}
	if strings.Contains(entries[2].Name, "secret") || strings.Contains(entries[2].Name, "postgres://") {
		t.Errorf("derived name exposes DSN: %q", entries[2].Name)
	}
}

func TestLoad_DatabaseURLsListQuotedSeparators(t *testing.T) {
	clearEnv(t)
	t.Setenv("PGPEEK_DATABASE_URLS", `"postgres://u:p@h/one?options=a,b";"postgres://u:p@h/two?options=x;y"`)
	t.Setenv("PGPEEK_DATABASE_IDS", `"one";"two"`)
	t.Setenv("PGPEEK_DATABASE_NAMES", `"One, Primary";"Two; Replica"`)
	t.Setenv("PGPEEK_DEFAULT_DATABASE", "two")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.Databases) != 2 {
		t.Fatalf("databases = %d", len(c.Databases))
	}
	if c.Databases[0].ID != "one" || c.Databases[0].Name != "One, Primary" || c.Databases[0].DSN != "postgres://u:p@h/one?options=a,b" {
		t.Errorf("quoted comma entry = id %q dsn %q", c.Databases[0].ID, c.Databases[0].DSN)
	}
	if c.Databases[1].ID != "two" || c.Databases[1].Name != "Two; Replica" || c.DB.DSN != "postgres://u:p@h/two?options=x;y" {
		t.Errorf("quoted semicolon entry = id %q", c.Databases[1].ID)
	}
}

func TestLoad_NumberedDatabaseURLs(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "two")
	if err := os.WriteFile(path, []byte(" postgres://u:secret@h/two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PGPEEK_DATABASE_URL_1", "postgres://u:secret@h/one")
	t.Setenv("PGPEEK_DATABASE_ID_1", "one")
	t.Setenv("PGPEEK_DATABASE_NAME_1", "One")
	t.Setenv("PGPEEK_DATABASE_URL_2_FILE", path)
	t.Setenv("PGPEEK_DATABASE_ID_2", "two")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.Databases) != 2 {
		t.Fatalf("databases = %d", len(c.Databases))
	}
	if c.Databases[1].ID != "two" || c.Databases[1].Name != "Database 2" {
		t.Errorf("file-backed numbered entry id/name = %q/%q", c.Databases[1].ID, c.Databases[1].Name)
	}
	if c.Databases[1].DSN != "postgres://u:secret@h/two" {
		t.Error("file-backed numbered entry DSN was not read from file")
	}
}

func TestLoad_DatabasesJSONFile(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	secretPath := filepath.Join(dir, "west-url")
	if err := os.WriteFile(secretPath, []byte("postgres://u:secret@h/west\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "databases.json")
	body := `{"default":"west","databases":[{"id":"east","name":"East","url":"postgres://u:secret@h/east","iamAuth":true,"region":"us-east-1"},{"id":"west","name":"West","urlFile":` + strconv.Quote(secretPath) + `,"iamAuth":true}]}`
	if err := os.WriteFile(configPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PGPEEK_DATABASES_FILE", configPath)
	t.Setenv("AWS_REGION", "us-west-2")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.DefaultDatabaseID != "west" || c.DB.DSN != "postgres://u:secret@h/west" || !c.DB.IAMAuth || c.DB.Region != "us-west-2" {
		t.Fatalf("default DB selection failed; id = %q iam = %v region = %q", c.DefaultDatabaseID, c.DB.IAMAuth, c.DB.Region)
	}
	if c.Databases[0].Region != "us-east-1" || c.Databases[1].Region != "us-west-2" {
		t.Errorf("entry regions = %q/%q", c.Databases[0].Region, c.Databases[1].Region)
	}
}

func TestLoad_DatabasesJSONFileInheritsGlobalIAMAuth(t *testing.T) {
	clearEnv(t)
	configPath := filepath.Join(t.TempDir(), "databases.json")
	body := `{"databases":[{"id":"prod","url":"postgres://u@h/prod"}]}`
	if err := os.WriteFile(configPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PGPEEK_DATABASES_FILE", configPath)
	t.Setenv("PGPEEK_DB_IAM_AUTH", "true")
	t.Setenv("PGPEEK_AWS_REGION", "us-east-1")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !c.Databases[0].IAMAuth || c.Databases[0].Region != "us-east-1" {
		t.Fatalf("file database did not inherit global IAM config: %+v", c.Databases[0])
	}
}

func TestLoad_DatabasesJSONFileItemIAMAuthOverridesGlobalFalse(t *testing.T) {
	clearEnv(t)
	configPath := filepath.Join(t.TempDir(), "databases.json")
	body := `{"databases":[{"id":"prod","url":"postgres://u@h/prod","iamAuth":true}]}`
	if err := os.WriteFile(configPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PGPEEK_DATABASES_FILE", configPath)
	t.Setenv("PGPEEK_AWS_REGION", "us-east-1")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !c.Databases[0].IAMAuth {
		t.Fatalf("file database item IAM setting lost: %+v", c.Databases[0])
	}
}

func TestLoad_DatabaseValidationErrors(t *testing.T) {
	cases := []struct {
		name      string
		setup     func(t *testing.T)
		secretDSN string
	}{
		{"duplicate id", func(t *testing.T) {
			t.Setenv("PGPEEK_DATABASE_URLS", "postgres://u:secret@h/one,postgres://u:secret@h/two")
			t.Setenv("PGPEEK_DATABASE_IDS", "same,same")
		}, "postgres://u:secret@h/one"},
		{"invalid id", func(t *testing.T) {
			t.Setenv("PGPEEK_DATABASE_URLS", "postgres://u:secret@h/one")
			t.Setenv("PGPEEK_DATABASE_IDS", "bad id")
		}, "postgres://u:secret@h/one"},
		{"unknown default", func(t *testing.T) {
			t.Setenv("PGPEEK_DATABASE_URLS", "postgres://u:secret@h/one")
			t.Setenv("PGPEEK_DEFAULT_DATABASE", "missing")
		}, "postgres://u:secret@h/one"},
		{"iam missing region", func(t *testing.T) {
			t.Setenv("PGPEEK_DATABASE_URLS", "postgres://u:secret@h/one")
			t.Setenv("PGPEEK_DATABASES_FILE", "")
			t.Setenv("PGPEEK_DB_IAM_AUTH", "true")
		}, "postgres://u:secret@h/one"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearEnv(t)
			tc.setup(t)
			_, err := Load()
			if err == nil {
				t.Fatal("expected validation error")
			}
			if strings.Contains(err.Error(), tc.secretDSN) || strings.Contains(err.Error(), "secret") {
				t.Fatalf("error exposes DSN secret: %v", err)
			}
		})
	}
}
