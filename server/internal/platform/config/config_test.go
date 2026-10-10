package config

import (
	"maps"
	"strings"
	"testing"
	"time"
)

// requireBaseEnv sets the mandatory env so Load() reaches the assist
// mode-switch. ENV=dev also relaxes the JWT length check.
func requireBaseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("ENV", EnvDev)
	t.Setenv("JWT_SECRET", "dev-secret")
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/justpaint?sslmode=disable")
}

// TestLoad_AssistMode pins the ASSIST_MODE switch: fake is the default,
// gemini demands GEMINI_API_KEY at boot, and an unknown mode is rejected.
func TestLoad_AssistMode(t *testing.T) {
	t.Run("default is fake", func(t *testing.T) {
		requireBaseEnv(t)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.AssistMode != AssistModeFake {
			t.Errorf("AssistMode = %q, want %q", cfg.AssistMode, AssistModeFake)
		}
	})

	t.Run("gemini without a key is a boot error", func(t *testing.T) {
		requireBaseEnv(t)
		t.Setenv("ASSIST_MODE", "gemini")
		t.Setenv("GEMINI_API_KEY", "")
		_, err := Load()
		if err == nil {
			t.Fatal("expected a boot error when ASSIST_MODE=gemini without GEMINI_API_KEY")
		}
		if !strings.Contains(err.Error(), "GEMINI_API_KEY") {
			t.Errorf("error %q does not mention GEMINI_API_KEY", err)
		}
	})

	t.Run("gemini with a key loads, whatever the judge is doing", func(t *testing.T) {
		requireBaseEnv(t)
		t.Setenv("ASSIST_MODE", "gemini")
		t.Setenv("GEMINI_API_KEY", "test-key")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.AssistMode != AssistModeGemini {
			t.Errorf("AssistMode = %q, want %q", cfg.AssistMode, AssistModeGemini)
		}
		if cfg.JudgeMode != JudgeModeFake {
			t.Errorf("JudgeMode = %q, want the default %q — assist must not drag the judge along", cfg.JudgeMode, JudgeModeFake)
		}
	})

	t.Run("unknown mode is a boot error", func(t *testing.T) {
		requireBaseEnv(t)
		t.Setenv("ASSIST_MODE", "bogus")
		if _, err := Load(); err == nil {
			t.Fatal("expected a boot error for an unknown ASSIST_MODE")
		}
	})

	t.Run("the assist deadline is its own, and far longer than the judge's", func(t *testing.T) {
		requireBaseEnv(t)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.AssistTimeout != DefaultAssistTimeout {
			t.Errorf("AssistTimeout = %s, want %s", cfg.AssistTimeout, DefaultAssistTimeout)
		}
		if cfg.AssistTimeout <= cfg.JudgeTimeout {
			t.Errorf("AssistTimeout (%s) must exceed JudgeTimeout (%s): composing a picture is not scoring one",
				cfg.AssistTimeout, cfg.JudgeTimeout)
		}
	})

	t.Run("the assist deadline is overridable and must be positive", func(t *testing.T) {
		requireBaseEnv(t)
		t.Setenv("ASSIST_TIMEOUT", "90s")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.AssistTimeout != 90*time.Second {
			t.Errorf("AssistTimeout = %s, want 90s", cfg.AssistTimeout)
		}
		if cfg.JudgeTimeout != DefaultJudgeTimeout {
			t.Errorf("ASSIST_TIMEOUT moved JudgeTimeout to %s; the two knobs are separate", cfg.JudgeTimeout)
		}

		for _, bad := range []string{"0s", "-5s", "soon"} {
			t.Setenv("ASSIST_TIMEOUT", bad)
			if _, err := Load(); err == nil {
				t.Errorf("expected a boot error for ASSIST_TIMEOUT=%q", bad)
			}
		}
	})
}

