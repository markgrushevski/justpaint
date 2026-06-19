package document

import (
	"encoding/json"
	"fmt"
)

// ValidationError marks a client-fixable problem with a document: it maps to
// HTTP 400 validation_failed (docs/API.md §3). Internal errors are returned as
// ordinary errors instead.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

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

// Parse unmarshals a vector document. Unknown fields are tolerated
// (forward-compat, docs/DOCUMENT-FORMAT.md §7); structural problems surface as a
// *ValidationError.
func Parse(data []byte) (Document, error) {
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return Document{}, &ValidationError{Msg: "malformed document: " + err.Error()}
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
