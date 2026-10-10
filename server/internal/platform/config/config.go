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

	// TrustProxy trusts X-Forwarded-For and X-Request-Id. Set it only behind exactly
	// one trusted hop, or clients can forge their IP
	// (docs/NOTES.md "TRUST_PROXY decides what the client IP is").
	TrustProxy bool

	AutoMigrate bool
	StaticDir   string // the built SPA, served same-origin; empty in dev
	DBMaxConns  int32  // pgx's default scales with CPUs, not the DB's max_connections

	RenderMode    string
	RenderCLI     string // required when RenderMode is "node"
	RenderNodeBin string
	// JudgeConcurrency bounds concurrent judging passes and also sizes the renderer's
	// semaphore for the inline renders on /api/guess and /api/practice.
	JudgeConcurrency int

	// AI call ceilings per rolling 24h (docs/GAME.md §4.3). Kind names are checked at
	// the composition root, where internal/aibudget owns the valid set.
	AIDailyGlobal  int
	AIDailyPerUser map[string]int
	AIModelPerKind map[string]string // overrides GeminiModel per kind

	JudgeMode    string
	JudgeBaseURL string
	// PracticeMode and GuessMode pick the critic and the guesser; each defaults to
	// JUDGE_MODE, or to off when the judge is the external HTTP one.
	PracticeMode string
	GuessMode    string
	// JudgeTimeout bounds one attempt of a judge, critic or guesser call.
	JudgeTimeout  time.Duration
	GeminiAPIKey  string
	GeminiModel   string // the default for every Gemini-backed kind
	GeminiBaseURL string

	AssistMode string
	// AssistTimeout is separate from JudgeTimeout: raising the shared knob for assist
	// would break the duel's fit inside game.JudgePassBudget.
	AssistTimeout time.Duration

	// WSReadIdleTimeout must clear the client's 25s ping (WS_PING_MS in PlayView.vue).
	WSReadIdleTimeout   time.Duration
	WSHeartbeatInterval time.Duration
	WSMaxConns          int
	WSMaxConnsPerIP     int

	// WSAllowedOrigins are extra Origin hosts (path.Match patterns) allowed for the
	// WebSocket handshake; the request Host always is. Never "*": a cross-site page
	// could then open a socket riding the cookie (docs/API.md §9.1).
	WSAllowedOrigins []string
}

// Environments. ENV has no default on purpose — see Load.
const (
	EnvDev  = "dev"
	EnvProd = "prod"
)

// DefaultDBMaxConns suits a small managed Postgres, which caps connections low.
const DefaultDBMaxConns = 10

// DefaultJudgeConcurrency is 2: under RENDER_MODE=node each pass forks two
// node-canvas processes on a memory-poor instance.
const DefaultJudgeConcurrency = 2

// DefaultAIDailyGlobal is the daily ceiling per provider and model: under Google's
// free 20 requests per model per day. Raise it for a paid key (docs/GAME.md §4.3).
const DefaultAIDailyGlobal = 15

// WebSocket hardening defaults: the heartbeat probes a quiet socket twice before the
// idle timeout could evict it.
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

// Practice and guess modes. JUDGE_MODE=http has no endpoint for either, so off is
// their default there.
const (
	SeamModeFake   = "fake"
	SeamModeGemini = "gemini"
	SeamModeOff    = "off"
)

// DefaultJudgeTimeout bounds one judge attempt (docs/JUDGE.md §7).
const DefaultJudgeTimeout = 10 * time.Second

// DefaultAssistTimeout is loose on purpose: composing shapes is far slower than a
// verdict, and a tight bound fails whenever the provider is slow.
const DefaultAssistTimeout = 60 * time.Second

// DefaultGeminiModel is a pinned version, not the floating "gemini-flash-latest":
// the judge decides ratings, so a model that stops loudly beats one that changes
// silently.
const DefaultGeminiModel = "gemini-3.6-flash"

