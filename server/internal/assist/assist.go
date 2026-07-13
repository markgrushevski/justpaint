// Package assist is the AI-assist seam (docs/ASSIST.md, docs/DESIGN-ASSIST-PHASE-A.md):
// a natural-language prompt goes to an LLM, which emits a batch of validated
// document operations (the packages/document Op contract). The handler depends on
// the Assist interface, never a concrete impl, so the deterministic FakeAssist
// (the default in dev/CI/tests) and the real AnthropicAssist swap by config with
// no handler change — exactly like the render (internal/render) and judge
// (internal/judge) seams.
//
// Assist is STATELESS: no DB, no migration, no sqlc. Every request is
// self-contained — prompt + minimal doc summary in, validated ops out.
package assist

import (
	"context"
	"errors"

	"github.com/markgrushevski/justpaint/server/internal/document"
)

// Request is one assist call: the natural-language prompt, the minimal document
// summary (canvas + layer inventory — docs/ASSIST.md §4, never the full
// document), and an optional layer to bias generation onto. camelCase JSON, like
// the rest of the live API (docs/DESIGN-ASSIST-PHASE-A.md §1).
type Request struct {
	Prompt        string              `json:"prompt"`
	DocSummary    document.DocSummary `json:"docSummary"`
	TargetLayerID *string             `json:"targetLayerId"`
}

// Result is the impl's output: a validated op batch plus an optional human-facing
// note surfaced in the UI. The handler re-validates Ops with
// document.ValidateOpBatch before the client ever sees them (defense — the client
// applies ops as commands and must never receive an unvalidated batch).
type Result struct {
	Ops  []document.Op `json:"ops"`
	Note string        `json:"note"`
}

// ErrInvalidBatch marks retry-exhaustion: the impl could not produce a batch that
// passes validation within its retry budget. The handler maps it to
// 400 validation_failed — NEVER 422, which docs/API.md:68 reserves unused in v1
// (docs/DESIGN-ASSIST-PHASE-A.md §1 resolution 1).
var ErrInvalidBatch = errors.New("assist: model output failed validation after retries")

// Assist generates a validated op batch from a prompt. The one thing the handler
// and tests depend on.
type Assist interface {
	GenerateOps(ctx context.Context, req Request) (Result, error)
}
