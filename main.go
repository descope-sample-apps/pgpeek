// Command pgpeek is a minimal, read-only, team-shared Postgres browser.
//
// It deliberately avoids the failure modes that made Adminer unreliable:
// connection pooling (not a connection per request), result-row capping
// (never buffer an unbounded set), a per-session statement timeout, and
// stateless pods (saved queries live in a small SQLite file on a PVC).
//
// All configuration comes from the environment (see internal/config), secrets
// may be supplied via mounted files, and the database connection supports
// RDS/Aurora IAM authentication.
package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/descope-sample-apps/pgpeek/internal/awsauth"
	"github.com/descope-sample-apps/pgpeek/internal/config"
	"github.com/descope-sample-apps/pgpeek/internal/db"
	"github.com/descope-sample-apps/pgpeek/internal/server"
	"github.com/descope-sample-apps/pgpeek/internal/store"
)

//go:embed web
var webFiles embed.FS

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	log.Info("starting pgpeek", "version", version)
	if err := run(context.Background(), log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	signalCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	entries := make([]db.RegistryEntry, 0, len(cfg.Databases))
	closeEntries := func() {
		for i := len(entries) - 1; i >= 0; i-- {
			entries[i].Pool.Close()
		}
	}
	for _, entry := range cfg.Databases {
		dbCfg := db.Config{
			DSN:              entry.DSN,
			MaxConns:         cfg.DB.MaxConns,
			StatementTimeout: cfg.DB.StatementTimeout,
			IdleTxTimeout:    cfg.DB.IdleTxTimeout,
			RowCap:           cfg.RowCap,
		}
		if entry.IAMAuth {
			provider, perr := awsauth.New(signalCtx, entry.Region)
			if perr != nil {
				closeEntries()
				return fmt.Errorf("create IAM auth provider for database %q: %w", entry.ID, perr)
			}
			dbCfg.BeforeConnect = provider.BeforeConnect
			log.Info("RDS IAM authentication enabled", "databaseID", entry.ID, "databaseName", entry.Name)
		}
		log.Info("connecting database", "databaseID", entry.ID, "databaseName", entry.Name, "default", entry.ID == cfg.DefaultDatabaseID)
		pool, perr := db.New(signalCtx, dbCfg)
		if perr != nil {
			closeEntries()
			return fmt.Errorf("connect database %q: %w", entry.ID, perr)
		}
		entries = append(entries, db.RegistryEntry{ID: entry.ID, Name: entry.Name, Pool: pool, Default: entry.ID == cfg.DefaultDatabaseID})
	}
	registry, err := db.NewRegistry(entries)
	if err != nil {
		closeEntries()
		return fmt.Errorf("create database registry: %w", err)
	}
	defer registry.Close()
	log.Info("database registry ready", "databaseCount", len(entries), "defaultDatabaseID", registry.DefaultID())

	st, err := store.Open(cfg.StorePath)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	if err := st.SeedPresets(signalCtx, store.DefaultPresets); err != nil {
		log.Warn("seed presets", "err", err)
	}

	web := mustSubFS(webFiles, "web")
	srv := server.NewWithRegistry(server.NewDatabaseRegistry(registry), st, web, log, cfg.DB.StatementTimeout+5*time.Second)
	httpSrv := &http.Server{
		Addr:              cfg.Server.Listen,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		IdleTimeout:       cfg.Server.IdleTimeout,
	}
	return serve(signalCtx, log, httpSrv, cfg.Server)
}

// serve runs the HTTP server until ctx is cancelled or the server fails, then
// shuts down gracefully.
func serve(ctx context.Context, log *slog.Logger, httpSrv *http.Server, sc config.Server) error {
	errCh := make(chan error, 1)
	go func() {
		var serveErr error
		if sc.TLSEnabled() {
			log.Info("listening", "addr", httpSrv.Addr, "tls", true)
			serveErr = httpSrv.ListenAndServeTLS(sc.TLSCertFile, sc.TLSKeyFile)
		} else {
			log.Info("listening", "addr", httpSrv.Addr, "tls", false)
			serveErr = httpSrv.ListenAndServe()
		}
		if errors.Is(serveErr, http.ErrServerClosed) {
			serveErr = nil
		}
		errCh <- serveErr
	}()

	select {
	case <-ctx.Done():
		log.Info("shutting down")
		shutCtx, cancel := context.WithTimeout(context.Background(), sc.ShutdownTimeout)
		defer cancel()
		return httpSrv.Shutdown(shutCtx)
	case err := <-errCh:
		return err
	}
}

// mustSubFS returns the sub-filesystem rooted at dir. The embed path is a
// compile-time constant, so this never fails at runtime.
func mustSubFS(f fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(f, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
