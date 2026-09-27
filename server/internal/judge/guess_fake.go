package judge

import (
	"context"
	"fmt"
)

// FakeGuesser is the zero-ML default for /draw's guess button, mirroring
// FakeCritic: it "reads" the drawing by ink coverage, so the whole loop runs
// with no API key, quota or network, deterministic in the bytes. It never
// looks at the picture, and the label says so in the one field the player is
// guaranteed to read — a real-sounding guess from counted pixels would be a
// lie a player cannot detect.
type FakeGuesser struct{}

// NewFakeGuesser returns the in-process fake guesser.
func NewFakeGuesser() *FakeGuesser { return &FakeGuesser{} }

var _ Guesser = (*FakeGuesser)(nil)

// Guess implements Guesser. Confidence is the ink coverage — the same trick
// as FakeCritic's score, so the UI's confidence bar has something non-trivial
// to render — and the label is what keeps it from being read as a real
// answer. No alternatives, ever: a guesser that never looked has no second
// opinion to offer.
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
