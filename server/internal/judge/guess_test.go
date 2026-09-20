package judge

import (
	"context"
	"errors"
	"math"
	"slices"
	"strings"
	"testing"
)

func TestGuess_Validate(t *testing.T) {
	tests := []struct {
		name    string
		g       Guess
		wantErr bool
	}{
		{"a confident guess", Guess{Label: "a cat wearing a hat", Confidence: 0.82}, false},
		{"zero confidence is legal", Guess{Label: "a scribble", Confidence: 0}, false},
		{"full confidence is legal", Guess{Label: "a circle", Confidence: 1}, false},
		{"no alternatives is the expected shape", Guess{Label: "a house", Confidence: 0.9}, false},
		{"one alternative", Guess{Label: "a rabbit", Confidence: 0.5, Alternatives: []string{"an owl"}}, false},
		{"two alternatives", Guess{Label: "a rabbit", Confidence: 0.4, Alternatives: []string{"an owl", "a cat"}}, false},
		{"a label exactly at the cap", Guess{Label: strings.Repeat("a", maxGuessLabelLen), Confidence: 0.5}, false},
		// A cap in RUNES, not bytes: 70 two-byte runes are 140 bytes and still inside
		// an 80-character label.
		{"a multibyte label is counted in runes", Guess{Label: strings.Repeat("ф", 70), Confidence: 0.5}, false},

		{"confidence above 1", Guess{Label: "a cat", Confidence: 1.0001}, true},
		{"confidence below 0", Guess{Label: "a cat", Confidence: -0.0001}, true},
		{"NaN is not a confidence", Guess{Label: "a cat", Confidence: math.NaN()}, true},
		{"+Inf is not a confidence", Guess{Label: "a cat", Confidence: math.Inf(1)}, true},
		// The one place this is stricter than Critique: feedback may be empty, a guess
		// may not, because there is no other field for the answer to live in.
		{"an empty label is not a guess", Guess{Label: "", Confidence: 0.9}, true},
		{"a whitespace label is not a guess", Guess{Label: "   ", Confidence: 0.9}, true},
		{"a label one rune over the cap", Guess{Label: strings.Repeat("a", maxGuessLabelLen+1), Confidence: 0.5}, true},
		{"three alternatives", Guess{Label: "a cat", Confidence: 0.3, Alternatives: []string{"a", "b", "c"}}, true},
		{"a blank alternative is a hole in the list", Guess{Label: "a cat", Confidence: 0.3, Alternatives: []string{"an owl", "  "}}, true},
		{"an over-long alternative", Guess{Label: "a cat", Confidence: 0.3, Alternatives: []string{strings.Repeat("a", maxGuessLabelLen+1)}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.g.Validate()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Validate() = nil, want an error for %+v", tt.g)
				}
				if !errors.Is(err, ErrInvalidGuess) {
					t.Errorf("error %v should wrap ErrInvalidGuess", err)
				}
				return
			}
			if err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestFakeGuesser_Guess(t *testing.T) {
	fake := NewFakeGuesser()
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
			got, err := fake.Guess(ctx, img)
			if err != nil {
				t.Fatalf("Guess: %v", err)
			}
			if err := got.Validate(); err != nil {
				t.Errorf("the fake produced a guess violating its own contract: %v", err)
			}
			if math.Abs(got.Confidence-tt.coverage) > 0.05 {
				t.Errorf("confidence = %.3f, want ~%.3f (ink coverage)", got.Confidence, tt.coverage)
			}
			// The whole reason this type is allowed to exist: it must never let a player
			// believe a label came from looking at the picture. The disclaimer lives in
			// the LABEL because that is the one field the player is guaranteed to read.
			if !strings.Contains(got.Label, "sees ink, not meaning") {
				t.Errorf("label %q must say the fake never looked at the drawing", got.Label)
			}
			// No second opinion, because there was never a first one.
			if len(got.Alternatives) != 0 {
				t.Errorf("alternatives = %v, want none from a guesser that never looked", got.Alternatives)
			}
		})
	}

	// Deterministic in the bytes, like FakeCritic — tests and runs are reproducible.
	img := pngCoverage(t, 10, 10, 0.4)
	first, err := fake.Guess(ctx, img)
	if err != nil {
		t.Fatalf("Guess: %v", err)
	}
	second, err := fake.Guess(ctx, img)
	if err != nil {
		t.Fatalf("Guess: %v", err)
	}
	if first.Label != second.Label || first.Confidence != second.Confidence ||
		!slices.Equal(first.Alternatives, second.Alternatives) {
		t.Errorf("the same bytes produced %+v then %+v — the fake must be deterministic", first, second)
	}
}

func TestFakeGuesser_RejectsUndecodableImage(t *testing.T) {
	if _, err := NewFakeGuesser().Guess(context.Background(), []byte("not a png")); err == nil {
		t.Fatal("expected an error on an undecodable image, got a guess")
	}
}
