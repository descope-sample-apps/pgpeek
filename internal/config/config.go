// Package config loads all runtime settings from environment variables, with a
// "<VAR>_FILE" fallback so secrets can be supplied via mounted files (Docker
// secrets / k8s projected volumes) instead of inline env values.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the fully-resolved application configuration.
type Config struct {
	Server            Server
	DB                DB
	Databases         []DatabaseEntry
	DefaultDatabaseID string
	StorePath         string
	RowCap            int
}

// Server holds HTTP server settings.
type Server struct {
	Listen            string
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	TLSCertFile       string
	TLSKeyFile        string
}

// TLSEnabled reports whether a cert+key pair was configured.
func (s Server) TLSEnabled() bool {
	return s.TLSCertFile != "" && s.TLSKeyFile != ""
}

// DB holds database connection settings.
type DB struct {
	DSN              string
	MaxConns         int32
	StatementTimeout time.Duration
	IdleTxTimeout    time.Duration

	// IAMAuth enables RDS/Aurora IAM authentication: the password is replaced
	// per-connection by a short-lived token minted from AWS credentials.
	IAMAuth bool
	Region  string
}

// Load reads and validates configuration from the environment.
func Load() (*Config, error) {
	stmtTimeout := envDur("PGPEEK_STATEMENT_TIMEOUT", 30*time.Second)
	iamAuth := envBool("PGPEEK_DB_IAM_AUTH", false)
	region := env("PGPEEK_AWS_REGION", os.Getenv("AWS_REGION"))
	databases, defaultDatabaseID, err := loadDatabases(iamAuth, region)
	if err != nil {
		return nil, err
	}

	c := &Config{
		Server: Server{
			Listen:            env("PGPEEK_LISTEN", ":8080"),
			ReadHeaderTimeout: envDur("PGPEEK_READ_HEADER_TIMEOUT", 10*time.Second),
			WriteTimeout:      envDur("PGPEEK_WRITE_TIMEOUT", stmtTimeout+30*time.Second),
			IdleTimeout:       envDur("PGPEEK_IDLE_TIMEOUT", 120*time.Second),
			ShutdownTimeout:   envDur("PGPEEK_SHUTDOWN_TIMEOUT", 15*time.Second),
			TLSCertFile:       os.Getenv("PGPEEK_TLS_CERT_FILE"),
			TLSKeyFile:        os.Getenv("PGPEEK_TLS_KEY_FILE"),
		},
		DB: DB{
			DSN:              "",
			MaxConns:         int32(envInt("PGPEEK_MAX_CONNS", 8)),
			StatementTimeout: stmtTimeout,
			IdleTxTimeout:    envDur("PGPEEK_IDLE_TX_TIMEOUT", 30*time.Second),
			IAMAuth:          iamAuth,
			Region:           region,
		},
		Databases:         databases,
		DefaultDatabaseID: defaultDatabaseID,
		StorePath:         env("PGPEEK_STORE_PATH", "/data/pgpeek.db"),
		RowCap:            envInt("PGPEEK_ROW_CAP", 1000),
	}
	if err := applyDefaultDatabase(c); err != nil {
		return nil, err
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Config) validate() error {
	if c.RowCap <= 0 {
		return errors.New("PGPEEK_ROW_CAP must be > 0")
	}
	if c.DB.MaxConns <= 0 {
		return errors.New("PGPEEK_MAX_CONNS must be > 0")
	}
	if c.DB.IAMAuth && c.DB.Region == "" {
		return errors.New("PGPEEK_DB_IAM_AUTH requires PGPEEK_AWS_REGION (or AWS_REGION)")
	}
	if err := validateDatabases(c.Databases); err != nil {
		return err
	}
	if (c.Server.TLSCertFile == "") != (c.Server.TLSKeyFile == "") {
		return errors.New("PGPEEK_TLS_CERT_FILE and PGPEEK_TLS_KEY_FILE must be set together")
	}
	return nil
}

// envOrFile returns the value of key, or the trimmed contents of the file named
// by "<key>_FILE", or "" if neither is set. This is the mounted-secret path.
func envOrFile(key string) (string, error) {
	if v := os.Getenv(key); v != "" {
		return v, nil
	}
	if path := os.Getenv(key + "_FILE"); path != "" {
		// The path is supplied by the operator (env var / mounted-secret
		// convention), not by any request input — this is the intended use.
		b, err := os.ReadFile(path) //nolint:gosec // operator-controlled secret path
		if err != nil {
			return "", fmt.Errorf("read %s_FILE: %w", key, err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	return "", nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func envDur(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
