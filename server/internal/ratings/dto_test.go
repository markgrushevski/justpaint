package ratings

import (
	"testing"

	"github.com/markgrushevski/justpaint/server/internal/db"
)

func strptr(s string) *string { return &s }

// TestParseLimit pins the clamp semantics: default 20, max 100, and every bad input
// folds to a valid limit rather than erroring — the endpoint has no 400 path
// (docs/API.md §11).
func TestParseLimit(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"blank → default", "", 20},
		{"in range", "5", 5},
		{"exactly max", "100", 100},
		{"over max clamps", "500", 100},
		{"one over max clamps", "101", 100},
		{"zero → default", "0", 20},
		{"negative → default", "-3", 20},
		{"non-numeric → default", "abc", 20},
		{"lower bound", "1", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseLimit(c.in); got != c.want {
				t.Errorf("parseLimit(%q) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}

// TestBuildLeaderboard pins the pure DTO build: rank is the 1-based row number over
// the already-ordered rows, int32 db fields map to JSON ints, a nil display_name
// maps straight through to a null displayName, and the response echoes the clamped
// limit (docs/API.md §11).
func TestBuildLeaderboard(t *testing.T) {
	t.Run("ranks are row-number; fields and nullable displayName map through", func(t *testing.T) {
		rows := []db.ListTopRatingsRow{
			{ID: "u-ada", DisplayName: strptr("Ada"), Rating: 1432, GamesPlayed: 13, Wins: 10, Losses: 3},
			{ID: "u-bo", DisplayName: nil, Rating: 1300, GamesPlayed: 4, Wins: 1, Losses: 3},
			{ID: "u-cy", DisplayName: strptr("Cy"), Rating: 1200, GamesPlayed: 2, Wins: 1, Losses: 1},
		}
		resp := buildLeaderboard(rows, 20)

		if resp.Limit != 20 {
			t.Errorf("limit = %d, want 20 (echoes the clamped request limit)", resp.Limit)
		}
		if len(resp.Leaderboard) != 3 {
			t.Fatalf("entries = %d, want 3", len(resp.Leaderboard))
		}

		// Rank is position + 1, preserving the query's rating-desc order.
		for i, e := range resp.Leaderboard {
			if e.Rank != i+1 {
				t.Errorf("entry %d rank = %d, want %d", i, e.Rank, i+1)
			}
		}

		top := resp.Leaderboard[0]
		if top.UserID != "u-ada" || top.DisplayName == nil || *top.DisplayName != "Ada" {
			t.Errorf("top = %+v, want Ada", top)
		}
		if top.Rating != 1432 || top.GamesPlayed != 13 || top.Wins != 10 || top.Losses != 3 {
			t.Errorf("top stats = %+v, want rating 1432 / 13-10-3", top)
		}

		// A nil display_name must serialize as null, i.e. stay a nil pointer.
		if resp.Leaderboard[1].DisplayName != nil {
			t.Errorf("Bo displayName = %v, want nil (null)", *resp.Leaderboard[1].DisplayName)
		}
	})

	t.Run("empty rows → non-nil empty slice (serializes as [], not null)", func(t *testing.T) {
		resp := buildLeaderboard(nil, 50)
		if resp.Leaderboard == nil {
			t.Error("leaderboard is nil, want a non-nil empty slice for a [] JSON body")
		}
		if len(resp.Leaderboard) != 0 {
			t.Errorf("entries = %d, want 0", len(resp.Leaderboard))
		}
		if resp.Limit != 50 {
			t.Errorf("limit = %d, want 50", resp.Limit)
		}
	})
}
