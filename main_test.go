package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/descope-sample-apps/pgpeek/internal/config"
)

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func clearAppEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"DATABASE_URL", "DATABASE_URL_FILE", "PGPEEK_LISTEN", "PGPEEK_STORE_PATH",
		"PGPEEK_ROW_CAP", "PGPEEK_MAX_CONNS", "PGPEEK_STATEMENT_TIMEOUT",
		"PGPEEK_DB_IAM_AUTH", "PGPEEK_AWS_REGION", "AWS_REGION",
		"PGPEEK_TLS_CERT_FILE", "PGPEEK_TLS_KEY_FILE",
	} {
		t.Setenv(k, "")
	}
}

// --- serve() -------------------------------------------------------------

func TestServe_ShutdownOnContextCancel(t *testing.T) {
	srv := &http.Server{Addr: "127.0.0.1:0", Handler: http.NewServeMux()}
	sc := config.Server{ShutdownTimeout: 2 * time.Second}
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- serve(ctx, testLogger(), srv, sc) }()
	time.Sleep(50 * time.Millisecond) // let it bind
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("serve returned %v, want nil on clean shutdown", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("serve did not return after cancel")
	}
}

func TestServe_ListenError(t *testing.T) {
	// An unbindable address makes ListenAndServe fail immediately.
	srv := &http.Server{Addr: "127.0.0.1:999999", Handler: http.NewServeMux()}
	sc := config.Server{ShutdownTimeout: time.Second}
	err := serve(context.Background(), testLogger(), srv, sc)
	if err == nil {
		t.Fatal("expected listen error")
	}
}

func TestServe_TLS(t *testing.T) {
	cert, key := writeSelfSignedCert(t)
	srv := &http.Server{Addr: "127.0.0.1:0", Handler: http.NewServeMux()}
	sc := config.Server{ShutdownTimeout: 2 * time.Second, TLSCertFile: cert, TLSKeyFile: key}
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- serve(ctx, testLogger(), srv, sc) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("TLS serve returned %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("TLS serve did not return")
	}
}

func TestServe_TLSError(t *testing.T) {
	srv := &http.Server{Addr: "127.0.0.1:0", Handler: http.NewServeMux()}
	sc := config.Server{ShutdownTimeout: time.Second, TLSCertFile: "/no/cert", TLSKeyFile: "/no/key"}
	if err := serve(context.Background(), testLogger(), srv, sc); err == nil {
		t.Fatal("expected TLS cert load error")
	}
}

// --- run() error paths (no database needed) ------------------------------

func TestRun_ConfigError(t *testing.T) {
	clearAppEnv(t) // no DATABASE_URL
	if err := run(context.Background(), testLogger()); err == nil {
		t.Fatal("expected config error")
	}
}

func TestRun_DBConnectError(t *testing.T) {
	clearAppEnv(t)
	// Valid DSN shape, unreachable host -> db.New fails on ping quickly.
	t.Setenv("DATABASE_URL", "postgres://u:p@127.0.0.1:1/db?connect_timeout=1&sslmode=disable")
	t.Setenv("PGPEEK_STORE_PATH", filepath.Join(t.TempDir(), "s.db"))
	if err := run(context.Background(), testLogger()); err == nil {
		t.Fatal("expected db connect error")
	}
}

func TestRun_IAMPathThenDBError(t *testing.T) {
	clearAppEnv(t)
	t.Setenv("DATABASE_URL", "postgres://u@127.0.0.1:1/db?connect_timeout=1&sslmode=disable")
	t.Setenv("PGPEEK_DB_IAM_AUTH", "true")
	t.Setenv("PGPEEK_AWS_REGION", "us-east-1")
	t.Setenv("PGPEEK_STORE_PATH", filepath.Join(t.TempDir(), "s.db"))
	// awsauth.New succeeds (offline), BeforeConnect is wired, db.New then fails.
	if err := run(context.Background(), testLogger()); err == nil {
		t.Fatal("expected db error after IAM setup")
	}
}

func TestMustSubFS(t *testing.T) {
	if fsys := mustSubFS(webFiles, "web"); fsys == nil {
		t.Fatal("expected sub FS")
	}
}

func TestMustSubFS_Panics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on invalid sub path")
		}
	}()
	mustSubFS(webFiles, "../escape") // invalid fs path -> fs.Sub errors -> panic
}

// --- run() happy path (requires a real Postgres) -------------------------

func TestRun_HappyPath(t *testing.T) {
	dsn := os.Getenv("PGPEEK_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PGPEEK_TEST_DATABASE_URL not set")
	}
	clearAppEnv(t)
	t.Setenv("DATABASE_URL", dsn)
	t.Setenv("PGPEEK_LISTEN", "127.0.0.1:0")
	t.Setenv("PGPEEK_STORE_PATH", filepath.Join(t.TempDir(), "s.db"))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- run(ctx, testLogger()) }()
	time.Sleep(300 * time.Millisecond) // let it connect + bind
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("run returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return after cancel")
	}
}

func TestRun_StoreOpenError(t *testing.T) {
	dsn := os.Getenv("PGPEEK_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PGPEEK_TEST_DATABASE_URL not set")
	}
	clearAppEnv(t)
	t.Setenv("DATABASE_URL", dsn)
	t.Setenv("PGPEEK_LISTEN", "127.0.0.1:0")
	// Store path inside a non-existent directory -> Open/migrate fails after the
	// database connects successfully.
	t.Setenv("PGPEEK_STORE_PATH", filepath.Join(t.TempDir(), "missing", "s.db"))
	if err := run(context.Background(), testLogger()); err == nil {
		t.Fatal("expected store open error")
	}
}

// writeSelfSignedCert generates a throwaway cert+key and returns their paths.
func writeSelfSignedCert(t *testing.T) (certPath, keyPath string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Unix(0, 0),
		NotAfter:     time.Unix(1<<31-1, 0),
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")

	certOut, _ := os.Create(certPath)
	_ = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der})
	_ = certOut.Close()

	keyDER, _ := x509.MarshalPKCS8PrivateKey(priv)
	keyOut, _ := os.Create(keyPath)
	_ = pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	_ = keyOut.Close()
	return certPath, keyPath
}
