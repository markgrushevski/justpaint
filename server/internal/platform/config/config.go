// Package config loads runtime configuration from the environment,
// applying defaults and failing fast when a mandatory value is missing.
package config

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Config holds runtime configuration for the server.
type Config struct {
	Addr         string // host:port to listen on (default ":8080")
	DatabaseURL  string // Postgres DSN — required
	JWTSecret    string // HS256 signing secret — required, never defaulted
	Env          string // "dev" | "prod" — required, never defaulted
	CookieSecure bool   // Secure flag on the session cookie

	// TrustProxy says whether an X-Forwarded-For / X-Request-Id header may be
	// believed. It is a real fork, not a formality: left false behind a proxy,
	// every client collapses into the proxy's own IP — one shared rate-limit
	// bucket and useless abuse logs; set true anywhere NOT behind exactly one
	// trusted hop, a client can forge the header and evade per-IP limits
	// entirely. Default false — the safe half.
	TrustProxy bool

	// AutoMigrate applies the embedded goose migrations at boot. On by default
	// because the deployment target has no shell — a one-off command against the
	// production database is a paid feature there, so "migrate, then roll the
	// binary" is not a sequence anyone can perform. Set false where a human (or a
	// pipeline) owns migrations and the server must never touch the schema.
	AutoMigrate bool

	// StaticDir is the built SPA (apps/web/dist) the server also serves, making
	// the API and the frontend one origin — required, since the session cookie
	// and the WS upgrade are same-origin and no CORS headers are sent. Empty
	// (the dev default) disables it: Vite serves the SPA and proxies /api here.
	StaticDir string

	// DBMaxConns bounds the pgx pool. pgx would default to max(4, NumCPU), which
	// is sized to the app host and knows nothing about the database's own
	// max_connections budget — on a small managed Postgres a few app instances
	// can exhaust it. Explicit and small by default.
	DBMaxConns int32

	// RenderMode selects the judge-raster renderer: "stub" (default; in-process,
	// zero-dep, loop-proving) or "node" (the authoritative Node Konva worker).
	RenderMode string
	// RenderCLI is the bundled Node worker entry (packages/render/dist/render.mjs).
	// Required when RenderMode == "node".
	RenderCLI string
	// RenderNodeBin is the node executable (default "node").
	RenderNodeBin string
	// JudgeConcurrency bounds two things, and the second one was added after a
	// review found the first was not enough.
	//
	// It bounds how many judging passes (authoritative render + judge call) run at
	// once — under RenderMode "node" each pass spawns OS child processes rasterizing
	// a 1024² canvas, so an unbounded backlog drain is a fork bomb on a small
	// instance. It ALSO sizes the semaphore inside the node renderer itself
	// (internal/render), which is what bounds the SYNCHRONOUS renders on
	// /api/guess and /api/practice: those two render inside the request and have no
	// pass-level limiter of their own, so without it one client's request burst is
	// one subprocess per request.
	JudgeConcurrency int

	// AIDailyGlobal and AIDailyPerUser cap how many AI calls a rolling 24h window
	// may spend. They guard a resource the per-IP rate limiter cannot see: a free
	// tier's binding limit is requests per DAY, one call is one request, and a
	// duel is only ~4 write requests — so a single IP inside the write limiter can
	// burn a daily quota in the hundreds within minutes, after which NOBODY gets
	// served. The per-user half is what keeps one abuser from denying service to
	// everyone.
	//
	// AIDailyGlobal is applied PER PROVIDER AND MODEL, not once across all of them:
	// Google running dry must never refuse an Anthropic-backed feature that still
	// has quota, and — since Google meters its free tier per MODEL — a kind pinned
	// to one model must not be refused because a different kind emptied a different
	// model's pool. Which provider and model back which kind is decided at the
	// composition root from JUDGE_MODE / ASSIST_MODE and AIModelPerKind, never
	// configured here.
	//
	// AIDailyPerUser maps an AI-call kind ("duel", "practice", …) to that kind's
	// own per-player ceiling; a kind absent from the map keeps its code default.
	// The kind NAMES are not validated here — the valid set belongs to
	// internal/aibudget and grows with the code, and config must stay free of
	// domain imports — so the composition root rejects an unknown one at boot,
	// which is the same instant with a better error.
	//
	// There is deliberately no "unlimited" sentinel: an AI call always has a
	// budget, and an operator who wants effectively none sets a large number.
	// Every value is rejected below 1 at boot.
	AIDailyGlobal  int
	AIDailyPerUser map[string]int

	// AIModelPerKind overrides, per AI-call kind ("duel", "guess", …), the model
	// that kind runs on; GeminiModel is the default for every kind that names no
	// override. It exists because the kinds' jobs differ in difficulty — the duel
	// judge's number feeds Elo and has to be the steadiest, while the guesser only
	// names what it sees and its mistakes cost nothing — so one model id for all of
	// them is either an overpay or an underserve.
	//
	// It takes the SAME kind=value comma-list shape as AIDailyPerUser, on purpose:
	// one person maintains this, and two env vars about the same set of kinds
	// reading the same way is worth more than either being shorter.
	//
	// Like AIDailyPerUser, the kind NAMES are not validated here — the valid set
	// belongs to internal/aibudget and grows with the code — so the composition root
	// rejects an unknown one at boot, which is the same instant with a better error.
	AIModelPerKind map[string]string

	// JudgeMode selects the judge impl (docs/JUDGE.md): "fake" (default; the
	// zero-dependency ink-coverage stand-in that never reads the prompt), "http"
	// (the collaborator's ML over the §6 contract) or "gemini" (a vision LLM
	// scoring both rasters in one call — the real verdict while the ML is built).
	JudgeMode string
	// JudgeBaseURL is the collaborator's service root, required for JudgeMode
	// "http" (§7).
	JudgeBaseURL string
	// JudgeTimeout bounds ONE judging call, retries excluded (§7 pins 10s). The
	// critic and the guesser share it — all three ask a vision model one short
	// question and get a handful of scalars back. Assist does not; see AssistTimeout.
	JudgeTimeout time.Duration
	// GeminiAPIKey is the server-side key for JudgeMode "gemini". Never reaches
	// the client. Required when that mode is selected.
	GeminiAPIKey string
	// GeminiModel is the DEFAULT model id for every Gemini-backed kind (judge,
	// critic, guesser, assist). Configurable rather than hardcoded because Google's
	// free-tier model names and quotas move faster than our releases — a changed
	// name must not need a code change. AIModelPerKind overrides it per kind.
	GeminiModel string
	// GeminiBaseURL is the API root, overridable for the same reason and so a
	// test can point at an httptest server.
	GeminiBaseURL string

	// AssistMode selects the AI-assist impl (docs/ASSIST.md): "fake" (default;
	// deterministic canned ops that ignore the prompt, zero API dependency),
	// "gemini" (the real impl — a prompt really becomes shapes) or "anthropic" (the
	// Phase A scaffold that never made a call and is superseded by gemini).
	// Mirrors the RenderMode mode-switch.
	AssistMode string
	// AnthropicAPIKey is the server-side key for AssistMode == "anthropic" — never
	// reaches the client (docs/ASSIST.md §1). Required when the anthropic mode is
	// selected.
	AnthropicAPIKey string
	// AssistModel is the model id for the ANTHROPIC impl only (default
	// "claude-opus-4-8"). The Gemini assist takes its model from the same place
	// every other Gemini seam does — GeminiModel, overridable by
	// AIModelPerKind["assist"] — because that is where its quota is counted too.
	AssistModel string

	// AssistTimeout bounds ONE assist call, retries excluded. It is separate from
	// JudgeTimeout, which every other AI seam shares, because the work is not
	// comparable: a verdict is four scalars, while composing a picture is a list of
	// shapes from a model that reasons first. Measured 4-9s healthy and far longer
	// when Google is busy (2026-09-20), against a judge that answers in one or two.
	//
	// Folding it into JUDGE_TIMEOUT was the first attempt and it was wrong in both
	// directions: the 10s default leaves no headroom at all for the slow case, and
	// raising the shared knob to fit would hand the duel a retry envelope that no
	// longer fits inside game.JudgePassBudget.
	AssistTimeout time.Duration

	// WSReadIdleTimeout evicts a socket that has gone this long without any proof
	// of life. It must clear the client's own ping cadence (apps/web PlayView.vue
	// WS_PING_MS, 25s) with margin for jitter and background-tab throttling.
	WSReadIdleTimeout time.Duration
	// WSHeartbeatInterval is how often the server probes a quiet socket itself, so
	// a duelist who is drawing in silence is never mistaken for a dead peer. Must
	// stay well under WSReadIdleTimeout.
	WSHeartbeatInterval time.Duration
	// WSMaxConns / WSMaxConnsPerIP bound concurrent sockets process-wide and per
	// client. Memory is the scarce resource on a small instance, and a socket is
	// cheap enough to open that "one per user per match" is not a bound at all.
	WSMaxConns      int
	WSMaxConnsPerIP int

	// WSAllowedOrigins are extra Origin hosts authorized for the WebSocket handshake
	// (coder/websocket path.Match patterns against the Origin header host, e.g.
	// "app.example.com" or "localhost:*"). The request Host is ALWAYS authorized, so a
	// true same-origin deployment needs no entry — this exists only for the split-host
	// case: the dev Vite proxy sets changeOrigin, so the backend sees Host=:8080 while
	// the browser Origin is :7777, which the default same-origin check would reject.
	// NEVER contains "*" (that would open the socket to cross-site CSRF via the auto-
	// attached cookie) — docs/DESIGN-PHASE3-LIVE.md §3.4.
	WSAllowedOrigins []string
}

