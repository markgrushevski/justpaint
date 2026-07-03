package game

import "testing"

func TestComputeElo(t *testing.T) {
	tests := []struct {
		name         string
		ra, rb       int
		sa           float64
		wantA, wantB int
	}{
		// Equal ratings: E = 0.5 each. Win moves ±16 (K/2).
		{"equal, A wins", 1200, 1200, scoreWin, 1216, 1184},
		{"equal, B wins", 1200, 1200, scoreLoss, 1184, 1216},
		{"equal, tie", 1200, 1200, scoreTie, 1200, 1200},
		// Favorite (A, +200) beats underdog: small gain. E_A ≈ 0.7597.
		{"favorite A wins", 1400, 1200, scoreWin, 1408, 1192},
		// Upset: underdog A (-200) beats favorite: big gain.
		{"underdog A wins", 1200, 1400, scoreWin, 1224, 1376},
		// Tie between unequal: the lower-rated gains, the higher loses.
		{"unequal tie", 1400, 1200, scoreTie, 1392, 1208},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotA, gotB := computeElo(tt.ra, tt.rb, tt.sa)
			if gotA != tt.wantA || gotB != tt.wantB {
				t.Errorf("computeElo(%d,%d,%.1f) = (%d,%d), want (%d,%d)",
					tt.ra, tt.rb, tt.sa, gotA, gotB, tt.wantA, tt.wantB)
			}
			// Elo is zero-sum: total rating is conserved regardless of outcome.
			if gotA+gotB != tt.ra+tt.rb {
				t.Errorf("rating not conserved: %d+%d=%d, want %d",
					gotA, gotB, gotA+gotB, tt.ra+tt.rb)
			}
		})
	}
}

func TestExpectedScore_Symmetry(t *testing.T) {
	// E_A(a,b) + E_B(b,a) must sum to 1 for any pair.
	if e := expectedScore(1500, 1300) + expectedScore(1300, 1500); e < 0.999999 || e > 1.000001 {
		t.Errorf("expected scores sum to %.6f, want 1.0", e)
	}
	// Equal ratings ⇒ 0.5.
	if e := expectedScore(1200, 1200); e != 0.5 {
		t.Errorf("equal-rating expected = %.6f, want 0.5", e)
	}
}
