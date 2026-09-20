package assist

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/markgrushevski/justpaint/server/internal/document"
)

// TestGeminiAssist_Live calls the REAL Generative Language API.
//
// Everything else in this package asserts what we SEND, against a local stand-in.
// That proves our request is the one we meant to build; it cannot prove Google
// accepts it — and here that is a bigger gap than usual, because this seam is the
// first to send a schema with an ARRAY in it, nested two deep (a list of shape
// objects, each carrying a list of numbers). Whether the API takes that shape at
// all, and whether a model can fill it with a drawing rather than a shrug, is
// exactly what a stub cannot say. This is the test that settles it.
//
// Opt-in on purpose, and NOT merely gated on the key being present: a key that
// happens to be in the environment must never quietly spend a daily quota. Run it
// deliberately:
//
//	GEMINI_LIVE=1 GEMINI_API_KEY=… go test ./internal/assist/ -run Live -v
//
// GEMINI_MODEL and GEMINI_BASE_URL override the defaults, which is the point of
// their being configurable at all. ASSIST_LIVE_PROMPT overrides the prompt, so the
// same test doubles as a way to try one by hand and read what came back.
func TestGeminiAssist_Live(t *testing.T) {
	if os.Getenv("GEMINI_LIVE") != "1" {
		t.Skip("set GEMINI_LIVE=1 (and GEMINI_API_KEY) to call the real API — it spends daily quota")
	}
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		t.Fatal("GEMINI_LIVE=1 but GEMINI_API_KEY is empty")
	}
	model := envOr("GEMINI_MODEL", "gemini-3.6-flash")
	base := envOr("GEMINI_BASE_URL", "https://generativelanguage.googleapis.com/v1beta")
	prompt := envOr("ASSIST_LIVE_PROMPT", "a simple house with a door")

	// The canvas the editor actually uses, so the coordinates the model chooses are
	// judged against the real frame rather than a convenient one.
	summary := document.DocSummary{
		Canvas: document.SummaryCanvas{Width: 1080, Height: 1080},
		Layers: []document.SummaryLayer{{ID: "l1", Name: "Layer 1", StrokeCount: 0}},
	}

	// Deliberately far above the service's own default. A healthy call is 4-9s, but
	// the free tier answers 503 "high demand" often enough that a tight bound here
	// would report "this does not work" about a busy afternoon — a different finding
	// from the one this test exists to make.
	a := NewGeminiAssist(key, model, base, 120*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	res, err := a.GenerateOps(ctx, Request{Prompt: prompt, DocSummary: summary})
	if err != nil {
		t.Fatalf("live GenerateOps against %s (%s): %v", base, model, err)
	}

	// Printed in full, because the point of running this is to READ what a real
	// model produced — not only to learn that it parsed.
	pretty, _ := json.MarshalIndent(res.Ops, "", "  ")
	t.Logf("prompt: %q\nnote: %s\nops (%d):\n%s", prompt, res.Note, len(res.Ops), pretty)

	// The one thing that must hold for this to be the feature at all: the batch the
	// client would apply passes the contract it is applied under.
	if err := document.ValidateOpBatch(res.Ops, summary); err != nil {
		t.Errorf("live batch violates the document contract: %v", err)
	}
	if len(res.Ops) < 2 {
		t.Errorf("got %d ops — a drawing is a layer plus at least one shape", len(res.Ops))
	}
	if res.Note == "" {
		t.Error("note is empty — the UI shows it beside the preview")
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
