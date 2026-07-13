package assist

import (
	"context"
	"strconv"
	"sync/atomic"

	"github.com/markgrushevski/justpaint/server/internal/document"
)

// FakeAssist is the zero-dependency default (docs/ASSIST.md §3, §6), mirroring
// render.StubRenderer / judge.FakeJudge: in-process and prompt-independent. It
// ignores the prompt and the doc summary and returns a fixed "house" op batch —
// an add_layer plus a rect body, a polygon roof, two rect windows, and a door —
// sized for the ~1080² editor canvas. The batch SHAPE is constant, but every call
// gets FRESH ids (a per-instance counter suffixed onto the layer + stroke ids,
// e.g. ai-house-3 / ai-house-3-body) so that after a user accepts one batch the
// next request — whose doc summary now carries the accepted layer id — does not
// collide on the single id namespace (document.ValidateOpBatch → duplicate id).
// The batch is self-contained (it creates its own layer) and passes
// document.ValidateOpBatch against any summary that does not already use its ids
// (verified in fake_test.go), so the whole client flow — prompt → ghost preview →
// accept → prompt again — is demonstrable with zero API dependency.
type FakeAssist struct {
	seq atomic.Uint64 // monotonic per-instance suffix source; keeps ids unique across calls
}

// NewFakeAssist returns the in-process fake assist.
func NewFakeAssist() *FakeAssist { return &FakeAssist{} }

var _ Assist = (*FakeAssist)(nil)

// GenerateOps implements Assist. It ignores ctx and req; the returned ops honor
// the Op contract exactly (single id namespace, non-freehand strokes, intra-batch
// layer references resolved in array order), so the handler's re-validation is a
// no-op for the fake and the accept path is fully exercised. Each call advances
// the instance counter so successive batches carry disjoint ids.
func (f *FakeAssist) GenerateOps(_ context.Context, _ Request) (Result, error) {
	n := f.seq.Add(1)
	return Result{Ops: houseBatch(n), Note: houseNote}, nil
}

const houseNote = "Drew a house: a rectangular body, a polygon roof, two windows, and a door (fake assist)."

// houseBatch builds the canned batch fresh on each call so a caller mutating the
// returned slice can't corrupt a shared instance. The structure and geometry are
// constant; only the ids vary — layerID is ai-house-<n> and every stroke id is
// suffixed onto it (ai-house-<n>-body, …), keeping the whole batch inside the
// single id namespace while staying unique across calls.
func houseBatch(n uint64) []document.Op {
	layerID := "ai-house-" + strconv.FormatUint(n, 10)
	sid := func(part string) string { return layerID + "-" + part }
	body := document.Color("#e8c9a0")
	frame := document.Color("#6b4423")
	roof := document.Color("#b0342a")
	roofEdge := document.Color("#7a2118")
	glass := document.Color("#a9d3f0")
	door := document.Color("#7a4a24")
	doorEdge := document.Color("#4a2c14")

	return []document.Op{
		&document.AddLayerOp{Kind: document.OpAddLayer, ID: layerID, Name: "AI House"},
		// Body.
		&document.AddStrokeOp{Kind: document.OpAddStroke, LayerID: layerID, Stroke: &document.RectStroke{
			StrokeBase:  document.StrokeBase{ID: sid("body"), Type: document.StrokeRect, Composite: document.CompositeSourceOver},
			X:           340,
			Y:           560,
			Width:       400,
			Height:      320,
			Fill:        &body,
			Stroke:      &frame,
			StrokeWidth: f64(6),
		}},
		// Roof (triangle over the body).
		&document.AddStrokeOp{Kind: document.OpAddStroke, LayerID: layerID, Stroke: &document.PolygonStroke{
			StrokeBase:  document.StrokeBase{ID: sid("roof"), Type: document.StrokePolygon, Composite: document.CompositeSourceOver},
			Points:      []document.Point{{300, 560}, {540, 380}, {780, 560}},
			Fill:        &roof,
			Stroke:      &roofEdge,
			StrokeWidth: f64(6),
		}},
		// Left window.
		&document.AddStrokeOp{Kind: document.OpAddStroke, LayerID: layerID, Stroke: &document.RectStroke{
			StrokeBase:  document.StrokeBase{ID: sid("window-left"), Type: document.StrokeRect, Composite: document.CompositeSourceOver},
			X:           400,
			Y:           620,
			Width:       90,
			Height:      90,
			Fill:        &glass,
			Stroke:      &frame,
			StrokeWidth: f64(4),
		}},
		// Right window.
		&document.AddStrokeOp{Kind: document.OpAddStroke, LayerID: layerID, Stroke: &document.RectStroke{
			StrokeBase:  document.StrokeBase{ID: sid("window-right"), Type: document.StrokeRect, Composite: document.CompositeSourceOver},
			X:           590,
			Y:           620,
			Width:       90,
			Height:      90,
			Fill:        &glass,
			Stroke:      &frame,
			StrokeWidth: f64(4),
		}},
		// Door.
		&document.AddStrokeOp{Kind: document.OpAddStroke, LayerID: layerID, Stroke: &document.RectStroke{
			StrokeBase:  document.StrokeBase{ID: sid("door"), Type: document.StrokeRect, Composite: document.CompositeSourceOver},
			X:           505,
			Y:           740,
			Width:       70,
			Height:      140,
			Fill:        &door,
			Stroke:      &doorEdge,
			StrokeWidth: f64(4),
		}},
	}
}

func f64(v float64) *float64 { return &v }