// Environments. ENV has no default on purpose — see Load.
const (
	EnvDev  = "dev"
	EnvProd = "prod"
)

// DefaultDBMaxConns is the pool ceiling unless DB_MAX_CONNS overrides it. Sized
// for a small managed Postgres (free-tier instances cap connections low), not
// for the app host's CPU count.
const DefaultDBMaxConns = 10

// DefaultJudgeConcurrency is the ceiling on simultaneous judging passes unless
// JUDGE_CONCURRENCY overrides it. Two, because each pass under RENDER_MODE=node
// forks two node-canvas processes and the target instance is memory-poor.
const DefaultJudgeConcurrency = 2

// DefaultAIDailyGlobal is the per-provider daily ceiling, in AI calls per rolling
// 24h window. The per-KIND player ceilings live in internal/aibudget beside the
// kinds they bound, since adding a kind must not mean editing config.
//
// It used to be 200, chosen against an UNVERIFIED belief about the free tier. On
// 2026-09-20 the API answered the question itself, in a 429 body:
//
//	"quotaId": "GenerateRequestsPerDayPerProjectPerModel-FreeTier",
//	"quotaDimensions": {"model": "gemini-3.6-flash"},
//	"quotaValue": "20"
//
// Twenty requests a day, per project, PER MODEL — which is also why the provider
// key carries the model (aibudget.Provider.WithModel): the two numbers are only
// comparable when they count the same thing.
//
// So 15, and be honest about what it does and does not buy. On a free key it is
// not the ceiling that binds; Google's is, and it arrives as ErrQuotaExhausted,
// which every synchronous handler now turns into the same calm refusal our own
// ceiling produces. Nor is 15 "safe": the ledger records one row per judging
// PASS, while a pass may retry up to geminiMaxAttempts times inside the seam, so
// 15 rows can be anywhere from 15 to 45 real requests. The honest reading is that
// this number keeps ONE player from spending the day's quota in a burst, and that
// the operator is expected to raise it the moment the key is not a free one.
const DefaultAIDailyGlobal = 15