// TestLoad_AIModelPerKind pins the per-kind model map, which shares
// AI_DAILY_PER_USER's grammar and its unvalidated kind names.
func TestLoad_AIModelPerKind(t *testing.T) {
	t.Run("unset means every kind takes GEMINI_MODEL", func(t *testing.T) {
		requireBaseEnv(t)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if len(cfg.AIModelPerKind) != 0 {
			t.Errorf("AIModelPerKind = %v, want empty", cfg.AIModelPerKind)
		}
		if cfg.GeminiModel != DefaultGeminiModel {
			t.Errorf("GeminiModel = %q, want %q", cfg.GeminiModel, DefaultGeminiModel)
		}
	})

	t.Run("entries are parsed, trimmed, and left verbatim as model ids", func(t *testing.T) {
		requireBaseEnv(t)
		t.Setenv("AI_MODEL_PER_KIND", "duel=gemini-3.6-pro, guess=gemini-3.6-flash-lite")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		want := map[string]string{"duel": "gemini-3.6-pro", "guess": "gemini-3.6-flash-lite"}
		if !maps.Equal(cfg.AIModelPerKind, want) {
			t.Errorf("AIModelPerKind = %v, want %v", cfg.AIModelPerKind, want)
		}
	})

	// Same malformed entries AI_DAILY_PER_USER rejects, asserted against the
	// model map so a future edit cannot quietly loosen one of them.
	rejected := []struct {
		name  string
		value string
	}{
		{"an entry with no value", "duel"},
		{"an entry naming no kind", "=gemini-3.6-pro"},
		{"an entry with an empty model", "duel="},
		{"an entry whose model is only spaces", "duel=   "},
	}
	for _, tc := range rejected {
		t.Run(tc.name+" is a boot error", func(t *testing.T) {
			requireBaseEnv(t)
			t.Setenv("AI_MODEL_PER_KIND", tc.value)
			_, err := Load()
			if err == nil {
				t.Fatalf("expected a boot error for AI_MODEL_PER_KIND=%q", tc.value)
			}
			if !strings.Contains(err.Error(), "AI_MODEL_PER_KIND") {
				t.Errorf("error %q does not name the knob AI_MODEL_PER_KIND", err)
			}
		})
	}
}

// TestLoad_Env pins ENV as mandatory and closed-valued: it decides
// CookieSecure, the JWT-length floor and the WS origin default.
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

// TestLoad_DBMaxConns pins the pool ceiling: a default, an override, and a
// boot error rather than a silent fallback for an unusable value.
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

// TestLoad_TrustProxy pins the proxy-trust fork: an unparseable value is a
// boot error, never a silent false.
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
// combinations that would silently defeat them: a heartbeat no more frequent
// than the idle timeout, and a per-IP cap above the global one.
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

// TestLoad_DatabaseURLShape pins the boot-time shape check on DATABASE_URL,
// catching the common mistake of pasting a provider's project URL instead of
// the actual connection string.
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

// TestLoad_JudgeMode pins the JUDGE_MODE switch: a non-fake mode missing its
// dependency (base URL or API key) is a boot error, not a first-duel failure.
func TestLoad_JudgeMode(t *testing.T) {
	t.Run("default is fake with the pinned timeout", func(t *testing.T) {
		requireBaseEnv(t)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.JudgeMode != JudgeModeFake {
			t.Errorf("JudgeMode = %q, want %q", cfg.JudgeMode, JudgeModeFake)
		}
		if cfg.JudgeTimeout != DefaultJudgeTimeout {
			t.Errorf("JudgeTimeout = %s, want %s", cfg.JudgeTimeout, DefaultJudgeTimeout)
		}
	})

	t.Run("http without a base url is a boot error", func(t *testing.T) {
		requireBaseEnv(t)
		t.Setenv("JUDGE_MODE", "http")
		t.Setenv("JUDGE_BASE_URL", "")
		_, err := Load()
		if err == nil {
			t.Fatal("expected a boot error when JUDGE_MODE=http without JUDGE_BASE_URL")
		}
		if !strings.Contains(err.Error(), "JUDGE_BASE_URL") {
			t.Errorf("error %q does not name the missing knob", err)
		}
	})

	t.Run("gemini without a key is a boot error", func(t *testing.T) {
		requireBaseEnv(t)
		t.Setenv("JUDGE_MODE", "gemini")
		t.Setenv("GEMINI_API_KEY", "")
		_, err := Load()
		if err == nil {
			t.Fatal("expected a boot error when JUDGE_MODE=gemini without GEMINI_API_KEY")
		}
		if !strings.Contains(err.Error(), "GEMINI_API_KEY") {
			t.Errorf("error %q does not name the missing knob", err)
		}
	})

	t.Run("gemini with a key loads, model and base url overridable", func(t *testing.T) {
		requireBaseEnv(t)
		t.Setenv("JUDGE_MODE", "gemini")
		t.Setenv("GEMINI_API_KEY", "test-key")
		t.Setenv("GEMINI_MODEL", "gemini-3-flash-lite")
		// Trailing slash trimmed so callers can join paths without doubling it.
		t.Setenv("GEMINI_BASE_URL", "http://127.0.0.1:1/v1beta/")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.GeminiModel != "gemini-3-flash-lite" {
			t.Errorf("GeminiModel = %q, want the override", cfg.GeminiModel)
		}
		if cfg.GeminiBaseURL != "http://127.0.0.1:1/v1beta" {
			t.Errorf("GeminiBaseURL = %q, want the trailing slash trimmed", cfg.GeminiBaseURL)
		}
	})

	t.Run("http trims the base url trailing slash", func(t *testing.T) {
		requireBaseEnv(t)
		t.Setenv("JUDGE_MODE", "http")
		t.Setenv("JUDGE_BASE_URL", "https://judge.example.com/")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.JudgeBaseURL != "https://judge.example.com" {
			t.Errorf("JudgeBaseURL = %q, want the trailing slash trimmed", cfg.JudgeBaseURL)
		}
	})

	t.Run("a non-positive timeout is a boot error", func(t *testing.T) {
		requireBaseEnv(t)
		t.Setenv("JUDGE_TIMEOUT", "0s")
		if _, err := Load(); err == nil {
			t.Fatal("expected a boot error for JUDGE_TIMEOUT=0s")
		}
	})

	t.Run("unknown mode is a boot error", func(t *testing.T) {
		requireBaseEnv(t)
		t.Setenv("JUDGE_MODE", "bogus")
		if _, err := Load(); err == nil {
			t.Fatal("expected a boot error for an unknown JUDGE_MODE")
		}
	})
}

