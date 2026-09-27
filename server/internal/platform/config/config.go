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

	// TrustProxy says whether X-Forwarded-For / X-Request-Id may be trusted.
	// Left false, every client behind a proxy collapses into the proxy's own
	// IP, breaking per-IP rate limits and abuse logs. Set true only behind
	// exactly one trusted hop — otherwise a client can forge the header and
	// evade per-IP limits entirely.
	TrustProxy bool

	// AutoMigrate applies the embedded goose migrations at boot. Default true
	// because the deploy target has no shell to run migrations separately. Set
	// false where a human or pipeline owns the schema instead.
	AutoMigrate bool

	// StaticDir is the built SPA (apps/web/dist) the server also serves,
	// keeping API and frontend same-origin — required since the session
	// cookie and the WS upgrade are same-origin with no CORS headers sent.
	// Empty in dev, where Vite serves the SPA and proxies /api here.
	StaticDir string

	// DBMaxConns bounds the pgx pool. pgx's own default scales with host CPU
	// count, not with the database's max_connections, which a few app
	// instances can exhaust on a small managed Postgres.
	DBMaxConns int32

	// RenderMode selects the judge-raster renderer: "stub" (default; in-process,
	// zero-dep, loop-proving) or "node" (the authoritative Node Konva worker).
	RenderMode string
	// RenderCLI is the bundled Node worker entry (packages/render/dist/render.mjs).
	// Required when RenderMode == "node".
	RenderCLI string
	// RenderNodeBin is the node executable (default "node").
	RenderNodeBin string
	// JudgeConcurrency bounds concurrent judging passes (authoritative render +
	// judge call), avoiding a fork bomb under RenderMode "node" (each pass
	// forks OS processes for a 1024x1024 render). It also sizes the render
	// semaphore that bounds inline synchronous renders on /api/guess and
	// /api/practice.
	JudgeConcurrency int

	// AIDailyGlobal and AIDailyPerUser cap AI calls over a rolling 24h window —
	// a resource the per-IP rate limiter cannot see, since a provider's quota
	// is metered per day, not per request rate. AIDailyGlobal applies per
	// provider and model; AIDailyPerUser maps a call kind ("duel", "practice",
	// …) to its own per-player ceiling, and a kind absent from the map keeps
	// its code default. Kind names are not validated here — internal/aibudget
	// owns the valid set, and config stays free of domain imports — so the
	// composition root rejects an unknown kind at boot. No value may be below
	// 1; there is no "unlimited" sentinel. Full rule: docs/GAME.md §4.3.
	AIDailyGlobal  int
	AIDailyPerUser map[string]int

	// AIModelPerKind overrides, per AI-call kind, the model that kind runs on;
	// GeminiModel is the default for a kind with no override. Kinds differ in
	// how much accuracy matters — the duel judge's number feeds Elo, the
	// guesser's mistakes cost nothing — so one model for all of them is either
	// an overpay or an underserve. Same kind=value comma-list shape and the
	// same unvalidated-name handling as AIDailyPerUser.
	AIModelPerKind map[string]string

	// JudgeMode selects the judge impl (docs/JUDGE.md): "fake" (default; the
	// zero-dependency ink-coverage stand-in that never reads the prompt), "http"
	// (the external ML judge over the §6 contract) or "gemini" (a vision LLM
	// scoring both rasters in one call — the real verdict while the ML is built).
	JudgeMode string
	// JudgeBaseURL is the external ML judge's service root, required for JudgeMode
	// "http" (§7).
	JudgeBaseURL string
	// JudgeTimeout bounds one judging call, retries excluded (§7 pins 10s). The
	// critic and the guesser share it — all three ask a vision model one short
	// question and get a handful of scalars back. Assist does not; see AssistTimeout.
	JudgeTimeout time.Duration
	// GeminiAPIKey is the server-side key for JudgeMode "gemini". Never reaches
	// the client. Required when that mode is selected.
	GeminiAPIKey string
	// GeminiModel is the default model id for every Gemini-backed kind (judge,
	// critic, guesser, assist); AIModelPerKind overrides it per kind.
	// Configurable because Google's free-tier model names and quotas move
	// faster than releases here.
	GeminiModel string
	// GeminiBaseURL is the API root, overridable for the same reason and so a
	// test can point at an httptest server.
	GeminiBaseURL string

	// AssistMode selects the AI-assist impl (docs/ASSIST.md): "fake" (default;
	// deterministic canned ops that ignore the prompt, zero API dependency) or
	// "gemini" (the real impl — a prompt becomes shapes). Mirrors the
	// RenderMode switch.
	//
	// Assist has no model knob of its own: it takes GeminiModel, overridden
	// for this kind alone by AIModelPerKind["assist"], because that is also
	// where its quota is counted (per provider and model).
	AssistMode string

	// AssistTimeout bounds one assist call, retries excluded. Separate from
	// JudgeTimeout because the work is not comparable: a verdict is four
	// scalars, while composing a picture is a list of shapes from a model
	// that reasons first. Folding it into JUDGE_TIMEOUT would leave no
	// headroom for the slow case, or hand the duel a retry envelope that no
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

	// WSAllowedOrigins are extra Origin hosts authorized for the WebSocket
	// handshake (coder/websocket path.Match patterns against the Origin
	// header host, e.g. "app.example.com" or "localhost:*"). The request Host
	// is always authorized, so a true same-origin deployment needs no entry;
	// this exists only for a split-host deployment, where the dev Vite proxy's
	// changeOrigin makes the backend see Host=:8080 while the browser Origin
	// is :7777. Never "*" — that would let a cross-site page open a socket
	// riding the auto-attached cookie. docs/API.md §9.1.
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

