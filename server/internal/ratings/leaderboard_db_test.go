package ratings

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/markgrushevski/justpaint/server/internal/db"
)

// TestListTopRatings_DB exercises the leaderboard query against a real Postgres: it
// seats users with 0 / 1 / 2 finished matches (including a forfeit and an abandoned
// one) and asserts the query hides the 0-games user, orders by (rating desc, id asc),
// and counts wins/losses off winner_player_id with the abandoned match excluded
// (docs/GAME.md §8). Needs a migrated + seeded DATABASE_URL (docker compose up +
// goose up); skips otherwise, matching game/deadline_db_test.go — including its
// pool-close-via-t.Cleanup ordering so no fixture leaks.
func TestListTopRatings_DB(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB-backed leaderboard test")
	}
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	// Close via Cleanup, NOT defer: a test-body defer runs BEFORE t.Cleanup callbacks,
	// so a deferred Close would shut the pool before the row cleanup below. Registered
	// first, this runs LAST (Cleanup is LIFO), after the row cleanup.
	t.Cleanup(func() { pool.Close() })
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("postgres unreachable: %v", err)
	}

	q := db.New(pool)

	// --- fixtures ---------------------------------------------------------
	// Cleanup registered BEFORE any row is created (slices captured by reference), so
	// a mid-setup t.Fatalf still tears down whatever landed. FK order: match_players,
	// then matches, then users.
	var userIDs, matchIDs []string
	t.Cleanup(func() {
		for _, mid := range matchIDs {
			_, _ = pool.Exec(ctx, "delete from match_players where match_id = $1", mid)
			_, _ = pool.Exec(ctx, "delete from matches where id = $1", mid)
		}
		for _, uid := range userIDs {
			_, _ = pool.Exec(ctx, "delete from users where id = $1", uid)
		}
	})

	ns := time.Now().UnixNano()
	mkUser := func(tag string, rating int) string {
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Login:        fmt.Sprintf("lb-%s-%d", tag, ns),
			PasswordHash: "test-only-not-a-real-hash",
		})
		if err != nil {
			t.Fatalf("create user %s: %v", tag, err)
		}
		userIDs = append(userIDs, u.ID)
		if _, err := pool.Exec(ctx, "update users set rating = $2 where id = $1", u.ID, rating); err != nil {
			t.Fatalf("set rating %s: %v", tag, err)
		}
		return u.ID
	}

	prompt, err := q.PickRandomActivePrompt(ctx)
	if err != nil {
		t.Fatalf("pick prompt (is the DB migrated + seeded? migration 00002): %v", err)
	}

	// mkMatch seats players on a fresh match then forces its status; winner may be nil
	// (a tie, or an abandoned match). The leaderboard query only reads status + winner,
	// so no drawings/submissions are needed.
	mkMatch := func(status string, winner *string, players ...string) string {
		m, err := q.CreateMatch(ctx, prompt.ID)
		if err != nil {
			t.Fatalf("create match: %v", err)
		}
		matchIDs = append(matchIDs, m.ID)
		for _, uid := range players {
			if err := q.AddMatchPlayer(ctx, db.AddMatchPlayerParams{MatchID: m.ID, UserID: uid}); err != nil {
				t.Fatalf("add player: %v", err)
			}
		}
		if _, err := pool.Exec(ctx,
			"update matches set status = $2, winner_player_id = $3 where id = $1",
			m.ID, status, winner); err != nil {
			t.Fatalf("set match status: %v", err)
		}
		return m.ID
	}

	// Ratings chosen so the expected subset order is deterministic: A alone at the top,
	// then B and C tied (id-asc tiebreak), and Zero highest-rated but game-less (hidden).
	a := mkUser("a", 1500)
	b := mkUser("b", 1400)
	c := mkUser("c", 1400)
	zero := mkUser("zero", 1600)

	mkMatch("done", &a, a, b)          // done judged: A beats B
	mkMatch("done", &a, a, c)          // done forfeit-shaped: A beats C (resolution is irrelevant to the count)
	mkMatch("done", nil, b, c)         // done tie: B vs C, no winner
	mkMatch("abandoned", nil, a, zero) // abandoned: must NOT count for A or Zero

	// --- run --------------------------------------------------------------
	// A large limit so all eligible users are returned; assertions are scoped to the
	// four seeded ids, so unrelated rows from a shared DB don't matter.
	rows, err := q.ListTopRatings(ctx, 100)
	if err != nil {
		t.Fatalf("ListTopRatings: %v", err)
	}

	mine := map[string]bool{a: true, b: true, c: true, zero: true}
	byID := map[string]db.ListTopRatingsRow{}
	var order []string // ids of my users, in the returned (globally-sorted) order
	for _, r := range rows {
		if mine[r.ID] {
			byID[r.ID] = r
			order = append(order, r.ID)
		}
	}

	// --- case 1: the 0-games user is hidden -------------------------------
	if _, ok := byID[zero]; ok {
		t.Errorf("Zero (rating 1600, only an abandoned match) appears; want hidden (0 finished games)")
	}

	// --- case 2: win/loss counts, abandoned excluded ----------------------
	assertRow := func(tag, id string, games, wins, losses int32) {
		t.Helper()
		r, ok := byID[id]
		if !ok {
			t.Fatalf("%s (%s) missing from leaderboard, want present", tag, id)
		}
		if r.GamesPlayed != games || r.Wins != wins || r.Losses != losses {
			t.Errorf("%s stats = games %d wins %d losses %d, want %d/%d/%d",
				tag, r.GamesPlayed, r.Wins, r.Losses, games, wins, losses)
		}
	}
	// A: 2 done matches, both wins (the abandoned one is NOT counted → games=2, not 3).
	assertRow("A", a, 2, 2, 0)
	// B: the loss to A + the tie with C → 2 games, 0 wins, 1 loss (the tie is neither).
	assertRow("B", b, 2, 0, 1)
	// C: the loss to A + the tie with B → same shape as B.
	assertRow("C", c, 2, 0, 1)

	// --- case 3: order is rating desc, then id asc ------------------------
	// A (1500) leads; B and C tie at 1400 so the smaller uuid comes first.
	wantSecond, wantThird := b, c
	if c < b {
		wantSecond, wantThird = c, b
	}
	want := []string{a, wantSecond, wantThird}
	if len(order) != len(want) {
		t.Fatalf("my ranked ids = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("rank %d = %s, want %s (order must be rating desc, id asc)", i+1, order[i], want[i])
		}
	}
}