// DefaultGeminiBaseURL is the public Generative Language API root.
const DefaultGeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// Load reads configuration from the environment and fails fast on any missing or
// malformed value. ENV, JWT_SECRET and DATABASE_URL have no defaults: an empty secret
// lets anyone forge a session, and CookieSecure derives from ENV.
func Load() (Config, error) {
	env := strings.ToLower(strings.TrimSpace(os.Getenv("ENV")))
	cfg := Config{
		Addr:        getenv("ADDR", ":8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		Env:         env,
		// Browsers drop Secure cookies over plain http://localhost.
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

	// Dev defaults to the Vite proxy's origin, whose changeOrigin splits Host from Origin.
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

	if cfg.Env != EnvDev && cfg.Env != EnvProd {
		return Config{}, fmt.Errorf("config: ENV must be %q or %q, got %q", EnvDev, EnvProd, cfg.Env)
	}

	if err := validateDatabaseURL(cfg.DatabaseURL); err != nil {
		return Config{}, err
	}

	// A short HS256 key can be brute-forced offline from any captured token, and a
	// forged token is full account takeover.
	const minSecretLen = 32
	if cfg.Env != EnvDev && len(cfg.JWTSecret) < minSecretLen {
		return Config{}, fmt.Errorf("config: JWT_SECRET must be at least %d bytes outside dev", minSecretLen)
	}

	switch cfg.RenderMode {
	case RenderModeStub:
	case RenderModeNode:
		// A bad path or a missing Node runtime would only surface as stuck matches, since
		// judging runs out of band, so both are checked at boot.
		if cfg.RenderCLI == "" {
			return Config{}, fmt.Errorf("config: RENDER_CLI is required when RENDER_MODE=node (path to packages/render/dist/render.mjs)")
		}
		if _, err := os.Stat(cfg.RenderCLI); err != nil {
			return Config{}, fmt.Errorf("config: RENDER_CLI not found (%s) — build it with `npm run build -w @justpaint/render`: %w", cfg.RenderCLI, err)
		}
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

	if cfg.PracticeMode, err = seamMode("PRACTICE_MODE", cfg.JudgeMode, cfg.GeminiAPIKey); err != nil {
		return Config{}, err
	}
	if cfg.GuessMode, err = seamMode("GUESS_MODE", cfg.JudgeMode, cfg.GeminiAPIKey); err != nil {
		return Config{}, err
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
		if cfg.GeminiAPIKey == "" {
			return Config{}, fmt.Errorf("config: GEMINI_API_KEY is required when ASSIST_MODE=%s", AssistModeGemini)
		}
	default:
		return Config{}, fmt.Errorf("config: ASSIST_MODE must be %q or %q", AssistModeFake, AssistModeGemini)
	}

	return cfg, nil
}

// seamMode reads PRACTICE_MODE or GUESS_MODE, defaulting to the judge's mode.
func seamMode(name, judgeMode, apiKey string) (string, error) {
	fallback := judgeMode
	if fallback == JudgeModeHTTP {
		fallback = SeamModeOff
	}
	mode := strings.ToLower(getenv(name, fallback))
	switch mode {
	case SeamModeFake, SeamModeOff:
	case SeamModeGemini:
		if apiKey == "" {
			return "", fmt.Errorf("config: GEMINI_API_KEY is required when %s=%s", name, SeamModeGemini)
		}
	default:
		return "", fmt.Errorf("config: %s must be %q, %q or %q", name, SeamModeFake, SeamModeGemini, SeamModeOff)
	}
	return mode, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// validateDatabaseURL rejects a non-Postgres DSN with a useful message: a provider's
// project URL (https://<ref>.supabase.co) otherwise fails deep in the driver.
func validateDatabaseURL(dsn string) error {
	switch {
	case strings.HasPrefix(dsn, "postgres://"), strings.HasPrefix(dsn, "postgresql://"):
		return nil
	case strings.Contains(dsn, "host="):
		return nil
	case strings.HasPrefix(dsn, "http://"), strings.HasPrefix(dsn, "https://"):
		return fmt.Errorf("config: DATABASE_URL is an HTTP URL (%s…), which is a provider's project/API endpoint, not a database connection string — copy the Postgres URI instead (postgresql://user:password@host:5432/dbname)", firstRunes(dsn, 30))
	default:
		return fmt.Errorf("config: DATABASE_URL is not a Postgres connection string — expected postgres://… , postgresql://… or a keyword/value DSN (host=… user=…), got %q", firstRunes(dsn, 30))
	}
}

// firstRunes truncates without splitting a rune, and keeps a DSN's password out of logs.
func firstRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n])
}

// renamedAIBudgetEnv are names loadAIBudget no longer reads. Setting one is a
// boot error: the deployment would otherwise run on the defaults, with the
// ceiling the operator meant silently gone.
var renamedAIBudgetEnv = []struct{ old, replacement string }{
	{"JUDGE_DAILY_BUDGET", "AI_DAILY_GLOBAL"},
	{"JUDGE_DAILY_PER_USER", "AI_DAILY_PER_USER=duel=N,practice=N"},
}

// loadAIBudget reads the daily AI-call ceilings (server/.env.example). A value
// below 1 is a boot error, never "off". Whether a per-player allowance sits
// below the global one is checked against the built impls, in aibudget.Policies.
func loadAIBudget(cfg *Config) error {
	for _, r := range renamedAIBudgetEnv {
		if strings.TrimSpace(os.Getenv(r.old)) != "" {
			return fmt.Errorf("config: %s was renamed; set %s", r.old, r.replacement)
		}
	}

	global, err := getenvInt("AI_DAILY_GLOBAL", DefaultAIDailyGlobal)
	if err != nil {
		return err
	}
	if global < 1 {
		return fmt.Errorf("config: AI_DAILY_GLOBAL must be >= 1, got %d (0 would refuse every AI call; there is no unlimited setting — set a large number if you mean effectively none)", global)
	}

	perUser := map[string]int{}
	rawPerUser, err := parseKindList("AI_DAILY_PER_USER", "duel=3,guess=2")
	if err != nil {
		return err
	}
	for kind, raw := range rawPerUser {
		v, err := strconv.Atoi(raw)
		if err != nil {
			return fmt.Errorf("config: AI_DAILY_PER_USER entry %q must be of the form kind=number, got %q: %w", kind+"="+raw, raw, err)
		}
		if v < 1 {
			return fmt.Errorf("config: AI_DAILY_PER_USER entry %q must be >= 1, got %d (0 would refuse every call of that kind; there is no unlimited setting)", kind+"="+raw, v)
		}
		perUser[kind] = v
	}

	perKindModel, err := parseKindList("AI_MODEL_PER_KIND", "duel=gemini-3.6-pro,guess=gemini-3.6-flash-lite")
	if err != nil {
		return err
	}

	cfg.AIDailyGlobal = global
	cfg.AIDailyPerUser = perUser
	cfg.AIModelPerKind = perKindModel
	return nil
}

// parseKindList reads a "kind=value,…" env var into a map; a duplicate kind is
// last-wins.
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

// loadWSLimits reads the WebSocket knobs and rejects combinations that disable what
// they configure: an unlimited cap, or a heartbeat no shorter than the idle timeout.
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

// getenvDuration reads a Go duration env var, falling back when unset.
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

// getenvBool reads a boolean env var, falling back when unset. A malformed value is a
// boot error: TRUST_PROXY="yes" read as false would silently disable a security switch.
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

// getenvInt reads an integer env var, falling back when unset; a malformed value is
// a boot error.
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

// splitList splits a comma-separated value, trimming and dropping empty entries.
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