// DefaultAIDailyGlobal is the per-provider-and-model daily ceiling, in AI
// calls per rolling 24h window. Per-kind player ceilings live in
// internal/aibudget beside the kinds they bound, so adding a kind never means
// editing config.
//
// Google's free tier grants 20 requests/day per project per model
// (aibudget.Provider.WithModel keys the ledger the same way), so 15 leaves
// margin without being the number that actually binds — a free key's real
// ceiling is Google's own quota, surfaced as ErrQuotaExhausted. The ledger
// also counts one row per judging pass, and a pass may retry, so 15 rows can
// be more than 15 real requests. Raise this once the key is not a free one.
// Full rule: docs/GAME.md §4.3.
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

// Judge and assist modes.
const (
	JudgeModeFake   = "fake"
	JudgeModeHTTP   = "http"
	JudgeModeGemini = "gemini"

	AssistModeFake   = "fake"
	AssistModeGemini = "gemini"
)

// DefaultJudgeTimeout bounds one judging call (docs/JUDGE.md §7). ML inference
// and a vision LLM are both slow; JUDGE_TIMEOUT overrides it.
const DefaultJudgeTimeout = 10 * time.Second

// DefaultAssistTimeout bounds one assist call. Six times the judge's, because
// a verdict is four scalars while a drawing is a list of shapes composed by a
// model that thinks first, and it runs measurably slower. Deliberately loose:
// the cost of loose is a wedged call holding a request goroutine for a
// minute; the cost of tight is a feature that fails whenever Google is slow,
// which is undiagnosable from outside. ASSIST_TIMEOUT overrides it.
const DefaultAssistTimeout = 60 * time.Second

// DefaultGeminiModel is pinned deliberately, not the floating
// "gemini-flash-latest" alias: a judge decides ratings, so a model that
// changes under us silently is worse than one that stops answering and names
// its replacement. Override GEMINI_MODEL rather than editing this.
const DefaultGeminiModel = "gemini-3.6-flash"

