// Command server runs the omi-board API.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/getsentry/sentry-go"

	"github.com/omi-teams/omi-board/server/internal/api"
	"github.com/omi-teams/omi-board/server/internal/auth"
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
	switch flag.Arg(0) {
	case "migrate":
		run = migrate
	case "create-user":
		run = func() error { return createUser(flag.Arg(1), os.Stdin) }
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

// minPasswordLen is checked only here: create-user is the only way to add users.
const minPasswordLen = 8

// createUser adds a user with the email argument and the password read from stdin
// (one line, so it never appears in the process list or shell history). It needs only DATABASE_URL.
func createUser(email string, stdin io.Reader) error {
	if email == "" {
		return errors.New("usage: server create-user <email> < password-file")
	}
	line, err := io.ReadAll(io.LimitReader(stdin, 1<<16))
	if err != nil {
		return err
	}
	password := strings.TrimRight(string(line), "\r\n")
	if len(password) < minPasswordLen {
		return fmt.Errorf("password on stdin must be at least %d characters", minPasswordLen)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := db.Open(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()
	if _, err := db.Migrate(ctx, pool, migrations.FS); err != nil {
		return err
	}
	user, err := auth.Store{DB: pool}.CreateUser(ctx, email, password)
	if err != nil {
		return err
	}
	slog.Info("user created", "id", user.ID)
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
	authn := auth.New(pool, cfg.SessionSecret, cfg.PublicWebURL)
	server := api.Server{DB: pool, Auth: authn, CookieDomain: cfg.CookieDomain}
	api.HandlerFromMux(api.NewStrictHandlerWithOptions(server, nil, api.ErrorHandlers()), mux)
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           obs.Middleware(slog.Default(), reporter, authn.Middleware(mux)),
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
