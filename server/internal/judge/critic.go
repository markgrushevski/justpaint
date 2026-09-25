package judge

import (
	"context"
	"errors"
	"fmt"
	"math"
	"unicode/utf8"
)

// --- THIS IS OURS, AND IT IS NOT THE EXTERNAL JUDGE'S CONTRACT ---------------
//
// Everything above in this package mirrors docs/JUDGE.md, which is FROZEN: it is
// an agreement with an external ML judge service, it takes TWO images and answers
// a comparative question (scoreA / scoreB / winner), and it does not change
// unilaterally.
//
// Practice asks a different question — "how well does this ONE drawing depict the
// prompt?" — for a player who has no opponent (docs/GAME.md §10: the duel needs
// two people and, until there is a player base, that makes the product unplayable
// by its first visitor). Squeezing that into Judge would mean either sending the
// same image twice and reading scoreA (a lie the winner field then has to
// answer), or widening the frozen interface. So it gets its own, small, LOCAL
// seam instead.
//
// The external ML judge implements Judge. It does NOT implement Critic — that
// endpoint does not exist — which is precisely why JUDGE_MODE=http leaves
// practice unconfigured rather than silently faking it.

// maxFeedbackLen bounds the player-facing critique. Same number as the duel's
// reason cap and for the same reason (it is one or two sentences on a result
// screen), but a separate constant: this contract is ours to move, and JUDGE.md's
// is not.
const maxFeedbackLen = 500

// Critic scores ONE drawing against the prompt it was drawn for. Ours, not the
// external judge's — see the note above.
type Critic interface {
	Critique(ctx context.Context, req CritiqueRequest) (Critique, error)
}

// CritiqueRequest carries the prompt and the ONE pre-rendered PNG. Like Request,
// it is the authoritative server-side raster and never a client's own picture
// (the trust boundary — DOCUMENT-FORMAT §10).
type CritiqueRequest struct {
	Prompt string
	Image  []byte
}

// Critique is a verdict on a single drawing: an absolute similarity-to-prompt
// score in [0,1] (the same scale a duel's scoreA/scoreB use, so a practice score
// and a duel score mean the same thing to a player), plus plain-text feedback
// addressed to whoever drew it.
type Critique struct {
	Score    float64
	Feedback string
}

// ErrInvalidCritique marks a critique that violates the contract above.
var ErrInvalidCritique = errors.New("invalid critique")

// Validate mirrors Result.Validate's strictness, deliberately: a model that
// answers 1.4, NaN, or a page of prose is not handing us a lenient verdict, it is
// handing us a broken one, and a practice score shown to a player must mean the
// same thing every time.
func (c Critique) Validate() error {
	if math.IsNaN(c.Score) || math.IsInf(c.Score, 0) || c.Score < 0 || c.Score > 1 {
		return fmt.Errorf("%w: score %v not in [0,1]", ErrInvalidCritique, c.Score)
	}
	if utf8.RuneCountInString(c.Feedback) > maxFeedbackLen {
		return fmt.Errorf("%w: feedback exceeds %d chars", ErrInvalidCritique, maxFeedbackLen)
	}
	return nil
}
