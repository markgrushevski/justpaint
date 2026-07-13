package document

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

// MaxOpsPerBatch caps the number of ops in one AI-assist batch (docs/ASSIST.md
// §2). Per-batch, not per-document — the assist endpoint has only the summary.
const MaxOpsPerBatch = 64

// OpKind is the discriminant of the Op union.
type OpKind string

const (
	OpAddLayer  OpKind = "add_layer"
	OpAddStroke OpKind = "add_stroke"
)

// Op is a sealed union of AI-assist operations: only the concrete op types in
// this package implement it (opKind() is unexported). The Go mirror of the TS
// Op union (packages/document/src/types.ts); both validate against ASSIST.md §2,
// not each other's code.
type Op interface {
	opKind() OpKind
}

// AddLayerOp appends a new layer with an LLM-assigned id + name. The id lives in
// the same single namespace as document layer/stroke ids.
type AddLayerOp struct {
	Kind OpKind `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (o *AddLayerOp) opKind() OpKind { return OpAddLayer }

// AddStrokeOp appends a stroke onto a layer that resolves to either a summary
// layer id or an earlier add_layer id in the same batch. Stroke is the shared
// Stroke union, but validation rejects freehand (ASSIST.md §2).
type AddStrokeOp struct {
	Kind    OpKind `json:"kind"`
	LayerID string `json:"layerId"`
	Stroke  Stroke `json:"stroke"`
}

func (o *AddStrokeOp) opKind() OpKind { return OpAddStroke }

// UnmarshalJSON decodes an add_stroke op, dispatching its stroke on the "type"
// discriminant like Layer.UnmarshalJSON does (Stroke is an interface and cannot
// be decoded directly).
func (o *AddStrokeOp) UnmarshalJSON(data []byte) error {
	var raw struct {
		Kind    OpKind          `json:"kind"`
		LayerID string          `json:"layerId"`
		Stroke  json.RawMessage `json:"stroke"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	o.Kind = raw.Kind
	o.LayerID = raw.LayerID
	if len(raw.Stroke) > 0 {
		s, err := unmarshalStroke(raw.Stroke)
		if err != nil {
			return err
		}
		o.Stroke = s
	}
	return nil
}

// SummaryCanvas is the logical canvas size in a DocSummary.
type SummaryCanvas struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// SummaryLayer is one layer entry in a DocSummary.
type SummaryLayer struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	StrokeCount int    `json:"strokeCount"`
}

// DocSummary is the minimal document summary the assist endpoint receives (Phase
// A): just enough to seed the id namespace and resolve layer references, never
// the full document (docs/DESIGN-ASSIST-PHASE-A.md §1 resolution 3). It is the
// client's own already-validated data, so it is trusted here — only the ops are
// validated. Mirror of the TS DocSummary.
type DocSummary struct {
	Canvas SummaryCanvas  `json:"canvas"`
	Layers []SummaryLayer `json:"layers"`
}

// ParseAndValidateOpBatch decodes a raw op-batch JSON array, enforces required
// keys, then validates it against the summary — the assist entry point mirroring
// ParseAndValidate.
func ParseAndValidateOpBatch(data []byte, summary DocSummary) ([]Op, error) {
	ops, err := decodeOpBatch(data)
	if err != nil {
		return nil, err
	}
	if err := requiredOpKeys(data); err != nil {
		return nil, err
	}
	if err := ValidateOpBatch(ops, summary); err != nil {
		return nil, err
	}
	return ops, nil
}

// decodeOpBatch decodes the op array, dispatching each op on its "kind"
// discriminant (mirrors unmarshalStroke's dispatch on "type").
func decodeOpBatch(data []byte) ([]Op, error) {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, &ValidationError{Msg: "malformed op batch: " + err.Error(), Err: err}
	}
	ops := make([]Op, 0, len(raw))
	for i, ro := range raw {
		op, err := unmarshalOp(ro)
		if err != nil {
			return nil, fmt.Errorf("op %d: %w", i, err)
		}
		ops = append(ops, op)
	}
	return ops, nil
}