// WebSocket hardening defaults. The idle timeout clears the client's 25s ping
// with margin; the heartbeat sits well under the timeout so a quiet-but-healthy
// socket is probed twice before it could ever be evicted.
const (
	DefaultWSReadIdleTimeout   = 60 * time.Second
	DefaultWSHeartbeatInterval = 20 * time.Second
	DefaultWSMaxConns          = 500
	DefaultWSMaxConnsPerIP     = 20
)

// Render modes.
const (
	RenderModeStub = "stub"
	RenderModeNode = "node"
)

// Assist modes.
const (
	JudgeModeFake   = "fake"
	JudgeModeHTTP   = "http"
	JudgeModeGemini = "gemini"

	AssistModeFake      = "fake"
	AssistModeGemini    = "gemini"
	AssistModeAnthropic = "anthropic"
)

// DefaultAssistModel is the model id used by the real assist impl unless
// ASSIST_MODEL overrides it (docs/ASSIST.md §3.2).
const DefaultAssistModel = "claude-opus-4-8"

// DefaultJudgeTimeout bounds one judging call (docs/JUDGE.md §7). ML inference
// and a vision LLM are both slow; JUDGE_TIMEOUT overrides it.
const DefaultJudgeTimeout = 10 * time.Second

// DefaultAssistTimeout bounds one assist call. Six times the judge's, because the
// work is: a verdict is four scalars, a drawing is a list of shapes composed by a
// model that thinks first.
//
// A healthy call measured 4-9s live (2026-09-20, gemini-3.1-flash-lite), so this is
// deliberately loose rather than tuned. The cost of loose is a wedged call holding
// a request goroutine for a minute; the cost of tight is a feature that fails
// whenever Google is slow, which is the failure nobody can diagnose from the
// outside. ASSIST_TIMEOUT overrides it.
const DefaultAssistTimeout = 60 * time.Second

