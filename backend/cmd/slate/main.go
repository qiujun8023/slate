package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
	// Dynamic content renders in user-chosen time zones; the runtime image has no zoneinfo.
	_ "time/tzdata"

	"github.com/jackc/pgx/v5"

	"slate/internal/blob"
	"slate/internal/config"
	"slate/internal/dynamic"
	"slate/internal/httpapi"
	"slate/internal/legacyimport"
	"slate/internal/media"
	"slate/internal/store"
	"slate/migrations"
)

const usage = `usage: slate <command>

  serve                      apply migrations, then run the HTTP server and background workers (default)
  migrate                    apply database migrations
  import <mysql-url>         copy a v0.1.x MySQL deployment into an empty database`

func main() {
	if err := config.LoadDotenv(".env"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	level, err := config.LogLevel()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cmd, args := "serve", os.Args[1:]
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "serve":
		err = serve(ctx)
	case "migrate":
		err = migrate(ctx)
	case "import":
		err = importLegacy(ctx, args)
	default:
		err = errors.New(usage)
	}
	if err != nil {
		slog.Error("command failed", "command", cmd, "error", err.Error())
		os.Exit(1)
	}
}

func migrate(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pc, err := pgx.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return err
	}
	pc.RuntimeParams["timezone"] = "UTC"
	return migrations.Apply(ctx, pc)
}

func serve(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := migrate(ctx); err != nil {
		return err
	}
	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Pool.Close()
	blobs, err := blob.Open(cfg.BlobDir)
	if err != nil {
		return err
	}
	m := media.New()
	engine, err := dynamic.New(cfg, st, blobs, m)
	if err != nil {
		return err
	}
	// Cancelled on every exit path, so workers and in-flight requests stop before the
	// pool closes.
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	if cfg.BackgroundWorkers {
		engine.Start(ctx)
	}
	a := &httpapi.API{Config: cfg, Store: st, Blobs: blobs, Dynamic: engine, Media: m}
	// Requests outlive ctx so Shutdown can drain them; reqCtx ends them if it times out.
	reqCtx, cancelRequests := context.WithCancel(context.Background())
	defer cancelRequests()
	srv := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Port),
		Handler:           a.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		MaxHeaderBytes:    64 << 10,
		BaseContext:       func(net.Listener) context.Context { return reqCtx },
	}
	errs := make(chan error, 1)
	go func() { errs <- srv.ListenAndServe() }()
	slog.Info("server listening", "port", cfg.Port, "backgroundWorkers", cfg.BackgroundWorkers)
	select {
	case err = <-errs:
	case <-ctx.Done():
	}
	stop()
	shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if serr := srv.Shutdown(shutdown); serr != nil {
		slog.Warn("shutdown timed out; closing open requests", "error", serr.Error())
		cancelRequests()
		_ = srv.Close()
	}
	if werr := engine.Wait(shutdown); werr != nil {
		slog.Warn("background workers still running at exit", "error", werr.Error())
	}
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	return err
}

func importLegacy(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: slate import mysql://user:password@host:3306/db")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := migrate(ctx); err != nil {
		return err
	}
	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Pool.Close()
	blobs, err := blob.Open(cfg.BlobDir)
	if err != nil {
		return err
	}
	stats, err := legacyimport.Run(ctx, args[0], cfg.BlobDir, st, blobs)
	if err != nil {
		return err
	}
	slog.Info("legacy import finished", "users", stats.Users, "groups", stats.Groups, "devices", stats.Devices, "contents", stats.Contents, "skipped", stats.Skipped)
	return nil
}
