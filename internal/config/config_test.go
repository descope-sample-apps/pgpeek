package config

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// clearEnv blanks every variable Load reads, so each test starts from defaults.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"DATABASE_URL", "DATABASE_URL_FILE",
		"PGPEEK_LISTEN", "PGPEEK_READ_HEADER_TIMEOUT", "PGPEEK_WRITE_TIMEOUT",
		"PGPEEK_IDLE_TIMEOUT", "PGPEEK_SHUTDOWN_TIMEOUT",
		"PGPEEK_TLS_CERT_FILE", "PGPEEK_TLS_KEY_FILE",
		"PGPEEK_MAX_CONNS", "PGPEEK_STATEMENT_TIMEOUT", "PGPEEK_IDLE_TX_TIMEOUT",
		"PGPEEK_DB_IAM_AUTH", "PGPEEK_AWS_REGION", "AWS_REGION",
		"PGPEEK_STORE_PATH", "PGPEEK_ROW_CAP",
		"PGPEEK_DATABASE_URLS", "PGPEEK_DATABASE_IDS", "PGPEEK_DATABASE_NAMES",
		"PGPEEK_DATABASES_FILE", "PGPEEK_DEFAULT_DATABASE",
	} {
		t.Setenv(k, "")
	}
	for i := 1; i <= maxNumberedDatabases; i++ {
		t.Setenv("PGPEEK_DATABASE_URL_"+strconv.Itoa(i), "")
		t.Setenv("PGPEEK_DATABASE_URL_"+strconv.Itoa(i)+"_FILE", "")
		t.Setenv("PGPEEK_DATABASE_ID_"+strconv.Itoa(i), "")
		t.Setenv("PGPEEK_DATABASE_NAME_"+strconv.Itoa(i), "")
	}
}

func TestLoad_Defaults(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://u:p@h:5432/db?sslmode=require")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Server.Listen != ":8080" {
		t.Errorf("Listen = %q", c.Server.Listen)
	}
	if c.RowCap != 1000 {
		t.Errorf("RowCap = %d", c.RowCap)
	}
	if c.DB.MaxConns != 8 {
		t.Errorf("MaxConns = %d", c.DB.MaxConns)
	}
	if c.DB.StatementTimeout != 30*time.Second {
		t.Errorf("StatementTimeout = %v", c.DB.StatementTimeout)
	}
	// WriteTimeout defaults to statementTimeout + 30s.
	if c.Server.WriteTimeout != 60*time.Second {
		t.Errorf("WriteTimeout = %v, want 60s", c.Server.WriteTimeout)
	}
	if c.StorePath != "/data/pgpeek.db" {
		t.Errorf("StorePath = %q", c.StorePath)
	}
	if c.Server.TLSEnabled() {
		t.Error("TLS should be disabled by default")
	}
	if c.DefaultDatabaseID != "default" || len(c.Databases) != 1 {
		t.Fatalf("default database registry = %q/%d", c.DefaultDatabaseID, len(c.Databases))
	}
	if c.Databases[0].ID != "default" || c.Databases[0].Name != "Default" || c.Databases[0].DSN != c.DB.DSN {
		t.Errorf("default database entry = %+v", c.Databases[0])
	}
}

func TestLoad_Overrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://h/db")
	t.Setenv("PGPEEK_LISTEN", ":9999")
	t.Setenv("PGPEEK_ROW_CAP", "50")
	t.Setenv("PGPEEK_MAX_CONNS", "4")
	t.Setenv("PGPEEK_STATEMENT_TIMEOUT", "10s")
	t.Setenv("PGPEEK_WRITE_TIMEOUT", "5s")
	t.Setenv("PGPEEK_IDLE_TIMEOUT", "7s")
	t.Setenv("PGPEEK_READ_HEADER_TIMEOUT", "3s")
	t.Setenv("PGPEEK_SHUTDOWN_TIMEOUT", "8s")
	t.Setenv("PGPEEK_STORE_PATH", "/tmp/x.db")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Server.Listen != ":9999" || c.RowCap != 50 || c.DB.MaxConns != 4 {
		t.Errorf("overrides not applied: %+v", c)
	}
	if c.Server.WriteTimeout != 5*time.Second || c.Server.IdleTimeout != 7*time.Second {
		t.Errorf("server timeouts: %+v", c.Server)
	}
}