// DefaultGeminiModel is a PINNED version, deliberately, not the floating
// "gemini-flash-latest" alias. A judge decides ratings, so a model that changes
// under us without a word is worse than one that stops: a pin is retired LOUDLY
// — verified 2026-09-19, when the previous default answered "models/
// gemini-2.5-flash is no longer available to new users" and named its own
// replacement, which is about as good as a deprecation gets. Override
// GEMINI_MODEL rather than editing this.
const DefaultGeminiModel = "gemini-3.6-flash"

// DefaultGeminiBaseURL is the public Generative Language API root.
const DefaultGeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// Load reads configuration from the environment and fails fast on any missing
// required value.
//
// ENV, JWT_SECRET and DATABASE_URL are mandatory: the server refuses to start
// without them rather than falling back to an empty signing secret (a known red
// flag — an empty secret lets anyone forge a session), a nil database, or a
// silently insecure cookie.
//
// ENV deliberately has NO default. It used to default to "dev", which made
// CookieSecure default to false: forgetting ENV=prod on a deploy downgraded the
// session cookie to non-Secure and the server booted happily, so the mistake was
// invisible until someone read the Set-Cookie header. A missing ENV is now a
// boot error — the one failure mode you cannot miss.
func Load() (Config, error) {
	env := strings.ToLower(strings.TrimSpace(os.Getenv("ENV")))
	cfg := Config{
		Addr:        getenv("ADDR", ":8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		Env:         env,
		// Secure cookies are dropped by browsers over plain http://localhost,
		// so relax the flag in dev; require it everywhere else.
		CookieSecure:    env != EnvDev,
		StaticDir:       strings.TrimSpace(os.Getenv("STATIC_DIR")),
		RenderMode:      strings.ToLower(getenv("RENDER_MODE", RenderModeStub)),
		RenderCLI:       os.Getenv("RENDER_CLI"),
		RenderNodeBin:   getenv("RENDER_NODE_BIN", "node"),
		JudgeMode:       strings.ToLower(getenv("JUDGE_MODE", JudgeModeFake)),
		JudgeBaseURL:    strings.TrimRight(os.Getenv("JUDGE_BASE_URL"), "/"),
		GeminiAPIKey:    os.Getenv("GEMINI_API_KEY"),
		GeminiModel:     getenv("GEMINI_MODEL", DefaultGeminiModel),
		GeminiBaseURL:   strings.TrimRight(getenv("GEMINI_BASE_URL", DefaultGeminiBaseURL), "/"),
		AssistMode:      strings.ToLower(getenv("ASSIST_MODE", AssistModeFake)),
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		AssistModel:     getenv("ASSIST_MODEL", DefaultAssistModel),
	}

	// WS origins: explicit env wins; otherwise dev allows the local Vite proxy origin
	// (whose changeOrigin splits Host from Origin — see the field doc). Outside dev the
	// default is empty (same-origin only) — a split-host prod sets WS_ALLOWED_ORIGINS.
	cfg.WSAllowedOrigins = splitList(os.Getenv("WS_ALLOWED_ORIGINS"))
	if len(cfg.WSAllowedOrigins) == 0 && env == EnvDev {
		cfg.WSAllowedOrigins = []string{"localhost:*", "127.0.0.1:*"}
	}

	maxConns, err := getenvInt("DB_MAX_CONNS", DefaultDBMaxConns)
	if err != nil {
		return Config{}, err
	}
	if maxConns < 1 {
		return Config{}, fmt.Errorf("config: DB_MAX_CONNS must be >= 1, got %d", maxConns)
	}
	cfg.DBMaxConns = int32(maxConns)

	trustProxy, err := getenvBool("TRUST_PROXY", false)
	if err != nil {
		return Config{}, err
	}
	cfg.TrustProxy = trustProxy

	autoMigrate, err := getenvBool("AUTO_MIGRATE", true)
	if err != nil {
		return Config{}, err
	}
	cfg.AutoMigrate = autoMigrate

	judgeConcurrency, err := getenvInt("JUDGE_CONCURRENCY", DefaultJudgeConcurrency)
	if err != nil {
		return Config{}, err
	}
	if judgeConcurrency < 1 {
		return Config{}, fmt.Errorf("config: JUDGE_CONCURRENCY must be >= 1, got %d (0 would stop judging entirely)", judgeConcurrency)
	}
	cfg.JudgeConcurrency = judgeConcurrency

	if err := loadAIBudget(&cfg); err != nil {
		return Config{}, err
	}

	if err := loadWSLimits(&cfg); err != nil {
		return Config{}, err
	}

	var missing []string
	if cfg.Env == "" {
		missing = append(missing, "ENV")
	}
	if cfg.JWTSecret == "" {
		missing = append(missing, "JWT_SECRET")
	}
	if cfg.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("config: missing required env: %s (ENV must be %q locally or %q on a deploy)", strings.Join(missing, ", "), EnvDev, EnvProd)
	}

	// A typo'd ENV is as dangerous as a missing one: it decides CookieSecure, the
	// JWT-length floor and the WS origin default, so only the two known values pass.
	if cfg.Env != EnvDev && cfg.Env != EnvProd {
		return Config{}, fmt.Errorf("config: ENV must be %q or %q, got %q", EnvDev, EnvProd, cfg.Env)
	}

	if err := validateDatabaseURL(cfg.DatabaseURL); err != nil {
		return Config{}, err
	}

	// Outside dev, require a strong HS256 secret: a short/guessable key is
	// brute-forceable offline against any captured token, and a forged token is
	// full account takeover (the JWT subject is trusted as the owner id).
	const minSecretLen = 32
	if cfg.Env != EnvDev && len(cfg.JWTSecret) < minSecretLen {
		return Config{}, fmt.Errorf("config: JWT_SECRET must be at least %d bytes outside dev", minSecretLen)
	}

	switch cfg.RenderMode {
	case RenderModeStub:
	case RenderModeNode:
		// The Node worker path must be given explicitly; guessing it is worse than
		// failing fast (a wrong path would silently fall over on every judging —
		// out-of-band, so it only shows as a stuck match, not a boot error).
		if cfg.RenderCLI == "" {
			return Config{}, fmt.Errorf("config: RENDER_CLI is required when RENDER_MODE=node (path to packages/render/dist/render.mjs)")
		}
		// Surface a typo'd / unbuilt path at boot rather than at first judging.
		if _, err := os.Stat(cfg.RenderCLI); err != nil {
			return Config{}, fmt.Errorf("config: RENDER_CLI not found (%s) — build it with `npm run build -w @justpaint/render`: %w", cfg.RenderCLI, err)
		}
		// Same reason for the interpreter itself: a container that ships the Go
		// binary without a Node runtime passes the RENDER_CLI stat (the file is
		// there) and then fails every single judging out of band. The image is
		// either built with both or it must not claim RENDER_MODE=node.
		if _, err := exec.LookPath(cfg.RenderNodeBin); err != nil {
			return Config{}, fmt.Errorf("config: RENDER_NODE_BIN %q is not executable on PATH — RENDER_MODE=node needs a Node runtime alongside the server: %w", cfg.RenderNodeBin, err)
		}
	default:
		return Config{}, fmt.Errorf("config: RENDER_MODE must be %q or %q", RenderModeStub, RenderModeNode)
	}

	judgeTimeout, err := getenvDuration("JUDGE_TIMEOUT", DefaultJudgeTimeout)
	if err != nil {
		return Config{}, err
	}
	if judgeTimeout <= 0 {
		return Config{}, fmt.Errorf("config: JUDGE_TIMEOUT must be > 0, got %s", judgeTimeout)
	}
	cfg.JudgeTimeout = judgeTimeout

	// Same fail-fast shape as RENDER_CLI and ANTHROPIC_API_KEY: a judge mode whose
	// dependency is missing would fail out of band on the first duel, long after
	// the deploy that broke it. A boot error names the cause.
	switch cfg.JudgeMode {
	case JudgeModeFake:
	case JudgeModeHTTP:
		if cfg.JudgeBaseURL == "" {
			return Config{}, fmt.Errorf("config: JUDGE_BASE_URL is required when JUDGE_MODE=%s", JudgeModeHTTP)
		}
	case JudgeModeGemini:
		if cfg.GeminiAPIKey == "" {
			return Config{}, fmt.Errorf("config: GEMINI_API_KEY is required when JUDGE_MODE=%s", JudgeModeGemini)
		}
	default:
		return Config{}, fmt.Errorf("config: JUDGE_MODE must be %q, %q or %q", JudgeModeFake, JudgeModeHTTP, JudgeModeGemini)
	}

	assistTimeout, err := getenvDuration("ASSIST_TIMEOUT", DefaultAssistTimeout)
	if err != nil {
		return Config{}, err
	}
	if assistTimeout <= 0 {
		return Config{}, fmt.Errorf("config: ASSIST_TIMEOUT must be > 0, got %s", assistTimeout)
	}
	cfg.AssistTimeout = assistTimeout

	switch cfg.AssistMode {
	case AssistModeFake:
	case AssistModeGemini:
		// Same key, same quota and same API as the Gemini judge — deliberately, since
		// they meter together — so the same fail-fast applies. Note that this is
		// INDEPENDENT of JUDGE_MODE: a deployment may want a real assist while the
		// duel still runs on the fake judge, and nothing about the two decisions is
		// the same decision.
		if cfg.GeminiAPIKey == "" {
			return Config{}, fmt.Errorf("config: GEMINI_API_KEY is required when ASSIST_MODE=%s", AssistModeGemini)
		}
	case AssistModeAnthropic:
		// The real impl needs a server-side key; guessing/defaulting it is worse than
		// failing fast (a keyless anthropic mode would 500 on every request, out of
		// band — a boot error is the honest signal). Mirrors the RENDER_CLI fail-fast.
		if cfg.AnthropicAPIKey == "" {
			return Config{}, fmt.Errorf("config: ANTHROPIC_API_KEY is required when ASSIST_MODE=anthropic")
		}
	default:
		return Config{}, fmt.Errorf("config: ASSIST_MODE must be %q, %q or %q", AssistModeFake, AssistModeGemini, AssistModeAnthropic)
	}

	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// validateDatabaseURL rejects a DATABASE_URL that is not a Postgres DSN at all,
// with a message that says what to paste instead.
//
// The mistake this exists for is specific and easy to make: a managed provider's
// dashboard shows a project URL (https://<ref>.supabase.co) next to the actual
// connection string, and pasting the former gets you a parse error from deep
// inside the driver — "failed to parse as keyword/value" — which reads like a
// bug in the app rather than a wrong value in one env var.
func validateDatabaseURL(dsn string) error {
	switch {
	case strings.HasPrefix(dsn, "postgres://"), strings.HasPrefix(dsn, "postgresql://"):
		return nil
	// The keyword/value form ("host=... user=...") is equally valid for pgx.
	case strings.Contains(dsn, "host="):
		return nil
	case strings.HasPrefix(dsn, "http://"), strings.HasPrefix(dsn, "https://"):
		return fmt.Errorf("config: DATABASE_URL is an HTTP URL (%s…), which is a provider's project/API endpoint, not a database connection string — copy the Postgres URI instead (postgresql://user:password@host:5432/dbname)", firstRunes(dsn, 30))
	default:
		return fmt.Errorf("config: DATABASE_URL is not a Postgres connection string — expected postgres://… , postgresql://… or a keyword/value DSN (host=… user=…), got %q", firstRunes(dsn, 30))
	}
}

// firstRunes truncates for an error message without splitting a rune, and
// without echoing a whole DSN (it may carry a password) into the logs.
func firstRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n])
}

