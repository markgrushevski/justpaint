package judge

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

const (
	// MaxGuessLabelLen bounds the guess itself, deliberately short — a sixth
	// of the critic's feedback cap — because the shortness is the product:
	// "a cat wearing a hat" is a guess, three sentences about the upper left
	// corner is an essay. Runes, not bytes, like every other player-facing
	// cap in this package.
	MaxGuessLabelLen = 80
	// maxGuessAlternatives bounds the runner-ups at two: one alternative
	// reads as a hedge, two reads as options, three reads as a model
	// guessing nouns until something sticks.
	maxGuessAlternatives = 2
)

// Guesser is a third, local seam — like Critic, ours to move and not part of
// the frozen external-judge contract (JUDGE.md §8.3 owns why it is not a
// widened Judge, and the closer call, why it is not a Critique: a guess has
// no prompt to score against, only a confidence in an answer the model made
// up itself). It names what is in ONE drawing and takes the raw PNG rather
// than a request struct because there is nothing else to send — no prompt,
// no opponent, no second image. The bytes are the authoritative server-side
// raster, never a client's own picture (trust boundary, DOCUMENT-FORMAT.md
// §10); internal/guess covers why that matters more here than anywhere
// else. A guess scores nothing, ranks nothing and gates nothing — it's a
// sentence on the screen of whoever drew the picture.
type Guesser interface {
	Guess(ctx context.Context, img []byte) (Guess, error)
}

// Guess is what the model thinks the picture is.
//
// Confidence is in [0,1] but means something different from a Critique's
// Score despite the shared scale: a score says how well a drawing matched a
// prompt we handed the player; a confidence says how sure the model is of a
// label nobody handed anyone. The two must never be shown side by side as if
// they were comparable.
//
// Alternatives holds 0-2 runner-up guesses — genuinely different answers,
// not rewordings of Label. Zero is a good answer for a clear drawing; a
// model that always finds two runner-ups is padding.
type Guess struct {
	Label        string
	Confidence   float64
	Alternatives []string
}

// ErrInvalidGuess marks a guess that violates the contract above.
var ErrInvalidGuess = errors.New("invalid guess")

// Validate mirrors Critique.Validate's strictness: a confidence of 1.4, a
// NaN, or nine alternatives is not generosity, it is not a guess, and that
// has to be caught here rather than on a player's screen.
//
// One place it is stricter than Critique: an empty label is invalid, while
// empty feedback on a Critique is legal. Feedback is commentary on a score
// that stands without it; a guess with no label is nothing at all — there is
// no other field for the answer to live in.
func (g Guess) Validate() error {
	if math.IsNaN(g.Confidence) || math.IsInf(g.Confidence, 0) || g.Confidence < 0 || g.Confidence > 1 {
		return fmt.Errorf("%w: confidence %v not in [0,1]", ErrInvalidGuess, g.Confidence)
	}
	if strings.TrimSpace(g.Label) == "" {
		return fmt.Errorf("%w: empty label", ErrInvalidGuess)
	}
	if n := utf8.RuneCountInString(g.Label); n > MaxGuessLabelLen {
		return fmt.Errorf("%w: label is %d chars, over the %d cap", ErrInvalidGuess, n, MaxGuessLabelLen)
	}
	if len(g.Alternatives) > maxGuessAlternatives {
		return fmt.Errorf("%w: %d alternatives (max %d)", ErrInvalidGuess, len(g.Alternatives), maxGuessAlternatives)
	}
	for i, alt := range g.Alternatives {
		// A blank runner-up is a hole in a list the client will render as a row, so it
		// is rejected rather than tolerated. Impls that are handed optional fields
		// drop the blanks themselves before they build the slice.
		if strings.TrimSpace(alt) == "" {
			return fmt.Errorf("%w: alternative %d is empty", ErrInvalidGuess, i)
		}
		if n := utf8.RuneCountInString(alt); n > MaxGuessLabelLen {
			return fmt.Errorf("%w: alternative %d is %d chars, over the %d cap", ErrInvalidGuess, i, n, MaxGuessLabelLen)
		}
	}
	return nil
}
