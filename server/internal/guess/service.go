// Package guess is "ask the AI what I drew" — the button on /draw that sends the
// free-draw canvas to a model and gets one short answer back. One route, one
// call, nothing stored.
//
// # Why nothing is persisted
//
// There is no guesses table and no row anywhere, and that is a decision rather
// than an omission. The drawing on /draw may never be saved at all — the point of
// free draw is a canvas you can scribble on and close — so there is frequently
// nothing for a guess to hang off, and a row pointing at a drawing that was never
// written is a record about nothing. Beyond that, a guess IS a moment and not a
// record: it is funny once, on the screen of the person who drew the picture, and
// it scores nothing, ranks nothing and gates nothing, so there is no later
// question that reading it back would answer. The one durable fact worth keeping
// is that a provider call was made, and the ledger already keeps exactly that
// (internal/aibudget, migration 00007) — which is half the reason the ledger
// exists.
//
// # Module shape
//
// Practice's smaller sibling: a service over the render and guesser seams plus an
// HTTP handler, wired in main.go. Unlike practice it holds no *db.Queries at all,
// because it writes nothing — the only database row this feature produces is the
// ledger's, written through the budget port it is handed.
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

// RunBudget bounds one guess end to end: the authoritative render plus the model
// call, retries included.
//
// Same hard ceiling as practice's RunBudget and for the same reason — the work
// happens INSIDE the request and the HTTP server's WriteTimeout is 30s
// (cmd/server/main.go), where an overrun is not an error a player can read but a
// response cut off mid-write. 25s leaves the margin, and a clean 500 beats a dead
// socket.
//
// It is the same number as practice's even though the player's patience is
// shorter here — this is a novelty button on the editor, not the point of the
// session — and the thing that settles it is the allowance rather than the
// patience. aibudget.DefaultPerUser gives guess TWO calls a day, so a call cut
// short at 20s that would have answered at 22s costs a player half of their day's
// guesses. Where the quota is that small, the full margin is the kinder trade.
//
// The arithmetic worth knowing, as in practice: with the default JUDGE_TIMEOUT of
// 10s and the JUDGE.md §7 policy of 3 attempts, a guesser that keeps timing out
// needs ~30s and will be cut short here.
const RunBudget = 25 * time.Second

// maxRasterBytes caps the rendered PNG before it is base64'd and sent to a third
// party under our API key.
//
// It should never fire today, and that is not a reason to drop it. Both wired
// renderers pin the judge frame (render.JudgeFrameSize, 1024²) whatever the
// document's own canvas says, and an honest 1024² line drawing is tens of
// kilobytes. This guards the SEAM, not the current impl: Renderer is chosen by
// config, free draw allows canvases all the way to the document format's 8192²
// (document.MaxCanvasDimension) where a duel is pinned to one square size, and
// the day a renderer honours the document's own dimensions an 8-megapixel raster
// becomes several megabytes — which base64 then inflates by a further third on
// its way out. The failure that guard prevents is silent and billable, which is
// the worst combination to discover from an invoice.
//
// 4 MiB is the number: roughly fifty times any raster we actually produce, above
// even a pathological full-frame 1024² of noise, and about 5.3 MiB once encoded,
// which stays comfortably inside the inline-request limits of the API on the
// other end. A raster over it is a renderer fault or a frame size nobody costed,
// and the right answer to both is a clean 500 with a legible reason rather than a
// multi-megabyte upload nobody decided to make.
const maxRasterBytes = 4 << 20 // 4 MiB

// Sentinel errors the handler maps onto HTTP responses.
var (
	// ErrNotConfigured: no guesser is wired, which today means JUDGE_MODE=http —
	// the collaborator's service implements the two-image Judge contract and has no
	// endpoint that looks at one drawing and names it (docs/JUDGE.md §2 is frozen).
	// → 500.
	//
	// It is deliberately NOT a silent fallback to judge.FakeGuesser. The fake reads
	// ink coverage and has never looked at a picture; presenting its label as the
	// AI's answer is a lie the player cannot detect, and it is a more convincing lie
	// than a fake score would be — a wrong guess is funny, so it reads as the
	// feature working rather than as the feature being off.
	ErrNotConfigured = errors.New("guess: no guesser configured")
	// ErrRasterTooLarge: the render came back bigger than maxRasterBytes, so we
	// refuse to send it. A server-side fault, not the client's — the document
	// already passed validation — so → 500.
	ErrRasterTooLarge = errors.New("guess: rendered raster is too large to send")
)

