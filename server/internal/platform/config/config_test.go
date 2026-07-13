package config

import (
	"strings"
	"testing"
)

// requireBaseEnv sets the always-mandatory env so Load() reaches the assist
// mode-switch. ENV defaults to "dev", which relaxes the JWT length check, so a
// short secret is fine here.
func requireBaseEnv(t *testing.T) {
	t.Helper()
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
