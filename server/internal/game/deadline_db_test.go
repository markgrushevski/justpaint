package game

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/markgrushevski/justpaint/server/internal/db"
	"github.com/markgrushevski/justpaint/server/internal/document"
)

// TestResolveExpiry_DB exercises the round-deadline resolver against a real Postgres:
// the forfeit path (one submitter wins, Elo reaches users.rating, resolution=forfeit),
// the abandoned path (nobody drew, no Elo), and the late-submit rejection (a submit
// after the deadline is refused with ErrRoundExpired and NOT stamped). Needs a
// migrated DATABASE_URL (docker compose up + goose up); skips otherwise, matching
// reveal_test.go — including its pool-close-via-t.Cleanup ordering so no fixture leaks.
func TestResolveExpiry_DB(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB-backed deadline test")
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
	// A real (discard) logger: resolveExpiry only logs on the defensive n==2 path,
	// which these cases don't hit, but nil would panic if they did.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(pool, q, nil, nil, logger)

	// --- fixtures ---------------------------------------------------------
	// Cleanup registered BEFORE any row is created (slices captured by reference), so
	// a mid-setup t.Fatalf still tears down whatever landed. FK order: match_players,
	// then drawings, then matches, then users.
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
	mkUser := func(tag string) string {
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Login:        fmt.Sprintf("deadline-%s-%d", tag, ns),
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

	// mkExpiredDrawing creates a match, seats players, submits one marked drawing for
	// each in submitters (the same CreateDrawing + StampSubmission path Submit uses),
	// forces the match to `drawing`, then backdates its deadline so the round is
	// expired on the DB clock. Returns the match id.
	mkExpiredDrawing := func(submitters map[string]bool, players ...string) string {
		m, err := q.CreateMatch(ctx, prompt.ID)
		if err != nil {
			t.Fatalf("create match: %v", err)
		}
		matchIDs = append(matchIDs, m.ID)
		for _, uid := range players {
			if err := q.AddMatchPlayer(ctx, db.AddMatchPlayerParams{MatchID: m.ID, UserID: uid}); err != nil {
				t.Fatalf("add player: %v", err)
			}
			if !submitters[uid] {
				continue
			}
			raw := []byte(markedDocJSON("mark-" + uid))
			doc, err := document.ParseAndValidate(raw)
			if err != nil {
				t.Fatalf("fixture doc rejected: %v", err)
			}
			d, err := q.CreateDrawing(ctx, db.CreateDrawingParams{
				OwnerID:    uid,
				MatchID:    &m.ID,
				DocVersion: int32(doc.Version),
				Width:      int32(doc.Width),
				Height:     int32(doc.Height),
				Document:   raw,
			})
			if err != nil {
				t.Fatalf("create drawing: %v", err)
			}
			if _, err := q.StampSubmission(ctx, db.StampSubmissionParams{
				MatchID: m.ID, UserID: uid, DrawingID: &d.ID,
			}); err != nil {
				t.Fatalf("stamp submission: %v", err)
			}
		}
		if _, err := q.UpdateMatchStatus(ctx, db.UpdateMatchStatusParams{ID: m.ID, Status: statusDrawing}); err != nil {
			t.Fatalf("set drawing: %v", err)
		}
		// Backdate the deadline into the past (DB clock) so the round is expired.
		if _, err := pool.Exec(ctx,
			"update matches set drawing_deadline = now() - make_interval(secs => 5) where id = $1", m.ID); err != nil {
			t.Fatalf("backdate deadline: %v", err)
		}
		return m.ID
	}

	// mkPlainMatch creates a match, seats players (optionally submitting a marked
	// drawing for each), and forces a status — the fixture for the sweep-path cases
	// (stale judging / stale open). Returns the match id.
	mkPlainMatch := func(status string, submit bool, players ...string) string {
		m, err := q.CreateMatch(ctx, prompt.ID)
		if err != nil {
			t.Fatalf("create match: %v", err)
		}
		matchIDs = append(matchIDs, m.ID)
		for _, uid := range players {
			if err := q.AddMatchPlayer(ctx, db.AddMatchPlayerParams{MatchID: m.ID, UserID: uid}); err != nil {
				t.Fatalf("add player: %v", err)
			}
			if !submit {
				continue
			}
			raw := []byte(markedDocJSON("mark-" + uid))
			doc, err := document.ParseAndValidate(raw)
			if err != nil {
				t.Fatalf("fixture doc rejected: %v", err)
			}
			d, err := q.CreateDrawing(ctx, db.CreateDrawingParams{
				OwnerID:    uid,
				MatchID:    &m.ID,
				DocVersion: int32(doc.Version),
				Width:      int32(doc.Width),
				Height:     int32(doc.Height),
				Document:   raw,
			})
			if err != nil {
				t.Fatalf("create drawing: %v", err)
			}
			if _, err := q.StampSubmission(ctx, db.StampSubmissionParams{
				MatchID: m.ID, UserID: uid, DrawingID: &d.ID,
			}); err != nil {
				t.Fatalf("stamp submission: %v", err)
			}
		}
		if status != statusOpen { // CreateMatch already lands in `open`
			if _, err := q.UpdateMatchStatus(ctx, db.UpdateMatchStatusParams{ID: m.ID, Status: status}); err != nil {
				t.Fatalf("set status %s: %v", status, err)
			}
		}
		return m.ID
	}

	// resolve locks the match and runs resolveExpiry in its own committed tx — the
	// exact per-row path the sweeper uses.
	resolve := func(mid string) resolveOutcome {
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
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit: %v", err)
		}
		return out
	}

	rating := func(uid string) int32 {
		t.Helper()
		u, err := q.GetUserByID(ctx, uid)
		if err != nil {
			t.Fatalf("get user rating: %v", err)
		}
		return u.Rating
	}

	// --- case 1: forfeit --------------------------------------------------
	t.Run("one submitted → forfeit: submitter wins, Elo moves, resolution=forfeit", func(t *testing.T) {
		sub, forf := mkUser("sub"), mkUser("forf")
		mid := mkExpiredDrawing(map[string]bool{sub: true}, sub, forf)

		subBefore, forfBefore := rating(sub), rating(forf)

		if out := resolve(mid); out != outcomeForfeit {
			t.Fatalf("outcome = %v, want forfeit", out)
		}

		m, err := q.GetMatch(ctx, mid)
		if err != nil {
			t.Fatalf("get match: %v", err)
		}
		if m.Status != statusDone {
			t.Errorf("status = %q, want done", m.Status)
		}
		if m.WinnerPlayerID == nil || *m.WinnerPlayerID != sub {
			t.Errorf("winner = %v, want the submitter %s", m.WinnerPlayerID, sub)
		}
		if m.Resolution == nil || *m.Resolution != resolutionForfeit {
			t.Errorf("resolution = %v, want %q", m.Resolution, resolutionForfeit)
		}

		// Elo reached the ladder (users.rating), not just match_players.
		if got := rating(sub); got <= subBefore {
			t.Errorf("submitter rating %d → %d, want increase", subBefore, got)
		}
		if got := rating(forf); got >= forfBefore {
			t.Errorf("forfeiter rating %d → %d, want decrease", forfBefore, got)
		}

		// No judge similarity score for either (nil), but the Elo snapshot is set.
		players, err := q.ListMatchPlayers(ctx, mid)
		if err != nil {
			t.Fatalf("list players: %v", err)
		}
		for _, p := range players {
			if p.Score != nil {
				t.Errorf("player %s score = %v, want nil on forfeit", p.UserID, *p.Score)
			}
			if p.RatingAfter == nil {
				t.Errorf("player %s rating_after nil, want the Elo snapshot set", p.UserID)
			}
		}
	})

	// --- case 2: abandoned ------------------------------------------------
	t.Run("nobody submitted → abandoned, ratings unchanged", func(t *testing.T) {
		a, b := mkUser("aband-a"), mkUser("aband-b")
		mid := mkExpiredDrawing(nil, a, b)
		aBefore, bBefore := rating(a), rating(b)

		if out := resolve(mid); out != outcomeAbandoned {
			t.Fatalf("outcome = %v, want abandoned", out)
		}

		m, err := q.GetMatch(ctx, mid)
		if err != nil {
			t.Fatalf("get match: %v", err)
		}
		if m.Status != statusAbandoned {
			t.Errorf("status = %q, want abandoned", m.Status)
		}
		if m.WinnerPlayerID != nil {
			t.Errorf("winner = %v, want nil on abandon", m.WinnerPlayerID)
		}
		if m.Resolution != nil {
			t.Errorf("resolution = %v, want nil on abandon (no result)", m.Resolution)
		}
		if rating(a) != aBefore || rating(b) != bBefore {
			t.Errorf("ratings changed on abandon: a %d→%d, b %d→%d", aBefore, rating(a), bBefore, rating(b))
		}
	})

	// --- case 3: late submit rejected -------------------------------------
	t.Run("submit after the deadline → ErrRoundExpired, slot not stamped", func(t *testing.T) {
		a, b := mkUser("late-a"), mkUser("late-b")
		mid := mkExpiredDrawing(nil, a, b) // neither submitted; deadline already past

		raw := []byte(markedDocJSON("mark-late"))
		doc, err := document.ParseAndValidate(raw)
		if err != nil {
			t.Fatalf("doc: %v", err)
		}

		if _, err := svc.Submit(ctx, a, mid, doc, raw); !errors.Is(err, ErrRoundExpired) {
			t.Fatalf("Submit err = %v, want ErrRoundExpired", err)
		}

		// The late submitter's slot must NOT be stamped, and no drawing linked.
		p, err := q.GetMatchPlayer(ctx, db.GetMatchPlayerParams{MatchID: mid, UserID: a})
		if err != nil {
			t.Fatalf("get player: %v", err)
		}
		if p.SubmittedAt != nil {
			t.Errorf("late submitter stamped at %v, want nil (not stamped)", *p.SubmittedAt)
		}
		if p.DrawingID != nil {
			t.Errorf("late submitter drawing_id = %v, want nil", *p.DrawingID)
		}

		// With nobody submitted at expiry, the match resolves to abandoned.
		m, err := q.GetMatch(ctx, mid)
		if err != nil {
			t.Fatalf("get match: %v", err)
		}
		if m.Status != statusAbandoned {
			t.Errorf("status = %q, want abandoned (n=0 at expiry)", m.Status)
		}
	})

	// --- case 4: stuck-judging watchdog -----------------------------------
	t.Run("stuck-judging watchdog re-stamps a stale attempt; caps out; never reverts done", func(t *testing.T) {
		a, b := mkUser("stuck-a"), mkUser("stuck-b")
		mid := mkPlainMatch(statusJudging, true, a, b)
		// Backdate the attempt well past the stale window, with one prior attempt.
		if _, err := pool.Exec(ctx,
			"update matches set judging_started_at = now() - make_interval(secs => $2::int), judge_attempts = 1 where id = $1",
			mid, judgeStaleSecs+60); err != nil {
			t.Fatalf("backdate judging attempt: %v", err)
		}

		before, err := q.GetMatch(ctx, mid)
		if err != nil {
			t.Fatalf("get before: %v", err)
		}

		refired, err := svc.refireJudging(ctx, mid)
		if err != nil {
			t.Fatalf("refireJudging: %v", err)
		}
		if !refired {
			t.Fatal("refired = false, want true (a stale judging attempt must re-fire)")
		}

		after, err := q.GetMatch(ctx, mid)
		if err != nil {
			t.Fatalf("get after: %v", err)
		}
		if after.Status != statusJudging {
			t.Errorf("status = %q, want judging (still)", after.Status)
		}
		if after.JudgeAttempts != before.JudgeAttempts+1 {
			t.Errorf("judge_attempts = %d, want %d (bumped)", after.JudgeAttempts, before.JudgeAttempts+1)
		}
		if before.JudgingStartedAt == nil || after.JudgingStartedAt == nil ||
			!after.JudgingStartedAt.After(*before.JudgingStartedAt) {
			t.Errorf("judging_started_at not re-stamped fresh: before=%v after=%v",
				before.JudgingStartedAt, after.JudgingStartedAt)
		}

		// Cap reached: with judge_attempts at the max, refireJudging must decline under
		// the lock (the JudgeAttempts>=max recheck) even though status is still judging.
		if _, err := pool.Exec(ctx,
			"update matches set judge_attempts = $2::int, judging_started_at = now() - make_interval(secs => $3::int) where id = $1",
			mid, maxJudgeAttempts, judgeStaleSecs+60); err != nil {
			t.Fatalf("set attempts to the cap: %v", err)
		}
		capped, err := svc.refireJudging(ctx, mid)
		if err != nil {
			t.Fatalf("refireJudging (capped): %v", err)
		}
		if capped {
			t.Error("refired = true at the attempt cap, want false (retries exhausted)")
		}
		atCap, err := q.GetMatch(ctx, mid)
		if err != nil {
			t.Fatalf("get at-cap: %v", err)
		}
		if atCap.JudgeAttempts != maxJudgeAttempts {
			t.Errorf("judge_attempts = %d at cap, want %d (untouched)", atCap.JudgeAttempts, maxJudgeAttempts)
		}

		// Control: a `done` match must NOT be reverted, and refireJudging returns false.
		dm := mkPlainMatch(statusDone, false)
		reverted, err := svc.refireJudging(ctx, dm)
		if err != nil {
			t.Fatalf("refireJudging (done): %v", err)
		}
		if reverted {
			t.Error("refired = true on a done match, want false (must not revert)")
		}
		dmAfter, err := q.GetMatch(ctx, dm)
		if err != nil {
			t.Fatalf("get done: %v", err)
		}
		if dmAfter.Status != statusDone {
			t.Errorf("done match status = %q, want done (not reverted)", dmAfter.Status)
		}
	})

	// --- case 5: stale-open reaper ----------------------------------------
	t.Run("stale-open reaper abandons an open match; leaves a non-open one", func(t *testing.T) {
		creator := mkUser("open-creator")
		mid := mkPlainMatch(statusOpen, false, creator)
		// Backdate created_at past the TTL (the List query's filter; reapOpenMatch
		// itself only rechecks status=='open').
		if _, err := pool.Exec(ctx,
			"update matches set created_at = now() - make_interval(secs => $2::int) where id = $1",
			mid, openTTLSecs+60); err != nil {
			t.Fatalf("backdate created_at: %v", err)
		}

		if err := svc.reapOpenMatch(ctx, mid); err != nil {
			t.Fatalf("reapOpenMatch: %v", err)
		}
		m, err := q.GetMatch(ctx, mid)
		if err != nil {
			t.Fatalf("get open: %v", err)
		}
		if m.Status != statusAbandoned {
			t.Errorf("status = %q, want abandoned (stale open reaped)", m.Status)
		}

		// Control: a `drawing` match must be left alone (reapOpenMatch rechecks 'open').
		dm := mkPlainMatch(statusDrawing, false)
		if err := svc.reapOpenMatch(ctx, dm); err != nil {
			t.Fatalf("reapOpenMatch (drawing): %v", err)
		}
		dmAfter, err := q.GetMatch(ctx, dm)
		if err != nil {
			t.Fatalf("get drawing: %v", err)
		}
		if dmAfter.Status != statusDrawing {
			t.Errorf("drawing match status = %q, want drawing (reaper must skip non-open)", dmAfter.Status)
		}
	})
}
