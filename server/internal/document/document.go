// Package document is the Go mirror of the vector document schema
// (docs/DOCUMENT-FORMAT.md): types, parsing, and write-edge validation. It is
// the server-side half of the keystone contract; the canonical TS lives in
// packages/document. Both validate against the spec, not each other's code.
package document

// StrokeType is the discriminant of the Stroke union.
type StrokeType string

const (
	StrokeFreehand StrokeType = "freehand"
	StrokeLine     StrokeType = "line"
	StrokeRect     StrokeType = "rect"
	StrokeEllipse  StrokeType = "ellipse"
	StrokePolygon  StrokeType = "polygon"
)

// Composite is the per-stroke blend against earlier content on the same layer.
type Composite string

const (
	CompositeSourceOver     Composite = "source-over"
	CompositeDestinationOut Composite = "destination-out" // erase
)

type LineCap string

const (
	CapButt   LineCap = "butt"
	CapRound  LineCap = "round"
	CapSquare LineCap = "square"
)

type LineJoin string

const (
	JoinMiter LineJoin = "miter"
	JoinRound LineJoin = "round"
	JoinBevel LineJoin = "bevel"
)

// Color is a lowercase hex string: #rrggbb or #rrggbbaa.
type Color string

// FreehandPoint is [x, y, pressure] in logical coords; pressure ∈ [0,1].
type FreehandPoint [3]float64

// Point is [x, y] in logical coords.
type Point [2]float64

// BBox is an optional, derived axis-aligned bounds cache (advisory).
type BBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Document is the canonical vector drawing (docs/DOCUMENT-FORMAT.md §3).
type Document struct {
	Version    int     `json:"version"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	Background *Color  `json:"background"` // null = transparent
	Layers     []Layer `json:"layers"`
	Meta       *Meta   `json:"meta,omitempty"`
}

// Meta is advisory, non-rendering metadata.
type Meta struct {
	Generator       string `json:"generator,omitempty"`
	FreehandVersion string `json:"freehandVersion,omitempty"`
	CreatedAt       string `json:"createdAt,omitempty"`
	UpdatedAt       string `json:"updatedAt,omitempty"`
}

// Layer holds an ordered, bottom-to-top list of strokes. Strokes is a
// heterogeneous union; it is decoded by Layer.UnmarshalJSON (see parse.go).
type Layer struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Visible bool     `json:"visible"`
	Opacity float64  `json:"opacity"`
	Strokes []Stroke `json:"strokes"`
}

// Stroke is a sealed union: only the concrete types in this package implement
// it (the base() method is unexported).
type Stroke interface {
	base() *StrokeBase
}

// StrokeBase carries the fields every stroke shares. Embedded into each variant,
// so its JSON keys (id/type/composite/bbox) are promoted on the wire.
type StrokeBase struct {
	ID        string     `json:"id"`
	Type      StrokeType `json:"type"`
	Composite Composite  `json:"composite"`
	BBox      *BBox      `json:"bbox,omitempty"`
}

func (s *StrokeBase) base() *StrokeBase { return s }

// FreehandStroke is pen / brush / eraser (perfect-freehand input).
type FreehandStroke struct {
	StrokeBase
	Color  Color           `json:"color"`
	Points []FreehandPoint `json:"points"`
	Brush  BrushOptions    `json:"brush"`
}

// BrushOptions is the curated subset of perfect-freehand getStroke() options.
type BrushOptions struct {
	Size             float64 `json:"size"`
	Thinning         float64 `json:"thinning"`
	Smoothing        float64 `json:"smoothing"`
	Streamline       float64 `json:"streamline"`
	SimulatePressure bool    `json:"simulatePressure"`
	TaperStart       float64 `json:"taperStart"`
	TaperEnd         float64 `json:"taperEnd"`
}

// LineStroke is a straight line / open polyline (≥2 points).
type LineStroke struct {
	StrokeBase
	Points      []Point   `json:"points"`
	Stroke      Color     `json:"stroke"`
	StrokeWidth float64   `json:"strokeWidth"`
	Cap         *LineCap  `json:"cap,omitempty"`
	Join        *LineJoin `json:"join,omitempty"`
}

// RectStroke is a rectangle (top-left + size).
type RectStroke struct {
	StrokeBase
	X            float64  `json:"x"`
	Y            float64  `json:"y"`
	Width        float64  `json:"width"`
	Height       float64  `json:"height"`
	CornerRadius *float64 `json:"cornerRadius,omitempty"`
	Fill         *Color   `json:"fill,omitempty"`
	Stroke       *Color   `json:"stroke,omitempty"`
	StrokeWidth  *float64 `json:"strokeWidth,omitempty"`
}

// EllipseStroke is an ellipse (center + radii).
type EllipseStroke struct {
	StrokeBase
	CX          float64  `json:"cx"`
	CY          float64  `json:"cy"`
	RX          float64  `json:"rx"`
	RY          float64  `json:"ry"`
	Fill        *Color   `json:"fill,omitempty"`
	Stroke      *Color   `json:"stroke,omitempty"`
	StrokeWidth *float64 `json:"strokeWidth,omitempty"`
}

// PolygonStroke is a closed N-gon (triangle = 3 points).
type PolygonStroke struct {
	StrokeBase
	Points      []Point   `json:"points"`
	Fill        *Color    `json:"fill,omitempty"`
	Stroke      *Color    `json:"stroke,omitempty"`
	StrokeWidth *float64  `json:"strokeWidth,omitempty"`
	Join        *LineJoin `json:"join,omitempty"`
}
