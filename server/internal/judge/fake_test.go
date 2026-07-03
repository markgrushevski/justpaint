package judge

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"reflect"
	"testing"
)

// pngCoverage builds a w×h opaque-white PNG with the top `coverage` fraction of
// rows painted black, so ink coverage == rows/h ≈ coverage.
func pngCoverage(t *testing.T, w, h int, coverage float64) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	rows := int(coverage*float64(h) + 0.5)
	for y := 0; y < rows; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.Black)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestFakeJudge_Score(t *testing.T) {
	tests := []struct {
		name       string
		covA, covB float64
		wantWinner string
	}{
		{"A wins on more coverage", 0.50, 0.10, WinnerA},
		{"B wins on more coverage", 0.10, 0.50, WinnerB},
		{"equal coverage is a tie", 0.30, 0.30, WinnerTie},
		{"within epsilon is a tie", 0.30, 0.31, WinnerTie},
		{"blank vs blank is a tie", 0.0, 0.0, WinnerTie},
	}
	fake := NewFakeJudge()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := Request{
				Prompt: "a fox riding a bicycle",
				ImageA: pngCoverage(t, 20, 100, tt.covA),
				ImageB: pngCoverage(t, 20, 100, tt.covB),
			}
			res, err := fake.Score(context.Background(), req)
			if err != nil {
				t.Fatalf("Score: %v", err)
			}
			if res.Winner != tt.wantWinner {
				t.Errorf("winner = %q, want %q", res.Winner, tt.wantWinner)
			}
			if math.Abs(res.ScoreA-tt.covA) > 0.001 {
				t.Errorf("scoreA = %v, want ~%v", res.ScoreA, tt.covA)
			}
			if math.Abs(res.ScoreB-tt.covB) > 0.001 {
				t.Errorf("scoreB = %v, want ~%v", res.ScoreB, tt.covB)
			}
			if res.Reason == "" {
				t.Error("reason is empty")
			}
			if err := res.Validate(); err != nil {
				t.Errorf("result violates the §2 contract: %v", err)
			}
		})
	}
}

func TestFakeJudge_Deterministic(t *testing.T) {
	fake := NewFakeJudge()
	req := Request{
		ImageA: pngCoverage(t, 32, 32, 0.4),
		ImageB: pngCoverage(t, 32, 32, 0.2),
	}
	r1, err := fake.Score(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := fake.Score(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r1, r2) {
		t.Errorf("non-deterministic: %+v vs %+v", r1, r2)
	}
}

func TestFakeJudge_InvalidPNG(t *testing.T) {
	fake := NewFakeJudge()
	_, err := fake.Score(context.Background(), Request{
		ImageA: []byte("not a png"),
		ImageB: pngCoverage(t, 8, 8, 0.5),
	})
	if err == nil {
		t.Fatal("expected a decode error for a malformed image")
	}
}

func TestResult_Validate(t *testing.T) {
	long := make([]byte, maxReasonLen+1)
	for i := range long {
		long[i] = 'x'
	}
	tests := []struct {
		name    string
		res     Result
		wantErr bool
	}{
		{"valid A", Result{ScoreA: 0.8, ScoreB: 0.6, Winner: WinnerA, Reason: "ok"}, false},
		{"valid tie, empty reason", Result{ScoreA: 0.5, ScoreB: 0.5, Winner: WinnerTie}, false},
		{"score above 1", Result{ScoreA: 1.2, ScoreB: 0.5, Winner: WinnerA}, true},
		{"score below 0", Result{ScoreA: -0.1, ScoreB: 0.5, Winner: WinnerB}, true},
		{"NaN score", Result{ScoreA: math.NaN(), ScoreB: 0.5, Winner: WinnerA}, true},
		{"Inf score", Result{ScoreA: math.Inf(1), ScoreB: 0.5, Winner: WinnerA}, true},
		{"bad winner", Result{ScoreA: 0.5, ScoreB: 0.5, Winner: "left"}, true},
		{"reason too long", Result{ScoreA: 0.5, ScoreB: 0.5, Winner: WinnerTie, Reason: string(long)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.res.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