// loadAIBudget reads the daily AI-call ceilings: one global number applied per
// provider, plus an optional per-kind map of player ceilings.
//
// Zero or negative is a boot error rather than a synonym for "off": a 0 here
// would read as "no budget" to an operator and behave as "refuse everything" to
// the code, and the mode where the budget genuinely does not apply is a fake impl
// (JUDGE_MODE=fake, ASSIST_MODE=fake), which needs no sentinel because there is
// no external quota to protect. Same fail-fast shape, and the same reasoning, as
// JUDGE_CONCURRENCY.
//
// A per-user cap ABOVE the global one is allowed on purpose: that is how you say
// "effectively no per-player limit, the global budget is the only ceiling".
//
// # The legacy names
//
// JUDGE_DAILY_BUDGET and JUDGE_DAILY_PER_USER predate the per-kind budget and are
// live in deployed environments that are edited BY HAND. They keep working, and
// they keep meaning exactly what they meant, so the deploy that lands this changes
// no behaviour until an operator opts in:
//
//   - JUDGE_DAILY_BUDGET is a straight alias of AI_DAILY_GLOBAL. Both set to
//     DIFFERENT values is a boot error naming both — the two cannot be reconciled
//     and guessing which was meant is how a ceiling ends up at a number nobody
//     chose.
//   - JUDGE_DAILY_PER_USER seeds the player ceiling for "duel" and "practice"
//     ONLY — the two kinds that existed when it was named, which shared one number.
//     It deliberately does NOT reach kinds invented later: an operator who set it
//     to 20 was not consenting to 20 AI guesses a day for a feature that did not
//     exist, and a new kind quietly inheriting an old number is the failure this
//     whole per-kind split exists to prevent. Newer kinds take their code default
//     until AI_DAILY_PER_USER names them.
func loadAIBudget(cfg *Config) error {
	global, from, err := getenvIntAliased("AI_DAILY_GLOBAL", "JUDGE_DAILY_BUDGET", DefaultAIDailyGlobal)
	if err != nil {
		return err
	}
	if global < 1 {
		// Named after the variable the operator actually set, not after the
		// current name: being told to fix AI_DAILY_GLOBAL when you set
		// JUDGE_DAILY_BUDGET sends you looking for a variable you never wrote.
		return fmt.Errorf("config: %s must be >= 1, got %d (0 would refuse every AI call; there is no unlimited setting — set a large number if you mean effectively none)", from, global)
	}

	perUser := map[string]int{}
	// The legacy single number first, so an explicit per-kind entry below wins.
	if legacy := strings.TrimSpace(os.Getenv("JUDGE_DAILY_PER_USER")); legacy != "" {
		v, err := strconv.Atoi(legacy)
		if err != nil {
			return fmt.Errorf("config: JUDGE_DAILY_PER_USER must be an integer, got %q: %w", legacy, err)
		}
		if v < 1 {
			return fmt.Errorf("config: JUDGE_DAILY_PER_USER must be >= 1, got %d (0 would refuse every duel; there is no unlimited setting — set a large number if you mean effectively none)", v)
		}
		perUser["duel"] = v
		perUser["practice"] = v
	}
	rawPerUser, err := parseKindList("AI_DAILY_PER_USER", "duel=20,guess=2")
	if err != nil {
		return err
	}
	for kind, raw := range rawPerUser {
		v, err := strconv.Atoi(raw)
		if err != nil {
			return fmt.Errorf("config: AI_DAILY_PER_USER entry %q must be of the form kind=number, got %q: %w", kind+"="+raw, raw, err)
		}
		if v < 1 {
			return fmt.Errorf("config: AI_DAILY_PER_USER entry %q must be >= 1, got %d (0 would refuse every call of that kind; there is no unlimited setting — set a large number if you mean effectively none)", kind+"="+raw, v)
		}
		perUser[kind] = v
	}

	// The per-kind MODEL map, in the same shape for the same reason the ceilings are
	// per kind: the kinds' jobs differ in difficulty, so pinning them all to one
	// model is either an overpay for the cheap ones or an underserve for the duel,
	// whose number feeds Elo. Values are model ids and are used verbatim — the kind
	// names are checked at the composition root, where the valid set lives.
	perKindModel, err := parseKindList("AI_MODEL_PER_KIND", "duel=gemini-3.6-pro,guess=gemini-3.6-flash-lite")
	if err != nil {
		return err
	}

	cfg.AIDailyGlobal = global
	cfg.AIDailyPerUser = perUser
	cfg.AIModelPerKind = perKindModel
	return nil
}

