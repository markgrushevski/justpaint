package assist

import (
	"context"
	"errors"
)

// AnthropicAssist is the real LLM-backed impl — SCAFFOLDED ONLY in Phase A
// (docs/DESIGN-ASSIST-PHASE-A.md §1: fake-first; the Anthropic Go SDK is
// deliberately NOT yet a go.mod dependency). It is selected by ASSIST_MODE=anthropic,
// which config.Load fail-fasts unless ANTHROPIC_API_KEY is set — so this
// constructor always receives a non-empty key. The real call
// (client.Messages.New with structured outputs + a validate→retry loop —
// docs/ASSIST.md §3.2/§3.3) lands in a later phase.
type AnthropicAssist struct {
	apiKey string
	model  string
}

// NewAnthropicAssist stores the server-side credentials + model for the (not yet
// built) real impl. apiKey never leaves the server (docs/ASSIST.md §1).
func NewAnthropicAssist(apiKey, model string) *AnthropicAssist {
	return &AnthropicAssist{apiKey: apiKey, model: model}
}

var _ Assist = (*AnthropicAssist)(nil)

// GenerateOps is not implemented in Phase A — the Anthropic SDK call is scaffolded
// but not wired (fake-first). Selecting ASSIST_MODE=anthropic boots fine (config
// validates the key) but every request fails clearly until the impl lands.
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
// Flip it to true in the same change that wires the SDK. That is the whole cost
// of keeping the ledger honest, and it sits here, in the file that has to change
// anyway, rather than in a switch three packages away.
func (a *AnthropicAssist) CallsProvider() bool { return false }
