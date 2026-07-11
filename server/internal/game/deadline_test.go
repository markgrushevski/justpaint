package game

import (
	"testing"
	"time"

	"github.com/markgrushevski/justpaint/server/internal/db"
)

// TestIsExpiredDrawing pins the single guard both Submit and resolveExpiry share:
// only a still-`drawing` round with a stamped deadline that the DB clock has reached
// counts as expired. Boundary (serverNow == deadline) is expired.
func TestIsExpiredDrawing(t *testing.T) {
	base := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	at := func(t time.Time) *time.Time { return &t }

	tests := []struct {
		name     string
		status   string
		deadline *time.Time
		now      time.Time
		want     bool
	}{
		{"drawing, now past deadline → expired", statusDrawing, at(base), base.Add(time.Second), true},
		{"drawing, now exactly at deadline → expired (>= boundary)", statusDrawing, at(base), base, true},
		{"drawing, now before deadline → not expired", statusDrawing, at(base), base.Add(-time.Second), false},
		{"drawing, nil deadline → not expired", statusDrawing, nil, base, false},
		{"open, past 'deadline' → not expired (wrong status)", statusOpen, at(base), base.Add(time.Hour), false},
		{"judging, past deadline → not expired", statusJudging, at(base), base.Add(time.Hour), false},
		{"done, past deadline → not expired", statusDone, at(base), base.Add(time.Hour), false},
		{"abandoned, past deadline → not expired", statusAbandoned, at(base), base.Add(time.Hour), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := db.GetMatchForUpdateRow{Status: tt.status, DrawingDeadline: tt.deadline, ServerNow: tt.now}
			if got := isExpiredDrawing(row); got != tt.want {
				t.Errorf("isExpiredDrawing = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestDecideExpiry covers the outcome selection by submitted-count and — the
// load-bearing assertion — that a forfeit maps the WIN to the submitter and the LOSS
// to the non-submitter by user_id, NOT by seat order.
func TestDecideExpiry(t *testing.T) {
	at := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	submitted := func(id string, rating int32) db.GetMatchPlayersForResolveRow {
		return db.GetMatchPlayersForResolveRow{UserID: id, SubmittedAt: &at, Rating: rating}
	}
	missing := func(id string, rating int32) db.GetMatchPlayersForResolveRow {
		return db.GetMatchPlayersForResolveRow{UserID: id, SubmittedAt: nil, Rating: rating}
	}

	t.Run("nobody submitted → abandoned", func(t *testing.T) {
		out, _ := decideExpiry([]db.GetMatchPlayersForResolveRow{missing("a", 1200), missing("b", 1200)})
		if out != outcomeAbandoned {
			t.Errorf("outcome = %v, want abandoned", out)
		}
	})

	t.Run("both submitted → judging (defensive)", func(t *testing.T) {
		out, _ := decideExpiry([]db.GetMatchPlayersForResolveRow{submitted("a", 1200), submitted("b", 1200)})
		if out != outcomeJudging {
			t.Errorf("outcome = %v, want judging", out)
		}
	})

	t.Run("one submitted → forfeit; the submitter wins regardless of seat", func(t *testing.T) {
		// Forfeiter is seat 0, submitter seat 1 — and the submitter is the LOWER-rated
		// underdog. A seat-order mapping would crown the forfeiter; user_id mapping
		// crowns the submitter.
		out, fr := decideExpiry([]db.GetMatchPlayersForResolveRow{
			missing("forfeiter", 1200),
			submitted("submitter", 1000),
		})
		if out != outcomeForfeit {
			t.Fatalf("outcome = %v, want forfeit", out)
		}
		if fr.winner == nil || *fr.winner != "submitter" {
			t.Fatalf("winner = %v, want submitter", fr.winner)
		}
		if fr.resolution != resolutionForfeit {
			t.Errorf("resolution = %q, want %q", fr.resolution, resolutionForfeit)
		}

		byID := map[string]playerResult{}
		for _, p := range fr.players {
			byID[p.userID] = p
		}
		sub, forf := byID["submitter"], byID["forfeiter"]
		if sub.score != nil || forf.score != nil {
			t.Error("forfeit scores must be nil for both (no judge ran)")
		}
		if sub.after <= sub.before {
			t.Errorf("submitter Elo did not rise: before=%d after=%d", sub.before, sub.after)
		}
		if forf.after >= forf.before {
			t.Errorf("forfeiter Elo did not fall: before=%d after=%d", forf.before, forf.after)
		}
		// Zero-sum: the submitter's gain equals the forfeiter's loss.
		if (sub.after - sub.before) != (forf.before - forf.after) {
			t.Errorf("not zero-sum: submitter +%d vs forfeiter -%d",
				sub.after-sub.before, forf.before-forf.after)
		}
	})

	t.Run("malformed single-seat roster → judging (defensive), no panic", func(t *testing.T) {
		// One submitted, ZERO missing (a degenerate non-two-player roster). The forfeit
		// branch requires len(missing)==1, so this must fall to the judging default
		// rather than indexing missing[0] out of range.
		out, _ := decideExpiry([]db.GetMatchPlayersForResolveRow{submitted("solo", 1200)})
		if out != outcomeJudging {
			t.Errorf("outcome = %v, want judging (defensive) for a 1-seat roster", out)
		}
	})
}
