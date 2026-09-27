package gemini

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/markgrushevski/justpaint/server/internal/assist"
	"github.com/markgrushevski/justpaint/server/internal/document"
)

// TestAssist_Live calls the real Generative Language API — the only way
// to prove Google accepts this schema's two-deep ARRAY nesting and that a
// model fills it with a drawing rather than a shrug (docs/ASSIST.md §6).
//
// Opt-in, and not merely gated on the key being present: a key that happens
// to be in the environment must never quietly spend a daily quota.
//
//	GEMINI_LIVE=1 GEMINI_API_KEY=… go test ./internal/gemini/ -run TestAssist_Live -v
//
// GEMINI_MODEL, GEMINI_BASE_URL and ASSIST_LIVE_PROMPT override the defaults.
func TestAssist_Live(t *testing.T) {
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

	// The canvas the editor actually uses, so coordinates are judged against the
	// real frame.
	summary := document.DocSummary{
		Canvas: document.SummaryCanvas{Width: 1080, Height: 1080},
		Layers: []document.SummaryLayer{{ID: "l1", Name: "Layer 1", StrokeCount: 0}},
	}

	// Well above the service's default: the free tier answers 503 "high demand"
	// often enough that a tight bound would report a busy afternoon as broken.
	a := NewAssist(key, model, base, 120*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	res, err := a.GenerateOps(ctx, assist.Request{Prompt: prompt, DocSummary: summary})
	if err != nil {
		t.Fatalf("live GenerateOps against %s (%s): %v", base, model, err)
	}

	// Printed in full: the point of running this is to see what a real model
	// produced, not just that it parsed.
	pretty, _ := json.MarshalIndent(res.Ops, "", "  ")
	t.Logf("prompt: %q\nnote: %s\nops (%d):\n%s", prompt, res.Note, len(res.Ops), pretty)

	// The one thing that must hold: the batch the client would apply passes the
	// contract it's applied under.
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
