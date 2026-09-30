package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/markgrushevski/justpaint/server/internal/aibudget"
	"github.com/markgrushevski/justpaint/server/internal/assist"
	"github.com/markgrushevski/justpaint/server/internal/auth"
	"github.com/markgrushevski/justpaint/server/internal/db"
	"github.com/markgrushevski/justpaint/server/internal/drawings"
	"github.com/markgrushevski/justpaint/server/internal/game"
	"github.com/markgrushevski/justpaint/server/internal/gemini"
	"github.com/markgrushevski/justpaint/server/internal/guess"
	"github.com/markgrushevski/justpaint/server/internal/platform/config"
	"github.com/markgrushevski/justpaint/server/internal/platform/postgres"
	"github.com/markgrushevski/justpaint/server/internal/platform/ratelimit"
	"github.com/markgrushevski/justpaint/server/internal/platform/web"
	"github.com/markgrushevski/justpaint/server/internal/practice"
	"github.com/markgrushevski/justpaint/server/internal/ratings"
	"github.com/markgrushevski/justpaint/server/internal/render"
	"github.com/markgrushevski/justpaint/server/internal/ws"
)

// app is the whole HTTP surface and the background work behind it. Building it
// starts nothing; run starts the workers once every fallible step has passed.
type app struct {
	handler http.Handler
	workers []func(context.Context)
}

// newApp builds every module over one pool and registers their routes.
func newApp(cfg config.Config, pool *pgxpool.Pool, logger *slog.Logger) (app, error) {
	queries := db.New(pool)

	authService, err := auth.NewService(queries, cfg.JWTSecret)
	if err != nil {
		return app{}, err
	}
	authHandler := auth.NewHandler(authService, cfg.CookieSecure, logger)
	drawingsHandler := drawings.NewHandler(drawings.NewService(queries), logger)

	renderer := newRenderer(cfg, logger)
	ai, err := newAI(cfg, logger)
	if err != nil {
		return app{}, err
	}
	budget, err := newBudget(cfg, queries, ai.providers, logger)
	if err != nil {
		return app{}, err
	}

	gameSvc := game.NewServiceWithConcurrency(pool, queries, renderer, ai.judge, logger, cfg.JudgeConcurrency)
	// Duel billing has two ports because its ledger has two moments: players are
	// billed when their round starts, the provider each time judging is entered
	// (docs/GAME.md §4.3) — including a stuck-judging re-fire.
	duelCheck, _ := budget.For(aibudget.KindDuel)
	billDuelPlayers, billDuelProvider := budget.ForSplit(aibudget.KindDuel)
	gameSvc.SetBudget(duelCheck, billDuelPlayers, billDuelProvider)
	gameHandler := game.NewHandler(gameSvc, logger)

	practiceCheck, practiceSpend := budget.For(aibudget.KindPractice)
	practiceHandler := practice.NewHandler(
		practice.NewService(queries, renderer, ai.critic, practiceCheck, practiceSpend, logger), logger)

	guessCheck, guessSpend := budget.For(aibudget.KindGuess)
	guessHandler := guess.NewHandler(
		guess.NewService(renderer, ai.guesser, guessCheck, guessSpend, logger), logger)

	// Assist's token bucket bounds request rate and lives in this process, so it
	// resets on every deploy; the daily quota lives in Postgres and actually holds
	// (docs/ASSIST.md §3.4).
	assistLimiter := assist.NewRateLimiter(assist.DefaultBurst, assist.DefaultRefillInterval)
	assistCheck, assistSpend := budget.For(aibudget.KindAssist)
	assistHandler := assist.NewHandler(ai.assist, assistLimiter, assistCheck, assistSpend, renderer,
		gemini.AssistRunBudget(cfg.AssistTimeout), logger)

	ratingsHandler := ratings.NewHandler(ratings.NewService(queries), logger)

	// The WS hub (docs/GAME.md §9) implements game.Publisher, so game never imports ws.
	hub := ws.NewHub(gameSvc, logger)
	gameSvc.SetPublisher(hub)
	wsHandler := ws.NewHandler(hub, gameSvc, cfg.WSAllowedOrigins, logger, ws.Limits{
		ReadIdleTimeout:   cfg.WSReadIdleTimeout,
		HeartbeatInterval: cfg.WSHeartbeatInterval,
		MaxConns:          cfg.WSMaxConns,
		MaxConnsPerIP:     cfg.WSMaxConnsPerIP,
		// Same client-IP notion as the HTTP rate limiter (docs/NOTES.md) — behind a
		// proxy this decides whether MaxConnsPerIP means one visitor or all of them.
		TrustProxy: cfg.TrustProxy,
	})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	mux.HandleFunc("GET /readyz", readyz(pool, logger))
	// The built SPA, when this instance serves both API and client (prod).
	// Registered on "/" as the catch-all: Go 1.22 ServeMux gives every API
	// pattern precedence, so anything left over gets the client-side shell.
	if cfg.StaticDir != "" {
		spa, err := web.SPA(cfg.StaticDir)
		if err != nil {
			return app{}, err
		}
		mux.Handle("/", spa)
		logger.Info("serving the built SPA", "dir", cfg.StaticDir)
	}

	protect := authHandler.RequireAuth
	authHandler.Routes(mux)
	drawingsHandler.Routes(mux, protect)
	gameHandler.Routes(mux, protect)
	assistHandler.Routes(mux, protect)
	ratingsHandler.Routes(mux, protect)
	practiceHandler.Routes(mux, protect)
	guessHandler.Routes(mux, protect)
	wsHandler.Routes(mux, protect)

	policies, limiters := ratePolicies()
	if !cfg.TrustProxy {
		// Behind a proxy this collapses every client into one bucket, which looks
		// like a working limiter and is not (docs/NOTES.md).
		logger.Info("rate limiting keyed by the direct peer address (TRUST_PROXY=false)")
	}

	workers := []func(context.Context){
		hub.Run,
		// Deadline sweeps (forfeit / abandon / stuck-judging re-fire / stale-open
		// reaper, docs/GAME.md §4.1), so a round resolves even with nobody polling.
		// Drains the backlog at boot, then ticks every 3s.
		func(ctx context.Context) { gameSvc.RunSweeper(ctx, 3*time.Second) },
		// Ledger retention: a week's worth of rows, swept hourly (docs/GAME.md §4.3).
		func(ctx context.Context) { budget.RunSweeper(ctx, aibudget.DefaultSweepInterval) },
	}
	for _, l := range limiters {
		workers = append(workers, func(ctx context.Context) { l.RunSweeper(ctx, 2*time.Minute) })
	}

	// Order matters: Recover sits inside LogRequests so a panic still logs with its
	// request id, and the limiter sits inside both so a throttled request is still
	// logged and recovered.
	handler := web.LogRequests(logger, cfg.TrustProxy,
		web.Recover(logger,
			web.RateLimit(cfg.TrustProxy, policies, logger)(mux)))
	return app{handler: handler, workers: workers}, nil
}