func TestLoad_DSNFromFile(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "url")
	if err := os.WriteFile(path, []byte("  postgres://from-file/db\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_URL_FILE", path)

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.DB.DSN != "postgres://from-file/db" {
		t.Errorf("DSN = %q (should be trimmed file contents)", c.DB.DSN)
	}
}

func TestLoad_DSNFileMissing(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL_FILE", filepath.Join(t.TempDir(), "does-not-exist"))
	if _, err := Load(); err == nil {
		t.Fatal("expected error reading missing DATABASE_URL_FILE")
	}
}

func TestLoad_DSNRequired(t *testing.T) {
	clearEnv(t)
	if _, err := Load(); err == nil {
		t.Fatal("expected error when DATABASE_URL is unset")
	}
}

func TestLoad_IAMAuth(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://u@h:5432/db?sslmode=require")
	t.Setenv("PGPEEK_DB_IAM_AUTH", "true")
	t.Setenv("PGPEEK_AWS_REGION", "us-east-1")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !c.DB.IAMAuth || c.DB.Region != "us-east-1" {
		t.Errorf("IAM config: %+v", c.DB)
	}
}

func TestLoad_IAMAuthFallsBackToAWSRegion(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://u@h/db")
	t.Setenv("PGPEEK_DB_IAM_AUTH", "1")
	t.Setenv("AWS_REGION", "eu-west-1")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.DB.Region != "eu-west-1" {
		t.Errorf("Region = %q, want eu-west-1", c.DB.Region)
	}
}

func TestValidate_Errors(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T)
	}{
		{"row cap zero", func(t *testing.T) { t.Setenv("PGPEEK_ROW_CAP", "0") }},
		{"max conns zero", func(t *testing.T) { t.Setenv("PGPEEK_MAX_CONNS", "0") }},
		{"iam without region", func(t *testing.T) { t.Setenv("PGPEEK_DB_IAM_AUTH", "true") }},
		{"tls cert only", func(t *testing.T) { t.Setenv("PGPEEK_TLS_CERT_FILE", "/c.pem") }},
		{"tls key only", func(t *testing.T) { t.Setenv("PGPEEK_TLS_KEY_FILE", "/k.pem") }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("DATABASE_URL", "postgres://h/db")
			c.setup(t)
			if _, err := Load(); err == nil {
				t.Errorf("%s: expected validation error", c.name)
			}
		})
	}
}

func TestTLSEnabledAndValid(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://h/db")
	t.Setenv("PGPEEK_TLS_CERT_FILE", "/c.pem")
	t.Setenv("PGPEEK_TLS_KEY_FILE", "/k.pem")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !c.Server.TLSEnabled() {
		t.Error("TLSEnabled should be true when both files set")
	}
}

func TestEnvHelperFallbacks(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://h/db")
	// Garbage values must fall back to defaults, not crash.
	t.Setenv("PGPEEK_ROW_CAP", "notanint")
	t.Setenv("PGPEEK_STATEMENT_TIMEOUT", "notaduration")
	t.Setenv("PGPEEK_DB_IAM_AUTH", "notabool")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.RowCap != 1000 {
		t.Errorf("RowCap fallback = %d", c.RowCap)
	}
	if c.DB.StatementTimeout != 30*time.Second {
		t.Errorf("StatementTimeout fallback = %v", c.DB.StatementTimeout)
	}
	if c.DB.IAMAuth {
		t.Error("IAMAuth should fall back to false on garbage")
	}
}
