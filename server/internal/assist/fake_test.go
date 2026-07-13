package assist

import (
	"context"
	"testing"

	"github.com/markgrushevski/justpaint/server/internal/document"
)

// TestFakeAssist_OpsValidate is the contract check called out in the build plan:
// the canned batch MUST pass document.ValidateOpBatch, or the accept path would
// reject the fake's own output. The fake ignores the summary, so it is validated
// against an empty summary (a fresh document) — the batch is self-contained
// (it creates its own layer).
func TestFakeAssist_OpsValidate(t *testing.T) {
	f := NewFakeAssist()
	res, err := f.GenerateOps(context.Background(), Request{Prompt: "draw a house with a red roof"})
	if err != nil {
		t.Fatalf("GenerateOps: %v", err)
	}
	if len(res.Ops) == 0 {
		t.Fatal("expected a non-empty canned batch")
	}
	if res.Note == "" {
		t.Error("expected a human-facing note")
	}
	if err := document.ValidateOpBatch(res.Ops, document.DocSummary{}); err != nil {
		t.Fatalf("canned batch failed ValidateOpBatch: %v", err)
	}
}

// TestFakeAssist_ValidatesAgainstNonEmptySummary confirms the batch also passes
// when the client already has layers — the fake's ids don't collide with a
// typical document's ids, so the single id namespace stays clean.
func TestFakeAssist_ValidatesAgainstNonEmptySummary(t *testing.T) {
	summary := document.DocSummary{
		Canvas: document.SummaryCanvas{Width: 1080, Height: 1080},
		Layers: []document.SummaryLayer{{ID: "l1", Name: "Layer 1", StrokeCount: 3}},
	}
	res, _ := NewFakeAssist().GenerateOps(context.Background(), Request{Prompt: "x", DocSummary: summary})
	if err := document.ValidateOpBatch(res.Ops, summary); err != nil {
		t.Fatalf("canned batch failed ValidateOpBatch against a non-empty summary: %v", err)
	}
}

// strokeType reports the discriminant of a stroke by its concrete Go type (the
// Stroke union's own type field is behind an unexported accessor).
func strokeType(t *testing.T, s document.Stroke) document.StrokeType {
	t.Helper()
	switch s.(type) {
	case *document.RectStroke:
		return document.StrokeRect
	case *document.PolygonStroke:
		return document.StrokePolygon
	case *document.EllipseStroke:
		return document.StrokeEllipse
	case *document.LineStroke:
		return document.StrokeLine
	case *document.FreehandStroke:
		return document.StrokeFreehand
	default:
		t.Fatalf("unexpected stroke type %T", s)
		return ""
	}
}

// TestFakeAssist_Deterministic pins the batch SHAPE (not byte-identical ids —
// each call now carries fresh ids so the accept-then-regenerate path stays clean).
// Two calls must yield the same op kinds, count, and stroke types, and both must
// validate. The house is: add_layer + rect body + polygon roof + 2 rect windows +
// rect door.
func TestFakeAssist_Deterministic(t *testing.T) {
	f := NewFakeAssist()
	a, _ := f.GenerateOps(context.Background(), Request{})
	b, _ := f.GenerateOps(context.Background(), Request{})

	// Same op count across calls.
	if len(a.Ops) != len(b.Ops) {
		t.Fatalf("op count differs: a=%d b=%d", len(a.Ops), len(b.Ops))
	}
	const wantOps = 6
	if len(a.Ops) != wantOps {
		t.Fatalf("op count = %d, want %d", len(a.Ops), wantOps)
	}

	// op[0] is the add_layer; op[1..] are add_stroke with a fixed stroke-type shape.
	wantStrokes := []document.StrokeType{
		document.StrokeRect,    // body
		document.StrokePolygon, // roof
		document.StrokeRect,    // left window
		document.StrokeRect,    // right window
		document.StrokeRect,    // door
	}
	for _, res := range []Result{a, b} {
		if _, ok := res.Ops[0].(*document.AddLayerOp); !ok {
			t.Fatalf("op 0 = %T, want *document.AddLayerOp", res.Ops[0])
		}
		for i, want := range wantStrokes {
			op, ok := res.Ops[i+1].(*document.AddStrokeOp)
			if !ok {
				t.Fatalf("op %d = %T, want *document.AddStrokeOp", i+1, res.Ops[i+1])
			}
			if got := strokeType(t, op.Stroke); got != want {
				t.Errorf("op %d stroke type = %q, want %q", i+1, got, want)
			}
		}
		if err := document.ValidateOpBatch(res.Ops, document.DocSummary{}); err != nil {
			t.Fatalf("batch failed ValidateOpBatch: %v", err)
		}
	}
}

// TestFakeAssist_AcceptThenRegenerate is the repeatable-demo regression: after a
// user accepts one batch, the accepted layer id lands in the next request's doc
// summary. A SECOND generated batch must still validate against that summary — the
// per-call id counter keeps the layer id disjoint, so the single id namespace does
// not report a duplicate (which previously broke draw → accept → draw-again).
func TestFakeAssist_AcceptThenRegenerate(t *testing.T) {
	f := NewFakeAssist()

	first, _ := f.GenerateOps(context.Background(), Request{Prompt: "draw a house"})
	layer, ok := first.Ops[0].(*document.AddLayerOp)
	if !ok {
		t.Fatalf("first op = %T, want *document.AddLayerOp", first.Ops[0])
	}

	// Simulate the client accepting the first batch: its layer now exists in the
	// document, so the next request's summary carries that id.
	afterAccept := document.DocSummary{
		Canvas: document.SummaryCanvas{Width: 1080, Height: 1080},
		Layers: []document.SummaryLayer{{ID: layer.ID, Name: layer.Name, StrokeCount: 5}},
	}

	second, _ := f.GenerateOps(context.Background(), Request{Prompt: "draw another house", DocSummary: afterAccept})
	if secondLayer := second.Ops[0].(*document.AddLayerOp); secondLayer.ID == layer.ID {
		t.Fatalf("second batch reused the first layer id %q — ids must be unique per call", layer.ID)
	}
	if err := document.ValidateOpBatch(second.Ops, afterAccept); err != nil {
		t.Fatalf("second batch failed to validate against a summary holding the first batch's layer: %v", err)
	}
}