func unmarshalOp(data []byte) (Op, error) {
	var head struct {
		Kind OpKind `json:"kind"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, err
	}
	switch head.Kind {
	case OpAddLayer:
		var o AddLayerOp
		if err := json.Unmarshal(data, &o); err != nil {
			return nil, err
		}
		return &o, nil
	case OpAddStroke:
		var o AddStrokeOp
		if err := json.Unmarshal(data, &o); err != nil {
			return nil, err
		}
		return &o, nil
	default:
		return nil, invalid("unknown op kind %q", head.Kind)
	}
}

// requiredOpKeys asserts every REQUIRED op key is physically present in the raw
// JSON. encoding/json silently zero-fills an absent field — absent
// add_layer.name → "", absent add_stroke.layerId → "" — so struct decoding alone
// would let Go accept ops the TS validator rejects, breaking keystone parity
// (mirrors requiredKeys in parse.go). Value/shape is left to ValidateOpBatch.
func requiredOpKeys(data []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil // not an array — decodeOpBatch reports it
	}
	for i, ro := range raw {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(ro, &obj); err != nil {
			continue // not an object — decode/validate reports it
		}
		where := fmt.Sprintf("op %d", i)
		if err := requireFields(obj, where, "kind"); err != nil {
			return err
		}
		var kind OpKind
		if rawKind, ok := obj["kind"]; ok {
			_ = json.Unmarshal(rawKind, &kind)
		}
		switch kind {
		case OpAddLayer:
			if err := requireFields(obj, where, "id", "name"); err != nil {
				return err
			}
		case OpAddStroke:
			if err := requireFields(obj, where, "layerId", "stroke"); err != nil {
				return err
			}
		}
	}
	return nil
}

// ValidateOpBatch validates a decoded op batch against a doc summary. It seeds
// the single shared id namespace from the summary's layer ids, then validates
// each op IN ARRAY ORDER:
//   - add_layer: checkID against the shared namespace (collision with a summary
//     id or an intra-batch duplicate → error) + the checkStroke/validateLayer
//     name rule; then the id becomes a resolvable layer ref.
//   - add_stroke: layerId must resolve to a summary layer id or an add_layer id
//     appearing EARLIER in this batch (a dangling or forward reference → error);
//     then the stroke is validated by the reused checkStroke, freehand rejected.
//
// Only per-op and per-batch caps are enforced here (batch size; per-stroke point
// count comes free from checkStroke). Whole-document caps (MaxLayers/MaxStrokes/
// MaxTotalPoints) stay at the save write-edge — this validator has only the
// summary, never the full document. Mirrors the TS validateOpBatch.
func ValidateOpBatch(ops []Op, summary DocSummary) error {
	if len(ops) > MaxOpsPerBatch {
		return invalid("too many ops: %d (max %d)", len(ops), MaxOpsPerBatch)
	}

	ids := make(map[string]struct{})       // single id namespace across layers + strokes
	layerRefs := make(map[string]struct{}) // resolvable layer ids (summary + earlier add_layer)
	for _, l := range summary.Layers {
		ids[l.ID] = struct{}{}
		layerRefs[l.ID] = struct{}{}
	}

	for i, op := range ops {
		switch o := op.(type) {
		case *AddLayerOp:
			if err := checkID(o.ID, ids); err != nil {
				return fmt.Errorf("op %d: %w", i, err)
			}
			if n := utf8.RuneCountInString(o.Name); n < 1 || n > maxNameLen {
				return invalid("op %d: name must be 1-%d chars", i, maxNameLen)
			}
			layerRefs[o.ID] = struct{}{}
		case *AddStrokeOp:
			if _, ok := layerRefs[o.LayerID]; !ok {
				return invalid("op %d: unknown layer id %q", i, o.LayerID)
			}
			if err := checkOpStroke(o.Stroke, ids); err != nil {
				return fmt.Errorf("op %d: %w", i, err)
			}
		default:
			return invalid("op %d: unknown op kind", i)
		}
	}
	return nil
}

// checkOpStroke validates the stroke of an add_stroke op: reuse checkStroke
// verbatim, but reject freehand (excluded from AI ops — ASSIST.md §2). The base
// checkStroke accepts freehand, so this op-level allowlist is required; it is not
// implied by reuse.
func checkOpStroke(s Stroke, ids map[string]struct{}) error {
	if s == nil {
		return invalid("stroke is required")
	}
	if s.base().Type == StrokeFreehand {
		return invalid("freehand strokes are not allowed in ops")
	}
	_, err := checkStroke(s, ids)
	return err
}
