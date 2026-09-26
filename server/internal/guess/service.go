// Package guess is "ask the AI what I drew" — the button on /draw that renders
// the free-draw canvas server-side and asks a model for one label. One call,
// nothing stored: see docs/JUDGE.md §8.3 and docs/DECISIONS.md 2026-09-20.
package guess

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/markgrushevski/justpaint/server/internal/aibudget"
	"github.com/markgrushevski/justpaint/server/internal/document"
	"github.com/markgrushevski/justpaint/server/internal/judge"
	"github.com/markgrushevski/justpaint/server/internal/render"
)

// RunBudget bounds one guess end to end (render plus the model call, retries
// included). The work happens inside the request and the HTTP server's
// WriteTimeout is 30s (cmd/server/main.go), so 25s leaves margin for a clean 500
// instead of a response cut off mid-write.
const RunBudget = 25 * time.Second

// maxRasterBytes caps the rendered PNG before it is base64'd and sent to a third
// party under our API key. It guards the seam, not today's renderer output, which
// stays far below it; see docs/JUDGE.md §8.3 for the full sizing rationale.
const maxRasterBytes = 4 << 20 // 4 MiB

// Sentinel errors the handler maps onto HTTP responses.
var (
	// ErrNotConfigured means no guesser is wired (JUDGE_MODE=http has none), → 500.
	// Deliberately not a fallback to judge.FakeGuesser: see docs/JUDGE.md §8.3 for
	// why a fabricated label is a more convincing lie than a fabricated score.
	ErrNotConfigured = errors.New("guess: no guesser configured")
	// ErrRasterTooLarge means the render exceeded maxRasterBytes, → 500. A
	// server-side fault, not the client's — the document already passed
	// validation.
	ErrRasterTooLarge = errors.New("guess: rendered raster is too large to send")
)

// budget and spend are aibudget's func types bound to aibudget.KindGuess at the
// composition root; nil means unbudgeted, which every test and fake-configured
// deployment gets.

// Service runs the guess loop: render, then ask. It holds no database handle —
// a guess is never written down.
type Service struct {
	renderer render.Renderer
	// guesser is judge.Guesser — ours, not the external judge's frozen Judge. Nil
	// means the feature is unconfigured; see ErrNotConfigured.
	guesser judge.Guesser
	budget  aibudget.Check
	spend   aibudget.Spend
	logger  *slog.Logger
}

// NewService builds the guess service. A nil guesser is a legitimate, deliberate
// state — the endpoint then refuses honestly instead of inventing an answer — so
// it is not an error here; main.go logs it at boot.
func NewService(renderer render.Renderer, guesser judge.Guesser, budget aibudget.Check, spend aibudget.Spend, logger *slog.Logger) *Service {
	return &Service{renderer: renderer, guesser: guesser, budget: budget, spend: spend, logger: logger}
}

// GuessView is one answer: what the model thinks the drawing is, how sure it is,
// and 0-2 runner-ups.
type GuessView struct {
	Label        string
	Confidence   float64
	Alternatives []string
}

// Guess renders the caller's drawing and asks the model what it is. doc is
// already validated by the handler (document.ParseAndValidate, not
// game.ValidateSubmission — see the handler for why).
//
// Order matters: guesser configured, then budget checked, then render, then the
// ledger spent immediately before the call (it re-checks the cap at write time,
// tighter than the earlier check), then the call and its answer re-validated.
func (s *Service) Guess(ctx context.Context, userID string, doc document.Document) (GuessView, error) {
	if s.guesser == nil {
		return GuessView{}, ErrNotConfigured
	}
	if s.budget != nil {
		if err := s.budget(ctx, userID); err != nil {
			return GuessView{}, err // an aibudget refusal; the handler maps it
		}
	}

	// Bounds the render and the model call only; the ledger write below still runs
	// on the caller's own ctx, not workCtx.
	workCtx, cancel := context.WithTimeout(ctx, RunBudget)
	defer cancel()

	// Rendered here, from the validated document, never taken from the client: the
	// standing trust boundary (docs/GAME.md §6) plus the risk specific to this
	// route — a client PNG would turn it into an open pipe to a third party under
	// our API key. Full reasoning: docs/JUDGE.md §8.3.
	img, err := s.renderer.Render(workCtx, doc)
	if err != nil {
		return GuessView{}, fmt.Errorf("guess: render: %w", err)
	}
	if len(img) > maxRasterBytes {
		return GuessView{}, fmt.Errorf("%w: %d bytes (max %d)", ErrRasterTooLarge, len(img), maxRasterBytes)
	}

	// Billed here, between the render and the call: a call that fails still spent
	// the provider's quota, and the render is ours and free. This write also
	// re-checks the cap at the instant of writing, tighter than the check above
	// (docs/GAME.md §4.3, same rule as internal/practice).
	if s.spend != nil {
		if err := s.spend(ctx, userID); err != nil {
			return GuessView{}, fmt.Errorf("guess: bill call: %w", err)
		}
	}

	startedAt := time.Now()
	g, err := s.guesser.Guess(workCtx, img)
	if err != nil {
		return GuessView{}, fmt.Errorf("guess: ask the guesser: %w", err)
	}
	// Re-checked here even though every impl validates its own answer: the seam is
	// swappable, and the client renders whatever comes back.
	if err := g.Validate(); err != nil {
		return GuessView{}, fmt.Errorf("guess: guesser result: %w", err)
	}

	// Confirms the feature works and would surface latency creeping toward
	// RunBudget. Never the label: that is model-authored text about a private
	// drawing and does not belong in a log.
	s.logger.Info("drawing guessed",
		"confidence", g.Confidence,
		"alternatives", len(g.Alternatives),
		"guess_ms", time.Since(startedAt).Milliseconds(),
		"render_bytes", len(img),
	)

	return GuessView{Label: g.Label, Confidence: g.Confidence, Alternatives: g.Alternatives}, nil
}
