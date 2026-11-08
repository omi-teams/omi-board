// Command server runs the omi-board API.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/getsentry/sentry-go"

	"github.com/omi-teams/omi-board/server/internal/api"
	"github.com/omi-teams/omi-board/server/internal/config"
	"github.com/omi-teams/omi-board/server/internal/db"
	"github.com/omi-teams/omi-board/server/internal/health"
	"github.com/omi-teams/omi-board/server/internal/obs"
	"github.com/omi-teams/omi-board/server/migrations"
)

func main() {
	os.Exit(mainExit())
}

func mainExit() int {
	slog.SetDefault(obs.Logger(os.Stdout))
	healthcheck := flag.Bool("healthcheck", false, "probe the local /healthz endpoint and exit")
	flag.Parse()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	if *healthcheck {
		return probe(port)
	}
	reporter, err := obs.NewReporter(os.Getenv("SENTRY_DSN"))
	if err != nil {
		slog.Error("invalid SENTRY_DSN")
		return 1
	}
	if reporter != nil {
		defer reporter.Flush(2 * time.Second)
	}
	run := func() error { return run(reporter) }
	if flag.Arg(0) == "migrate" {
		run = migrate
	}
	if err := run(); err != nil {
		obs.Capture(reporter, err, "")
		// Configuration/database errors can contain credentials; keep them out of logs.
		slog.Error("server command failed")
		return 1
	}
	return 0
}

// migrate applies pending migrations and exits; it needs only DATABASE_URL.
func migrate() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := db.Open(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()
	applied, err := db.Migrate(ctx, pool, migrations.FS)
	if err != nil {
		return err
	}
	slog.Info("migrations applied", "migrations", applied)
	return nil
}

func probe(port string) int {
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + net.JoinHostPort("127.0.0.1", port) + "/healthz")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

func run(reporter *sentry.Client) error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if _, err := db.Migrate(ctx, pool, migrations.FS); err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.Handle("GET /healthz", health.Handler(pool))
	api.HandlerFromMux(api.NewStrictHandler(api.Server{DB: pool}, nil), mux)
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           obs.Middleware(slog.Default(), reporter, mux),
		ErrorLog:          slog.NewLogLogger(slog.Default().Handler(), slog.LevelError),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	slog.Info("listening", "address", srv.Addr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
