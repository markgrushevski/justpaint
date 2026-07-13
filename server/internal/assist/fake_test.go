package assist

import (
	"context"
	"encoding/json"
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

// TestFakeAssist_Deterministic pins that two calls produce the identical batch —
// a fresh slice each call (no shared mutable state) but equal on the wire.
func TestFakeAssist_Deterministic(t *testing.T) {
	f := NewFakeAssist()
	a, _ := f.GenerateOps(context.Background(), Request{})
	b, _ := f.GenerateOps(context.Background(), Request{})
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	if string(aj) != string(bj) {
		t.Errorf("fake assist is not deterministic:\n a=%s\n b=%s", aj, bj)
	}
}
