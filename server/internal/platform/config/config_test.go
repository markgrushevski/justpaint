package config

import (
	"strings"
	"testing"
	"time"
)

// requireBaseEnv sets the always-mandatory env so Load() reaches the assist
// mode-switch. ENV is mandatory and set to dev here, which relaxes the JWT length
// check, so a short secret is fine.
func requireBaseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("ENV", EnvDev)
	t.Setenv("JWT_SECRET", "dev-secret")
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/justpaint?sslmode=disable")
}

// TestLoad_AssistMode pins the ASSIST_MODE mode-switch, mirroring the RENDER_MODE
// fail-fast: the anthropic mode demands ANTHROPIC_API_KEY at boot, an unknown mode
// is rejected, and fake is the default.
func TestLoad_AssistMode(t *testing.T) {
	t.Run("default is fake with the default model", func(t *testing.T) {
		requireBaseEnv(t)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.AssistMode != AssistModeFake {
			t.Errorf("AssistMode = %q, want %q", cfg.AssistMode, AssistModeFake)
		}
		if cfg.AssistModel != DefaultAssistModel {
			t.Errorf("AssistModel = %q, want %q", cfg.AssistModel, DefaultAssistModel)
		}
	})

	t.Run("anthropic without a key is a boot error", func(t *testing.T) {
		requireBaseEnv(t)
		t.Setenv("ASSIST_MODE", "anthropic")
		t.Setenv("ANTHROPIC_API_KEY", "")
		_, err := Load()
		if err == nil {
			t.Fatal("expected a boot error when ASSIST_MODE=anthropic without ANTHROPIC_API_KEY")
		}
		if !strings.Contains(err.Error(), "ANTHROPIC_API_KEY") {
			t.Errorf("error %q does not mention ANTHROPIC_API_KEY", err)
		}
	})

	t.Run("anthropic with a key loads", func(t *testing.T) {
		requireBaseEnv(t)
		t.Setenv("ASSIST_MODE", "anthropic")
		t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")
		t.Setenv("ASSIST_MODEL", "claude-haiku-4-5")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.AssistMode != AssistModeAnthropic {
			t.Errorf("AssistMode = %q, want %q", cfg.AssistMode, AssistModeAnthropic)
		}
		if cfg.AssistModel != "claude-haiku-4-5" {
			t.Errorf("AssistModel = %q, want overridden claude-haiku-4-5", cfg.AssistModel)
		}
	})

	t.Run("unknown mode is a boot error", func(t *testing.T) {
		requireBaseEnv(t)
		t.Setenv("ASSIST_MODE", "bogus")
		if _, err := Load(); err == nil {
			t.Fatal("expected a boot error for an unknown ASSIST_MODE")
		}
	})
}

// TestLoad_Env pins ENV as mandatory and closed-valued. It decides CookieSecure,
// the JWT-length floor and the WS origin default, and it used to default to
// "dev" — so a deploy that forgot ENV=prod booted happily with a non-Secure
// session cookie. A missing or misspelled ENV must be a boot error instead.
func TestLoad_Env(t *testing.T) {
	base := func(t *testing.T) {
		t.Helper()
		t.Setenv("JWT_SECRET", strings.Repeat("x", 32))
		t.Setenv("DATABASE_URL", "postgres://localhost:5432/justpaint?sslmode=disable")
	}

	tests := []struct {
		name       string
		env        string
		wantErr    bool
		wantSecure bool
	}{
		{name: "missing is a boot error", env: "", wantErr: true},
		{name: "typo is a boot error", env: "production", wantErr: true},
		{name: "dev relaxes the cookie", env: EnvDev, wantSecure: false},
		{name: "prod secures the cookie", env: EnvProd, wantSecure: true},
		{name: "case is normalized", env: "PROD", wantSecure: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base(t)
			t.Setenv("ENV", tt.env)
			cfg, err := Load()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Load() with ENV=%q: expected a boot error", tt.env)
				}
				if !strings.Contains(err.Error(), "ENV") {
					t.Errorf("error %q does not name ENV", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.CookieSecure != tt.wantSecure {
				t.Errorf("CookieSecure = %v, want %v for ENV=%q", cfg.CookieSecure, tt.wantSecure, tt.env)
			}
		})
	}
}

// TestLoad_DBMaxConns pins the pool ceiling: an explicit default (pgx's own
// max(4, NumCPU) is sized to the app host, not to the database's connection
// budget), an override, and a boot error rather than a silent fallback when the
// value is unusable.
func TestLoad_DBMaxConns(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    int32
		wantErr bool
	}{
		{name: "unset uses the default", raw: "", want: DefaultDBMaxConns},
		{name: "override applies", raw: "3", want: 3},
		{name: "zero is rejected", raw: "0", wantErr: true},
		{name: "negative is rejected", raw: "-1", wantErr: true},
		{name: "garbage is rejected", raw: "many", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireBaseEnv(t)
			t.Setenv("DB_MAX_CONNS", tt.raw)
			cfg, err := Load()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Load() with DB_MAX_CONNS=%q: expected a boot error", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.DBMaxConns != tt.want {
				t.Errorf("DBMaxConns = %d, want %d", cfg.DBMaxConns, tt.want)
			}
		})
	}
}

