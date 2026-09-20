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
	// no loop change. RENDER_MODE=node uses the authoritative Konva worker.
	var renderer render.Renderer
	if cfg.RenderMode == config.RenderModeNode {
		// JUDGE_CONCURRENCY bounds the node-canvas subprocesses too, not just judging
		// passes: /api/guess and /api/practice render inline on the request goroutine,
		// so without a bound INSIDE the renderer one burst under the write rate-limit
		// tier is thirty simultaneous workers on a 512 MB host (internal/render/node.go).
		renderer = render.NewNodeRenderer(cfg.RenderNodeBin, cfg.RenderCLI, cfg.JudgeConcurrency)
		logger.Info("render: node worker (authoritative)", "cli", cfg.RenderCLI, "max_workers", cfg.JudgeConcurrency)
	} else {
		renderer = render.NewStubRenderer()
		logger.Info("render: stub (set RENDER_MODE=node for the authoritative render)")
	}

	// --- the AI impls, and WHO each one bills -------------------------------
	//
	// Every impl below is chosen and, in the same breath, says whose quota it
	// spends. The pairing is deliberate and it is the fix for a real bug: the
	// provider map used to be a SECOND switch over the same mode envs, four
	// independent switches that had to agree by hand, and they did not — an
	// ASSIST_MODE=anthropic scaffold that makes no network call at all was still
	// handed a provider, so every assist request wrote ledger rows and then 500'd.
	// A provider is now a fact about the impl that was actually built: it is set
	// beside the constructor that built it, travels in a local, and is read exactly
	// once, by aibudget.Policies below. An empty provider means "this impl calls
	// nobody", which is exactly what the budget wants to hear (internal/aibudget).

	// Who actually decides the duel. The fake never reads the prompt, so it is a
	// loop-prover, not a judge — the two real impls are the collaborator's ML over
	// the JUDGE.md §6 contract, and a vision model scoring both rasters in one
	// call. config.Load has already proven each mode's dependency exists, so
	// nothing here can fail.
	var arbiter judge.Judge
	var duelProvider aibudget.Provider
	switch cfg.JudgeMode {
	case config.JudgeModeHTTP:
		arbiter = judge.NewHTTPJudge(cfg.JudgeBaseURL, cfg.JudgeTimeout)
		// His service, his quota — still worth a ceiling, because a bug here would
		// hammer it and we cannot see how much of it is left.
		duelProvider = aibudget.ProviderCollaborator
		logger.Info("judge: http (the collaborator's ML)", "base_url", cfg.JudgeBaseURL, "timeout", cfg.JudgeTimeout)
	case config.JudgeModeGemini:
		arbiter = judge.NewGeminiJudge(cfg.GeminiAPIKey, cfg.GeminiModel, cfg.GeminiBaseURL, cfg.JudgeTimeout)
		duelProvider = aibudget.ProviderGoogle
		// The key is deliberately absent from this line: it is server-side only.
		logger.Info("judge: gemini vision", "model", cfg.GeminiModel, "timeout", cfg.JudgeTimeout)
	default:
		arbiter = judge.NewFakeJudge()
		logger.Info("judge: fake (ink coverage — it never reads the prompt; set JUDGE_MODE for a real verdict)")
	}
	// A real judge on the stub renderer scores a rectangle, not a drawing: the stub
	// paints ink coverage proportional to stroke count and never reproduces the
	// art (internal/render/stub.go). The pairing WORKS, which is exactly why it is
	// dangerous — the verdicts are confident and meaningless. Not a boot error,
	// because it is a legitimate way to exercise the wiring in dev.
	if cfg.JudgeMode != config.JudgeModeFake && cfg.RenderMode != config.RenderModeNode {
		logger.Warn("judge: a real judge is scoring STUB rasters, which are ink-coverage blocks and not the drawings — set RENDER_MODE=node",
			"judge_mode", cfg.JudgeMode, "render_mode", cfg.RenderMode)
	}
	// The judge retries up to 3 times (docs/JUDGE.md §7), and the whole pass — both
	// renders included — has to fit inside game.JudgePassBudget. A JUDGE_TIMEOUT
	// generous enough to overflow it would not fail; it would silently truncate the
	// last attempt, which is the kind of misconfiguration that only shows up as an
	// occasional lost duel.
	if envelope := 3 * cfg.JudgeTimeout; envelope >= game.JudgePassBudget {
		logger.Warn("judge: JUDGE_TIMEOUT leaves no room for its own retries inside the judging pass",
			"timeout", cfg.JudgeTimeout, "retry_envelope", envelope, "pass_budget", game.JudgePassBudget)
	}

	// Single-player practice (internal/practice). It exists because a duel needs two
	// people at once and, without a player base, the first visitor waits alone and
	// gets an abandoned match — the product is unplayable by the person most likely
	// to try it. Every part needed to score ONE drawing already exists here: prompts,
	// the same renderer, the same quota.
	//
	// The critic is a seam of OURS (judge.Critic), NOT the collaborator's frozen
	// two-image contract, and it follows JUDGE_MODE so a real judge and a real critic
	// are never mismatched. JUDGE_MODE=http is the one mode with no critic: the
	// collaborator's service answers "which of these two is better" and has no
	// critique endpoint. Practice then refuses honestly rather than quietly falling
	// back to the fake — a made-up score presented as a real one is worse than a 500,
	// because the player cannot tell.
	var critic judge.Critic
	var practiceProvider aibudget.Provider
	switch cfg.JudgeMode {
	case config.JudgeModeGemini:
		critic = judge.NewGeminiCritic(cfg.GeminiAPIKey, cfg.GeminiModel, cfg.GeminiBaseURL, cfg.JudgeTimeout)
		practiceProvider = aibudget.ProviderGoogle
		logger.Info("practice: gemini critic (scores one drawing against its prompt)", "model", cfg.GeminiModel)
	case config.JudgeModeHTTP:
		// No critic at all, so no calls and no quota to protect — the empty provider
		// is not a special case here, it is the same rule as a fake.
		logger.Warn("practice: DISABLED — JUDGE_MODE=http has no critique endpoint (docs/JUDGE.md §2 is a two-image contract); /api/practice answers 500 until JUDGE_MODE is fake or gemini")
	default:
		critic = judge.NewFakeCritic()
		logger.Info("practice: fake critic (ink coverage — it never reads the prompt; set JUDGE_MODE=gemini for a real critique)")
	}

	// "What did I draw?" on /draw — the third vision seam off JUDGE_MODE, beside the
	// judge (two images, comparative) and the critic (one image against a prompt).
	// This one has no prompt at all: nobody supplied an answer, so there is nothing
	// to score and the model is simply asked what it sees.
	//
	// It follows JUDGE_MODE for the same reason practice does, and is unavailable
	// under http for the same reason: the collaborator's service answers one frozen
	// comparative question and has no endpoint for this. A nil guesser refuses
	// honestly rather than quietly answering with the fake, whose "guess" is a
	// canned string — a made-up answer presented as the AI's is a lie the player
	// cannot detect.
	var guesser judge.Guesser
	var guessProvider aibudget.Provider
	switch cfg.JudgeMode {
	case config.JudgeModeGemini:
		guesser = judge.NewGeminiGuesser(cfg.GeminiAPIKey, cfg.GeminiModel, cfg.GeminiBaseURL, cfg.JudgeTimeout)
		guessProvider = aibudget.ProviderGoogle
		logger.Info("guess: gemini vision (names what one drawing depicts)", "model", cfg.GeminiModel)
	case config.JudgeModeHTTP:
		logger.Warn("guess: DISABLED — JUDGE_MODE=http has no endpoint for it (docs/JUDGE.md §2 is a two-image contract); /api/guess answers 500 until JUDGE_MODE is fake or gemini")
	default:
		guesser = judge.NewFakeGuesser()
		logger.Info("guess: fake (a canned answer that never looks at the drawing; set JUDGE_MODE=gemini for a real one)")
	}

	// AI assist is a seam like render/judge (docs/ASSIST.md §3): the deterministic
	// FakeAssist runs the whole client flow with zero API dependency, and the real
	// AnthropicAssist swaps in by ASSIST_MODE with no handler change. The endpoint is
	// stateless (no DB) and rate-limited per user (each call can cost API money).
	assistLimiter := assist.NewRateLimiter(assist.DefaultBurst, assist.DefaultRefillInterval)
	var assistImpl assist.Assist
	if cfg.AssistMode == config.AssistModeAnthropic {
		assistImpl = assist.NewAnthropicAssist(cfg.AnthropicAPIKey, cfg.AssistModel)
		// Not "real LLM": the impl behind this mode is still the Phase A scaffold, and
		// the line right after CallsProvider below says so. A boot log that promises a
		// working feature is how a scaffold reaches production unnoticed.
		logger.Info("assist: anthropic", "model", cfg.AssistModel)
	} else {
		assistImpl = assist.NewFakeAssist()
		logger.Info("assist: fake (deterministic canned ops; set ASSIST_MODE=anthropic for the real LLM)")
	}
	// The impl is ASKED whether it calls anybody rather than inferred from the mode
	// (assist.CallsProvider). Today the anthropic impl is a scaffold that returns an
	// error without any network I/O, so ASSIST_MODE=anthropic bills nothing — which
	// is the truth, and which the boot line below says out loud instead of writing
	// ledger rows for calls that never happen.
	var assistProvider aibudget.Provider
	if assist.CallsProvider(assistImpl) {
		assistProvider = aibudget.ProviderAnthropic
	} else if cfg.AssistMode == config.AssistModeAnthropic {
		logger.Warn("assist: ASSIST_MODE=anthropic, but the impl makes no external call yet (scaffold) — nothing is billed and every request will fail until the SDK lands")
	}

	// The daily AI-call budget (internal/aibudget). The per-IP write limiter bounds
	// the request RATE; this bounds the scarce thing BEHIND the requests — a free
	// tier's per-DAY quota, one call per request. Each consumer below gets anonymous
	// funcs and never learns its own kind's name.
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
	// A ceiling configured for a feature that calls nobody is inert, not wrong — but
	// an inert ceiling and an enforced one look identical from the outside, which is
	// how an operator ends up believing a number that nothing reads.
	for _, kind := range aibudget.InertAllowances(aiPolicyByKind, cfg.AIDailyPerUser) {
		logger.Warn("ai budget: AI_DAILY_PER_USER sets an allowance for a kind whose impl calls no provider — nothing reads it",
			"kind", kind, "per_user", aiPolicyByKind[kind].PerUser)
	}

	gameSvc := game.NewServiceWithConcurrency(pool, queries, renderer, arbiter, logger, cfg.JudgeConcurrency)
	// One advisory check plus TWO billing ports, because a duel's two ledger facts
	// happen at different moments: the players are granted their round when the
	// second seat fills, and the judge request is billed at each entry into judging
	// — never for a round that ended abandoned or forfeited, and once more each time
	// the stuck-judging sweep re-fires a wedged attempt.
	duelCheck, _ := aiBudget.For(aibudget.KindDuel)
	billDuelPlayers, billDuelProvider := aiBudget.ForSplit(aibudget.KindDuel)
	gameSvc.SetBudget(duelCheck, billDuelPlayers, billDuelProvider)
	gameHandler := game.NewHandler(gameSvc, logger)

	// Practice has its own per-player allowance, and — whenever its critic and the
	// judge are backed by the same provider, which is every mode where both exist —
	// the same global ceiling, counted by ONE rule for both rather than by
	// per-feature copies that drift (internal/aibudget, docs/GAME.md §4.3). Practice
	// no longer reaches into the game module to ask: solo mode owed the duel nothing
	// but that one function.
	practiceCheck, practiceSpend := aiBudget.For(aibudget.KindPractice)
	practiceHandler := practice.NewHandler(
		practice.NewService(queries, renderer, critic, practiceCheck, practiceSpend, logger), logger)

	guessCheck, guessSpend := aiBudget.For(aibudget.KindGuess)
	guessHandler := guess.NewHandler(
		guess.NewService(renderer, guesser, guessCheck, guessSpend, logger), logger)

	// Assist gains the durable half of its ceiling. Its token bucket bounds the
	// RATE and lives in this process, so the host has been resetting it on every
	// deploy and every wake from idle; the daily quota lives in Postgres and
	// therefore actually holds.
	assistCheck, assistSpend := aiBudget.For(aibudget.KindAssist)
	assistHandler := assist.NewHandler(assistImpl, assistLimiter, assistCheck, assistSpend, logger)

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

	// Ledger retention. Nothing is READ past the rolling 24h window; rows are kept
	// a week so "why was I refused last Tuesday" has an answer, and swept after so
	// the two partial indexes stay at working-set size.
	background.Go(func() { aiBudget.RunSweeper(ctx, aibudget.DefaultSweepInterval) })

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
	practiceHandler.Routes(mux, authHandler.RequireAuth)
	guessHandler.Routes(mux, authHandler.RequireAuth)
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
		// A practice run costs a render AND a judge call — the same work a duel
		// submission costs, from one player instead of two. It belongs in the write
		// tier, not the generous default. GET /api/practice/prompt is a cheap read and
		// is left to the catch-all: this row matches POST only.
		{Name: "practice-write", Match: web.MethodPrefix("/api/practice", http.MethodPost), Limiter: writeLimiter},
		// A guess costs a render too, so it belongs in the same tier for the same
		// reason. The daily budget is NOT a substitute here: under JUDGE_MODE=fake
		// guess has no provider and is therefore unbudgeted by design, which would
		// leave the catch-all tier as the only thing between an authenticated caller
		// and 5 renders a second — and under RENDER_MODE=node each of those forks a
		// node-canvas subprocess. The ceiling guards a provider's quota; this guards
		// our own machine.
		{Name: "guess-write", Match: web.MethodPrefix("/api/guess", http.MethodPost), Limiter: writeLimiter},
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

