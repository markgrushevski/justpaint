package judge

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

// --- ALSO OURS, AND ALSO NOT THE EXTERNAL JUDGE'S CONTRACT --------------------
//
// This is the THIRD seam in this package and the second LOCAL one, for exactly
// the reason Critic is local: docs/JUDGE.md is FROZEN, it is an agreement with an
// external ML judge service, it takes TWO images and answers a comparative
// question, and it does not change unilaterally. The external ML judge
// implements Judge. It does NOT implement Guesser — that endpoint does not exist.
//
// What makes a guess a different question is what is MISSING from it. A Critique
// scores ONE drawing against the prompt it was drawn for; a guess has no prompt,
// because on /draw nobody supplied one. The player opened a blank canvas, drew
// whatever they felt like, and asked "what is this?" — so there is nothing to
// score against and therefore no score. A confidence is how sure the model is of
// an answer it made up itself; a Critique's score is how well a picture matched
// an answer WE gave it. Reusing Critique would mean inventing a prompt to compare
// against, which is precisely the thing nobody did, and the number would then
// quietly mean something different from every other number on that scale.
//
// The blast radius of the whole seam is worth stating once, here, because it
// governs how much machinery it deserves: a guess scores nothing, ranks nothing
// and gates nothing. It is a sentence on the screen of the person who drew the
// picture.

const (
	// maxGuessLabelLen bounds the guess itself, and it is deliberately SHORT —
	// a sixth of the critic's feedback cap — because the shortness IS the product.
	// "a cat wearing a hat" is a guess; three sentences about what the model
	// noticed in the upper left is an essay, and this cap is what keeps a model
	// that wants to explain itself from turning the button into one. Runes, not
	// bytes, like every other player-facing cap in this package.
	maxGuessLabelLen = 80
	// maxGuessAlternatives bounds the runner-ups at two, and the number is a
	// product decision rather than a technical one: one alternative reads as a
	// hedge, two reads as "here are the other things it could be", and three reads
	// as a model with no idea listing nouns until something sticks.
	maxGuessAlternatives = 2
)

// Guesser names what is in ONE drawing. Ours, not the external judge's — see the
// note above.
//
// It takes the raw PNG rather than a request struct because there is genuinely
// nothing else to send: no prompt, no opponent, no second image. A one-field
// struct here would be a promise that something else is coming, and nothing is.
// The bytes are the authoritative server-side raster, never a client's own
// picture (the trust boundary — DOCUMENT-FORMAT §10); internal/guess says what
// that buys and why it matters more here than anywhere else.
type Guesser interface {
	Guess(ctx context.Context, img []byte) (Guess, error)
}

// Guess is what the model thinks the picture is.
//
// Confidence is in [0,1] and means something DIFFERENT from a Critique's Score
// despite sharing the scale: a score says how well a drawing matched a prompt we
// handed the player, a confidence says how sure the model is of a label nobody
// handed anyone. The two are not comparable and must never be shown side by side
// as if they were.
//
// Alternatives holds 0-2 runner-up guesses — genuinely different answers, not
// rewordings of Label. Zero is a perfectly good answer and the expected one for a
// clear drawing: a model that always finds two runner-ups is padding.
type Guess struct {
	Label        string
	Confidence   float64
	Alternatives []string
}

// ErrInvalidGuess marks a guess that violates the contract above.
var ErrInvalidGuess = errors.New("invalid guess")

// Validate mirrors Critique.Validate's strictness, deliberately and for the same
// reason: a model that answers with a confidence of 1.4, a NaN, or a list of nine
// alternatives is not being generous with us, it is handing us something that is
// not a guess, and the difference has to be caught here rather than discovered on
// a player's screen.
//
// The one place it is STRICTER than Critique is the label. Empty feedback is
// legal because feedback is commentary on a score that stands without it; an
// empty label is not, because a guess with no label is nothing at all — there is
// no other field for the answer to live in.
func (g Guess) Validate() error {
	if math.IsNaN(g.Confidence) || math.IsInf(g.Confidence, 0) || g.Confidence < 0 || g.Confidence > 1 {
		return fmt.Errorf("%w: confidence %v not in [0,1]", ErrInvalidGuess, g.Confidence)
	}
	if strings.TrimSpace(g.Label) == "" {
		return fmt.Errorf("%w: empty label", ErrInvalidGuess)
	}
	if n := utf8.RuneCountInString(g.Label); n > maxGuessLabelLen {
		return fmt.Errorf("%w: label is %d chars, over the %d cap", ErrInvalidGuess, n, maxGuessLabelLen)
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
		if n := utf8.RuneCountInString(alt); n > maxGuessLabelLen {
			return fmt.Errorf("%w: alternative %d is %d chars, over the %d cap", ErrInvalidGuess, i, n, maxGuessLabelLen)
		}
	}
	return nil
}