// parseKindList reads a "kind=value" comma-separated env var into a map, which is
// the shape BOTH per-kind knobs use (AI_DAILY_PER_USER, AI_MODEL_PER_KIND). One
// parser rather than two so the two variables cannot diverge on whitespace,
// duplicate keys or what counts as an empty entry — a single maintainer should
// have to learn the form once.
//
// It only produces strings. What a value MEANS is the caller's business: one wants
// an integer in a range, the other a model id used verbatim, and folding either
// meaning in here would make the parser answer a question it cannot see.
//
// example is shown in the syntax error, so the message names the variable the
// operator actually set and shows a line that would have worked. Duplicate keys
// resolve last-wins, matching how an operator reads a list left to right.
func parseKindList(key, example string) (map[string]string, error) {
	out := map[string]string{}
	for _, entry := range splitList(os.Getenv(key)) {
		kind, value, found := strings.Cut(entry, "=")
		if !found {
			return nil, fmt.Errorf("config: %s entry %q must be of the form kind=value, e.g. %q", key, entry, example)
		}
		kind, value = strings.TrimSpace(kind), strings.TrimSpace(value)
		if kind == "" {
			return nil, fmt.Errorf("config: %s entry %q names no kind", key, entry)
		}
		if value == "" {
			return nil, fmt.Errorf("config: %s entry %q names no value for kind %q", key, entry, kind)
		}
		out[kind] = value
	}
	return out, nil
}

