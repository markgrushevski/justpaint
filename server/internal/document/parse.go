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
func (p *FreehandPoint) UnmarshalJSON(data []byte) error {
	var raw []float64
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if len(raw) != 3 {
		return fmt.Errorf("freehand point must be [x,y,pressure] (3 numbers), got %d", len(raw))
	}
	*p = FreehandPoint{raw[0], raw[1], raw[2]}
	return nil
}

func (p *Point) UnmarshalJSON(data []byte) error {
	var raw []float64
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if len(raw) != 2 {
		return fmt.Errorf("point must be [x,y] (2 numbers), got %d", len(raw))
	}
	*p = Point{raw[0], raw[1]}
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
	if err := Validate(doc); err != nil {
		return Document{}, err
	}
	return doc, nil
}
