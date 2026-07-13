// Package config loads runtime configuration from the environment,
// applying defaults and failing fast when a mandatory value is missing.
package config

import (
	"fmt"
	"os"
	"strings"
)

// Config holds runtime configuration for the server.
type Config struct {
	Addr         string // host:port to listen on (default ":8080")
	DatabaseURL  string // Postgres DSN — required
	JWTSecret    string // HS256 signing secret — required, never defaulted
	Env          string // "dev" | "prod"
	CookieSecure bool   // Secure flag on the session cookie

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
// JWT_SECRET and DATABASE_URL are mandatory: the server refuses to start without
// them rather than falling back to an empty signing secret (a known red flag —
// an empty secret lets anyone forge a session) or a nil database.
func Load() (Config, error) {
	env := strings.ToLower(getenv("ENV", "dev"))
	cfg := Config{
		Addr:        getenv("ADDR", ":8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		Env:         env,
		// Secure cookies are dropped by browsers over plain http://localhost,
		// so relax the flag in dev; require it everywhere else.
		CookieSecure:    env != "dev",
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
	if len(cfg.WSAllowedOrigins) == 0 && env == "dev" {
		cfg.WSAllowedOrigins = []string{"localhost:*", "127.0.0.1:*"}
	}

	var missing []string
	if cfg.JWTSecret == "" {
		missing = append(missing, "JWT_SECRET")
	}
	if cfg.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("config: missing required env: %s", strings.Join(missing, ", "))
	}

	// Outside dev, require a strong HS256 secret: a short/guessable key is
	// brute-forceable offline against any captured token, and a forged token is
	// full account takeover (the JWT subject is trusted as the owner id).
	const minSecretLen = 32
	if cfg.Env != "dev" && len(cfg.JWTSecret) < minSecretLen {
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