// getenvIntAliased reads an integer that answers to two names, one current and
// one kept alive for already-deployed environments. Both set to the SAME value is
// fine (an operator mid-migration); both set to different values is a boot error,
// because silently preferring one would put a bound at a number nobody chose.
//
// It also returns WHICH name supplied the value, so a later range check can blame
// the variable the operator actually wrote. When neither is set that is the
// current name, since a default out of range would be our bug, not theirs.
func getenvIntAliased(current, legacy string, fallback int) (value int, from string, err error) {
	cur := strings.TrimSpace(os.Getenv(current))
	old := strings.TrimSpace(os.Getenv(legacy))
	switch {
	case cur == "" && old == "":
		return fallback, current, nil
	case cur == "":
		v, err := getenvInt(legacy, fallback)
		return v, legacy, err
	case old == "":
		v, err := getenvInt(current, fallback)
		return v, current, err
	}

	// Both set. "The same value" is a question about the NUMBERS, not about the
	// spelling: 100 and 0100 are the same bound, and refusing to boot over a
	// leading zero would be a fault report about nothing. Each is parsed through
	// the same helper as the single-name cases, so a non-integer is still named
	// after the variable that carries it.
	curVal, err := getenvInt(current, fallback)
	if err != nil {
		return 0, current, err
	}
	oldVal, err := getenvInt(legacy, fallback)
	if err != nil {
		return 0, legacy, err
	}
	if curVal == oldVal {
		return curVal, current, nil
	}
	return 0, current, fmt.Errorf("config: %s=%q and %s=%q disagree — %s is the current name and %s is kept only for already-deployed environments; set one, or set both to the same value", current, cur, legacy, old, current, legacy)
}

