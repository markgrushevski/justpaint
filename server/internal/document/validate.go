package document

import (
	"math"
	"regexp"
	"unicode/utf8"
)

// Limits — the DoS caps (docs/API.md §6) and structural bounds
// (docs/DOCUMENT-FORMAT.md §7). The 8 MB request-body cap is enforced earlier,
// at the HTTP layer (http.MaxBytesReader), not here.
const (
	MaxCanvasDimension = 8192
	MaxLayers          = 64
	MaxStrokes         = 5_000
	MaxPointsPerStroke = 10_000
	MaxTotalPoints     = 100_000 // the binding semantic cap

	maxIDLen   = 64
	maxNameLen = 64
)

var hexColorRe = regexp.MustCompile(`^#([0-9a-f]{6}|[0-9a-f]{8})$`)

// Validate enforces the document invariants. It returns a *ValidationError on
// any client-fixable problem (→ 400). Defaults (e.g. omitted strokeWidth) are
// the consumer's concern; here we only reject what is actually invalid.
func Validate(doc Document) error {
	if doc.Version != 1 {
		return invalid("unknown document version %d", doc.Version)
	}
	if doc.Width < 1 || doc.Width > MaxCanvasDimension || doc.Height < 1 || doc.Height > MaxCanvasDimension {
		return invalid("width and height must be in [1, %d]", MaxCanvasDimension)
	}
	if doc.Background != nil {
		if err := checkColor(*doc.Background, "background"); err != nil {
			return err
		}
	}
	if len(doc.Layers) < 1 {
		return invalid("document must have at least one layer")
	}
	if len(doc.Layers) > MaxLayers {
		return invalid("too many layers: %d (max %d)", len(doc.Layers), MaxLayers)
	}

	ids := make(map[string]struct{}) // single id namespace across layers + strokes
	totalStrokes := 0
	totalPoints := 0

	for i := range doc.Layers {
		layer := &doc.Layers[i]
		if err := checkID(layer.ID, ids); err != nil {
			return err
		}
		if n := utf8.RuneCountInString(layer.Name); n < 1 || n > maxNameLen {
			return invalid("layer %s: name must be 1-%d chars", layer.ID, maxNameLen)
		}
		if !finite(layer.Opacity) || layer.Opacity < 0 || layer.Opacity > 1 {
			return invalid("layer %s: opacity must be in [0,1]", layer.ID)
		}

		totalStrokes += len(layer.Strokes)
		if totalStrokes > MaxStrokes {
			return invalid("too many strokes (max %d)", MaxStrokes)
		}

		for _, s := range layer.Strokes {
			points, err := checkStroke(s, ids)
			if err != nil {
				return err
			}
			totalPoints += points
			if totalPoints > MaxTotalPoints {
				return invalid("too many total points (max %d)", MaxTotalPoints)
			}
		}
	}
	return nil
}

// checkStroke validates one stroke and returns its input-point count (0 for the
// parametric shapes rect/ellipse, which don't carry input points).
func checkStroke(s Stroke, ids map[string]struct{}) (int, error) {
	b := s.base()
	if err := checkID(b.ID, ids); err != nil {
		return 0, err
	}
	if b.Composite != CompositeSourceOver && b.Composite != CompositeDestinationOut {
		return 0, invalid("stroke %s: invalid composite %q", b.ID, b.Composite)
	}

	switch st := s.(type) {
	case *FreehandStroke:
		return checkFreehand(st)
	case *LineStroke:
		return checkLine(st)
	case *PolygonStroke:
		return checkPolygon(st)
	case *RectStroke:
		return 0, checkRect(st)
	case *EllipseStroke:
		return 0, checkEllipse(st)
	default:
		return 0, invalid("stroke %s: unknown type", b.ID)
	}
}

func checkFreehand(s *FreehandStroke) (int, error) {
	if err := checkColor(s.Color, "freehand "+s.ID+" color"); err != nil {
		return 0, err
	}
	n := len(s.Points)
	if n < 1 {
		return 0, invalid("freehand %s: needs at least 1 point", s.ID)
	}
	if n > MaxPointsPerStroke {
		return 0, invalid("freehand %s: too many points (max %d)", s.ID, MaxPointsPerStroke)
	}
	for _, p := range s.Points {
		if !finite(p[0]) || !finite(p[1]) || !finite(p[2]) {
			return 0, invalid("freehand %s: non-finite point", s.ID)
		}
		if p[2] < 0 || p[2] > 1 {
			return 0, invalid("freehand %s: pressure must be in [0,1]", s.ID)
		}
	}
	if err := checkBrush(s.Brush, s.ID); err != nil {
		return 0, err
	}
	return n, nil
}

func checkBrush(b BrushOptions, id string) error {
	switch {
	case !finite(b.Size) || b.Size < 0:
		return invalid("freehand %s: brush.size must be >= 0", id)
	case !finite(b.Thinning) || b.Thinning < -1 || b.Thinning > 1:
		return invalid("freehand %s: brush.thinning must be in [-1,1]", id)
	case !finite(b.Smoothing) || b.Smoothing < 0 || b.Smoothing > 1:
		return invalid("freehand %s: brush.smoothing must be in [0,1]", id)
	case !finite(b.Streamline) || b.Streamline < 0 || b.Streamline > 1:
		return invalid("freehand %s: brush.streamline must be in [0,1]", id)
	case !finite(b.TaperStart) || b.TaperStart < 0 || !finite(b.TaperEnd) || b.TaperEnd < 0:
		return invalid("freehand %s: brush tapers must be >= 0", id)
	}
	return nil
}

