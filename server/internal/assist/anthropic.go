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
