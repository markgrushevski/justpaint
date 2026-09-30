package main

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/markgrushevski/justpaint/server/internal/aibudget"
	"github.com/markgrushevski/justpaint/server/internal/assist"
	"github.com/markgrushevski/justpaint/server/internal/db"
	"github.com/markgrushevski/justpaint/server/internal/game"
	"github.com/markgrushevski/justpaint/server/internal/gemini"
	"github.com/markgrushevski/justpaint/server/internal/judge"
	"github.com/markgrushevski/justpaint/server/internal/platform/config"
)

// aiImpls are the AI seams this boot built, and the provider each one bills. Each
// constructor names its provider in the same breath it builds the impl, rather
// than a second switch inferring it from the mode; an empty provider means the
// impl calls nobody.
type aiImpls struct {
	judge     judge.Judge
	critic    judge.Critic  // nil under PRACTICE_MODE=off
	guesser   judge.Guesser // nil under GUESS_MODE=off
	assist    assist.Assist
	providers map[aibudget.Kind]aibudget.Provider
}

// newAI builds each seam from its mode. config.Load already proved each mode's
// dependency exists, so only the per-kind model table can fail here.
func newAI(cfg config.Config, logger *slog.Logger) (aiImpls, error) {
	// GEMINI_MODEL is the default and AI_MODEL_PER_KIND swaps in a different model
	// per kind (docs/DECISIONS.md). An unknown kind name fails here at boot, not at
	// first use.
	models, err := aibudget.Models(cfg.GeminiModel, cfg.AIModelPerKind)
	if err != nil {
		return aiImpls{}, err
	}
	logAIModels(logger, models, cfg.GeminiModel)

	ai := aiImpls{providers: make(map[aibudget.Kind]aibudget.Provider, 4)}
	ai.judge, ai.providers[aibudget.KindDuel] = newJudge(cfg, models[aibudget.KindDuel], logger)
	ai.critic, ai.providers[aibudget.KindPractice] = newCritic(cfg, models[aibudget.KindPractice], logger)
	ai.guesser, ai.providers[aibudget.KindGuess] = newGuesser(cfg, models[aibudget.KindGuess], logger)
	ai.assist, ai.providers[aibudget.KindAssist] = newAssist(cfg, models[aibudget.KindAssist], logger)

	// A real model on the stub renderer sees ink-coverage blocks, not the drawing
	// (docs/NOTES.md) — confident, meaningless answers, since the pairing works.
	// Not a boot error: still legitimate for exercising the wiring in dev.
	realModel := cfg.JudgeMode != config.JudgeModeFake ||
		cfg.PracticeMode == config.SeamModeGemini || cfg.GuessMode == config.SeamModeGemini ||
		cfg.AssistMode == config.AssistModeGemini
	if realModel && cfg.RenderMode != config.RenderModeNode {
		logger.Warn("render: a real model is reading STUB rasters, which are ink-coverage blocks and not the drawings — set RENDER_MODE=node",
			"judge_mode", cfg.JudgeMode, "practice_mode", cfg.PracticeMode, "guess_mode", cfg.GuessMode,
			"assist_mode", cfg.AssistMode, "render_mode", cfg.RenderMode)
	}
	return ai, nil
}

