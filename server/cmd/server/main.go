// Command server is the justpaint HTTP API, one binary over one Postgres
// (docs/ARCHITECTURE.md §4). main.go runs the process, app.go builds the modules
// and their routes, and ai.go picks the AI impls and the budget that bills them.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/markgrushevski/justpaint/server/internal/platform/config"
	"github.com/markgrushevski/justpaint/server/internal/platform/logging"
	"github.com/markgrushevski/justpaint/server/internal/platform/migrate"
	"github.com/markgrushevski/justpaint/server/internal/platform/postgres"
)

func main() {
	// run owns cleanup via defer; main only turns its error into a non-zero exit
	// code, so a failed bind is distinguishable from a clean shutdown.
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	logger := logging.New(os.Getenv("LOG_LEVEL"))

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Stop on the first SIGINT/SIGTERM; a second one force-kills.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := openDatabase(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer pool.Close()

	a, err := newApp(cfg, pool, logger)
	if err != nil {
		return err
	}

	// background tracks the long-lived goroutines so shutdown can wait for them:
	// srv.Shutdown only drains in-flight HTTP handlers, not background work.
	var background sync.WaitGroup
	for _, work := range a.workers {
		background.Go(func() { work(ctx) })
	}

	return serve(ctx, cfg, a.handler, &background, logger)
}

// openDatabase connects and, unless AUTO_MIGRATE=false, migrates the schema.
func openDatabase(ctx context.Context, cfg config.Config, logger *slog.Logger) (*pgxpool.Pool, error) {
	pool, err := postgres.New(ctx, cfg.DatabaseURL, cfg.DBMaxConns)
	if err != nil {
		return nil, fmt.Errorf("database connect: %w", err)
	}
	logger.Info("database connected")

	if !cfg.AutoMigrate {
		logger.Info("migrations: skipped (AUTO_MIGRATE=false) — the schema is someone else's job")
		return pool, nil
	}
	// A database that exists but was never migrated is the default first state on
	// a host with no shell, not an edge case — fail loud here rather than run a
	// service that reports itself live and errors on every game query.
	migrateCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if _, err := migrate.Run(migrateCtx, cfg.DatabaseURL, logger); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// serve runs the HTTP server until ctx is cancelled, then shuts it down and waits
// for the background workers. It returns early if the server fails to start.
func serve(ctx context.Context, cfg config.Config, handler http.Handler, background *sync.WaitGroup, logger *slog.Logger) error {
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		// WriteTimeout bounds slow-reading clients but doesn't cut WS upgrades:
		// websocket.Accept hijacks the connection and net/http's Hijack clears the
		// conn deadlines, so the long-lived socket runs on the hub's own timeouts.
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("server listening", "addr", cfg.Addr, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutting down")
	case err := <-serverErr:
		return fmt.Errorf("server: %w", err) // e.g. failed to bind the port
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	// The hub and sweepers are unwinding now that ctx is cancelled; wait for them
	// before the caller closes the pool, or a final transition can lose its
	// connection mid-write. Bounded so a wedged goroutine delays exit rather than
	// blocking it forever.
	drained := make(chan struct{})
	go func() {
		background.Wait()
		close(drained)
	}()
	select {
	case <-drained:
	case <-shutdownCtx.Done():
		logger.Warn("background workers did not stop in time")
	}

	logger.Info("stopped")
	return nil
}
