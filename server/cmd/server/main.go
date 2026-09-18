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
	"github.com/markgrushevski/justpaint/server/internal/platform/migrate"
	"github.com/markgrushevski/justpaint/server/internal/platform/postgres"
	"github.com/markgrushevski/justpaint/server/internal/platform/ratelimit"
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

	// Before anything reads or writes: a database that exists but was never
	// migrated is not an edge case on a host with no shell, it is the default
	// first state. Failing here is loud; the alternative is a service that
	// reports itself live and errors on every game query.
	if cfg.AutoMigrate {
		migrateCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		if _, err := migrate.Run(migrateCtx, cfg.DatabaseURL, logger); err != nil {
			return err
		}
	} else {
		logger.Info("migrations: skipped (AUTO_MIGRATE=false) — the schema is someone else's job")
	}

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
	gameSvc := game.NewServiceWithConcurrency(pool, queries, renderer, judge.NewFakeJudge(), logger, cfg.JudgeConcurrency)
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
	wsHandler := ws.NewHandler(hub, gameSvc, cfg.WSAllowedOrigins, logger, ws.Limits{
		ReadIdleTimeout:   cfg.WSReadIdleTimeout,
		HeartbeatInterval: cfg.WSHeartbeatInterval,
		MaxConns:          cfg.WSMaxConns,
		MaxConnsPerIP:     cfg.WSMaxConnsPerIP,
		// Same notion of "who is this client" as the HTTP rate limiter: behind a
		// proxy, keying the per-IP cap on the peer would make every visitor share
		// one bucket and turn MaxConnsPerIP into a far lower global cap.
		TrustProxy: cfg.TrustProxy,
	})

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
		readyCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		w.Header().Set("Content-Type", "application/json")
		// "Reachable" is not "usable": a database that exists but was never
		// migrated accepts connections and fails every real query, which is the
		// one state a probe must not call healthy.
		if err := postgres.Ready(readyCtx, pool, "matches"); err != nil {
			dependency := "database"
			if errors.Is(err, postgres.ErrSchemaMissing) {
				dependency = "schema"
			}
			logger.Warn("readiness: not ready", "dependency", dependency, "error", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprintf(w, `{"status":"unavailable","dependency":%q}`, dependency)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	// The built SPA, when this instance is the one origin serving both (prod).
	// Registered last and on "/" so it is the catch-all: Go 1.22 ServeMux gives
	// every API pattern above precedence, and anything left over is a client-side
	// route that gets the shell.
	if cfg.StaticDir != "" {
		spa, err := web.SPA(cfg.StaticDir)
		if err != nil {
			return err
		}
		mux.Handle("/", spa)
		logger.Info("serving the built SPA", "dir", cfg.StaticDir)
	}

	authHandler.Routes(mux)
	drawingsHandler.Routes(mux, authHandler.RequireAuth)
	gameHandler.Routes(mux, authHandler.RequireAuth)
	assistHandler.Routes(mux, authHandler.RequireAuth)
	ratingsHandler.Routes(mux, authHandler.RequireAuth)
	wsHandler.Routes(mux, authHandler.RequireAuth)

	// Abuse protection, keyed by client IP (docs/DECISIONS.md, docs/IDEAS.md: due
	// before any public deploy). Three tiers, each with its OWN limiter so one
	// tier's traffic cannot drain another's budget:
	//   auth    — bcrypt is expensive and login is the credential-stuffing target;
	//   writes  — a match creates work (render + judge), a drawing costs storage;
	//   default — everything else, including the SPA's own asset fetches, so a
	//             normal page load never comes close.
	// Rows are first-match-wins, so the catch-all must stay last.
	authLimiter := ratelimit.New(10, 6*time.Second, 0, 0)
	writeLimiter := ratelimit.New(30, 2*time.Second, 0, 0)
	defaultLimiter := ratelimit.New(300, 200*time.Millisecond, 0, 0)
	for _, l := range []*ratelimit.Limiter{authLimiter, writeLimiter, defaultLimiter} {
		background.Go(func() { l.RunSweeper(ctx, 2*time.Minute) })
	}
	policies := []web.RatePolicy{
		{Name: "auth-strict", Match: web.MethodPrefix("/api/auth/", http.MethodPost), Limiter: authLimiter},
		{Name: "matches-write", Match: web.MethodPrefix("/api/matches", http.MethodPost, http.MethodPut, http.MethodDelete), Limiter: writeLimiter},
		{Name: "drawings-write", Match: web.MethodPrefix("/api/drawings", http.MethodPost, http.MethodPut, http.MethodDelete), Limiter: writeLimiter},
		{Name: "default", Match: func(*http.Request) bool { return true }, Limiter: defaultLimiter},
	}
	if !cfg.TrustProxy {
		// Worth saying out loud at boot: behind a proxy this collapses every
		// client into one bucket, which looks like a working limiter and is not.
		logger.Info("rate limiting keyed by the direct peer address (TRUST_PROXY=false)")
	}

	srv := &http.Server{
		Addr: cfg.Addr,
		// Order matters. Recover must sit INSIDE LogRequests so a panic still
		// logs with its request id, and the limiter sits inside both so a
		// throttled request is still logged and still recovered.
		Handler: web.LogRequests(logger, cfg.TrustProxy,
			web.Recover(logger,
				web.RateLimit(cfg.TrustProxy, policies, logger)(mux))),
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