// DefaultGeminiBaseURL is the public Generative Language API root.
const DefaultGeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// Load reads configuration from the environment and fails fast on any missing
// required value.
//
// ENV, JWT_SECRET and DATABASE_URL are mandatory: refusing to start beats
// falling back to an empty signing secret (anyone could forge a session), a
// nil database, or a silently insecure cookie. ENV has no default for the
// same reason — CookieSecure derives from it, so a missing ENV must be a
// boot error rather than a cookie that quietly becomes non-Secure.
func Load() (Config, error) {
	env := strings.ToLower(strings.TrimSpace(os.Getenv("ENV")))
	cfg := Config{
		Addr:        getenv("ADDR", ":8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		Env:         env,
		// Secure cookies are dropped by browsers over plain http://localhost,
		// so relax the flag in dev; require it everywhere else.
		CookieSecure:  env != EnvDev,
		StaticDir:     strings.TrimSpace(os.Getenv("STATIC_DIR")),
		RenderMode:    strings.ToLower(getenv("RENDER_MODE", RenderModeStub)),
		RenderCLI:     os.Getenv("RENDER_CLI"),
		RenderNodeBin: getenv("RENDER_NODE_BIN", "node"),
		JudgeMode:     strings.ToLower(getenv("JUDGE_MODE", JudgeModeFake)),
		JudgeBaseURL:  strings.TrimRight(os.Getenv("JUDGE_BASE_URL"), "/"),
		GeminiAPIKey:  os.Getenv("GEMINI_API_KEY"),
		GeminiModel:   getenv("GEMINI_MODEL", DefaultGeminiModel),
		GeminiBaseURL: strings.TrimRight(getenv("GEMINI_BASE_URL", DefaultGeminiBaseURL), "/"),
		AssistMode:    strings.ToLower(getenv("ASSIST_MODE", AssistModeFake)),
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

	// Same fail-fast shape as RENDER_CLI and the assist switch below: a judge mode
	// whose dependency is missing would fail out of band on the first duel, long
	// after the deploy that broke it. A boot error names the cause.
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
		// Same key, quota and API as the Gemini judge, so the same fail-fast
		// applies. Independent of JUDGE_MODE: a deployment can run a real
		// assist while the duel still uses the fake judge.
		if cfg.GeminiAPIKey == "" {
			return Config{}, fmt.Errorf("config: GEMINI_API_KEY is required when ASSIST_MODE=%s", AssistModeGemini)
		}
	default:
		return Config{}, fmt.Errorf("config: ASSIST_MODE must be %q or %q", AssistModeFake, AssistModeGemini)
	}

	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// validateDatabaseURL rejects a DATABASE_URL that is not a Postgres DSN, with
// a message that says what to paste instead: a managed provider's dashboard
// often shows a project URL (https://<ref>.supabase.co) next to the actual
// connection string, and pasting the former fails deep inside the driver with
// a message that reads like an app bug rather than a wrong env var.
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

// loadAIBudget reads the daily AI-call ceilings: one global number applied
// per provider and model, plus an optional per-kind map of player ceilings.
// Zero or negative is a boot error, not a synonym for "off" — the mode where
// the budget does not apply is a fake impl, which needs no sentinel because
// there is no external quota to protect. A per-user cap above the global one
// is allowed: that is how an operator says "no real per-player limit".
//
// JUDGE_DAILY_BUDGET and JUDGE_DAILY_PER_USER are pre-per-kind aliases kept
// for already-deployed environments; server/.env.example documents what they
// alias and which kinds they seed.
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

	// Values are model ids used verbatim; the kind names are checked at the
	// composition root, where the valid set lives.
	perKindModel, err := parseKindList("AI_MODEL_PER_KIND", "duel=gemini-3.6-pro,guess=gemini-3.6-flash-lite")
	if err != nil {
		return err
	}

	cfg.AIDailyGlobal = global
	cfg.AIDailyPerUser = perUser
	cfg.AIModelPerKind = perKindModel
	return nil
}

// parseKindList reads a "kind=value" comma-separated env var into a map, the
// shape both per-kind knobs use (AI_DAILY_PER_USER, AI_MODEL_PER_KIND), so the
// two cannot diverge on whitespace, duplicate keys or empty entries. It only
// produces strings — what a value means is the caller's business. Duplicate
// keys resolve last-wins; example is shown in the syntax error.
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

// getenvIntAliased reads an integer that answers to two names, one current
// and one kept alive for already-deployed environments. Both set to the same
// value is fine; different values is a boot error, since silently preferring
// one would put a bound at a number nobody chose. It also returns which name
// supplied the value, so a later range check can blame the right one.
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

	// "The same value" compares the parsed numbers, not the spelling — 100 and
	// 0100 are the same bound.
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
