package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	"slate/internal/job"
	"slate/internal/legacyimport"
	"slate/internal/media"
	"slate/internal/store"
	"slate/migrations"
)

const usage = `usage: slate <command>

  serve                      apply migrations, then run the HTTP server and background workers (default)
  migrate                    apply database migrations
  import <mysql-url>         copy a v0.1.x MySQL deployment into an empty database
  job <name>                 run a dashboard push job (sub2api-usage-stats, claude-code-quota-monitor)
  claude-statusline          Claude Code statusLine command that also pushes quota data`

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
	case "job":
		if len(args) != 1 {
			err = errors.New(usage)
		} else {
			err = job.Run(ctx, args[0])
		}
	case "claude-statusline":
		err = job.StatusLine(ctx, args)
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
	if cfg.BackgroundWorkers {
		engine.Start(ctx)
	}
	a := &httpapi.API{Config: cfg, Store: st, Blobs: blobs, Dynamic: engine, Media: m}
	srv := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Port),
		Handler:           a.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
	errs := make(chan error, 1)
	go func() { errs <- srv.ListenAndServe() }()
	slog.Info("server listening", "port", cfg.Port, "backgroundWorkers", cfg.BackgroundWorkers)
	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err = srv.Shutdown(shutdown)
	// ctx is done, so the workers are winding down; give them the rest of the shutdown
	// budget to finish before the pool closes.
	if werr := engine.Wait(shutdown); werr != nil {
		slog.Warn("background workers still running at exit", "error", werr.Error())
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
