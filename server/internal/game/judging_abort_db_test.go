package game

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/markgrushevski/justpaint/server/internal/db"
	"github.com/markgrushevski/justpaint/server/internal/document"
)

// TestAbortExhaustedJudging_DB exercises the terminal fallback for a match whose
// judging retries are used up (sweepExhaustedJudging → abortJudging) against a real
// Postgres: the match leaves `judging` for `done` + resolution 'aborted' with NO
// winner and NO rating change, a match still under the cap is left for the re-fire
// sweep, and a verdict that already landed is never overwritten. It also pins the
// reachability property the whole fix rests on — the retry cap must not hide a row
// from the sweeper: ListExhaustedJudgingMatches is the exact complement of
// ListStuckJudgingMatches, so every wedged row is in exactly one of them.
//
// Needs a migrated DATABASE_URL (docker compose up + goose up — including migration
// 00005, which widens the resolution check constraint); skips otherwise, matching
// deadline_db_test.go, including its pool-close-via-t.Cleanup ordering.
func TestAbortExhaustedJudging_DB(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB-backed judging-abort test")
	}
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	// Close via Cleanup, NOT defer: a test-body defer runs BEFORE t.Cleanup callbacks,
	// so a deferred Close would shut the pool before the row cleanup below.
	t.Cleanup(func() { pool.Close() })
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("postgres unreachable: %v", err)
	}

	q := db.New(pool)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(pool, q, nil, nil, logger)
	rec := &recordingPublisher{}
	svc.SetPublisher(rec)

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
	seq := 0
	mkUser := func(tag string) string {
		seq++
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Login:        fmt.Sprintf("abort-%s-%d-%d", tag, seq, ns),
			PasswordHash: "test-only-not-a-real-hash",
		})
		if err != nil {
			t.Fatalf("create user %s: %v", tag, err)
		}
		userIDs = append(userIDs, u.ID)
		return u.ID
	}

	prompt, err := q.PickRandomActivePrompt(ctx)
	if err != nil {
		t.Fatalf("pick prompt (is the DB migrated + seeded? migration 00002): %v", err)
	}

	// mkJudging seats two fresh players, submits a drawing for BOTH (a `judging` match
	// always has a full roster — that is what makes an unscored abort a real loss),
	// forces the status, and backdates the attempt clock by staleSecs with attempts
	// already spent. Returns the match id and its two players.
	mkJudging := func(status string, attempts, staleSecs int) (string, string, string) {
		t.Helper()
		a, b := mkUser("a"), mkUser("b")
		m, err := q.CreateMatch(ctx, prompt.ID)
		if err != nil {
			t.Fatalf("create match: %v", err)
		}
		matchIDs = append(matchIDs, m.ID)
		for _, uid := range []string{a, b} {
			if err := q.AddMatchPlayer(ctx, db.AddMatchPlayerParams{MatchID: m.ID, UserID: uid}); err != nil {
				t.Fatalf("add player: %v", err)
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
		if _, err := q.UpdateMatchStatus(ctx, db.UpdateMatchStatusParams{ID: m.ID, Status: status}); err != nil {
			t.Fatalf("set status %s: %v", status, err)
		}
		if _, err := pool.Exec(ctx,
			"update matches set judge_attempts = $2::int, judging_started_at = now() - make_interval(secs => $3::int) where id = $1",
			m.ID, attempts, staleSecs); err != nil {
			t.Fatalf("backdate judging attempt: %v", err)
		}
		return m.ID, a, b
	}

	rating := func(uid string) int32 {
		t.Helper()
		u, err := q.GetUserByID(ctx, uid)
		if err != nil {
			t.Fatalf("get user rating: %v", err)
		}
		return u.Rating
	}

	// --- the transition, case by case ------------------------------------
	cases := []struct {
		name string
		// fixture
		status   string
		attempts int
		// expectations
		wantAborted    bool
		wantStatus     string
		wantResolution *string
	}{
		{
			name:        "retries exhausted → done + aborted",
			status:      statusJudging,
			attempts:    maxJudgeAttempts,
			wantAborted: true, wantStatus: statusDone, wantResolution: strptr(resolutionAborted),
		},
		{
			name:        "attempts past the cap (a raced double-bump) also abort",
			status:      statusJudging,
			attempts:    maxJudgeAttempts + 1,
			wantAborted: true, wantStatus: statusDone, wantResolution: strptr(resolutionAborted),
		},
		{
			name:        "retries left → left alone, the re-fire sweep owns it",
			status:      statusJudging,
			attempts:    maxJudgeAttempts - 1,
			wantAborted: false, wantStatus: statusJudging, wantResolution: nil,
		},
		{
			name:        "a verdict that landed first is never overwritten",
			status:      statusDone,
			attempts:    maxJudgeAttempts,
			wantAborted: false, wantStatus: statusDone, wantResolution: strptr(resolutionJudged),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mid, a, b := mkJudging(tc.status, tc.attempts, judgeStaleSecs+60)
			if tc.status == statusDone {
				// A real judged verdict already on the row: winner + resolution set.
				reason, resolution := "left image matches the prompt more closely", resolutionJudged
				if _, err := q.SetMatchResult(ctx, db.SetMatchResultParams{
					ID: mid, WinnerPlayerID: &a, JudgeReason: &reason, Resolution: &resolution,
				}); err != nil {
					t.Fatalf("pre-set judged result: %v", err)
				}
			}
			aBefore, bBefore := rating(a), rating(b)

			aborted, err := svc.abortJudging(ctx, mid)
			if err != nil {
				t.Fatalf("abortJudging: %v", err)
			}
			if aborted != tc.wantAborted {
				t.Fatalf("aborted = %v, want %v", aborted, tc.wantAborted)
			}

			m, err := q.GetMatch(ctx, mid)
			if err != nil {
				t.Fatalf("get match: %v", err)
			}
			if m.Status != tc.wantStatus {
				t.Errorf("status = %q, want %q", m.Status, tc.wantStatus)
			}
			switch {
			case tc.wantResolution == nil && m.Resolution != nil:
				t.Errorf("resolution = %q, want nil", *m.Resolution)
			case tc.wantResolution != nil && (m.Resolution == nil || *m.Resolution != *tc.wantResolution):
				t.Errorf("resolution = %v, want %q", m.Resolution, *tc.wantResolution)
			}

			if !tc.wantAborted {
				return
			}
			// An aborted round produced no verdict: no winner, a player-facing reason,
			// and — like `abandoned` — no rating movement anywhere.
			if m.WinnerPlayerID != nil {
				t.Errorf("winner = %v, want nil on abort (nothing was scored)", *m.WinnerPlayerID)
			}
			if m.JudgeReason == nil || *m.JudgeReason != abortedReason {
				t.Errorf("judge_reason = %v, want the player-facing abort reason", m.JudgeReason)
			}
			if rating(a) != aBefore || rating(b) != bBefore {
				t.Errorf("ratings moved on abort: a %d→%d, b %d→%d", aBefore, rating(a), bBefore, rating(b))
			}
			players, err := q.ListMatchPlayers(ctx, mid)
			if err != nil {
				t.Fatalf("list players: %v", err)
			}
			for _, p := range players {
				if p.Score != nil || p.RatingBefore != nil || p.RatingAfter != nil {
					t.Errorf("player %s got score=%v before=%v after=%v, want all nil (no judge, no Elo)",
						p.UserID, p.Score, p.RatingBefore, p.RatingAfter)
				}
			}
		})
	}

	// --- reachable from the sweeper, and published -------------------------
	t.Run("the sweep lists the exhausted row, aborts it, and publishes the result", func(t *testing.T) {
		mid, _, _ := mkJudging(statusJudging, maxJudgeAttempts, judgeStaleSecs+60)

		// The retry cap must not HIDE the row: it moves from one list to the other.
		exhausted, err := q.ListExhaustedJudgingMatches(ctx, db.ListExhaustedJudgingMatchesParams{
			StaleSecs: judgeStaleSecs, MaxAttempts: maxJudgeAttempts, Lim: sweepBatch,
		})
		if err != nil {
			t.Fatalf("list exhausted: %v", err)
		}
		if !slices.Contains(exhausted, mid) {
			t.Fatalf("match %s missing from ListExhaustedJudgingMatches — the cap would strand it", mid)
		}
		stuck, err := q.ListStuckJudgingMatches(ctx, db.ListStuckJudgingMatchesParams{
			StaleSecs: judgeStaleSecs, MaxAttempts: maxJudgeAttempts, Lim: sweepBatch,
		})
		if err != nil {
			t.Fatalf("list stuck: %v", err)
		}
		if slices.Contains(stuck, mid) {
			t.Errorf("match %s is in BOTH judging lists — the filters must be complementary", mid)
		}

		before := len(rec.resolved)
		if n := svc.sweepExhaustedJudging(ctx); n < 1 {
			t.Fatalf("sweepExhaustedJudging handled %d rows, want at least 1", n)
		}

		m, err := q.GetMatch(ctx, mid)
		if err != nil {
			t.Fatalf("get match: %v", err)
		}
		if m.Status != statusDone || m.Resolution == nil || *m.Resolution != resolutionAborted {
			t.Errorf("after sweep: status=%q resolution=%v, want done/%q", m.Status, m.Resolution, resolutionAborted)
		}
		// Both WS clients learn through the same seam a judged/forfeit result uses.
		if !slices.Contains(rec.resolved[before:], mid) {
			t.Errorf("publisher.Resolved not called for %s (published: %v)", mid, rec.resolved[before:])
		}
	})
}
