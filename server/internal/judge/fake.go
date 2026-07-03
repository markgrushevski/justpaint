package judge

import (
	"bytes"
	"context"
	"fmt"
	"image/color"
	"image/png"
)

// tieEpsilon: scores within this margin are declared a tie (JUDGE.md §8).
const tieEpsilon = 0.02

// inkThreshold: an 8-bit channel at/above this counts as background white. The
// margin below 255 absorbs anti-aliased stroke edges so they don't read as ink.
const inkThreshold = 250

// FakeJudge is the zero-ML default (JUDGE.md §8): deterministic, in-process, and
// prompt-independent. It scores each PNG by INK COVERAGE — the fraction of
// non-background pixels against the known opaque-white judged background (§5) —
// maps coverage directly to a score in [0,1], picks the higher as winner, and
// declares a tie when the two are within an epsilon. Same bytes ⇒ same result,
// so tests and matches are reproducible.
type FakeJudge struct{}

// NewFakeJudge returns the in-process fake judge.
func NewFakeJudge() *FakeJudge { return &FakeJudge{} }

var _ Judge = (*FakeJudge)(nil)

// Score implements Judge. It ignores the prompt and the context; the result
// honors the §2 contract exactly (valid scores, positional winner incl. "tie",
// plain-text reason) so every consumer path — including ties — is exercised.
func (FakeJudge) Score(_ context.Context, req Request) (Result, error) {
	a, err := inkCoverage(req.ImageA)
	if err != nil {
		return Result{}, fmt.Errorf("decode imageA: %w", err)
	}
	b, err := inkCoverage(req.ImageB)
	if err != nil {
		return Result{}, fmt.Errorf("decode imageB: %w", err)
	}

	res := Result{ScoreA: a, ScoreB: b}
	switch {
	case a-b > tieEpsilon:
		res.Winner = WinnerA
		res.Reason = fmt.Sprintf(
			"A covers %.0f%% of the canvas vs B's %.0f%% — A wins on ink coverage (fake judge).",
			a*100, b*100)
	case b-a > tieEpsilon:
		res.Winner = WinnerB
		res.Reason = fmt.Sprintf(
			"B covers %.0f%% of the canvas vs A's %.0f%% — B wins on ink coverage (fake judge).",
			b*100, a*100)
	default:
		res.Winner = WinnerTie
		res.Reason = fmt.Sprintf(
			"Both cover ~%.0f%% of the canvas — too close to call, a tie (fake judge).",
			(a+b)/2*100)
	}
	return res, nil
}

// inkCoverage decodes a PNG and returns the fraction of non-background (ink)
// pixels in [0,1]. Pure and deterministic in the bytes.
func inkCoverage(pngBytes []byte) (float64, error) {
	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return 0, err
	}
	b := img.Bounds()
	total := b.Dx() * b.Dy()
	if total == 0 {
		return 0, nil
	}
	ink := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if isInk(img.At(x, y)) {
				ink++
			}
		}
	}
	return float64(ink) / float64(total), nil
}

// isInk reports whether a pixel is drawing rather than near-white background.
// Transparent pixels count as background (the judged raster is opaque, but be
// defensive). Channels come back 16-bit from RGBA(); shift to 8-bit to compare.
func isInk(c color.Color) bool {
	r, g, b, a := c.RGBA()
	if a>>8 < inkThreshold {
		return false
	}
	return r>>8 < inkThreshold || g>>8 < inkThreshold || b>>8 < inkThreshold
}
