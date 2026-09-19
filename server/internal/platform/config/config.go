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
	// JudgeConcurrency bounds how many judging passes (authoritative render +
	// judge call) run at once. Under RenderMode "node" each pass spawns two OS
	// child processes rasterizing a 1024² canvas, so an unbounded backlog drain
	// is a fork bomb on a small instance.
	JudgeConcurrency int

	// JudgeDailyBudget and JudgeDailyPerUser cap how many judge calls a rolling
	// 24h window may spend, globally and per player. They guard a resource the
	// per-IP rate limiter cannot see: the free tier's binding limit is requests
	// per DAY, one duel costs exactly one call, and a duel is only ~4 write
	// requests — so a single IP inside the write limiter can burn a daily quota
	// in the hundreds within minutes, after which NOBODY's duels get judged. The
	// per-user half is what keeps one abuser from denying service to everyone.
	//
	// There is deliberately no "unlimited" sentinel: a judge always has a budget,
	// and an operator who wants effectively none sets a large number. Both are
	// rejected below 1 at boot.
	JudgeDailyBudget  int
	JudgeDailyPerUser int

	// JudgeMode selects the judge impl (docs/JUDGE.md): "fake" (default; the
	// zero-dependency ink-coverage stand-in that never reads the prompt), "http"
	// (the collaborator's ML over the §6 contract) or "gemini" (a vision LLM
	// scoring both rasters in one call — the real verdict while the ML is built).
	JudgeMode string
	// JudgeBaseURL is the collaborator's service root, required for JudgeMode
	// "http" (§7).
	JudgeBaseURL string
	// JudgeTimeout bounds ONE judging call, retries excluded (§7 pins 10s).
	JudgeTimeout time.Duration
	// GeminiAPIKey is the server-side key for JudgeMode "gemini". Never reaches
	// the client. Required when that mode is selected.
	GeminiAPIKey string
	// GeminiModel is the model id. Configurable rather than hardcoded because
	// Google's free-tier model names and quotas move faster than our releases —
	// a changed name must not need a code change.
	GeminiModel string
	// GeminiBaseURL is the API root, overridable for the same reason and so a
	// test can point at an httptest server.
	GeminiBaseURL string

	// AssistMode selects the AI-assist impl (docs/ASSIST.md): "fake" (default;
	// deterministic canned ops, zero API dependency) or "anthropic" (the real LLM
	// impl, scaffolded in Phase A). Mirrors the RenderMode mode-switch.
	AssistMode string
	// AnthropicAPIKey is the server-side key for AssistMode == "anthropic" — never
	// reaches the client (docs/ASSIST.md §1). Required when the anthropic mode is
	// selected.
	AnthropicAPIKey string
	// AssistModel is the model id for the real impl (default "claude-opus-4-8").
	AssistModel string

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

// Judge-budget defaults, in judge calls per rolling 24h window. 200 global sits
// under a free tier's daily request quota with headroom for the odd retry; 20 per
// player is a generous evening of duelling and still leaves the global budget
// reachable only by a real crowd, never by one person.
const (
	DefaultJudgeDailyBudget  = 200
	DefaultJudgeDailyPerUser = 20
)

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
	AssistModeAnthropic = "anthropic"
)

// DefaultAssistModel is the model id used by the real assist impl unless
// ASSIST_MODEL overrides it (docs/ASSIST.md §3.2).
const DefaultAssistModel = "claude-opus-4-8"

// DefaultJudgeTimeout bounds one judging call (docs/JUDGE.md §7). ML inference
// and a vision LLM are both slow; JUDGE_TIMEOUT overrides it.
const DefaultJudgeTimeout = 10 * time.Second

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

	if err := loadJudgeBudget(&cfg); err != nil {
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

	switch cfg.AssistMode {
	case AssistModeFake:
	case AssistModeAnthropic:
		// The real impl needs a server-side key; guessing/defaulting it is worse than
		// failing fast (a keyless anthropic mode would 500 on every request, out of
		// band — a boot error is the honest signal). Mirrors the RENDER_CLI fail-fast.
		if cfg.AnthropicAPIKey == "" {
			return Config{}, fmt.Errorf("config: ANTHROPIC_API_KEY is required when ASSIST_MODE=anthropic")
		}
	default:
		return Config{}, fmt.Errorf("config: ASSIST_MODE must be %q or %q", AssistModeFake, AssistModeAnthropic)
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

// loadJudgeBudget reads the two daily judge-call caps. Zero or negative is a boot
// error rather than a synonym for "off": a 0 here would read as "no budget" to an
// operator and behave as "refuse every duel" to the code, and the mode where the
// budget genuinely does not apply is JUDGE_MODE=fake, which needs no sentinel
// because there is no external quota to protect. Same fail-fast shape, and the same
// reasoning, as JUDGE_CONCURRENCY.
//
// A per-user cap ABOVE the global one is allowed on purpose: that is how you say
// "effectively no per-player limit, the global budget is the only ceiling".
func loadJudgeBudget(cfg *Config) error {
	global, err := getenvInt("JUDGE_DAILY_BUDGET", DefaultJudgeDailyBudget)
	if err != nil {
		return err
	}
	if global < 1 {
		return fmt.Errorf("config: JUDGE_DAILY_BUDGET must be >= 1, got %d (0 would refuse every duel; there is no unlimited setting — set a large number if you mean effectively none)", global)
	}
	perUser, err := getenvInt("JUDGE_DAILY_PER_USER", DefaultJudgeDailyPerUser)
	if err != nil {
		return err
	}
	if perUser < 1 {
		return fmt.Errorf("config: JUDGE_DAILY_PER_USER must be >= 1, got %d (0 would refuse every duel; there is no unlimited setting — set a large number if you mean effectively none)", perUser)
	}
	cfg.JudgeDailyBudget = global
	cfg.JudgeDailyPerUser = perUser
	return nil
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
