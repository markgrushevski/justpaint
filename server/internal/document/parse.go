package document

import (
	"encoding/json"
	"fmt"
)

// ValidationError marks a client-fixable problem with a document: it maps to
// HTTP 400 validation_failed (docs/API.md §3). Internal errors are returned as
// ordinary errors instead.
type ValidationError struct {
	Msg string
	Err error // optional wrapped cause (preserves the error chain for errors.Is/As)
}

func (e *ValidationError) Error() string { return e.Msg }
func (e *ValidationError) Unwrap() error { return e.Err }

func invalid(format string, args ...any) error {
	return &ValidationError{Msg: fmt.Sprintf(format, args...)}
}

// UnmarshalJSON decodes a layer, dispatching each stroke on its "type"
// discriminant into the right concrete struct.
func (l *Layer) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID      string            `json:"id"`
		Name    string            `json:"name"`
		Visible bool              `json:"visible"`
		Opacity float64           `json:"opacity"`
		Strokes []json.RawMessage `json:"strokes"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	l.ID = raw.ID
	l.Name = raw.Name
	l.Visible = raw.Visible
	l.Opacity = raw.Opacity
	l.Strokes = make([]Stroke, 0, len(raw.Strokes))
	for i, rs := range raw.Strokes {
		s, err := unmarshalStroke(rs)
		if err != nil {
			return fmt.Errorf("stroke %d: %w", i, err)
		}
		l.Strokes = append(l.Strokes, s)
	}
	return nil
}

func unmarshalStroke(data []byte) (Stroke, error) {
	var head struct {
		Type StrokeType `json:"type"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, err
	}

	var s Stroke
	switch head.Type {
	case StrokeFreehand:
		s = &FreehandStroke{}
	case StrokeLine:
		s = &LineStroke{}
	case StrokeRect:
		s = &RectStroke{}
	case StrokeEllipse:
		s = &EllipseStroke{}
	case StrokePolygon:
		s = &PolygonStroke{}
	default:
		return nil, fmt.Errorf("unknown stroke type %q", head.Type)
	}

	if err := json.Unmarshal(data, s); err != nil {
		return nil, err
	}
	return s, nil
}

// UnmarshalJSON enforces point arity that fixed-size arrays alone do not:
// encoding/json silently zero-fills a short array and drops extra elements, so
// without this a 2-element freehand point or a 4-element one would be accepted.
// DOCUMENT-FORMAT §7 requires rejecting mismatched arity.
// Decoded into []*float64, not []float64: encoding/json coerces a null element to
// the zero value (so [null,1] would silently become [0,1]), whereas the TS
// validator rejects a non-finite coordinate — a null element must be a nil pointer
// we can reject, to keep the two point decoders at parity.
func (p *FreehandPoint) UnmarshalJSON(data []byte) error {
	var raw []*float64
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if len(raw) != 3 {
		return fmt.Errorf("freehand point must be [x,y,pressure] (3 numbers), got %d", len(raw))
	}
	if raw[0] == nil || raw[1] == nil || raw[2] == nil {
		return fmt.Errorf("freehand point coords must be numbers, not null")
	}
	*p = FreehandPoint{*raw[0], *raw[1], *raw[2]}
	return nil
}

func (p *Point) UnmarshalJSON(data []byte) error {
	var raw []*float64
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if len(raw) != 2 {
		return fmt.Errorf("point must be [x,y] (2 numbers), got %d", len(raw))
	}
	if raw[0] == nil || raw[1] == nil {
		return fmt.Errorf("point coords must be numbers, not null")
	}
	*p = Point{*raw[0], *raw[1]}
	return nil
}

// Parse unmarshals a vector document. Unknown fields are tolerated
// (forward-compat, docs/DOCUMENT-FORMAT.md §7); structural problems surface as a
// *ValidationError.
func Parse(data []byte) (Document, error) {
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return Document{}, &ValidationError{Msg: "malformed document: " + err.Error(), Err: err}
	}
	return doc, nil
}

// ParseAndValidate parses then fully validates a document — the write-edge entry
// point used by the drawings/game handlers.
func ParseAndValidate(data []byte) (Document, error) {
	doc, err := Parse(data)
	if err != nil {
		return Document{}, err
	}
	if err := requiredKeys(data); err != nil {
		return Document{}, err
	}
	if err := Validate(doc); err != nil {
		return Document{}, err
	}
	return doc, nil
}

// requiredKeys asserts every REQUIRED object key is physically present in the raw
// JSON. encoding/json silently zero-fills an absent required field — absent
// "visible" → false, "opacity" → 0.0, "brush" → the zero BrushOptions, "strokes"
// → nil, "background" → nil — so struct decoding alone would let Go accept
// documents the TS validator rejects, breaking keystone parity (docs/NOTES.md).
// Presence is checked on the raw bytes so an explicit null still counts as
// "present" (background may legitimately be null = transparent). Shape/type of the
// values is left to Validate; this only guards presence.
func requiredKeys(data []byte) error {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil // not an object / malformed — Parse + Validate report that
	}
	if err := requireFields(doc, "document", "version", "width", "height", "background", "layers"); err != nil {
		return err
	}

	var layers []map[string]json.RawMessage
	if err := json.Unmarshal(doc["layers"], &layers); err != nil {
		return nil // layers not an array of objects — Validate reports it
	}
	for i, layer := range layers {
		where := fmt.Sprintf("layer %d", i)
		if err := requireFields(layer, where, "id", "name", "visible", "opacity", "strokes"); err != nil {
			return err
		}

		var strokes []map[string]json.RawMessage
		if err := json.Unmarshal(layer["strokes"], &strokes); err != nil {
			continue
		}
		for j, stroke := range strokes {
			var typ StrokeType
			if raw, ok := stroke["type"]; ok {
				_ = json.Unmarshal(raw, &typ)
			}
			// brush is required only on freehand strokes (the other variants have
			// no brush); the zero BrushOptions would otherwise pass checkBrush.
			if typ == StrokeFreehand {
				if err := requireFields(stroke, fmt.Sprintf("%s stroke %d", where, j), "brush"); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func requireFields(obj map[string]json.RawMessage, where string, keys ...string) error {
	for _, k := range keys {
		if _, ok := obj[k]; !ok {
			return invalid("%s: %q is required", where, k)
		}
	}
	return nil
}
