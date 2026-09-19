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
)

// TestJudgeBudget_DB proves the daily judge budget against a real Postgres: the two
// counts are SQL over rows the lifecycle already writes, so only a database can say
// whether they count the right ones.
//
//	(a) a player under their cap starts a duel, through the real CreateOrJoin;
//	(b) a player AT their cap is refused — and a different player is not, which is the
//	    whole point of having a per-player half: one abuser must not deny service to
//	    everyone;
//	(c) once the global budget is reached, everybody is refused;
//	(d) an unenforced budget (JUDGE_MODE=fake) skips both checks, even with caps that
//	    would otherwise refuse every duel;
//	(e) the window ROLLS: a duel older than 24h counts against nobody, on either half.
//
// Needs a migrated + seeded DATABASE_URL (docker compose up + goose up); skips
// otherwise, matching rating_db_test.go — including its pool-close-via-t.Cleanup
// ordering so no fixture leaks (Cleanup is LIFO: the pool.Close registered FIRST runs
// LAST).
func TestJudgeBudget_DB(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB-backed judge-budget test")
	}
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("postgres unreachable: %v", err)
	}

	q := db.New(pool)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	// No renderer/judge: nothing here reaches a judging pass — the budget refuses (or
	// allows) a duel long before anyone draws.
	svc := NewService(pool, q, nil, nil, logger)

	// --- fixtures ---------------------------------------------------------
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
			Login:        fmt.Sprintf("budget-%s-%d-%d", tag, ns, seq),
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

	// partner fills the second seat of every fixture duel: a roster row is what the
	// per-player count reads, so the duels have to be real two-seat matches.
	partner := mkUser("partner")

	// track hands a match id to the teardown. Every CreateOrJoin result goes through
	// it, including the ones a subtest expects to be refusals: a refusal returns no
	// id, but if the gate ever stops refusing, the match it silently created must
	// still be torn down rather than left in a shared dev database.
	track := func(mid string) {
		if mid != "" {
			matchIDs = append(matchIDs, mid)
		}
	}

	newMatch := func(players ...string) string {
		m, err := q.CreateMatch(ctx, prompt.ID)
		if err != nil {
			t.Fatalf("create match: %v", err)
		}
		track(m.ID)
		for _, uid := range players {
			if err := q.AddMatchPlayer(ctx, db.AddMatchPlayerParams{MatchID: m.ID, UserID: uid}); err != nil {
				t.Fatalf("add player: %v", err)
			}
		}
		return m.ID
	}

	// age backdates a match so it sits `d` in the past — how the rolling window is
	// exercised without waiting a day. created_at drives the per-player count,
	// judging_started_at the global one, so both move together.
	age := func(mid string, d time.Duration) {
		if _, err := pool.Exec(ctx, `update matches
			set created_at = now() - make_interval(secs => $2::int),
			    judging_started_at = case when judging_started_at is null then null
			                              else now() - make_interval(secs => $2::int) end
			where id = $1`, mid, int32(d/time.Second)); err != nil {
			t.Fatalf("backdate match: %v", err)
		}
	}

	// startedDuel is a duel that found an opponent: the roster fills and the round
	// starts, which is the transition that stamps drawing_deadline — the marker the
	// per-player count uses for "this one actually cost a judge call".
	startedDuel := func(uid string, ago time.Duration) string {
		mid := newMatch(uid, partner)
		if _, err := q.SetMatchDrawing(ctx, db.SetMatchDrawingParams{ID: mid, RoundSeconds: roundSeconds}); err != nil {
			t.Fatalf("start round: %v", err)
		}
		age(mid, ago)
		return mid
	}

	// joinableMatch seeds an open match and backdates it to the beginning of time, so
	// a later CreateOrJoin provably lands on THIS row: the matchmaking query takes the
	// OLDEST open match the caller is not in, and a shared dev database may well hold
	// open matches this test must not touch.
	joinableMatch := func(host string) string {
		mid := newMatch(host)
		age(mid, 10*365*24*time.Hour)
		return mid
	}

	globalSpent := func() int64 {
		n, err := q.CountJudgeCallsInWindow(ctx, int32(judgeBudgetWindow/time.Second))
		if err != nil {
			t.Fatalf("count judge calls: %v", err)
		}
		return n
	}

	// --- (a) under the cap, a duel starts ---------------------------------
	t.Run("a player under their cap starts a duel", func(t *testing.T) {
		host := mkUser("under-host")
		joiner := mkUser("under-joiner")
		mid := joinableMatch(host)

		svc.SetJudgeBudget(JudgeBudget{Enforced: true, Global: int(globalSpent()) + 10, PerUser: 2})

		view, err := svc.CreateOrJoin(ctx, joiner)
		if err != nil {
			t.Fatalf("CreateOrJoin under the cap: %v", err)
		}
		track(view.ID)
		if view.ID != mid {
			t.Fatalf("joined match %s, want the seeded %s", view.ID, mid)
		}
		if view.Status != statusDrawing {
			t.Errorf("status = %q, want %q — the round should have started", view.Status, statusDrawing)
		}
	})

	// --- (b) the per-player cap is per PLAYER -----------------------------
	t.Run("at their cap one player is refused and another still plays", func(t *testing.T) {
		const cap = 3
		heavy := mkUser("heavy")
		for i := 0; i < cap; i++ {
			startedDuel(heavy, time.Duration(i+1)*time.Hour)
		}
		innocent := mkUser("innocent")

		svc.SetJudgeBudget(JudgeBudget{Enforced: true, Global: int(globalSpent()) + 10, PerUser: cap})

		// Through the real entry point: a refusal returns before the matchmaking
		// transaction, so this cannot disturb a row it did not create.
		if view, err := svc.CreateOrJoin(ctx, heavy); !errors.Is(err, ErrDailyDuelsSpent) {
			track(view.ID)
			t.Errorf("CreateOrJoin at the cap: err = %v, want %v", err, ErrDailyDuelsSpent)
		}
		// The property that makes a per-player cap worth having.
		if err := svc.checkJudgeBudget(ctx, innocent); err != nil {
			t.Errorf("a different player was refused too (%v) — one abuser must not deny service to everyone", err)
		}

		// One duel short of the cap is still allowed, so the boundary is `>=`, not `>`.
		svc.SetJudgeBudget(JudgeBudget{Enforced: true, Global: int(globalSpent()) + 10, PerUser: cap + 1})
		if err := svc.checkJudgeBudget(ctx, heavy); err != nil {
			t.Errorf("one duel under the cap was refused: %v", err)
		}
	})

	// --- (b2) waiting alone is free ---------------------------------------
	t.Run("a match nobody ever joined costs the player nothing", func(t *testing.T) {
		lonely := mkUser("lonely")
		// The exact shape the open-match reaper leaves behind: a match the player
		// created, nobody joined, swept to `abandoned`. No opponent ever showed up, so
		// no judge call was ever spent — and a `status <> 'open'` count would bill this
		// player for having waited, which is a cap on patience, not on judge calls.
		reaped := newMatch(lonely)
		if _, err := q.SetMatchAbandoned(ctx, reaped); err != nil {
			t.Fatalf("reap open match: %v", err)
		}

		svc.SetJudgeBudget(JudgeBudget{Enforced: true, Global: int(globalSpent()) + 10, PerUser: 1})
		if err := svc.checkJudgeBudget(ctx, lonely); err != nil {
			t.Errorf("a never-joined match counted against its creator: %v", err)
		}
	})

	// --- (c) the global budget refuses everyone ---------------------------
	t.Run("the global budget refuses everyone once reached", func(t *testing.T) {
		before := globalSpent()
		mid := newMatch(mkUser("global-a"), mkUser("global-b"))
		if _, err := q.SetMatchJudging(ctx, mid); err != nil {
			t.Fatalf("to judging: %v", err)
		}
		// Proves the global count reads the judging transition itself, not `done`:
		// a forfeit reaches done without ever calling the judge.
		if after := globalSpent(); after != before+1 {
			t.Fatalf("global count = %d, want %d — a match entering judging must be counted", after, before+1)
		}

		spent := globalSpent()
		// Fresh players, each miles under any per-player cap: only the global half can
		// refuse them.
		svc.SetJudgeBudget(JudgeBudget{Enforced: true, Global: int(spent), PerUser: 1000})
		for _, tag := range []string{"bystander-1", "bystander-2"} {
			uid := mkUser(tag)
			if view, err := svc.CreateOrJoin(ctx, uid); !errors.Is(err, ErrJudgeBudgetSpent) {
				track(view.ID)
				t.Errorf("%s: err = %v, want %v", tag, err, ErrJudgeBudgetSpent)
			}
		}
		// One call of headroom and the same players are welcome again.
		svc.SetJudgeBudget(JudgeBudget{Enforced: true, Global: int(spent) + 1, PerUser: 1000})
		if err := svc.checkJudgeBudget(ctx, mkUser("bystander-3")); err != nil {
			t.Errorf("with budget left the duel was still refused: %v", err)
		}
	})

	// --- (d) an unenforced budget skips both checks -----------------------
	t.Run("an unenforced budget skips both halves", func(t *testing.T) {
		host := mkUser("fake-host")
		joiner := mkUser("fake-joiner")
		startedDuel(joiner, time.Hour) // already over any cap below
		mid := joinableMatch(host)

		// Caps that would refuse every duel on earth — the Enforced flag is the only
		// thing standing between them and this call, which is exactly the local
		// JUDGE_MODE=fake story: no external quota, so no ceiling.
		svc.SetJudgeBudget(JudgeBudget{Enforced: false, Global: 0, PerUser: 0})

		view, err := svc.CreateOrJoin(ctx, joiner)
		if err != nil {
			t.Fatalf("CreateOrJoin with an unenforced budget: %v", err)
		}
		track(view.ID)
		if view.ID != mid {
			t.Fatalf("joined match %s, want the seeded %s", view.ID, mid)
		}
	})

	// --- (e) the window rolls ---------------------------------------------
	t.Run("a duel older than the window counts against nobody", func(t *testing.T) {
		stale := judgeBudgetWindow + time.Hour

		roller := mkUser("roller")
		startedDuel(roller, stale)
		svc.SetJudgeBudget(JudgeBudget{Enforced: true, Global: int(globalSpent()) + 10, PerUser: 1})
		if err := svc.checkJudgeBudget(ctx, roller); err != nil {
			t.Errorf("a duel older than %s still counted against its player: %v", judgeBudgetWindow, err)
		}
		// The same duel inside the window does count — otherwise the case above would
		// pass for a count that is simply broken.
		startedDuel(roller, time.Hour)
		if err := svc.checkJudgeBudget(ctx, roller); !errors.Is(err, ErrDailyDuelsSpent) {
			t.Errorf("a duel inside the window did not count: err = %v, want %v", err, ErrDailyDuelsSpent)
		}

		// And the global half rolls on the same clock.
		before := globalSpent()
		old := newMatch(mkUser("roll-global"))
		if _, err := q.SetMatchJudging(ctx, old); err != nil {
			t.Fatalf("to judging: %v", err)
		}
		age(old, stale)
		if after := globalSpent(); after != before {
			t.Errorf("global count = %d, want %d — a judging pass older than %s must have rolled out", after, before, judgeBudgetWindow)
		}
	})
}
