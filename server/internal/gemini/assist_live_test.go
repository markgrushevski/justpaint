package gemini

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"

	"github.com/markgrushevski/justpaint/server/internal/assist"
	"github.com/markgrushevski/justpaint/server/internal/document"
	"github.com/markgrushevski/justpaint/server/internal/render"
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

// TestAssist_LiveSeesTheCanvas asks the real model to add to a drawing it can
// see: a house body with a door and no roof. The roof must sit on the walls.
//
//	GEMINI_LIVE=1 GEMINI_API_KEY=… go test ./internal/gemini/ -run TestAssist_LiveSeesTheCanvas -v
//
// RENDER_CLI points at the built render worker (default: the repo's own), and
// ASSIST_LIVE_OUT, if set, receives the document with the proposal applied.
func TestAssist_LiveSeesTheCanvas(t *testing.T) {
	if os.Getenv("GEMINI_LIVE") != "1" {
		t.Skip("set GEMINI_LIVE=1 (and GEMINI_API_KEY) to call the real API — it spends daily quota")
	}
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		t.Fatal("GEMINI_LIVE=1 but GEMINI_API_KEY is empty")
	}
	model := envOr("GEMINI_MODEL", "gemini-3.6-flash")
	base := envOr("GEMINI_BASE_URL", "https://generativelanguage.googleapis.com/v1beta")
	prompt := envOr("ASSIST_LIVE_PROMPT", "add a roof to the house")

	walls, door, outline := document.Color("#f2d0a4"), document.Color("#8b5a2b"), document.Color("#1b1b1b")
	width := 3.0
	doc := document.Document{Version: 1, Width: 1080, Height: 1080, Layers: []document.Layer{
		{ID: "l1", Name: "Layer 1", Visible: true, Opacity: 1, Strokes: []document.Stroke{
			&document.RectStroke{StrokeBase: document.StrokeBase{ID: "walls", Type: document.StrokeRect, Composite: document.CompositeSourceOver},
				X: 340, Y: 520, Width: 400, Height: 380, Fill: &walls, Stroke: &outline, StrokeWidth: &width},
			&document.RectStroke{StrokeBase: document.StrokeBase{ID: "door", Type: document.StrokeRect, Composite: document.CompositeSourceOver},
				X: 500, Y: 740, Width: 80, Height: 160, Fill: &door},
		}},
	}}
	if err := document.Validate(doc); err != nil {
		t.Fatalf("the test document is invalid: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	renderer := render.NewNodeRenderer("node", envOr("RENDER_CLI", "../../../packages/render/dist/render.mjs"), 1)
	img, err := renderer.Render(ctx, doc)
	if err != nil {
		t.Fatalf("render the canvas (build it with npm run build -w @justpaint/render): %v", err)
	}

	summary := document.Summarize(doc)
	a := NewAssist(key, model, base, 120*time.Second)
	res, err := a.GenerateOps(ctx, assist.Request{Prompt: prompt, Document: doc, DocSummary: summary, Image: img})
	if err != nil {
		t.Fatalf("live GenerateOps against %s (%s): %v", base, model, err)
	}
	pretty, _ := json.MarshalIndent(res.Ops, "", "  ")
	t.Logf("prompt: %q\nnote: %s\nops (%d):\n%s", prompt, res.Note, len(res.Ops), pretty)

	if err := document.ValidateOpBatch(res.Ops, summary); err != nil {
		t.Fatalf("live batch violates the document contract: %v", err)
	}
	if out := os.Getenv("ASSIST_LIVE_OUT"); out != "" {
		writeWithProposal(t, out, doc, res.Ops)
	}

	// The roof: a polygon whose base sits near the top of the walls and whose
	// span covers them.
	for _, op := range res.Ops {
		add, ok := op.(*document.AddStrokeOp)
		if !ok {
			continue
		}
		poly, ok := add.Stroke.(*document.PolygonStroke)
		if !ok {
			continue
		}
		minX, minY, maxX, maxY := pointBounds(poly.Points)
		if math.Abs(maxY-520) <= 40 && minY < 520 && minX <= 360 && maxX >= 720 {
			return
		}
		t.Logf("a polygon at x %.0f..%.0f, y %.0f..%.0f is not a roof on walls at x 340..740, top 520", minX, maxX, minY, maxY)
	}
	t.Error("no polygon sits on the walls — the model did not place against what it was shown")
}

// writeWithProposal saves doc with the ops applied as one new top layer, for a look.
func writeWithProposal(t *testing.T, path string, doc document.Document, ops []document.Op) {
	t.Helper()
	layer := document.Layer{ID: "proposal", Name: "Proposal", Visible: true, Opacity: 1}
	for _, op := range ops {
		if add, ok := op.(*document.AddStrokeOp); ok {
			layer.Strokes = append(layer.Strokes, add.Stroke)
		}
	}
	doc.Layers = append(doc.Layers, layer)
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}
