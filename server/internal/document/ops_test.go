package document_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/markgrushevski/justpaint/server/internal/document"
)

// AI-assist op-batch contract (docs/ASSIST.md §2). This table is mirrored 1:1 —
// identical case names — by packages/document/test/ops.test.ts. A schema change
// lands in both validators AND both test tables together (keystone parity).

// summaryWith builds a minimal summary with the given layer ids (each named after
// its id, 0 strokes).
func summaryWith(ids ...string) document.DocSummary {
	layers := make([]document.SummaryLayer, len(ids))
	for i, id := range ids {
		layers[i] = document.SummaryLayer{ID: id, Name: id, StrokeCount: 0}
	}
	return document.DocSummary{
		Canvas: document.SummaryCanvas{Width: 100, Height: 100},
		Layers: layers,
	}
}

// opsWith wraps a stroke in a one-op add_stroke batch on layerID (mirrors docWith).
func opsWith(layerID, stroke string) string {
	return `[{"kind":"add_stroke","layerId":"` + layerID + `","stroke":` + stroke + `}]`
}

// rect is a valid non-freehand stroke of the given id (a unit rect).
func rect(id string) string {
	return `{"id":"` + id + `","type":"rect","composite":"source-over","x":0,"y":0,"width":10,"height":10,"fill":"#000000"}`
}

func freehand(id string) string {
	return `{"id":"` + id + `","type":"freehand","composite":"source-over","color":"#000000","points":[[1,1,0.5]],` +
		`"brush":{"size":1,"thinning":0,"smoothing":0,"streamline":0,"simulatePressure":false,"taperStart":0,"taperEnd":0}}`
}

// nAddLayerOps builds a batch of n add_layer ops with unique ids.
func nAddLayerOps(n int) string {
	var b strings.Builder
	b.WriteByte('[')
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"kind":"add_layer","id":"L%d","name":"n"}`, i)
	}
	b.WriteByte(']')
	return b.String()
}

func TestValidateOpBatch(t *testing.T) {
	cases := []struct {
		name    string
		summary document.DocSummary
		ops     string
		wantErr bool
	}{
		// --- valid ---
		{"add_stroke onto existing summary layer", summaryWith("L1"), opsWith("L1", rect("s1")), false},
		{"add_layer then add_stroke referencing it", summaryWith(),
			`[{"kind":"add_layer","id":"new","name":"New"},{"kind":"add_stroke","layerId":"new","stroke":` + rect("s1") + `}]`, false},
		{"empty batch", summaryWith("L1"), `[]`, false},

		// --- invalid ---
		{"dangling add_stroke layerId", summaryWith("L1"), opsWith("nope", rect("s1")), true},
		{"forward ref: stroke before its layer", summaryWith(),
			`[{"kind":"add_stroke","layerId":"new","stroke":` + rect("s1") + `},{"kind":"add_layer","id":"new","name":"New"}]`, true},
		{"freehand in add_stroke", summaryWith("L1"), opsWith("L1", freehand("s1")), true},
		{"op id collides with summary id", summaryWith("L1"), `[{"kind":"add_layer","id":"L1","name":"dup"}]`, true},
		{"two ops share an id", summaryWith(),
			`[{"kind":"add_layer","id":"dup","name":"A"},{"kind":"add_layer","id":"dup","name":"B"}]`, true},
		{"add_stroke stroke id collides with summary layer id", summaryWith("L1"), opsWith("L1", rect("L1")), true},
		{"add_stroke zero-area rect", summaryWith("L1"),
			opsWith("L1", `{"id":"s1","type":"rect","composite":"source-over","x":0,"y":0,"width":0,"height":10,"fill":"#000000"}`), true},
		{"add_stroke polygon under three vertices", summaryWith("L1"),
			opsWith("L1", `{"id":"s1","type":"polygon","composite":"source-over","points":[[0,0],[10,10]],"fill":"#000000"}`), true},
		{"batch over maxOpsPerBatch", summaryWith(), nAddLayerOps(document.MaxOpsPerBatch + 1), true},
		{"add_layer empty name", summaryWith(), `[{"kind":"add_layer","id":"x","name":""}]`, true},
		{"add_layer name too long", summaryWith(),
			`[{"kind":"add_layer","id":"x","name":"` + strings.Repeat("x", 65) + `"}]`, true},
		{"unknown op kind", summaryWith(), `[{"kind":"delete_layer","id":"x"}]`, true},
		// required keys must be physically present (keystone parity with the TS table).
		{"missing required key: add_layer name", summaryWith(), `[{"kind":"add_layer","id":"x"}]`, true},
		{"missing required key: add_stroke layerId", summaryWith("L1"),
			`[{"kind":"add_stroke","stroke":` + rect("s1") + `}]`, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := document.ParseAndValidateOpBatch([]byte(tc.ops), tc.summary)
			if tc.wantErr && err == nil {
				t.Fatalf("expected an error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// Batch cap is checked programmatically too, mirroring the Stroke point-cap test.
func TestValidateOpBatch_BatchCap(t *testing.T) {
	ops := nAddLayerOps(document.MaxOpsPerBatch + 1)
	if _, err := document.ParseAndValidateOpBatch([]byte(ops), summaryWith()); err == nil {
		t.Fatalf("expected error for exceeding the per-batch op cap, got nil")
	}
}

// The op union must decode into the right concrete types.
func TestDecodeOpBatch_Union(t *testing.T) {
	batch := `[{"kind":"add_layer","id":"new","name":"New"},{"kind":"add_stroke","layerId":"new","stroke":` + rect("s1") + `}]`
	ops, err := document.ParseAndValidateOpBatch([]byte(batch), summaryWith())
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if len(ops) != 2 {
		t.Fatalf("got %d ops, want 2", len(ops))
	}
	if _, ok := ops[0].(*document.AddLayerOp); !ok {
		t.Errorf("op 0: got %T, want *AddLayerOp", ops[0])
	}
	if _, ok := ops[1].(*document.AddStrokeOp); !ok {
		t.Errorf("op 1: got %T, want *AddStrokeOp", ops[1])
	}
}
