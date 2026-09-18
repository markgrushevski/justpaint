package config

import (
	"strings"
	"testing"
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
