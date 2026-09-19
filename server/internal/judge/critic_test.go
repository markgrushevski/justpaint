package judge

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
)

func TestCritique_Validate(t *testing.T) {
	tests := []struct {
		name    string
		c       Critique
		wantErr bool
	}{
		{"a mid score", Critique{Score: 0.72, Feedback: "Your fox reads clearly."}, false},
		{"zero is legal", Critique{Score: 0}, false},
		{"one is legal", Critique{Score: 1}, false},
		{"empty feedback is legal", Critique{Score: 0.5, Feedback: ""}, false},
		{"feedback exactly at the cap", Critique{Score: 0.5, Feedback: strings.Repeat("a", maxFeedbackLen)}, false},
		{"score above 1", Critique{Score: 1.0001}, true},
		{"score below 0", Critique{Score: -0.0001}, true},
		{"NaN is not a score", Critique{Score: math.NaN()}, true},
		{"+Inf is not a score", Critique{Score: math.Inf(1)}, true},
		{"feedback one rune over the cap", Critique{Score: 0.5, Feedback: strings.Repeat("a", maxFeedbackLen+1)}, true},
		// A cap in RUNES, not bytes: 300 three-byte runes are 900 bytes and still
		// well inside a 500-character limit.
		{"multibyte feedback is counted in runes", Critique{Score: 0.5, Feedback: strings.Repeat("ф", 300)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.c.Validate()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Validate() = nil, want an error for %+v", tt.c)
				}
				if !errors.Is(err, ErrInvalidCritique) {
					t.Errorf("error %v should wrap ErrInvalidCritique", err)
				}
				return
			}
			if err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestFakeCritic_Critique(t *testing.T) {
	fake := NewFakeCritic()
	ctx := context.Background()

	tests := []struct {
		name     string
		coverage float64
	}{
		{"a blank canvas", 0},
		{"a light sketch", 0.1},
		{"a busy drawing", 0.6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img := pngCoverage(t, 10, 10, tt.coverage)
			got, err := fake.Critique(ctx, CritiqueRequest{Prompt: "a fox riding a bicycle", Image: img})
			if err != nil {
				t.Fatalf("Critique: %v", err)
			}
			if err := got.Validate(); err != nil {
				t.Errorf("the fake produced a critique violating its own contract: %v", err)
			}
			if math.Abs(got.Score-tt.coverage) > 0.05 {
				t.Errorf("score = %.3f, want ~%.3f (ink coverage)", got.Score, tt.coverage)
			}
			// The whole reason this type is allowed to exist: it must never let a
			// player believe a number came from reading the prompt.
			if !strings.Contains(got.Feedback, "has not read the prompt") {
				t.Errorf("feedback %q must say the fake never read the prompt", got.Feedback)
			}
		})
	}

	// Deterministic in the bytes, like FakeJudge — tests and runs are reproducible.
	img := pngCoverage(t, 10, 10, 0.4)
	first, err := fake.Critique(ctx, CritiqueRequest{Prompt: "a", Image: img})
	if err != nil {
		t.Fatalf("Critique: %v", err)
	}
	second, err := fake.Critique(ctx, CritiqueRequest{Prompt: "a completely different prompt", Image: img})
	if err != nil {
		t.Fatalf("Critique: %v", err)
	}
	if first != second {
		t.Errorf("the same bytes produced %+v then %+v — the fake must be deterministic and prompt-independent", first, second)
	}
}

func TestFakeCritic_RejectsUndecodableImage(t *testing.T) {
	if _, err := NewFakeCritic().Critique(context.Background(), CritiqueRequest{Prompt: "x", Image: []byte("not a png")}); err == nil {
		t.Fatal("expected an error on an undecodable image, got a score")
	}
}
