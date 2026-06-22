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
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/descope/pgpeek/internal/awsauth"
	"github.com/descope/pgpeek/internal/config"
	"github.com/descope/pgpeek/internal/db"
	"github.com/descope/pgpeek/internal/server"
	"github.com/descope/pgpeek/internal/store"
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

	dbCfg := db.Config{
		DSN:              cfg.DB.DSN,
		MaxConns:         cfg.DB.MaxConns,
		StatementTimeout: cfg.DB.StatementTimeout,
		IdleTxTimeout:    cfg.DB.IdleTxTimeout,
		RowCap:           cfg.RowCap,
	}
	if cfg.DB.IAMAuth {
		provider, perr := awsauth.New(ctx, cfg.DB.Region)
		if perr != nil {
			return perr
		}
		dbCfg.BeforeConnect = provider.BeforeConnect
		log.Info("RDS IAM authentication enabled", "region", cfg.DB.Region)
	}

	signalCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.New(signalCtx, dbCfg)
	if err != nil {
		return err
	}
	defer pool.Close()
	log.Info("connected to database", "maxConns", cfg.DB.MaxConns, "rowCap", cfg.RowCap,
		"statementTimeout", cfg.DB.StatementTimeout.String(), "iamAuth", cfg.DB.IAMAuth)

	st, err := store.Open(cfg.StorePath)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	if err := st.SeedPresets(signalCtx, store.DefaultPresets); err != nil {
		log.Warn("seed presets", "err", err)
	}

	web := mustSubFS(webFiles, "web")
	srv := server.New(pool, st, web, log, cfg.DB.StatementTimeout+5*time.Second)
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
