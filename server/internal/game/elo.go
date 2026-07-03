package game

import "math"

// eloK is the flat K-factor for v1 — simple and responsive (docs/GAME.md §8).
// Tiered / provisional K is Phase 4.
const eloK = 32

// Actual-score S values (docs/GAME.md §8): win = 1, loss = 0, tie = shared half.
const (
	scoreWin  = 1.0
	scoreLoss = 0.0
	scoreTie  = 0.5
)

// computeElo returns both players' post-match ratings from their pre-match
// ratings and player A's actual score sa (1 win / 0 loss / 0.5 tie); B's is
// 1-sa. Standard Elo, K=32, round-half-away-from-zero (docs/GAME.md §8).
//
// It is zero-sum for a decisive result (and for a tie): A's gain equals B's loss,
// because the pre-rounding deltas are exact negatives and math.Round is symmetric.
func computeElo(ratingA, ratingB int, sa float64) (afterA, afterB int) {
	ea := expectedScore(ratingA, ratingB)
	eb := 1 - ea
	afterA = int(math.Round(float64(ratingA) + eloK*(sa-ea)))
	afterB = int(math.Round(float64(ratingB) + eloK*((1-sa)-eb)))
	return afterA, afterB
}

// expectedScore is P(A scores against B) under Elo: 1/(1+10^((Rb-Ra)/400)).
func expectedScore(ratingA, ratingB int) float64 {
	return 1 / (1 + math.Pow(10, float64(ratingB-ratingA)/400))
}
