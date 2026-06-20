package document_test

import (
	"strings"
	"testing"

	"github.com/markgrushevski/justpaint/server/internal/document"
)

const validMinimal = `{"version":1,"width":1920,"height":1080,"background":"#ffffff",
  "layers":[{"id":"l1","name":"Layer 1","visible":true,"opacity":1,"strokes":[
    {"id":"s1","type":"freehand","composite":"source-over","color":"#1b1b1b",
     "points":[[10,10,0.5],[20,20,0.6]],
     "brush":{"size":16,"thinning":0.5,"smoothing":0.5,"streamline":0.5,"simulatePressure":true,"taperStart":0,"taperEnd":0}}
  ]}]}`

const validFull = `{"version":1,"width":1920,"height":1080,"background":null,"layers":[
  {"id":"shapes","name":"Shapes","visible":true,"opacity":1,"strokes":[
    {"id":"r","type":"rect","composite":"source-over","x":200,"y":150,"width":400,"height":260,"cornerRadius":12,"fill":"#cfe8ff","stroke":"#0050a0","strokeWidth":3},
    {"id":"e","type":"ellipse","composite":"source-over","cx":1100,"cy":400,"rx":180,"ry":120,"fill":"#ffe0b3","stroke":null},
    {"id":"t","type":"polygon","composite":"source-over","points":[[800,700],[950,950],[650,950]],"fill":"#d9ffd9","stroke":"#1b7a1b","strokeWidth":2,"join":"round"},
    {"id":"ln","type":"line","composite":"source-over","points":[[100,1000],[1820,1000]],"stroke":"#333333","strokeWidth":4,"cap":"round","join":"round"}
  ]},
  {"id":"ink","name":"Ink","visible":true,"opacity":0.85,"strokes":[
    {"id":"pen","type":"freehand","composite":"source-over","color":"#1b1b1b","points":[[300,200,0.4],[340,230,0.6]],"brush":{"size":22,"thinning":0.6,"smoothing":0.5,"streamline":0.6,"simulatePressure":true,"taperStart":0,"taperEnd":12}},
    {"id":"erase","type":"freehand","composite":"destination-out","color":"#000000","points":[[380,235,0.9],[410,245,0.9]],"brush":{"size":30,"thinning":0,"smoothing":0.5,"streamline":0.4,"simulatePressure":false,"taperStart":0,"taperEnd":0}}
  ]}
]}`

// docWith wraps one or more stroke objects in an otherwise-valid one-layer doc.
func docWith(strokes string) string {
	return `{"version":1,"width":100,"height":100,"background":null,"layers":[` +
		`{"id":"L","name":"L","visible":true,"opacity":1,"strokes":[` + strokes + `]}]}`
}