// TestLoad_AIBudget pins the daily AI-call ceilings: a mistyped or zeroed
// knob is a boot error, and there is no "unlimited" sentinel.
func TestLoad_AIBudget(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		requireBaseEnv(t)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.AIDailyGlobal != DefaultAIDailyGlobal {
			t.Errorf("AIDailyGlobal = %d, want %d", cfg.AIDailyGlobal, DefaultAIDailyGlobal)
		}
		// Empty, not populated — per-kind ceilings live in internal/aibudget.
		if len(cfg.AIDailyPerUser) != 0 {
			t.Errorf("AIDailyPerUser = %v, want empty", cfg.AIDailyPerUser)
		}
	})

	t.Run("both halves are read", func(t *testing.T) {
		requireBaseEnv(t)
		t.Setenv("AI_DAILY_GLOBAL", "77")
		t.Setenv("AI_DAILY_PER_USER", "duel=5, guess=2")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.AIDailyGlobal != 77 {
			t.Errorf("AIDailyGlobal = %d, want 77", cfg.AIDailyGlobal)
		}
		want := map[string]int{"duel": 5, "guess": 2}
		if !maps.Equal(cfg.AIDailyPerUser, want) {
			t.Errorf("AIDailyPerUser = %v, want %v", cfg.AIDailyPerUser, want)
		}
	})

	t.Run("an allowance above the global ceiling still loads", func(t *testing.T) {
		requireBaseEnv(t)
		t.Setenv("AI_DAILY_GLOBAL", "10")
		t.Setenv("AI_DAILY_PER_USER", "duel=50")
		// Whether that is an error depends on which kinds have a provider, which
		// config cannot know; aibudget.Policies decides it at boot.
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.AIDailyGlobal != 10 || cfg.AIDailyPerUser["duel"] != 50 {
			t.Errorf("AIDailyGlobal, AIDailyPerUser = %d, %v; want 10 and duel=50", cfg.AIDailyGlobal, cfg.AIDailyPerUser)
		}
	})

	// A renamed name is refused whatever it holds and whether or not the new name
	// is set too, so no environment runs on the defaults by accident.
	renamed := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{
			name:    "JUDGE_DAILY_BUDGET",
			env:     map[string]string{"JUDGE_DAILY_BUDGET": "10"},
			wantErr: "config: JUDGE_DAILY_BUDGET was renamed; set AI_DAILY_GLOBAL",
		},
		{
			name:    "JUDGE_DAILY_PER_USER",
			env:     map[string]string{"JUDGE_DAILY_PER_USER": "20"},
			wantErr: "config: JUDGE_DAILY_PER_USER was renamed; set AI_DAILY_PER_USER=duel=N,practice=N",
		},
		{
			name:    "JUDGE_DAILY_BUDGET with a value that would not have parsed",
			env:     map[string]string{"JUDGE_DAILY_BUDGET": "lots"},
			wantErr: "JUDGE_DAILY_BUDGET was renamed",
		},
		{
			name:    "JUDGE_DAILY_PER_USER beside the current name",
			env:     map[string]string{"JUDGE_DAILY_PER_USER": "0", "AI_DAILY_PER_USER": "duel=3"},
			wantErr: "JUDGE_DAILY_PER_USER was renamed",
		},
		{
			name:    "JUDGE_DAILY_BUDGET beside the same number under the current name",
			env:     map[string]string{"JUDGE_DAILY_BUDGET": "42", "AI_DAILY_GLOBAL": "42"},
			wantErr: "JUDGE_DAILY_BUDGET was renamed",
		},
	}
	for _, tc := range renamed {
		t.Run(tc.name+" is a boot error", func(t *testing.T) {
			requireBaseEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want one containing %q", err, tc.wantErr)
			}
		})
	}

	t.Run("a renamed name left empty is not set", func(t *testing.T) {
		requireBaseEnv(t)
		t.Setenv("JUDGE_DAILY_BUDGET", " ")
		t.Setenv("JUDGE_DAILY_PER_USER", "")
		if _, err := Load(); err != nil {
			t.Fatalf("Load: %v", err)
		}
	})

	rejected := []struct {
		name  string
		key   string
		value string
	}{
		{"a zero global budget", "AI_DAILY_GLOBAL", "0"},
		{"a negative global budget", "AI_DAILY_GLOBAL", "-1"},
		{"a non-integer global budget", "AI_DAILY_GLOBAL", "lots"},
		{"a per-kind entry with no number", "AI_DAILY_PER_USER", "duel"},
		{"a per-kind entry naming no kind", "AI_DAILY_PER_USER", "=20"},
		{"a per-kind entry with a non-integer", "AI_DAILY_PER_USER", "duel=plenty"},
		{"a zero per-kind entry", "AI_DAILY_PER_USER", "duel=0"},
		{"a negative per-kind entry", "AI_DAILY_PER_USER", "guess=-2"},
	}
	for _, tc := range rejected {
		t.Run(tc.name+" is a boot error", func(t *testing.T) {
			requireBaseEnv(t)
			t.Setenv(tc.key, tc.value)
			_, err := Load()
			if err == nil {
				t.Fatalf("expected a boot error for %s=%q", tc.key, tc.value)
			}
			if !strings.Contains(err.Error(), tc.key) {
				t.Errorf("error %q does not name the knob %s", err, tc.key)
			}
		})
	}
}

