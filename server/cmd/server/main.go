// Command server is the justpaint HTTP API: a Go modular monolith
// (auth + drawings + game + judge client). See docs/ARCHITECTURE.md.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/markgrushevski/justpaint/server/internal/aibudget"
	"github.com/markgrushevski/justpaint/server/internal/assist"
	"github.com/markgrushevski/justpaint/server/internal/auth"
	"github.com/markgrushevski/justpaint/server/internal/db"
	"github.com/markgrushevski/justpaint/server/internal/drawings"
	"github.com/markgrushevski/justpaint/server/internal/game"
	"github.com/markgrushevski/justpaint/server/internal/gemini"
	"github.com/markgrushevski/justpaint/server/internal/guess"
	"github.com/markgrushevski/justpaint/server/internal/judge"
	"github.com/markgrushevski/justpaint/server/internal/platform/config"
	"github.com/markgrushevski/justpaint/server/internal/platform/logging"
	"github.com/markgrushevski/justpaint/server/internal/platform/migrate"
	"github.com/markgrushevski/justpaint/server/internal/platform/postgres"
	"github.com/markgrushevski/justpaint/server/internal/platform/ratelimit"
	"github.com/markgrushevski/justpaint/server/internal/platform/web"
	"github.com/markgrushevski/justpaint/server/internal/practice"
	"github.com/markgrushevski/justpaint/server/internal/ratings"
	"github.com/markgrushevski/justpaint/server/internal/render"
	"github.com/markgrushevski/justpaint/server/internal/ws"
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

	pool, err := postgres.New(ctx, cfg.DatabaseURL, cfg.DBMaxConns)
	if err != nil {
		return fmt.Errorf("database connect: %w", err)
	}
	defer pool.Close()
	logger.Info("database connected")

	// A database that exists but was never migrated is the default first state on
	// a host with no shell, not an edge case — fail loud here rather than run a
	// service that reports itself live and errors on every game query.
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
	// no loop change. RENDER_MODE=node uses the authoritative Konva worker.
	var renderer render.Renderer
	if cfg.RenderMode == config.RenderModeNode {
		// JUDGE_CONCURRENCY also bounds these node-canvas subprocesses: /api/guess
		// and /api/practice render inline on the request goroutine, so without a cap
		// here a write-rate burst forks dozens of workers on a small host.
		renderer = render.NewNodeRenderer(cfg.RenderNodeBin, cfg.RenderCLI, cfg.JudgeConcurrency)
		logger.Info("render: node worker (authoritative)", "cli", cfg.RenderCLI, "max_workers", cfg.JudgeConcurrency)
	} else {
		renderer = render.NewStubRenderer()
		logger.Info("render: stub (set RENDER_MODE=node for the authoritative render)")
	}

	// Per-kind model override (docs/DECISIONS.md): GEMINI_MODEL is the default and
	// AI_MODEL_PER_KIND swaps in a different model per kind. An unknown kind name
	// fails here at boot, not at first use (internal/aibudget owns the valid set).
	aiModelByKind, err := aibudget.Models(cfg.GeminiModel, cfg.AIModelPerKind)
	if err != nil {
		return err
	}
	logAIModels(logger, aiModelByKind, cfg.GeminiModel)

	// The AI impls, and who each one bills: each constructor sets its provider
	// fact in the same breath it is built, rather than a second switch inferring
	// it from the mode. An empty provider means the impl calls nobody.

	// The fake never reads the prompt, so it's a loop-prover, not a judge. The two
	// real impls are the external ML judge (docs/JUDGE.md §6) and a vision model
	// scoring both rasters. config.Load already proved each mode's dependency
	// exists, so nothing here can fail.
	var arbiter judge.Judge
	var duelProvider aibudget.Provider
	switch cfg.JudgeMode {
	case config.JudgeModeHTTP:
		arbiter = judge.NewHTTPJudge(cfg.JudgeBaseURL, cfg.JudgeTimeout)
		// Bare, no model attached: one service behind however many models. Still
		// worth a ceiling — a bug here would hammer it and we can't see what's left
		// (docs/GAME.md §4.3).
		duelProvider = aibudget.ProviderCollaborator
		logger.Info("judge: http (external ML judge)", "base_url", cfg.JudgeBaseURL, "timeout", cfg.JudgeTimeout)
	case config.JudgeModeGemini:
		model := aiModelByKind[aibudget.KindDuel]
		arbiter = gemini.NewJudge(cfg.GeminiAPIKey, model, cfg.GeminiBaseURL, cfg.JudgeTimeout)
		// Keyed to the model, not just the provider: the free tier meters per model,
		// so two kinds on two models need two separate pools (docs/GAME.md §4.3,
		// aibudget.WithModel).
		duelProvider = aibudget.ProviderGoogle.WithModel(model)
		// No key here — it stays server-side, out of logs.
		logger.Info("judge: gemini vision", "model", model, "timeout", cfg.JudgeTimeout)
	default:
		arbiter = judge.NewFakeJudge()
		logger.Info("judge: fake (ink coverage — it never reads the prompt; set JUDGE_MODE for a real verdict)")
	}
	// A real model on the stub renderer sees ink-coverage blocks, not the drawing
	// (docs/NOTES.md) — confident, meaningless answers, since the pairing works.
	// Not a boot error: still legitimate for exercising the wiring in dev.
	realModel := cfg.JudgeMode != config.JudgeModeFake ||
		cfg.PracticeMode == config.SeamModeGemini || cfg.GuessMode == config.SeamModeGemini ||
		cfg.AssistMode == config.AssistModeGemini
	if realModel && cfg.RenderMode != config.RenderModeNode {
		logger.Warn("render: a real model is reading STUB rasters, which are ink-coverage blocks and not the drawings — set RENDER_MODE=node",
			"judge_mode", cfg.JudgeMode, "practice_mode", cfg.PracticeMode, "guess_mode", cfg.GuessMode,
			"assist_mode", cfg.AssistMode, "render_mode", cfg.RenderMode)
	}
	// The judge retries up to 3 times inside game.JudgePassBudget (docs/JUDGE.md
	// §7). A JUDGE_TIMEOUT loose enough to overflow that budget won't fail
	// outright — it silently truncates the last attempt, showing up only as an
	// occasional lost duel.
	if envelope := 3 * cfg.JudgeTimeout; envelope >= game.JudgePassBudget {
		logger.Warn("judge: JUDGE_TIMEOUT leaves no room for its own retries inside the judging pass",
			"timeout", cfg.JudgeTimeout, "retry_envelope", envelope, "pass_budget", game.JudgePassBudget)
	}

	// Single-player practice (internal/practice, docs/GAME.md §10) scores one
	// drawing alone on the same prompts, renderer and quota. PRACTICE_MODE picks its
	// critic (judge.Critic); off leaves practice refusing honestly rather than faking
	// a score (docs/JUDGE.md §8.2).
	var critic judge.Critic
	var practiceProvider aibudget.Provider
	switch cfg.PracticeMode {
	case config.SeamModeGemini:
		model := aiModelByKind[aibudget.KindPractice]
		critic = gemini.NewCritic(cfg.GeminiAPIKey, model, cfg.GeminiBaseURL, cfg.JudgeTimeout)
		practiceProvider = aibudget.ProviderGoogle.WithModel(model)
		logger.Info("practice: gemini critic (scores one drawing against its prompt)", "model", model)
	case config.SeamModeOff:
		// No critic, no calls, no quota — the empty provider here is the same rule as a fake.
		logger.Warn("practice: DISABLED — PRACTICE_MODE=off (the default under JUDGE_MODE=http, which has no critique endpoint); /api/practice answers 500 until PRACTICE_MODE is fake or gemini")
	default:
		critic = judge.NewFakeCritic()
		logger.Info("practice: fake critic (ink coverage — it never reads the prompt; set PRACTICE_MODE=gemini for a real critique)")
	}

	// "What did I draw?" on /draw (internal/guess, docs/JUDGE.md §8.3): the third
	// vision seam, with no prompt to score against, only to describe. GUESS_MODE picks
	// the guesser; off leaves a nil guesser that refuses honestly.
	var guesser judge.Guesser
	var guessProvider aibudget.Provider
	switch cfg.GuessMode {
	case config.SeamModeGemini:
		model := aiModelByKind[aibudget.KindGuess]
		guesser = gemini.NewGuesser(cfg.GeminiAPIKey, model, cfg.GeminiBaseURL, cfg.JudgeTimeout)
		guessProvider = aibudget.ProviderGoogle.WithModel(model)
		logger.Info("guess: gemini vision (names what one drawing depicts)", "model", model)
	case config.SeamModeOff:
		logger.Warn("guess: DISABLED — GUESS_MODE=off (the default under JUDGE_MODE=http, which has no endpoint for it); /api/guess answers 500 until GUESS_MODE is fake or gemini")
	default:
		guesser = judge.NewFakeGuesser()
		logger.Info("guess: fake (a canned answer that never looks at the drawing; set GUESS_MODE=gemini for a real one)")
	}

	// AI assist is a seam like render/judge (docs/ASSIST.md §3), swapped by
	// ASSIST_MODE with no handler change. Stateless and rate-limited per user
	// since each call can cost API money; read independently of JUDGE_MODE.
	assistLimiter := assist.NewRateLimiter(assist.DefaultBurst, assist.DefaultRefillInterval)
	var assistImpl assist.Assist
	// The provider a mode would bill, set beside the constructor that chose it,
	// same discipline as the judge above — adopted below only if the impl confirms
	// it really calls anybody.
	var assistVendor aibudget.Provider
	switch cfg.AssistMode {
	case config.AssistModeGemini:
		model := aiModelByKind[aibudget.KindAssist]
		// Its own timeout, not the judge's: composing a picture takes tens of
		// seconds on a thinking model, where a verdict takes one or two.
		assistImpl = gemini.NewAssist(cfg.GeminiAPIKey, model, cfg.GeminiBaseURL, cfg.AssistTimeout)
		assistVendor = aibudget.ProviderGoogle.WithModel(model)
		logger.Info("assist: gemini (a prompt really becomes shapes)", "model", model, "timeout", cfg.AssistTimeout)
	default:
		assistImpl = assist.NewFakeAssist()
		// Returns the same canned house whatever the user typed — a prompt box that
		// ignores the prompt. Fine in dev and CI, a lie in production.
		logger.Info("assist: fake (the same canned ops for every prompt; set ASSIST_MODE=gemini for a real one)")
	}
	// The impl is asked whether it calls anybody (assist.CallsProvider), rather
	// than inferred from the mode: Gemini answers true, giving assist a real daily
	// ceiling; the fake answers false and stays unbudgeted. The Warn below guards
	// against a future impl whose provider and behavior disagree.
	var assistProvider aibudget.Provider
	if assist.CallsProvider(assistImpl) {
		assistProvider = assistVendor
	} else if assistVendor != "" {
		logger.Warn("assist: this mode names a provider but its impl makes no external call — nothing is billed and every request will fail until the impl lands",
			"assist_mode", cfg.AssistMode)
	}

	// The daily AI-call budget (internal/aibudget): the per-IP write limiter bounds
	// request rate, this bounds the scarce thing behind it — a provider's per-day
	// quota. Each consumer gets anonymous funcs and never learns its own kind.
	aiPolicyByKind, err := aibudget.Policies(map[aibudget.Kind]aibudget.Provider{
		aibudget.KindDuel:     duelProvider,
		aibudget.KindPractice: practiceProvider,
		aibudget.KindGuess:    guessProvider,
		aibudget.KindAssist:   assistProvider,
	}, cfg.AIDailyPerUser)
	if err != nil {
		return err
	}
	aiBudget := aibudget.New(queries, aiPolicyByKind, cfg.AIDailyGlobal, logger)
	logAIBudget(logger, aiPolicyByKind, cfg.AIDailyGlobal)
	// A configured allowance for a feature that calls nobody is inert, not wrong —
	// but it looks identical to an enforced one from the outside, and an operator
	// can end up trusting a number that nothing reads.
	for _, kind := range aibudget.InertAllowances(aiPolicyByKind, cfg.AIDailyPerUser) {
		logger.Warn("ai budget: AI_DAILY_PER_USER sets an allowance for a kind whose impl calls no provider — nothing reads it",
			"kind", kind, "per_user", aiPolicyByKind[kind].PerUser)
	}

	gameSvc := game.NewServiceWithConcurrency(pool, queries, renderer, arbiter, logger, cfg.JudgeConcurrency)
	// Duel billing has two ports because its ledger has two moments: players are
	// billed when their round starts, the provider each time judging is entered
	// (docs/GAME.md §4.3) — including a stuck-judging re-fire.
	duelCheck, _ := aiBudget.For(aibudget.KindDuel)
	billDuelPlayers, billDuelProvider := aiBudget.ForSplit(aibudget.KindDuel)
	gameSvc.SetBudget(duelCheck, billDuelPlayers, billDuelProvider)
	gameHandler := game.NewHandler(gameSvc, logger)

	// Practice has its own per-player allowance, and — wherever its critic and the
	// judge share a provider — the same global ceiling, counted once rather than
	// by per-feature copies that drift (docs/GAME.md §4.3).
	practiceCheck, practiceSpend := aiBudget.For(aibudget.KindPractice)
	practiceHandler := practice.NewHandler(
		practice.NewService(queries, renderer, critic, practiceCheck, practiceSpend, logger), logger)

	guessCheck, guessSpend := aiBudget.For(aibudget.KindGuess)
	guessHandler := guess.NewHandler(
		guess.NewService(renderer, guesser, guessCheck, guessSpend, logger), logger)

	// Assist's token bucket bounds request rate and lives in this process, so it
	// resets on every deploy; the daily quota lives in Postgres and actually holds
	// (docs/ASSIST.md §3.4).
	assistCheck, assistSpend := aiBudget.For(aibudget.KindAssist)
	assistHandler := assist.NewHandler(assistImpl, assistLimiter, assistCheck, assistSpend, renderer, gemini.AssistRunBudget(cfg.AssistTimeout), logger)

	// The leaderboard is a read-only slice (docs/API.md §11): a small single-route
	// module over the shared queries, like assist — a global top-N read that shares
	// nothing with the match lifecycle, so it does not live on game.
	ratingsHandler := ratings.NewHandler(ratings.NewService(queries), logger)

	// The WS hub (docs/GAME.md §9) implements game.Publisher via SetPublisher so
	// game never imports ws, and runs on the shutdown ctx like the sweeper below.
	// background tracks both long-lived goroutines so shutdown can wait for them:
	// srv.Shutdown only drains in-flight HTTP handlers, not background work.
	var background sync.WaitGroup

	hub := ws.NewHub(gameSvc, logger)
	background.Go(func() { hub.Run(ctx) })
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

	// Background deadline sweeps (forfeit / abandon / stuck-judging re-fire /
	// stale-open reaper, docs/GAME.md §4.1) so a round resolves even with nobody
	// polling. Drains the backlog at boot, then ticks every 3s until ctx cancels.
	background.Go(func() { gameSvc.RunSweeper(ctx, 3*time.Second) })

	// Ledger retention: a week's worth of rows, swept hourly (docs/GAME.md §4.3) —
	// enough to answer "why was I refused last Tuesday" without the table growing
	// unbounded.
	background.Go(func() { aiBudget.RunSweeper(ctx, aibudget.DefaultSweepInterval) })

	mux := http.NewServeMux()
	// Liveness: is the process up? Deliberately dependency-free — a DB blip must
	// not make an orchestrator kill an otherwise healthy process.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	// Readiness: can this instance actually serve? Every route needs Postgres, so
	// an unreachable database is a 503 here — the signal a load balancer (or an
	// uptime pinger keeping a free instance awake) should read, not liveness above.
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		readyCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		w.Header().Set("Content-Type", "application/json")
		// "Reachable" isn't "usable": a database that exists but was never migrated
		// accepts connections and fails every real query — the one state this probe
		// must not call healthy.
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
	// The built SPA, when this instance serves both API and client (prod).
	// Registered last on "/" as the catch-all: Go 1.22 ServeMux gives every API
	// pattern above precedence, so anything left over gets the client-side shell.
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
	practiceHandler.Routes(mux, authHandler.RequireAuth)
	guessHandler.Routes(mux, authHandler.RequireAuth)
	wsHandler.Routes(mux, authHandler.RequireAuth)

	// Abuse protection, keyed by client IP (docs/API.md §3.1). Three tiers, each
	// with its own limiter so one tier's traffic can't drain another's budget.
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
		// A cheap GET is left to the catch-all; only the scoring POST costs a
		// render and a judge call (docs/API.md §3.1).
		{Name: "practice-write", Match: web.MethodPrefix("/api/practice", http.MethodPost), Limiter: writeLimiter},
		// The daily AI budget doesn't cover this route under JUDGE_MODE=fake (no
		// provider, unbudgeted by design) — this tier is what stops an authenticated
		// caller from forking a node-canvas render per request (docs/API.md §3.1).
		{Name: "guess-write", Match: web.MethodPrefix("/api/guess", http.MethodPost), Limiter: writeLimiter},
		{Name: "default", Match: func(*http.Request) bool { return true }, Limiter: defaultLimiter},
	}
	if !cfg.TrustProxy {
		// Behind a proxy this collapses every client into one bucket, which looks
		// like a working limiter and is not (docs/NOTES.md).
		logger.Info("rate limiting keyed by the direct peer address (TRUST_PROXY=false)")
	}

	srv := &http.Server{
		Addr: cfg.Addr,
		// Order matters: Recover sits inside LogRequests so a panic still logs
		// with its request id, and the limiter sits inside both so a throttled
		// request is still logged and recovered.
		Handler: web.LogRequests(logger, cfg.TrustProxy,
			web.Recover(logger,
				web.RateLimit(cfg.TrustProxy, policies, logger)(mux))),
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

	// The hub and sweeper are unwinding now that ctx is cancelled; wait for them
	// before the deferred pool.Close, or a final transition can lose its
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

// logAIModels logs which model each AI kind resolved to (docs/DECISIONS.md): an
// override that silently didn't apply would otherwise keep working on the
// default, with only a bill or quality level as the symptom. Kinds are listed
// in a stable order so two boots are diffable.
func logAIModels(logger *slog.Logger, models map[aibudget.Kind]string, defaultModel string) {
	pairs := make([]any, 0, 2*len(models))
	overridden := false
	for _, kind := range aibudget.AllKinds() {
		model := models[kind]
		if model != defaultModel {
			overridden = true
		}
		pairs = append(pairs, string(kind), model)
	}
	if !overridden {
		logger.Info("ai models: every kind on the default (set AI_MODEL_PER_KIND to give one its own)", "model", defaultModel)
		return
	}
	logger.Info("ai models: resolved per kind", append([]any{"default", defaultModel}, pairs...)...)
}

// logAIBudget logs the ceiling at boot, per kind: an unenforced budget and an
// enforced one look identical from the outside until the quota runs out. Kinds
// are listed in a stable order so two boots are diffable.
func logAIBudget(logger *slog.Logger, policies map[aibudget.Kind]aibudget.Policy, global int) {
	enforced := make([]any, 0, 2*len(policies))
	var unbilled []string
	for _, kind := range aibudget.AllKinds() {
		p, ok := policies[kind]
		if !ok {
			continue // a kind with no feature wired yet
		}
		if p.Provider == "" {
			// A fake, an unbuilt impl, or a mode with no impl for this kind — three
			// roads to the same fact: nobody's quota is at stake. Which road it took
			// is the per-impl line above's business.
			unbilled = append(unbilled, string(kind))
			continue
		}
		enforced = append(enforced, string(kind), fmt.Sprintf("%d/day via %s", p.PerUser, p.Provider))
	}
	if len(enforced) == 0 {
		logger.Info("ai budget: nothing enforced (no AI impl here calls a provider, so there is no external quota to protect)")
		return
	}
	// "per provider" is really per provider and model where the provider meters
	// that way (aibudget.Provider.WithModel) — each kind's value below spells out
	// which pool it draws from.
	logger.Info("ai budget: per rolling 24h", append([]any{"global_per_provider_pool", global}, enforced...)...)
	if len(unbilled) > 0 {
		logger.Info("ai budget: not enforced — these impls call no provider", "kinds", strings.Join(unbilled, ","))
	}
}