// TestLoad_TrustProxy pins the proxy-trust fork. It gates whether
// X-Forwarded-For is believed, which decides what rate limiting is keyed on, so
// an unparseable value must fail the boot rather than quietly resolve to false —
// a security control that silently does nothing is worse than an obvious error.
func TestLoad_TrustProxy(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    bool
		wantErr bool
	}{
		{name: "unset is untrusted", raw: "", want: false},
		{name: "true", raw: "true", want: true},
		{name: "1 is true", raw: "1", want: true},
		{name: "false stays false", raw: "false", want: false},
		{name: "yes is not a boolean", raw: "yes", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireBaseEnv(t)
			t.Setenv("TRUST_PROXY", tt.raw)
			cfg, err := Load()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Load() with TRUST_PROXY=%q: expected a boot error", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.TrustProxy != tt.want {
				t.Errorf("TrustProxy = %v, want %v", cfg.TrustProxy, tt.want)
			}
		})
	}
}

// TestLoad_WSLimits pins the WebSocket hardening knobs, including the two
// combinations that would silently defeat what they claim to configure: a
// heartbeat no more frequent than the idle timeout (a healthy but quiet socket
// gets evicted between probes — mid-round, for a player who is simply drawing),
// and a per-IP cap above the global one (it can never bind).
func TestLoad_WSLimits(t *testing.T) {
	t.Run("defaults are sane and ordered", func(t *testing.T) {
		requireBaseEnv(t)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.WSHeartbeatInterval >= cfg.WSReadIdleTimeout {
			t.Errorf("heartbeat %s must be shorter than idle timeout %s", cfg.WSHeartbeatInterval, cfg.WSReadIdleTimeout)
		}
		if cfg.WSMaxConnsPerIP > cfg.WSMaxConns {
			t.Errorf("per-IP cap %d exceeds global cap %d", cfg.WSMaxConnsPerIP, cfg.WSMaxConns)
		}
	})

	tests := []struct {
		name string
		env  map[string]string
	}{
		{name: "heartbeat equal to the timeout", env: map[string]string{"WS_HEARTBEAT_INTERVAL": "60s", "WS_READ_IDLE_TIMEOUT": "60s"}},
		{name: "heartbeat longer than the timeout", env: map[string]string{"WS_HEARTBEAT_INTERVAL": "90s", "WS_READ_IDLE_TIMEOUT": "60s"}},
		{name: "unlimited global cap", env: map[string]string{"WS_MAX_CONNS": "0"}},
		{name: "unlimited per-IP cap", env: map[string]string{"WS_MAX_CONNS_PER_IP": "0"}},
		{name: "per-IP cap above the global cap", env: map[string]string{"WS_MAX_CONNS": "10", "WS_MAX_CONNS_PER_IP": "50"}},
		{name: "malformed duration", env: map[string]string{"WS_READ_IDLE_TIMEOUT": "60"}},
	}

	for _, tt := range tests {
		t.Run(tt.name+" is a boot error", func(t *testing.T) {
			requireBaseEnv(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			if _, err := Load(); err == nil {
				t.Fatalf("expected a boot error for %v", tt.env)
			}
		})
	}

	t.Run("overrides apply", func(t *testing.T) {
		requireBaseEnv(t)
		t.Setenv("WS_READ_IDLE_TIMEOUT", "2m")
		t.Setenv("WS_HEARTBEAT_INTERVAL", "30s")
		t.Setenv("WS_MAX_CONNS", "50")
		t.Setenv("WS_MAX_CONNS_PER_IP", "5")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.WSReadIdleTimeout != 2*time.Minute || cfg.WSHeartbeatInterval != 30*time.Second {
			t.Errorf("durations = %s / %s, want 2m / 30s", cfg.WSReadIdleTimeout, cfg.WSHeartbeatInterval)
		}
		if cfg.WSMaxConns != 50 || cfg.WSMaxConnsPerIP != 5 {
			t.Errorf("caps = %d / %d, want 50 / 5", cfg.WSMaxConns, cfg.WSMaxConnsPerIP)
		}
	})
}

// TestLoad_DatabaseURLShape pins the boot-time shape check on DATABASE_URL. A
// managed provider's dashboard shows a project URL right next to the connection
// string, and pasting the wrong one used to surface as a driver parse error at
// runtime ("failed to parse as keyword/value") — unreadable as a config mistake.
func TestLoad_DatabaseURLShape(t *testing.T) {
	tests := []struct {
		name    string
		dsn     string
		wantErr string // substring the message must carry
	}{
		{name: "uri form", dsn: "postgres://u:p@localhost:5432/db"},
		{name: "postgresql scheme", dsn: "postgresql://u:p@db.example.com:5432/postgres?sslmode=require"},
		{name: "keyword/value form", dsn: "host=localhost user=justpaint dbname=justpaint"},
		{name: "supabase project url", dsn: "https://fesidgriglgmszjksorp.supabase.co", wantErr: "not a database connection string"},
		{name: "plain http url", dsn: "http://example.com", wantErr: "not a database connection string"},
		{name: "nonsense", dsn: "justpaint", wantErr: "not a Postgres connection string"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireBaseEnv(t)
			t.Setenv("DATABASE_URL", tt.dsn)
			_, err := Load()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Load with %q: %v", tt.dsn, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Load with %q: expected a boot error", tt.dsn)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err, tt.wantErr)
			}
		})
	}

	t.Run("the password is not echoed in full", func(t *testing.T) {
		requireBaseEnv(t)
		t.Setenv("DATABASE_URL", "https://user:sup3r-secret-password@example.com/db")
		_, err := Load()
		if err == nil {
			t.Fatal("expected a boot error")
		}
		if strings.Contains(err.Error(), "sup3r-secret-password") {
			t.Errorf("error leaks the credential: %q", err)
		}
	})
}
