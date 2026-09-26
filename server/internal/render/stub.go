package render

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"math"

	"github.com/markgrushevski/justpaint/server/internal/document"
)

// StubRenderer is the zero-dependency stand-in for the real (Konva +
// perfect-freehand) Node render worker, mirroring judge.FakeJudge: it emits a
// deterministic 1024² PNG whose ink coverage scales with stroke count, not the
// actual drawing (docs/NOTES.md). It exists to prove the submit → render →
// judge → result loop end-to-end, not to reproduce the art.
type StubRenderer struct{}

// NewStubRenderer returns the in-process stub renderer.
func NewStubRenderer() *StubRenderer { return &StubRenderer{} }

var _ Renderer = (*StubRenderer)(nil)

// Render paints the top coverage-fraction of the frame with ink over an opaque
// white background, so inkCoverage(png) ≈ coverage(doc). Deterministic in doc.
func (StubRenderer) Render(_ context.Context, doc document.Document) ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, JudgeFrameSize, JudgeFrameSize))
	white := color.RGBA{0xff, 0xff, 0xff, 0xff}
	black := color.RGBA{0x00, 0x00, 0x00, 0xff}

	inkRows := int(math.Round(coverage(doc) * float64(JudgeFrameSize)))
	for y := 0; y < JudgeFrameSize; y++ {
		c := white
		if y < inkRows {
			c = black
		}
		for x := 0; x < JudgeFrameSize; x++ {
			img.SetRGBA(x, y, c)
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// coverage maps "amount drawn" (total stroke count across all layers) to an ink
// fraction in [lo, hi]. Deterministic; monotonic in stroke count so a busier
// drawing scores higher under the ink-coverage fake judge.
func coverage(doc document.Document) float64 {
	n := 0
	for _, l := range doc.Layers {
		n += len(l.Strokes)
	}
	const (
		base = 0.05
		per  = 0.02
		lo   = 0.02
		hi   = 0.90
	)
	c := base + per*float64(n)
	if c < lo {
		return lo
	}
	if c > hi {
		return hi
	}
	return c
}