// newJudge picks the duel judge (docs/JUDGE.md). The fake never reads the prompt,
// so it proves the loop and nothing more; the real ones are the external ML judge
// (§6) and a vision model scoring both rasters.
func newJudge(cfg config.Config, model string, logger *slog.Logger) (judge.Judge, aibudget.Provider) {
	// The judge retries up to 3 times inside game.JudgePassBudget (docs/JUDGE.md
	// §7). A JUDGE_TIMEOUT loose enough to overflow that budget won't fail
	// outright — it silently truncates the last attempt, showing up only as an
	// occasional lost duel.
	if envelope := 3 * cfg.JudgeTimeout; envelope >= game.JudgePassBudget {
		logger.Warn("judge: JUDGE_TIMEOUT leaves no room for its own retries inside the judging pass",
			"timeout", cfg.JudgeTimeout, "retry_envelope", envelope, "pass_budget", game.JudgePassBudget)
	}

	switch cfg.JudgeMode {
	case config.JudgeModeHTTP:
		logger.Info("judge: http (external ML judge)", "base_url", cfg.JudgeBaseURL, "timeout", cfg.JudgeTimeout)
		// Bare, no model attached: one service behind however many models. Still
		// worth a ceiling — a bug here would hammer it and we can't see what's left
		// (docs/GAME.md §4.3).
		return judge.NewHTTPJudge(cfg.JudgeBaseURL, cfg.JudgeTimeout), aibudget.ProviderCollaborator
	case config.JudgeModeGemini:
		// No key in the log — it stays server-side.
		logger.Info("judge: gemini vision", "model", model, "timeout", cfg.JudgeTimeout)
		// Keyed to the model, not just the provider: the free tier meters per model,
		// so two kinds on two models need two separate pools (docs/GAME.md §4.3).
		return gemini.NewJudge(cfg.GeminiAPIKey, model, cfg.GeminiBaseURL, cfg.JudgeTimeout),
			aibudget.ProviderGoogle.WithModel(model)
	default:
		logger.Info("judge: fake (ink coverage — it never reads the prompt; set JUDGE_MODE for a real verdict)")
		return judge.NewFakeJudge(), ""
	}
}

// newCritic picks the practice critic (docs/GAME.md §10). Off leaves it nil, and
// practice refuses honestly rather than faking a score (docs/JUDGE.md §8.2).
func newCritic(cfg config.Config, model string, logger *slog.Logger) (judge.Critic, aibudget.Provider) {
	switch cfg.PracticeMode {
	case config.SeamModeGemini:
		logger.Info("practice: gemini critic (scores one drawing against its prompt)", "model", model)
		return gemini.NewCritic(cfg.GeminiAPIKey, model, cfg.GeminiBaseURL, cfg.JudgeTimeout),
			aibudget.ProviderGoogle.WithModel(model)
	case config.SeamModeOff:
		logger.Warn("practice: DISABLED — PRACTICE_MODE=off (the default under JUDGE_MODE=http, which has no critique endpoint); /api/practice answers 500 until PRACTICE_MODE is fake or gemini")
		return nil, ""
	default:
		logger.Info("practice: fake critic (ink coverage — it never reads the prompt; set PRACTICE_MODE=gemini for a real critique)")
		return judge.NewFakeCritic(), ""
	}
}

// newGuesser picks the "what did I draw?" guesser (docs/JUDGE.md §8.3). Off
// leaves it nil, and /api/guess refuses honestly.
func newGuesser(cfg config.Config, model string, logger *slog.Logger) (judge.Guesser, aibudget.Provider) {
	switch cfg.GuessMode {
	case config.SeamModeGemini:
		logger.Info("guess: gemini vision (names what one drawing depicts)", "model", model)
		return gemini.NewGuesser(cfg.GeminiAPIKey, model, cfg.GeminiBaseURL, cfg.JudgeTimeout),
			aibudget.ProviderGoogle.WithModel(model)
	case config.SeamModeOff:
		logger.Warn("guess: DISABLED — GUESS_MODE=off (the default under JUDGE_MODE=http, which has no endpoint for it); /api/guess answers 500 until GUESS_MODE is fake or gemini")
		return nil, ""
	default:
		logger.Info("guess: fake (a canned answer that never looks at the drawing; set GUESS_MODE=gemini for a real one)")
		return judge.NewFakeGuesser(), ""
	}
}

// newAssist picks the assist impl (docs/ASSIST.md §3). Its provider is adopted only
// if the impl says it calls anybody (assist.CallsProvider), not inferred from the
// mode: Gemini answers true and gets a real daily ceiling; the fake answers false
// and stays unbudgeted.
func newAssist(cfg config.Config, model string, logger *slog.Logger) (assist.Assist, aibudget.Provider) {
	var impl assist.Assist
	var vendor aibudget.Provider
	switch cfg.AssistMode {
	case config.AssistModeGemini:
		// Its own timeout, not the judge's: composing a picture takes tens of
		// seconds on a thinking model, where a verdict takes one or two.
		impl = gemini.NewAssist(cfg.GeminiAPIKey, model, cfg.GeminiBaseURL, cfg.AssistTimeout)
		vendor = aibudget.ProviderGoogle.WithModel(model)
		logger.Info("assist: gemini (a prompt really becomes shapes)", "model", model, "timeout", cfg.AssistTimeout)
	default:
		// The same canned house whatever the user typed — fine in dev and CI, a lie
		// in production.
		impl = assist.NewFakeAssist()
		logger.Info("assist: fake (the same canned ops for every prompt; set ASSIST_MODE=gemini for a real one)")
	}
	if assist.CallsProvider(impl) {
		return impl, vendor
	}
	if vendor != "" {
		logger.Warn("assist: this mode names a provider but its impl makes no external call — nothing is billed and every request will fail until the impl lands",
			"assist_mode", cfg.AssistMode)
	}
	return impl, ""
}