func TestParseAndValidate(t *testing.T) {
	cases := []struct {
		name    string
		json    string
		wantErr bool
	}{
		// --- valid ---
		{"minimal", validMinimal, false},
		{"full tool set", validFull, false},
		{"unknown field tolerated (forward-compat)",
			`{"version":1,"width":10,"height":10,"background":null,"future":42,"layers":[{"id":"l","name":"L","visible":true,"opacity":1,"strokes":[]}]}`, false},
		{"rect with fill only (no stroke)", docWith(`{"id":"s","type":"rect","composite":"source-over","x":0,"y":0,"width":10,"height":10,"fill":"#000000"}`), false},
		{"shape stroke can erase (destination-out)", docWith(`{"id":"s","type":"rect","composite":"destination-out","x":0,"y":0,"width":10,"height":10,"fill":"#000000"}`), false},

		// --- document level ---
		{"bad version", `{"version":2,"width":10,"height":10,"background":null,"layers":[{"id":"l","name":"L","visible":true,"opacity":1,"strokes":[]}]}`, true},
		{"dimension too large", `{"version":1,"width":9000,"height":10,"background":null,"layers":[{"id":"l","name":"L","visible":true,"opacity":1,"strokes":[]}]}`, true},
		{"dimension zero", `{"version":1,"width":0,"height":10,"background":null,"layers":[{"id":"l","name":"L","visible":true,"opacity":1,"strokes":[]}]}`, true},
		{"no layers", `{"version":1,"width":10,"height":10,"background":null,"layers":[]}`, true},
		{"bad background color", `{"version":1,"width":10,"height":10,"background":"#xyz","layers":[{"id":"l","name":"L","visible":true,"opacity":1,"strokes":[]}]}`, true},
		{"layer opacity out of range", `{"version":1,"width":10,"height":10,"background":null,"layers":[{"id":"l","name":"L","visible":true,"opacity":1.5,"strokes":[]}]}`, true},
		{"duplicate id", docWith(`{"id":"L","type":"rect","composite":"source-over","x":0,"y":0,"width":10,"height":10,"fill":"#000000"}`), true},

		// --- stroke level ---
		{"unknown stroke type", docWith(`{"id":"s","type":"blob","composite":"source-over"}`), true},
		{"bad composite", docWith(`{"id":"s","type":"rect","composite":"xor","x":0,"y":0,"width":10,"height":10,"fill":"#000000"}`), true},
		{"freehand zero points", docWith(`{"id":"s","type":"freehand","composite":"source-over","color":"#000000","points":[],"brush":{"size":1,"thinning":0,"smoothing":0,"streamline":0,"simulatePressure":false,"taperStart":0,"taperEnd":0}}`), true},
		{"freehand pressure out of range", docWith(`{"id":"s","type":"freehand","composite":"source-over","color":"#000000","points":[[1,1,2]],"brush":{"size":1,"thinning":0,"smoothing":0,"streamline":0,"simulatePressure":false,"taperStart":0,"taperEnd":0}}`), true},
		{"line under two points", docWith(`{"id":"s","type":"line","composite":"source-over","points":[[0,0]],"stroke":"#000000","strokeWidth":1}`), true},
		{"line strokeWidth zero", docWith(`{"id":"s","type":"line","composite":"source-over","points":[[0,0],[1,1]],"stroke":"#000000","strokeWidth":0}`), true},
		{"rect zero area", docWith(`{"id":"s","type":"rect","composite":"source-over","x":0,"y":0,"width":0,"height":10,"fill":"#000000"}`), true},
		{"ellipse zero radius", docWith(`{"id":"s","type":"ellipse","composite":"source-over","cx":5,"cy":5,"rx":0,"ry":5,"fill":"#000000"}`), true},
		{"polygon under three vertices", docWith(`{"id":"s","type":"polygon","composite":"source-over","points":[[0,0],[10,10]],"fill":"#000000"}`), true},
		{"shape strokeWidth zero with stroke present", docWith(`{"id":"s","type":"rect","composite":"source-over","x":0,"y":0,"width":10,"height":10,"stroke":"#000000","strokeWidth":0}`), true},
		{"freehand point wrong arity (2 elems)", docWith(`{"id":"s","type":"freehand","composite":"source-over","color":"#000000","points":[[1,1]],"brush":{"size":1,"thinning":0,"smoothing":0,"streamline":0,"simulatePressure":false,"taperStart":0,"taperEnd":0}}`), true},
		{"line point wrong arity (3 elems)", docWith(`{"id":"s","type":"line","composite":"source-over","points":[[0,0,0],[1,1,1]],"stroke":"#000000","strokeWidth":1}`), true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := document.ParseAndValidate([]byte(tc.json))
			if tc.wantErr && err == nil {
				t.Fatalf("expected an error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// Per-stroke point cap is checked programmatically (10k+ points is impractical
// to inline).
func TestParseAndValidate_PointCap(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"id":"s","type":"line","composite":"source-over","stroke":"#000000","strokeWidth":1,"points":[`)
	for i := 0; i <= document.MaxPointsPerStroke; i++ { // one over the cap
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`[0,0]`)
	}
	b.WriteString(`]}`)

	if _, err := document.ParseAndValidate([]byte(docWith(b.String()))); err == nil {
		t.Fatalf("expected error for exceeding per-stroke point cap, got nil")
	}
}

// The union must decode into the right concrete types.
func TestParse_DecodesUnion(t *testing.T) {
	doc, err := document.Parse([]byte(validFull))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := map[document.StrokeType]int{}
	for _, layer := range doc.Layers {
		for _, s := range layer.Strokes {
			switch s.(type) {
			case *document.RectStroke:
				got[document.StrokeRect]++
			case *document.EllipseStroke:
				got[document.StrokeEllipse]++
			case *document.PolygonStroke:
				got[document.StrokePolygon]++
			case *document.LineStroke:
				got[document.StrokeLine]++
			case *document.FreehandStroke:
				got[document.StrokeFreehand]++
			default:
				t.Fatalf("unexpected concrete type %T", s)
			}
		}
	}
	want := map[document.StrokeType]int{
		document.StrokeRect: 1, document.StrokeEllipse: 1, document.StrokePolygon: 1,
		document.StrokeLine: 1, document.StrokeFreehand: 2,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %d, want %d", k, got[k], v)
		}
	}
}