func checkLine(s *LineStroke) (int, error) {
	n := len(s.Points)
	if n < 2 {
		return 0, invalid("line %s: needs at least 2 points", s.ID)
	}
	if n > MaxPointsPerStroke {
		return 0, invalid("line %s: too many points (max %d)", s.ID, MaxPointsPerStroke)
	}
	if err := checkPoints(s.Points, "line "+s.ID); err != nil {
		return 0, err
	}
	if err := checkColor(s.Stroke, "line "+s.ID+" stroke"); err != nil {
		return 0, err
	}
	if !finite(s.StrokeWidth) || s.StrokeWidth <= 0 {
		return 0, invalid("line %s: strokeWidth must be > 0", s.ID)
	}
	if s.Cap != nil && !validCap(*s.Cap) {
		return 0, invalid("line %s: invalid cap %q", s.ID, *s.Cap)
	}
	if s.Join != nil && !validJoin(*s.Join) {
		return 0, invalid("line %s: invalid join %q", s.ID, *s.Join)
	}
	return n, nil
}

func checkPolygon(s *PolygonStroke) (int, error) {
	n := len(s.Points)
	if n < 3 {
		return 0, invalid("polygon %s: needs at least 3 vertices", s.ID)
	}
	if n > MaxPointsPerStroke {
		return 0, invalid("polygon %s: too many vertices (max %d)", s.ID, MaxPointsPerStroke)
	}
	if err := checkPoints(s.Points, "polygon "+s.ID); err != nil {
		return 0, err
	}
	if s.Join != nil && !validJoin(*s.Join) {
		return 0, invalid("polygon %s: invalid join %q", s.ID, *s.Join)
	}
	return n, checkFillStroke(s.ID, s.Fill, s.Stroke, s.StrokeWidth)
}

func checkRect(s *RectStroke) error {
	if !finite(s.X) || !finite(s.Y) || !finite(s.Width) || !finite(s.Height) {
		return invalid("rect %s: non-finite geometry", s.ID)
	}
	if s.Width <= 0 || s.Height <= 0 {
		return invalid("rect %s: width and height must be > 0 (zero-area rejected)", s.ID)
	}
	if s.CornerRadius != nil && (!finite(*s.CornerRadius) || *s.CornerRadius < 0) {
		return invalid("rect %s: cornerRadius must be >= 0", s.ID)
	}
	return checkFillStroke(s.ID, s.Fill, s.Stroke, s.StrokeWidth)
}

func checkEllipse(s *EllipseStroke) error {
	if !finite(s.CX) || !finite(s.CY) || !finite(s.RX) || !finite(s.RY) {
		return invalid("ellipse %s: non-finite geometry", s.ID)
	}
	if s.RX <= 0 || s.RY <= 0 {
		return invalid("ellipse %s: rx and ry must be > 0", s.ID)
	}
	return checkFillStroke(s.ID, s.Fill, s.Stroke, s.StrokeWidth)
}

// checkFillStroke validates the optional fill/stroke channels shared by the
// shapes. strokeWidth must be > 0 *when the stroke channel is present* — use a
// null/absent stroke to omit an outline, not strokeWidth 0
// (docs/DOCUMENT-FORMAT.md §7).
func checkFillStroke(id string, fill, stroke *Color, strokeWidth *float64) error {
	if fill != nil {
		if err := checkColor(*fill, "shape "+id+" fill"); err != nil {
			return err
		}
	}
	if stroke != nil {
		if err := checkColor(*stroke, "shape "+id+" stroke"); err != nil {
			return err
		}
		if strokeWidth != nil && (!finite(*strokeWidth) || *strokeWidth <= 0) {
			return invalid("shape %s: strokeWidth must be > 0 when stroke is present", id)
		}
	}
	return nil
}

// --- small helpers ---

func checkPoints(points []Point, what string) error {
	for _, p := range points {
		if !finite(p[0]) || !finite(p[1]) {
			return invalid("%s: non-finite point", what)
		}
	}
	return nil
}

func checkColor(c Color, what string) error {
	if !hexColorRe.MatchString(string(c)) {
		return invalid("%s: invalid color %q (want #rrggbb or #rrggbbaa)", what, c)
	}
	return nil
}

func checkID(id string, seen map[string]struct{}) error {
	if id == "" || utf8.RuneCountInString(id) > maxIDLen {
		return invalid("id must be 1-%d chars (got %q)", maxIDLen, id)
	}
	if _, dup := seen[id]; dup {
		return invalid("duplicate id %q", id)
	}
	seen[id] = struct{}{}
	return nil
}

func finite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }

func validCap(c LineCap) bool   { return c == CapButt || c == CapRound || c == CapSquare }
func validJoin(j LineJoin) bool { return j == JoinMiter || j == JoinRound || j == JoinBevel }
