package judge

import (
	"context"
	"errors"
	"fmt"
	"math"
	"unicode/utf8"
)

// MaxFeedbackLen bounds the player-facing critique. Same number as the
// duel's reason cap, but a separate constant: this contract is ours to
// move, JUDGE.md's is not.
const MaxFeedbackLen = 500

// Critic is a second, local seam — ours to move, not part of the frozen
// external-judge contract above. It scores ONE drawing against the prompt it
// was drawn for, answering practice's "how well does this depict the
// prompt" instead of Judge's comparative question (JUDGE.md §8.2 owns why a
// wider Judge was rejected). The external judge does not implement it,
// which is why JUDGE_MODE=http leaves practice unconfigured rather than
// faking it.
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
	if utf8.RuneCountInString(c.Feedback) > MaxFeedbackLen {
		return fmt.Errorf("%w: feedback exceeds %d chars", ErrInvalidCritique, MaxFeedbackLen)
	}
	return nil
}
