package judge

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
	"time"
)

// TestGeminiJudge_Live calls the REAL Generative Language API.
//
// Everything else in this package asserts what we SEND, against a local
// stand-in. That proves our request is the one we meant to build; it cannot
// prove Google accepts it. The wire types were written from documentation, and
// three choices in them were reconstructions (field-name casing, where
// systemInstruction sits, the schema enum spelling). This is the test that
// settled them, and it has been run: against the real API with a live key, the
// request shape is confirmed. (This used to cite an ISSUES-INNER id for those
// three reconstructions. The id was recycled for an unrelated defect and the
// entry is now closed, so the pointer is dropped rather than repaired.)
//
// Opt-in on purpose, and NOT merely gated on the key being present: a key that
// happens to be in the environment must never quietly spend a daily quota the
// free tier caps at 20 requests per model (measured from a 429 body — see
// internal/platform/config). Run it deliberately:
//
//	GEMINI_LIVE=1 GEMINI_API_KEY=… go test ./internal/judge/ -run Live -v
//
// GEMINI_MODEL and GEMINI_BASE_URL override the defaults, which is the point of
// their being configurable at all.
func TestGeminiJudge_Live(t *testing.T) {
	if os.Getenv("GEMINI_LIVE") != "1" {
		t.Skip("set GEMINI_LIVE=1 (and GEMINI_API_KEY) to call the real API — it spends daily quota")
	}
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		t.Fatal("GEMINI_LIVE=1 but GEMINI_API_KEY is empty")
	}
	model := envOr("GEMINI_MODEL", "gemini-3.6-flash")
	base := envOr("GEMINI_BASE_URL", "https://generativelanguage.googleapis.com/v1beta")

	// A prompt we can actually satisfy in code, so the assertion is about whether
	// the model LOOKED at the pixels — not about our drawing talent. One image
	// depicts the prompt; the other is an empty canvas, which §2 says is a 0.
	const prompt = "a large black circle in the middle of the page"
	circle := pngCircle(t)
	blank := pngBlank(t)

	j := NewGeminiJudge(key, model, base, 30*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	res, err := j.Score(ctx, Request{Prompt: prompt, ImageA: circle, ImageB: blank})
	if err != nil {
		t.Fatalf("live Score against %s (%s): %v", base, model, err)
	}
	t.Logf("verdict: scoreA=%.3f scoreB=%.3f winner=%s reason=%q", res.ScoreA, res.ScoreB, res.Winner, res.Reason)

	if err := res.Validate(); err != nil {
		t.Errorf("live result violates the contract: %v", err)
	}
	// The one judgement that must hold for this to be a judge at all: a drawing of
	// the prompt beats an empty canvas.
	if res.Winner != WinnerA {
		t.Errorf("winner = %q, want %q — the blank canvas must not win", res.Winner, WinnerA)
	}
	if res.ScoreA <= res.ScoreB {
		t.Errorf("scoreA %.3f should exceed scoreB %.3f (a drawing vs an empty canvas)", res.ScoreA, res.ScoreB)
	}
	if res.Reason == "" {
		t.Error("reason is empty — players are shown this")
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// pngCircle draws a filled black disc on white, at the judged frame size.
func pngCircle(t *testing.T) []byte {
	t.Helper()
	const size = 1024
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			dx, dy := float64(x-size/2), float64(y-size/2)
			if dx*dx+dy*dy <= 320*320 {
				img.Set(x, y, color.RGBA{0x11, 0x11, 0x11, 0xff})
			} else {
				img.Set(x, y, color.RGBA{0xff, 0xff, 0xff, 0xff})
			}
		}
	}
	return encodePNG(t, img)
}

// pngBlank is an empty white canvas — the §2 floor.
func pngBlank(t *testing.T) []byte {
	t.Helper()
	const size = 1024
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			img.Set(x, y, color.RGBA{0xff, 0xff, 0xff, 0xff})
		}
	}
	return encodePNG(t, img)
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}
