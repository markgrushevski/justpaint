package game

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/markgrushevski/justpaint/server/internal/db"
	"github.com/markgrushevski/justpaint/server/internal/document"
	"github.com/markgrushevski/justpaint/server/internal/judge"
	"github.com/markgrushevski/justpaint/server/internal/render"
)

// TestRating_DB is the concurrency proof for the cross-match ladder write: the
// Elo delta is applied atomically (ApplyRatingDelta: `rating = rating + delta`
// returning the true post-value), so a user seated in two matches resolving at
// once does not lose a delta — the match-row lock serializes per match, never
// per user. Four cases against a real Postgres:
//
//	(a) two forfeit matches over the same user, resolved concurrently with a
//	    barrier at the read→write seam → both deltas land (fails on an absolute
//	    SET);
//	(b) two sequential matches → rating_before(M2) == rating_after(M1), and each
//	    snapshot delta == the Elo gain (RETURNING-derived, not the pre-match read);
//	(c) a resolved match is ladder zero-sum (winner's gain == loser's loss);
//	(d) the judged path (runJudging + FakeJudge + stub renderer) also moves
//	    users.rating and lands the rating_before/after snapshot.
//
// Needs a migrated + seeded DATABASE_URL (docker compose up + goose up); skips
// otherwise. Cleanup order is LIFO: the pool.Close registered first runs last.
func TestRating_DB(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB-backed rating test")
	}
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	// Close via Cleanup, not defer: a test-body defer runs before t.Cleanup
	// callbacks, so a deferred Close would shut the pool before the row cleanup
	// below. Registered first, this runs last (Cleanup is LIFO).
	t.Cleanup(func() { pool.Close() })
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("postgres unreachable: %v", err)
	}

	q := db.New(pool)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	// The forfeit/concurrency cases need no seams (no judge/render on a forfeit);
	// the judged case (d) uses a service with the real fake seams.
	svc := NewService(pool, q, nil, nil, logger)
	svcJudged := NewService(pool, q, render.NewStubRenderer(), judge.NewFakeJudge(), logger)

	// --- fixtures ---------------------------------------------------------
	// Cleanup registered before any row is created (slices captured by reference),
	// so a mid-setup t.Fatalf still tears down whatever landed (FK order below).
	var userIDs, matchIDs []string
	t.Cleanup(func() {
		for _, mid := range matchIDs {
			_, _ = pool.Exec(ctx, "delete from match_players where match_id = $1", mid)
			_, _ = pool.Exec(ctx, "delete from drawings where match_id = $1", mid)
			_, _ = pool.Exec(ctx, "delete from matches where id = $1", mid)
		}
		for _, uid := range userIDs {
			_, _ = pool.Exec(ctx, "delete from users where id = $1", uid)
		}
	})

	ns := time.Now().UnixNano()
	seq := 0
	mkUser := func(tag string) string {
		seq++
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Login:        fmt.Sprintf("rating-%s-%d-%d", tag, ns, seq),
			PasswordHash: "test-only-not-a-real-hash",
		})
		if err != nil {
			t.Fatalf("create user %s: %v", tag, err)
		}
		userIDs = append(userIDs, u.ID)
		return u.ID
	}

	// A prompt from the seeded set (migration 00002); a migrated DB has these.
	prompt, err := q.PickRandomActivePrompt(ctx)
	if err != nil {
		t.Fatalf("pick prompt (is the DB migrated + seeded? migration 00002): %v", err)
	}

	// docWithStrokes builds the smallest valid v1 document with n line strokes, so
	// ink coverage scales with n and the FakeJudge picks a decisive winner (needed
	// to move Elo off 1200).
	docWithStrokes := func(marker string, n int) string {
		var b strings.Builder
		b.WriteString(`{"version":1,"width":10,"height":10,"background":null,"layers":[{"id":"l","name":"`)
		b.WriteString(marker)
		b.WriteString(`","visible":true,"opacity":1,"strokes":[`)
		for i := 0; i < n; i++ {
			if i > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(&b, `{"id":"s%d","type":"line","composite":"source-over","points":[[0,0],[1,1]],"stroke":"#000000","strokeWidth":1}`, i)
		}
		b.WriteString(`]}]}`)
		return b.String()
	}

	// submitDrawing runs the exact CreateDrawing + StampSubmission path Submit uses,
	// stamping player uid's drawing (with strokeCount strokes) into match mid.
	submitDrawing := func(mid, uid string, strokeCount int) {
		raw := []byte(docWithStrokes("mark-"+uid, strokeCount))
		doc, err := document.ParseAndValidate(raw)
		if err != nil {
			t.Fatalf("fixture doc rejected: %v", err)
		}
		d, err := q.CreateDrawing(ctx, db.CreateDrawingParams{
			OwnerID:    uid,
			MatchID:    &mid,
			DocVersion: int32(doc.Version),
			Width:      int32(doc.Width),
			Height:     int32(doc.Height),
			Document:   raw,
		})
		if err != nil {
			t.Fatalf("create drawing: %v", err)
		}
		if _, err := q.StampSubmission(ctx, db.StampSubmissionParams{
			MatchID: mid, UserID: uid, DrawingID: &d.ID,
		}); err != nil {
			t.Fatalf("stamp submission: %v", err)
		}
	}

	// mkExpiredForfeit creates a match, seats submitter + forfeiter, submits only
	// the submitter's drawing, and backdates the deadline past expiry.
	mkExpiredForfeit := func(submitter, forfeiter string) string {
		m, err := q.CreateMatch(ctx, prompt.ID)
		if err != nil {
			t.Fatalf("create match: %v", err)
		}
		matchIDs = append(matchIDs, m.ID)
		for _, uid := range []string{submitter, forfeiter} {
			if err := q.AddMatchPlayer(ctx, db.AddMatchPlayerParams{MatchID: m.ID, UserID: uid}); err != nil {
				t.Fatalf("add player: %v", err)
			}
		}
		submitDrawing(m.ID, submitter, 1)
		if _, err := q.UpdateMatchStatus(ctx, db.UpdateMatchStatusParams{ID: m.ID, Status: statusDrawing}); err != nil {
			t.Fatalf("set drawing: %v", err)
		}
		if _, err := pool.Exec(ctx,
			"update matches set drawing_deadline = now() - make_interval(secs => 5) where id = $1", m.ID); err != nil {
			t.Fatalf("backdate deadline: %v", err)
		}
		return m.ID
	}

	// resolveForfeit locks and resolves one match — the exact per-row path the
	// sweeper uses, for the sequential cases (case (a) inlines the steps instead).
	resolveForfeit := func(mid string) {
		t.Helper()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer tx.Rollback(ctx)
		qtx := q.WithTx(tx)
		row, err := qtx.GetMatchForUpdate(ctx, mid)
		if err != nil {
			t.Fatalf("lock match: %v", err)
		}
		out, err := svc.resolveExpiry(ctx, qtx, row)
		if err != nil {
			t.Fatalf("resolveExpiry: %v", err)
		}
		if out != outcomeForfeit {
			t.Fatalf("outcome = %v, want forfeit", out)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit: %v", err)
		}
	}

	rating := func(uid string) int32 {
		t.Helper()
		u, err := q.GetUserByID(ctx, uid)
		if err != nil {
			t.Fatalf("get user rating: %v", err)
		}
		return u.Rating
	}

	matchPlayer := func(mid, uid string) db.MatchPlayer {
		t.Helper()
		p, err := q.GetMatchPlayer(ctx, db.GetMatchPlayerParams{MatchID: mid, UserID: uid})
		if err != nil {
			t.Fatalf("get match player: %v", err)
		}
		return p
	}

	// winGain is the submitter's (winner's) Elo delta given both pre-match ratings —
	// the same computeElo the production forfeit/judged paths use (scoreWin, K=32).
	winGain := func(before, oppBefore int) int32 {
		after, _ := computeElo(before, oppBefore, scoreWin)
		return int32(after - before)
	}

	// --- case (a): concurrency regression — the proof --------------------
	t.Run("two matches sharing a user resolve concurrently → both deltas land", func(t *testing.T) {
		u := mkUser("shared")
		oppA := mkUser("oppA")
		oppB := mkUser("oppB")
		m1 := mkExpiredForfeit(u, oppA) // u wins M1 by forfeit
		m2 := mkExpiredForfeit(u, oppB) // u wins M2 by forfeit

		// Both matches read u.rating = 1200 (the barrier guarantees both read before
		// either writes), so each computes the same +winGain(1200,1200); the atomic
		// rating += delta must accumulate both.
		startRating := rating(u) // 1200
		d := winGain(int(startRating), int(startRating))

		// A 2-party rendezvous at the read→write seam: resolveExpiry couples the
		// rating read (GetMatchPlayersForResolve) and the write (writeFinalResult →
		// ApplyRatingDelta) in one call, so a barrier placed after resolveExpiry
		// can't force the losing interleave — the second tx's ApplyRatingDelta would
		// block on the first tx's user-row lock before reaching it. So each
		// goroutine here inlines resolveExpiry's own steps, with the barrier sitting
		// exactly between the read and the write, where the cross-match lost update
		// lives. Both goroutines call Done exactly once, so a pre-barrier error
		// can't hang the peer.
		var readGate sync.WaitGroup
		readGate.Add(2)

		run := func(mid string) error {
			tx, e := pool.Begin(ctx)
			if e != nil {
				readGate.Done()
				return e
			}
			defer tx.Rollback(ctx)
			qtx := q.WithTx(tx)
			row, e := qtx.GetMatchForUpdate(ctx, mid) // per-MATCH lock (disjoint rows — no contention)
			if e != nil {
				readGate.Done()
				return e
			}
			players, e := qtx.GetMatchPlayersForResolve(ctx, row.ID) // THE READ (pre-match ratings)
			if e != nil {
				readGate.Done()
				return e
			}
			outcome, fr := decideExpiry(players)
			// Block until BOTH txs have read ratings, then release together — both saw 1200.
			readGate.Done()
			readGate.Wait()
			if outcome != outcomeForfeit {
				return fmt.Errorf("outcome = %v, want forfeit", outcome)
			}
			if e := svc.writeFinalResult(ctx, qtx, row.ID, fr); e != nil { // THE WRITE (atomic += delta)
				return e
			}
			return tx.Commit(ctx)
		}

		errc := make(chan error, 2)
		go func() { errc <- run(m1) }()
		go func() { errc <- run(m2) }()
		for i := 0; i < 2; i++ {
			if err := <-errc; err != nil {
				t.Fatalf("concurrent resolve: %v", err)
			}
		}

		// Both deltas landed. An absolute SET (the old UpdateUserRating) would clobber:
		// both txs read 1200 and one SET of 1200+d overwrites the other → only 1200+d.
		want := startRating + 2*d
		if got := rating(u); got != want {
			t.Errorf("users.rating(shared) = %d, want %d (both concurrent deltas must land; "+
				"an absolute SET would give %d — one delta lost)", got, want, startRating+d)
		}
	})

	// --- case (b): chain invariant ---------------------------------------
	t.Run("sequential matches chain: rating_before(M2) == rating_after(M1)", func(t *testing.T) {
		u := mkUser("chain")
		oppA := mkUser("chain-oppA")
		oppB := mkUser("chain-oppB")

		m1 := mkExpiredForfeit(u, oppA)
		resolveForfeit(m1)
		p1 := matchPlayer(m1, u)

		m2 := mkExpiredForfeit(u, oppB)
		resolveForfeit(m2)
		p2 := matchPlayer(m2, u)

		if p1.RatingBefore == nil || p1.RatingAfter == nil || p2.RatingBefore == nil || p2.RatingAfter == nil {
			t.Fatalf("rating snapshots must be set: p1=%v/%v p2=%v/%v",
				p1.RatingBefore, p1.RatingAfter, p2.RatingBefore, p2.RatingAfter)
		}

		// Chain: the second match's before equals the first match's after — the snapshot
		// is derived from ApplyRatingDelta's RETURNING, not a stale pre-match read.
		if *p2.RatingBefore != *p1.RatingAfter {
			t.Errorf("rating_before(M2) = %d, want rating_after(M1) = %d (chain broken)",
				*p2.RatingBefore, *p1.RatingAfter)
		}
		// after − before == the Elo gain each (winner vs a fresh-1200 opponent).
		if got, want := *p1.RatingAfter-*p1.RatingBefore, winGain(int(*p1.RatingBefore), 1200); got != want {
			t.Errorf("M1 snapshot delta = %d, want Elo gain %d", got, want)
		}
		if got, want := *p2.RatingAfter-*p2.RatingBefore, winGain(int(*p2.RatingBefore), 1200); got != want {
			t.Errorf("M2 snapshot delta = %d, want Elo gain %d", got, want)
		}
		// And the ladder equals the final snapshot.
		if got := rating(u); got != *p2.RatingAfter {
			t.Errorf("users.rating = %d, want the final rating_after %d", got, *p2.RatingAfter)
		}
	})

	// --- case (c): ladder zero-sum ---------------------------------------
	t.Run("a resolved match is ladder zero-sum", func(t *testing.T) {
		u := mkUser("zsum-a")
		opp := mkUser("zsum-b")
		before := rating(u) + rating(opp)

		mid := mkExpiredForfeit(u, opp)
		resolveForfeit(mid)

		if after := rating(u) + rating(opp); after != before {
			t.Errorf("ladder sum changed: before %d, after %d (winner's gain must equal loser's loss)", before, after)
		}
		// Sanity: the winner actually moved up and the loser down (not a no-op).
		if rating(u) <= 1200 || rating(opp) >= 1200 {
			t.Errorf("expected u up / opp down from 1200: u=%d opp=%d", rating(u), rating(opp))
		}
	})

	// --- case (d): judged path moves the ladder --------------------------
	t.Run("judged path (runJudging) moves users.rating and lands the snapshot", func(t *testing.T) {
		p1 := mkUser("judge-a")
		p2 := mkUser("judge-b")

		m, err := q.CreateMatch(ctx, prompt.ID)
		if err != nil {
			t.Fatalf("create match: %v", err)
		}
		matchIDs = append(matchIDs, m.ID)
		for _, uid := range []string{p1, p2} {
			if err := q.AddMatchPlayer(ctx, db.AddMatchPlayerParams{MatchID: m.ID, UserID: uid}); err != nil {
				t.Fatalf("add player: %v", err)
			}
		}
		// Different stroke counts → different stub ink coverage → the FakeJudge picks
		// a decisive winner (a tie at equal ratings would leave the ladder at 1200).
		submitDrawing(m.ID, p1, 5)
		submitDrawing(m.ID, p2, 0)
		if _, err := q.SetMatchJudging(ctx, m.ID); err != nil {
			t.Fatalf("to judging: %v", err)
		}

		if err := svcJudged.runJudging(ctx, m.ID); err != nil {
			t.Fatalf("runJudging: %v", err)
		}

		mm, err := q.GetMatch(ctx, m.ID)
		if err != nil {
			t.Fatalf("get match: %v", err)
		}
		if mm.Status != statusDone {
			t.Fatalf("status = %q, want done", mm.Status)
		}

		r1, r2 := rating(p1), rating(p2)
		// The ladder moved for both (decisive result), and stayed zero-sum.
		if r1 == 1200 && r2 == 1200 {
			t.Errorf("neither rating moved (r1=%d r2=%d) — judged Elo did not reach the ladder", r1, r2)
		}
		if r1+r2 != 2400 {
			t.Errorf("judged ladder not zero-sum: r1+r2 = %d, want 2400", r1+r2)
		}
		// The per-player snapshot landed and matches the ladder (single match: after == live rating).
		for _, uid := range []string{p1, p2} {
			mp := matchPlayer(m.ID, uid)
			if mp.RatingBefore == nil || mp.RatingAfter == nil {
				t.Fatalf("player %s snapshot nil: before=%v after=%v", uid, mp.RatingBefore, mp.RatingAfter)
			}
			if *mp.RatingBefore != 1200 {
				t.Errorf("player %s rating_before = %d, want 1200", uid, *mp.RatingBefore)
			}
			if *mp.RatingAfter != rating(uid) {
				t.Errorf("player %s rating_after = %d, want live rating %d", uid, *mp.RatingAfter, rating(uid))
			}
			if mp.Score == nil {
				t.Errorf("player %s judge score nil, want a similarity score on a judged result", uid)
			}
		}
	})
}
