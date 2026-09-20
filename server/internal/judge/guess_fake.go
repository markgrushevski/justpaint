package judge

import (
	"context"
	"fmt"
)

// FakeGuesser is the zero-ML default for /draw's guess button, mirroring
// FakeCritic: it "reads" the drawing by INK COVERAGE — the fraction of
// non-background pixels in the authoritative raster — so the whole loop (render →
// guess → answer) runs with no API key, no quota and no network. Deterministic in
// the bytes.
//
// It never looks at the picture, and the LABEL says so in as many words, in the
// one field the player is guaranteed to read. That sentence is the point of this
// type, not an apology for it: "a cat wearing a hat" from something that only
// counted dark pixels is a lie a player cannot detect, and it would quietly teach
// them the feature works. The fake proves the loop; only a real guesser guesses.
type FakeGuesser struct{}

// NewFakeGuesser returns the in-process fake guesser.
func NewFakeGuesser() *FakeGuesser { return &FakeGuesser{} }

var _ Guesser = (*FakeGuesser)(nil)

// Guess implements Guesser. It ignores the context; the result honors the
// contract exactly (a valid confidence, a label within the cap) so every consumer
// path is exercised.
//
// Confidence is the ink coverage, exactly as FakeCritic's score is — the same
// trick, so the same document produces a non-trivial, non-constant number and a
// UI that renders a confidence bar has something to render. It is ink wearing a
// confidence's clothes, which is exactly why the label refuses to let anyone read
// it as anything else.
//
// No alternatives, ever: a guesser that never looked at the picture has no second
// opinion, because it never had a first one. Zero runner-ups is a legal shape and
// this is the honest occupant of it.
func (FakeGuesser) Guess(_ context.Context, img []byte) (Guess, error) {
	cov, err := inkCoverage(img)
	if err != nil {
		return Guess{}, fmt.Errorf("decode image: %w", err)
	}
	return Guess{
		Label:      inkLabel(cov) + " (the fake guesser sees ink, not meaning)",
		Confidence: cov,
	}, nil
}

// inkLabel describes how much was drawn, since how much is the only thing this
// impl can actually tell. The buckets are wide on purpose — a fake that reported
// "37% covered" would sound like a measurement worth trusting.
func inkLabel(coverage float64) string {
	switch {
	case coverage < 0.02:
		return "an empty canvas"
	case coverage < 0.25:
		return "a few marks"
	case coverage < 0.6:
		return "a drawing"
	default:
		return "a very busy drawing"
	}
}
