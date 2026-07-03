// Command server is the justpaint HTTP API: a Go modular monolith
// (auth + drawings + game + judge client). See docs/ARCHITECTURE.md.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/markgrushevski/justpaint/server/internal/auth"
	"github.com/markgrushevski/justpaint/server/internal/db"
	"github.com/markgrushevski/justpaint/server/internal/drawings"
	"github.com/markgrushevski/justpaint/server/internal/game"
	"github.com/markgrushevski/justpaint/server/internal/judge"
	"github.com/markgrushevski/justpaint/server/internal/platform/config"
	"github.com/markgrushevski/justpaint/server/internal/platform/logging"
	"github.com/markgrushevski/justpaint/server/internal/platform/postgres"
	"github.com/markgrushevski/justpaint/server/internal/platform/web"
	"github.com/markgrushevski/justpaint/server/internal/render"
)

func main() {
	// run() owns all cleanup via defer; main only maps an error to a non-zero
	// exit code (so a failed bind is distinguishable from a clean shutdown).
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

	pool, err := postgres.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database connect: %w", err)
	}
	defer pool.Close()
	logger.Info("database connected")

	queries := db.New(pool)

	authService, err := auth.NewService(queries, cfg.JWTSecret)
	if err != nil {
		return err
	}
	authHandler := auth.NewHandler(authService, cfg.CookieSecure, logger)
	drawingsHandler := drawings.NewHandler(drawings.NewService(queries), logger)
	// The render worker and judge are seams (docs/GAME.md §6, docs/JUDGE.md): the
	// in-process stub/fake run the full loop, and each swaps for a real impl with
	// no loop change. RENDER_MODE=node uses the authoritative Konva worker; the
	// judge stays fake until the collaborator's HTTP judge (Phase 4).
	var renderer render.Renderer
	if cfg.RenderMode == config.RenderModeNode {
		renderer = render.NewNodeRenderer(cfg.RenderNodeBin, cfg.RenderCLI)
		logger.Info("render: node worker (authoritative)", "cli", cfg.RenderCLI)
	} else {
		renderer = render.NewStubRenderer()
		logger.Info("render: stub (set RENDER_MODE=node for the authoritative render)")
	}
	gameHandler := game.NewHandler(
		game.NewService(pool, queries, renderer, judge.NewFakeJudge(), logger),
		logger,
	)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	authHandler.Routes(mux)
	drawingsHandler.Routes(mux, authHandler.RequireAuth)
	gameHandler.Routes(mux, authHandler.RequireAuth)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           web.LogRequests(logger, web.Recover(logger, mux)),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		// WriteTimeout bounds slow-reading clients. Revisit when the WS hub lands
		// (a global WriteTimeout would cut long-lived upgrades — exclude that route).
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
	logger.Info("stopped")
	return nil
}