// TestLoad_SeamModes pins PRACTICE_MODE and GUESS_MODE: they follow JUDGE_MODE,
// default to off under the external HTTP judge, and can be set on their own.
func TestLoad_SeamModes(t *testing.T) {
	tests := []struct {
		name         string
		env          map[string]string
		wantPractice string
		wantGuess    string
		wantErr      string
	}{
		{"follow the fake judge", nil, SeamModeFake, SeamModeFake, ""},
		{"follow the gemini judge",
			map[string]string{"JUDGE_MODE": "gemini", "GEMINI_API_KEY": "k"}, SeamModeGemini, SeamModeGemini, ""},
		{"off under the http judge",
			map[string]string{"JUDGE_MODE": "http", "JUDGE_BASE_URL": "https://judge.example"}, SeamModeOff, SeamModeOff, ""},
		{"an http judge with gemini practice and guess",
			map[string]string{"JUDGE_MODE": "http", "JUDGE_BASE_URL": "https://judge.example", "GEMINI_API_KEY": "k",
				"PRACTICE_MODE": "gemini", "GUESS_MODE": "Gemini"}, SeamModeGemini, SeamModeGemini, ""},
		{"turned off beside a gemini judge",
			map[string]string{"JUDGE_MODE": "gemini", "GEMINI_API_KEY": "k", "GUESS_MODE": "off"}, SeamModeGemini, SeamModeOff, ""},
		{"gemini without a key", map[string]string{"PRACTICE_MODE": "gemini"}, "", "", "GEMINI_API_KEY"},
		{"an unknown mode", map[string]string{"GUESS_MODE": "http"}, "", "", "GUESS_MODE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireBaseEnv(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			cfg, err := Load()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want one naming %s", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.PracticeMode != tt.wantPractice || cfg.GuessMode != tt.wantGuess {
				t.Errorf("practice, guess = %q, %q; want %q, %q", cfg.PracticeMode, cfg.GuessMode, tt.wantPractice, tt.wantGuess)
			}
		})
	}
}