// newBudget is the daily AI-call ceiling (internal/aibudget): the per-IP write
// limiter bounds request rate, this bounds the scarce thing behind it — a
// provider's per-day quota.
func newBudget(cfg config.Config, queries *db.Queries, providers map[aibudget.Kind]aibudget.Provider, logger *slog.Logger) (*aibudget.Budget, error) {
	policies, err := aibudget.Policies(providers, cfg.AIDailyPerUser)
	if err != nil {
		return nil, err
	}
	budget := aibudget.New(queries, policies, cfg.AIDailyGlobal, logger)
	logAIBudget(logger, policies, cfg.AIDailyGlobal)
	// A configured allowance for a feature that calls nobody is inert, not wrong —
	// but it looks identical to an enforced one from the outside, and an operator
	// can end up trusting a number that nothing reads.
	for _, kind := range aibudget.InertAllowances(policies, cfg.AIDailyPerUser) {
		logger.Warn("ai budget: AI_DAILY_PER_USER sets an allowance for a kind whose impl calls no provider — nothing reads it",
			"kind", kind, "per_user", policies[kind].PerUser)
	}
	return budget, nil
}

// logAIModels logs which model each AI kind resolved to (docs/DECISIONS.md): an
// override that silently didn't apply would otherwise keep working on the
// default, with only a bill or quality level as the symptom. Kinds are listed
// in a stable order so two boots are diffable.
func logAIModels(logger *slog.Logger, models map[aibudget.Kind]string, defaultModel string) {
	pairs := make([]any, 0, 2*len(models))
	overridden := false
	for _, kind := range aibudget.AllKinds() {
		model := models[kind]
		if model != defaultModel {
			overridden = true
		}
		pairs = append(pairs, string(kind), model)
	}
	if !overridden {
		logger.Info("ai models: every kind on the default (set AI_MODEL_PER_KIND to give one its own)", "model", defaultModel)
		return
	}
	logger.Info("ai models: resolved per kind", append([]any{"default", defaultModel}, pairs...)...)
}

// logAIBudget logs the ceiling at boot, per kind: an unenforced budget and an
// enforced one look identical from the outside until the quota runs out. Kinds
// are listed in a stable order so two boots are diffable.
func logAIBudget(logger *slog.Logger, policies map[aibudget.Kind]aibudget.Policy, global int) {
	enforced := make([]any, 0, 2*len(policies))
	var unbilled []string
	for _, kind := range aibudget.AllKinds() {
		p, ok := policies[kind]
		if !ok {
			continue // a kind with no feature wired yet
		}
		if p.Provider == "" {
			// A fake, an unbuilt impl, or a mode with no impl for this kind — three
			// roads to the same fact: nobody's quota is at stake.
			unbilled = append(unbilled, string(kind))
			continue
		}
		enforced = append(enforced, string(kind), fmt.Sprintf("%d/day via %s", p.PerUser, p.Provider))
	}
	if len(enforced) == 0 {
		logger.Info("ai budget: nothing enforced (no AI impl here calls a provider, so there is no external quota to protect)")
		return
	}
	// "per provider" is really per provider and model where the provider meters
	// that way (aibudget.Provider.WithModel) — each kind's value below spells out
	// which pool it draws from.
	logger.Info("ai budget: per rolling 24h", append([]any{"global_per_provider_pool", global}, enforced...)...)
	if len(unbilled) > 0 {
		logger.Info("ai budget: not enforced — these impls call no provider", "kinds", strings.Join(unbilled, ","))
	}
}