// The two budget ports are aibudget's own func types, bound to aibudget.KindGuess
// at the composition root: aibudget.Check asks whether this player may spend an
// AI call right now, aibudget.Spend records the one they just spent (and refuses,
// with the same error Check returns, if the ledger finds them at their cap by the
// time it writes).
//
// They used to be re-declared here as local BudgetCheck/BudgetSpend types, on the
// stated grounds that this package then never imported aibudget. That was not
// true — the handler has always imported it for WriteRefusal — so the copies
// bought a second name for one contract and a conversion at the wiring site, and
// nothing else. internal/game holds aibudget's types directly for the same reason.
//
// Nil means unbudgeted, which is what every test and every fake-configured
// deployment gets.

// Service runs the guess loop: render, then ask. It holds no database handle,
// because a guess is not written down (see the package comment).
type Service struct {
	renderer render.Renderer
	// guesser is the seam (judge.Guesser — ours, not the collaborator's frozen
	// Judge). Nil means the feature is not configured; see ErrNotConfigured.
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

// Guess renders the caller's drawing and asks the model what it is. The document
// is already validated by the handler (document.ParseAndValidate — see the note
// there about which validator this is NOT); doc is the parsed form the renderer
// needs.
//
// Order is deliberate:
//
//  1. the seam, before anything else — an unconfigured feature must not consult a
//     budget for a call it will never make;
//  2. the budget, before any expensive work — the point of a ceiling is to refuse
//     before the call, not after;
//  3. render, then the size guard, both of which are ours and cost no quota;
//  4. the ledger, immediately before the call — and it may refuse too, with the
//     same error (2) returns, because it writes under the cap as it stands at
//     that instant rather than as (2) read it a render ago;
//  5. the call, and its answer re-checked against the contract.
func (s *Service) Guess(ctx context.Context, userID string, doc document.Document) (GuessView, error) {
	if s.guesser == nil {
		return GuessView{}, ErrNotConfigured
	}
	if s.budget != nil {
		if err := s.budget(ctx, userID); err != nil {
			return GuessView{}, err // an aibudget refusal; the handler maps it
		}
	}

	// Bound the expensive half only, so the ledger write below runs on the caller's
	// own context and cannot be lost to a nearly-spent work budget.
	workCtx, cancel := context.WithTimeout(ctx, RunBudget)
	defer cancel()

	// The raster is rendered HERE, from the validated vector document, and is never
	// taken from the client. Two reasons, and the second is the one specific to this
	// endpoint:
	//
	//  1. the standing trust boundary — anything judged, scored or sent onward is
	//     derived server-side from the document, never from a picture the client
	//     supplied (docs/GAME.md §6, DOCUMENT-FORMAT §10);
	//  2. accepting a client PNG here would turn this route into an open pipe to a
	//     third party under OUR API key. Any authenticated caller could POST a photo,
	//     a screenshot, someone else's picture — any bytes at all — and have us pay to
	//     upload it to Google and hand back what the model said about it. Rendering it
	//     ourselves guarantees that the only images which ever leave the building are
	//     our own renderer's output of a document that passed our own validator.
	img, err := s.renderer.Render(workCtx, doc)
	if err != nil {
		return GuessView{}, fmt.Errorf("guess: render: %w", err)
	}
	if len(img) > maxRasterBytes {
		return GuessView{}, fmt.Errorf("%w: %d bytes (max %d)", ErrRasterTooLarge, len(img), maxRasterBytes)
	}

	// Billed immediately before the call and never after it: a call that fails still
	// spent the provider's quota, and a failure that costs nothing is the failure
	// the budget cannot see, precisely when a broken impl is draining it (the rule
	// internal/practice states in full).
	//
	// It sits AFTER the render for the other half of the same rule: the render is
	// our own subprocess and spends nobody's quota, so a renderer that fell over
	// must not cost a player one of the two guesses they get for the day. Between
	// the render and the call is the narrowest correct window, and internal/practice
	// bills at exactly the same point for exactly the same reason. Everything above
	// this line is free to re-run; nothing below it is.
	//
	// It is also the second and tighter of the two per-user gates: the check above
	// read a count a whole render ago, while this write refuses on the count as it
	// stands at the instant of writing — which matters most here, where the cap is
	// two. Its refusal is the same *KindSpentError the check returns, so the wrap
	// below still reaches the handler's 429 branch and needs no case of its own.
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

	// The line that says the feature worked, mirroring "practice run scored":
	// without it a live guesser is unobservable — you cannot tell a sane answer from
	// a degenerate one (every guess a 0.1 shrug), or notice latency creeping toward
	// RunBudget. The confidence and the latency, never the label: that is
	// model-authored text about a private drawing, it carries whatever the picture
	// provoked, and logs are not the place for either.
	s.logger.Info("drawing guessed",
		"confidence", g.Confidence,
		"alternatives", len(g.Alternatives),
		"guess_ms", time.Since(startedAt).Milliseconds(),
		"render_bytes", len(img),
	)

	return GuessView{Label: g.Label, Confidence: g.Confidence, Alternatives: g.Alternatives}, nil
}