// loadWSLimits reads the WebSocket hardening knobs and rejects a combination
// that would quietly disable what it claims to configure: an unlimited cap, or a
// heartbeat no more frequent than the idle timeout (which would let a healthy but
// silent socket be evicted between probes — mid-round, for a player who is simply
// drawing).
func loadWSLimits(cfg *Config) error {
	idle, err := getenvDuration("WS_READ_IDLE_TIMEOUT", DefaultWSReadIdleTimeout)
	if err != nil {
		return err
	}
	heartbeat, err := getenvDuration("WS_HEARTBEAT_INTERVAL", DefaultWSHeartbeatInterval)
	if err != nil {
		return err
	}
	if idle <= 0 || heartbeat <= 0 {
		return fmt.Errorf("config: WS_READ_IDLE_TIMEOUT and WS_HEARTBEAT_INTERVAL must be positive (got %s and %s)", idle, heartbeat)
	}
	if heartbeat >= idle {
		return fmt.Errorf("config: WS_HEARTBEAT_INTERVAL (%s) must be shorter than WS_READ_IDLE_TIMEOUT (%s), or a quiet connection is evicted before it is ever probed", heartbeat, idle)
	}

	maxConns, err := getenvInt("WS_MAX_CONNS", DefaultWSMaxConns)
	if err != nil {
		return err
	}
	maxPerIP, err := getenvInt("WS_MAX_CONNS_PER_IP", DefaultWSMaxConnsPerIP)
	if err != nil {
		return err
	}
	if maxConns < 1 || maxPerIP < 1 {
		return fmt.Errorf("config: WS_MAX_CONNS and WS_MAX_CONNS_PER_IP must be >= 1 (0 or less means unlimited, which is not a cap)")
	}
	if maxPerIP > maxConns {
		return fmt.Errorf("config: WS_MAX_CONNS_PER_IP (%d) exceeds WS_MAX_CONNS (%d), so the per-IP cap can never bind", maxPerIP, maxConns)
	}

	cfg.WSReadIdleTimeout = idle
	cfg.WSHeartbeatInterval = heartbeat
	cfg.WSMaxConns = maxConns
	cfg.WSMaxConnsPerIP = maxPerIP
	return nil
}

// getenvDuration reads a Go duration env var ("60s", "2m"), falling back when
// unset; a malformed value is a boot error, like getenvInt.
func getenvDuration(key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("config: %s must be a duration like \"60s\" or \"2m\", got %q: %w", key, raw, err)
	}
	return v, nil
}

// getenvBool reads a boolean env var, falling back when unset. Like getenvInt, a
// malformed value is a boot error: TRUST_PROXY="yes" silently read as false would
// be a security control quietly not doing what the operator meant.
func getenvBool(key string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("config: %s must be a boolean (true/false/1/0), got %q: %w", key, raw, err)
	}
	return v, nil
}

// getenvInt reads an integer env var, falling back when unset. A malformed value
// is a boot error, never a silent fallback — a typo'd bound is a bound nobody set.
func getenvInt(key string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("config: %s must be an integer, got %q: %w", key, raw, err)
	}
	return v, nil
}

// splitList parses a comma-separated env value into a trimmed, empty-free slice.
func splitList(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
