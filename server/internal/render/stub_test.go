package render

import (
	"bytes"
	"context"
	"image/png"
	"testing"

	"github.com/markgrushevski/justpaint/server/internal/document"
)

// docWithStrokes builds a minimal valid-shaped document with n strokes on one
// layer. The stub only counts strokes, so the concrete type is irrelevant.
func docWithStrokes(n int) document.Document {
	strokes := make([]document.Stroke, n)
	for i := range strokes {
		strokes[i] = &document.RectStroke{}
	}
	return document.Document{
		Version: 1, Width: 1080, Height: 1080,
		Layers: []document.Layer{{ID: "l1", Strokes: strokes}},
	}
}

// inkFraction decodes a PNG and returns the fraction of non-white pixels — the
// same signal the FakeJudge reads.
func inkFraction(t *testing.T, pngBytes []byte) float64 {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	b := img.Bounds()
	total := b.Dx() * b.Dy()
	ink := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if r>>8 < 250 || g>>8 < 250 || bl>>8 < 250 {
				ink++
			}
		}
	}
	return float64(ink) / float64(total)
}

func TestStubRenderer_ValidFrame(t *testing.T) {
	png0, err := StubRenderer{}.Render(context.Background(), docWithStrokes(3))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(png0))
	if err != nil {
		t.Fatalf("output is not a valid PNG: %v", err)
	}
	if b := img.Bounds(); b.Dx() != JudgeFrameSize || b.Dy() != JudgeFrameSize {
		t.Errorf("frame = %dx%d, want %dx%d", b.Dx(), b.Dy(), JudgeFrameSize, JudgeFrameSize)
	}
}

func TestStubRenderer_Deterministic(t *testing.T) {
	doc := docWithStrokes(7)
	a, _ := StubRenderer{}.Render(context.Background(), doc)
	b, _ := StubRenderer{}.Render(context.Background(), doc)
	if !bytes.Equal(a, b) {
		t.Error("same document produced different bytes; render must be deterministic")
	}
}

func TestStubRenderer_MoreStrokesMoreInk(t *testing.T) {
	few, _ := StubRenderer{}.Render(context.Background(), docWithStrokes(2))
	many, _ := StubRenderer{}.Render(context.Background(), docWithStrokes(20))
	if inkFraction(t, many) <= inkFraction(t, few) {
		t.Errorf("busier drawing must score more ink: few=%.3f many=%.3f",
			inkFraction(t, few), inkFraction(t, many))
	}
}

func TestCoverage_Clamped(t *testing.T) {
	// Far past the linear range must saturate at hi, never exceed 1.0.
	if c := coverage(docWithStrokes(1000)); c > 0.90+1e-9 {
		t.Errorf("coverage = %.3f, want ≤ 0.90 (clamped)", c)
	}
	if c := coverage(docWithStrokes(0)); c < 0.02 {
		t.Errorf("coverage = %.3f, want ≥ 0.02 (floor)", c)
	}
}