// logAIBudget states the ceiling at boot, per kind, because an unenforced budget
// and an enforced one look identical from the outside until the day the quota
// runs out. Kinds are listed in a stable order so two boots are diffable.
func logAIBudget(logger *slog.Logger, policies map[aibudget.Kind]aibudget.Policy, global int) {
	enforced := make([]any, 0, 2*len(policies))
	var unbilled []string
	for _, kind := range aibudget.AllKinds() {
		p, ok := policies[kind]
		if !ok {
			continue // a kind with no feature wired yet
		}
		if p.Provider == "" {
			// A fake, an impl that is not built yet, or a mode with no impl for this
			// kind at all — three roads to the same fact, which is that nobody's quota
			// is at stake. "Calls no provider" is the fact; which road it took is the
			// business of the per-impl line above.
			unbilled = append(unbilled, string(kind))
			continue
		}
		enforced = append(enforced, string(kind), fmt.Sprintf("%d/day via %s", p.PerUser, p.Provider))
	}
	if len(enforced) == 0 {
		logger.Info("ai budget: nothing enforced (no AI impl here calls a provider, so there is no external quota to protect)")
		return
	}
	logger.Info("ai budget: per rolling 24h", append([]any{"global_per_provider", global}, enforced...)...)
	if len(unbilled) > 0 {
		logger.Info("ai budget: not enforced — these impls call no provider", "kinds", strings.Join(unbilled, ","))
	}
}
