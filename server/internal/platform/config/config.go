// Package config loads runtime configuration from the environment,
// applying defaults and failing fast when a mandatory value is missing.
package config

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Config holds runtime configuration for the server.
type Config struct {
	Addr         string // host:port to listen on (default ":8080")
	DatabaseURL  string // Postgres DSN — required
	JWTSecret    string // HS256 signing secret — required, never defaulted
	Env          string // "dev" | "prod" — required, never defaulted
	CookieSecure bool   // Secure flag on the session cookie

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

// Render modes.
const (
	RenderModeStub = "stub"
	RenderModeNode = "node"
)

// Assist modes.
const (
	AssistModeFake      = "fake"
	AssistModeAnthropic = "anthropic"
)

// DefaultAssistModel is the model id used by the real assist impl unless
// ASSIST_MODEL overrides it (docs/ASSIST.md §3.2).
const DefaultAssistModel = "claude-opus-4-8"

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
		RenderMode:      strings.ToLower(getenv("RENDER_MODE", RenderModeStub)),
		RenderCLI:       os.Getenv("RENDER_CLI"),
		RenderNodeBin:   getenv("RENDER_NODE_BIN", "node"),
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
