package assist

import (
	"context"
	"errors"
)

// AnthropicAssist is the Phase A scaffold, and it is now SUPERSEDED: the real
// impl of this seam is GeminiAssist (gemini.go), because the service already
// holds one AI key, one quota and one HTTP client for the judge, the critic and
// the guesser, and a second vendor would have bought a second of each for the
// same job. The Anthropic SDK was never added to go.mod and is not going to be.
//
// It is kept only so an environment that already sets ASSIST_MODE=anthropic still
// boots, saying at boot exactly what it is. Nothing here calls anybody
// (CallsProvider below), so it spends no quota and writes no ledger row; every
// request fails clearly. Delete it once no deployment names the mode.
type AnthropicAssist struct {
	apiKey string
	model  string
}

// NewAnthropicAssist stores the server-side credentials + model that ASSIST_MODEL
// and ANTHROPIC_API_KEY still carry. apiKey never leaves the server
// (docs/ASSIST.md §1).
func NewAnthropicAssist(apiKey, model string) *AnthropicAssist {
	return &AnthropicAssist{apiKey: apiKey, model: model}
}

var _ Assist = (*AnthropicAssist)(nil)

// GenerateOps is not implemented and is not going to be. Selecting
// ASSIST_MODE=anthropic boots fine (config validates the key) but every request
// fails clearly; the mode that works is ASSIST_MODE=gemini.
func (a *AnthropicAssist) GenerateOps(_ context.Context, _ Request) (Result, error) {
	return Result{}, errors.New("assist: anthropic impl not built")
}

var _ ProviderCaller = (*AnthropicAssist)(nil)

// CallsProvider reports false while GenerateOps above is a scaffold: it returns an
// error without any network I/O, so no quota is spent and nothing should be
// billed. The composition root reads this instead of re-deriving a provider from
// ASSIST_MODE, which is how every assist request under ASSIST_MODE=anthropic came
// to write ledger rows for a call that never happened and then answer 500.
//
// It stays false forever here, because the impl this file was a placeholder for is
// never landing. GeminiAssist is the one that answers true, in its own file, which
// is the point of asking the impl rather than a switch three packages away.
func (a *AnthropicAssist) CallsProvider() bool { return false }
