// Package assist is the AI-assist seam (docs/ASSIST.md): a natural-language
// prompt goes to an LLM, which emits a batch of validated document operations
// (the Op contract). The handler depends on the Assist interface, never a
// concrete impl, so FakeAssist (deterministic, the default in dev/CI/tests)
// and the real gemini.Assist swap by config with no handler change, exactly
// like the render and judge seams.
//
// Assist is stateless: no DB, no migration, no sqlc. Every request is
// self-contained — prompt and the current document in, validated ops out.
package assist

import (
	"context"
	"errors"

	"github.com/markgrushevski/justpaint/server/internal/document"
)

// Request is one assist call: the prompt, the canvas as it is now and an optional
// layer to bias generation onto (docs/ASSIST.md §4). The handler builds it from a
// validated document; Image, the server-rendered PNG of that document, is set only
// for an impl that reads images.
type Request struct {
	Prompt        string
	Document      document.Document
	DocSummary    document.DocSummary
	Image         []byte
	TargetLayerID *string
}

// Result is the impl's output: a validated op batch plus an optional human-facing
// note surfaced in the UI. The handler re-validates Ops with
// document.ValidateOpBatch before the client ever sees them (defense — the client
// applies ops as commands and must never receive an unvalidated batch).
type Result struct {
	Ops  []document.Op `json:"ops"`
	Note string        `json:"note,omitempty"`
}

// ErrInvalidBatch marks retry-exhaustion: the impl could not produce a batch
// that passes validation within its retry budget. The handler maps it to
// 400 validation_failed, never 422, which docs/API.md §3 reserves unused in
// v1 (docs/ASSIST.md §3.3).
var ErrInvalidBatch = errors.New("assist: model output failed validation after retries")

// Assist generates a validated op batch from a prompt. The one thing the handler
// and tests depend on.
type Assist interface {
	GenerateOps(ctx context.Context, req Request) (Result, error)
}

// ImageReader is implemented by an Assist impl that looks at the rendered canvas.
type ImageReader interface {
	ReadsImage() bool
}

// ReadsImage reports whether the handler should render the canvas for this impl.
// An impl that does not say so is sent no image: rendering costs a worker process.
func ReadsImage(a Assist) bool {
	ir, ok := a.(ImageReader)
	return ok && ir.ReadsImage()
}