// newRenderer picks the judged-raster renderer (docs/GAME.md §6): the stub runs
// the full loop in-process, RENDER_MODE=node is the authoritative Konva worker.
func newRenderer(cfg config.Config, logger *slog.Logger) render.Renderer {
	if cfg.RenderMode != config.RenderModeNode {
		logger.Info("render: stub (set RENDER_MODE=node for the authoritative render)")
		return render.NewStubRenderer()
	}
	// JUDGE_CONCURRENCY also bounds these node-canvas subprocesses: /api/guess and
	// /api/practice render inline on the request goroutine, so without a cap here a
	// write-rate burst forks dozens of workers on a small host.
	logger.Info("render: node worker (authoritative)", "cli", cfg.RenderCLI, "max_workers", cfg.JudgeConcurrency)
	return render.NewNodeRenderer(cfg.RenderNodeBin, cfg.RenderCLI, cfg.JudgeConcurrency)
}

// healthz is liveness: is the process up? Deliberately dependency-free — a DB
// blip must not make an orchestrator kill an otherwise healthy process.
func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// readyz is readiness: can this instance actually serve? Every route needs
// Postgres, so an unreachable database is a 503 — the signal a load balancer (or
// an uptime pinger keeping a free instance awake) should read, not liveness.
func readyz(pool *pgxpool.Pool, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		w.Header().Set("Content-Type", "application/json")
		// "Reachable" isn't "usable": a database that exists but was never migrated
		// accepts connections and fails every real query — the one state this probe
		// must not call healthy.
		if err := postgres.Ready(ctx, pool, "matches"); err != nil {
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
	}
}

// ratePolicies are the per-IP abuse tiers (docs/API.md §3.1), each with its own
// limiter so one tier's traffic can't drain another's budget. Rows are
// first-match-wins, so the catch-all must stay last. The limiters come back
// alongside so their sweepers can run.
func ratePolicies() ([]web.RatePolicy, []*ratelimit.Limiter) {
	authLimiter := ratelimit.New(10, 6*time.Second, 0, 0)
	writeLimiter := ratelimit.New(30, 2*time.Second, 0, 0)
	defaultLimiter := ratelimit.New(300, 200*time.Millisecond, 0, 0)
	policies := []web.RatePolicy{
		{Name: "auth-strict", Match: web.MethodPrefix("/api/auth/", http.MethodPost), Limiter: authLimiter},
		{Name: "matches-write", Match: web.MethodPrefix("/api/matches", http.MethodPost, http.MethodPut, http.MethodDelete), Limiter: writeLimiter},
		{Name: "drawings-write", Match: web.MethodPrefix("/api/drawings", http.MethodPost, http.MethodPut, http.MethodDelete), Limiter: writeLimiter},
		// A cheap GET is left to the catch-all; only the scoring POST costs a
		// render and a judge call (docs/API.md §3.1).
		{Name: "practice-write", Match: web.MethodPrefix("/api/practice", http.MethodPost), Limiter: writeLimiter},
		// The daily AI budget doesn't cover this route under a fake guesser (no
		// provider, unbudgeted by design) — this tier is what stops an authenticated
		// caller from forking a node-canvas render per request (docs/API.md §3.1).
		{Name: "guess-write", Match: web.MethodPrefix("/api/guess", http.MethodPost), Limiter: writeLimiter},
		{Name: "default", Match: func(*http.Request) bool { return true }, Limiter: defaultLimiter},
	}
	return policies, []*ratelimit.Limiter{authLimiter, writeLimiter, defaultLimiter}
}
