// Command pgpeek is a minimal, read-only, team-shared Postgres browser.
//
// It deliberately avoids the failure modes that made Adminer unreliable:
// connection pooling (not a connection per request), result-row capping
// (never buffer an unbounded set), a per-session statement timeout, and
// stateless pods (saved queries live in a small SQLite file on a PVC).
package main

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/descope/pgpeek/internal/db"
	"github.com/descope/pgpeek/internal/server"
	"github.com/descope/pgpeek/internal/store"
)

//go:embed web
var webFiles embed.FS

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg := loadConfig()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.New(ctx, db.Config{
		DSN:              dsn,
		MaxConns:         cfg.maxConns,
		StatementTimeout: cfg.statementTimeout,
		IdleTxTimeout:    cfg.idleTxTimeout,
		RowCap:           cfg.rowCap,
	})
	if err != nil {
		return err // db.New wraps without leaking the DSN
	}
	defer pool.Close()
	log.Info("connected to database", "maxConns", cfg.maxConns, "rowCap", cfg.rowCap,
		"statementTimeout", cfg.statementTimeout.String())

	st, err := store.Open(cfg.storePath)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.SeedPresets(ctx, store.DefaultPresets); err != nil {
		log.Warn("seed presets", "err", err)
	}

	sub, err := fs.Sub(webFiles, "web")
	if err != nil {
		return err
	}

	srv := server.New(pool, st, sub, log, cfg.statementTimeout+5*time.Second)
	httpSrv := &http.Server{
		Addr:              cfg.listen,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      cfg.statementTimeout + 30*time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Info("listening", "addr", cfg.listen)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return httpSrv.Shutdown(shutCtx)
}

type config struct {
	listen           string
	storePath        string
	rowCap           int
	maxConns         int32
	statementTimeout time.Duration
	idleTxTimeout    time.Duration
}

func loadConfig() config {
	return config{
		listen:           env("PGPEEK_LISTEN", ":8080"),
		storePath:        env("PGPEEK_STORE_PATH", "/data/pgpeek.db"),
		rowCap:           envInt("PGPEEK_ROW_CAP", 1000),
		maxConns:         int32(envInt("PGPEEK_MAX_CONNS", 8)),
		statementTimeout: envDur("PGPEEK_STATEMENT_TIMEOUT", 30*time.Second),
		idleTxTimeout:    envDur("PGPEEK_IDLE_TX_TIMEOUT", 30*time.Second),
	}
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

func envDur(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
