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
	"sync"
	"syscall"
	"time"

	"github.com/markgrushevski/justpaint/server/internal/assist"
	"github.com/markgrushevski/justpaint/server/internal/auth"
	"github.com/markgrushevski/justpaint/server/internal/db"
	"github.com/markgrushevski/justpaint/server/internal/drawings"
	"github.com/markgrushevski/justpaint/server/internal/game"
	"github.com/markgrushevski/justpaint/server/internal/judge"
	"github.com/markgrushevski/justpaint/server/internal/platform/config"
	"github.com/markgrushevski/justpaint/server/internal/platform/logging"
	"github.com/markgrushevski/justpaint/server/internal/platform/postgres"
	"github.com/markgrushevski/justpaint/server/internal/platform/web"
	"github.com/markgrushevski/justpaint/server/internal/ratings"
	"github.com/markgrushevski/justpaint/server/internal/render"
	"github.com/markgrushevski/justpaint/server/internal/ws"
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

	pool, err := postgres.New(ctx, cfg.DatabaseURL, cfg.DBMaxConns)
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
	gameSvc := game.NewService(pool, queries, renderer, judge.NewFakeJudge(), logger)
	gameHandler := game.NewHandler(gameSvc, logger)

	// AI assist is a seam like render/judge (docs/ASSIST.md §3): the deterministic
	// FakeAssist runs the whole client flow with zero API dependency, and the real
	// AnthropicAssist swaps in by ASSIST_MODE with no handler change. The endpoint is
	// stateless (no DB) and rate-limited per user (each call can cost API money).
	assistLimiter := assist.NewRateLimiter(assist.DefaultBurst, assist.DefaultRefillInterval)
	var assistImpl assist.Assist
	if cfg.AssistMode == config.AssistModeAnthropic {
		assistImpl = assist.NewAnthropicAssist(cfg.AnthropicAPIKey, cfg.AssistModel)
		logger.Info("assist: anthropic (real LLM)", "model", cfg.AssistModel)
	} else {
		assistImpl = assist.NewFakeAssist()
		logger.Info("assist: fake (deterministic canned ops; set ASSIST_MODE=anthropic for the real LLM)")
	}
	assistHandler := assist.NewHandler(assistImpl, assistLimiter, logger)

	// The leaderboard is a read-only Phase 4 slice (docs/API.md §11): a small
	// single-route module over the shared queries, like assist — a global top-N
	// read, sharing nothing with the match lifecycle, so it does NOT live on game.
	ratingsHandler := ratings.NewHandler(ratings.NewService(queries), logger)

	// Live realtime (Phase 3 back-half): the in-memory WS hub pushes committed match
	// transitions to both duelists, Postgres stays authoritative, the poll loop is the
	// fallback. The hub implements game.Publisher and is injected via SetPublisher, so
	// game never imports ws (no cycle). Runs on the shutdown ctx — cancel drains it,
	// same as the sweeper (docs/DESIGN-PHASE3-LIVE.md §3).
	//
	// background tracks the long-lived goroutines (hub, sweeper) so shutdown can
	// wait for them. srv.Shutdown only drains in-flight HTTP handlers; without
	// this the process could exit while the hub was mid-fan-out or the sweeper
	// mid-transition.
	var background sync.WaitGroup

	hub := ws.NewHub(gameSvc, logger)
	background.Go(func() { hub.Run(ctx) })
	gameSvc.SetPublisher(hub)
	wsHandler := ws.NewHandler(hub, gameSvc, cfg.WSAllowedOrigins, logger)

	// Background deadline sweeps (forfeit / abandon / stuck-judging re-fire /
	// stale-open reaper) on the shutdown-cancellable context, so a round resolves
	// even if no client is polling (docs/DESIGN-PHASE3-LIVE.md §2.4). Boot-drains the
	// backlog, then ticks every 3s; returns when ctx is cancelled.
	background.Go(func() { gameSvc.RunSweeper(ctx, 3*time.Second) })

	mux := http.NewServeMux()
	// Liveness: is the process up? Deliberately dependency-free — a DB blip must
	// not make an orchestrator kill an otherwise healthy process.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	// Readiness: can this instance actually serve? Every route below needs
	// Postgres, so an unreachable database is a 503 here — that is the signal a
	// load balancer (or an uptime pinger keeping a free-tier dyno awake) should
	// read, not the liveness probe above.
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		pingCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		w.Header().Set("Content-Type", "application/json")
		if err := pool.Ping(pingCtx); err != nil {
			logger.Warn("readiness: database unreachable", "error", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"unavailable","dependency":"database"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	authHandler.Routes(mux)
	drawingsHandler.Routes(mux, authHandler.RequireAuth)
	gameHandler.Routes(mux, authHandler.RequireAuth)
	assistHandler.Routes(mux, authHandler.RequireAuth)
	ratingsHandler.Routes(mux, authHandler.RequireAuth)
	wsHandler.Routes(mux, authHandler.RequireAuth)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           web.LogRequests(logger, web.Recover(logger, mux)),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		// WriteTimeout bounds slow-reading clients. It does NOT cut the WS upgrades:
		// websocket.Accept hijacks the connection, and net/http's Hijack clears the
		// conn deadlines (server.go: rwc.SetDeadline(time.Time{})), so the long-lived
		// socket runs on the hub's own ctx-based timeouts, not this one.
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

	// ctx is already cancelled here (that is what woke us), so the hub and the
	// sweeper are unwinding; wait for them before the deferred pool.Close runs,
	// or a final transition can lose its connection mid-write. Bounded, so a
	// wedged goroutine delays the exit instead of blocking it forever.
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
