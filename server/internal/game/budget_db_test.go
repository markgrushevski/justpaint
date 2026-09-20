package game

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/markgrushevski/justpaint/server/internal/aibudget"
	"github.com/markgrushevski/justpaint/server/internal/db"
	"github.com/markgrushevski/justpaint/server/internal/document"
	"github.com/markgrushevski/justpaint/server/internal/judge"
	"github.com/markgrushevski/justpaint/server/internal/render"
)

// matchmakingLockID guards the seed-an-open-match-then-join window against the
// OTHER DB suites that drive CreateOrJoin, which `go test ./...` runs
// concurrently against this same database (internal/aibudget's budget_db_test.go
// holds the same id).
//
// It has to exist because matchmaking is global by design: FindOpenMatchToJoin
// takes the OLDEST open async match the caller is not in, anywhere in the table.
// Two suites that each seed a backdated open match and then join it will
// therefore steal each other's — not flakily, but whenever their windows overlap.
// A session-level advisory lock is the smallest thing that makes the two windows
// mutually exclusive; it costs one connection for a few milliseconds and needs no
// coordination beyond the shared number.
const matchmakingLockID int64 = 20260920

// withMatchmaking runs fn holding matchmakingLockID. The lock is session-scoped,
// so it is taken and released on ONE pooled connection; a t.Fatal inside fn still
// releases it, because Goexit runs deferred calls.
func withMatchmaking(t *testing.T, ctx context.Context, pool *pgxpool.Pool, fn func()) {
	t.Helper()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire a connection for the matchmaking lock: %v", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "select pg_advisory_lock($1)", matchmakingLockID); err != nil {
		t.Fatalf("take the matchmaking lock: %v", err)
	}
	defer func() {
		if _, err := conn.Exec(ctx, "select pg_advisory_unlock($1)", matchmakingLockID); err != nil {
			t.Errorf("release the matchmaking lock: %v", err)
		}
	}()
	fn()
}

// TestDuelBudgetBilling_DB pins WHERE a duel's two ledger facts are written, which
// is the whole of the fix that split aibudget.Spend into BillPlayers and
// BillProvider.
//
// The bug it exists to prevent: both rows used to be written together, in the
// matchmaking transaction, on the theory that a round that starts is a judge call.
// It is not. A round can end abandoned (nobody drew) or forfeit (one drew), and
// neither ever reaches a judge — so the ledger billed a provider request that was
// never made. It can also be judged MORE than once, because the stuck-judging
// sweep re-fires a wedged attempt — so the ledger undercounted the requests that
// were made. The count was therefore neither an upper nor a lower bound on real
// provider spend, which is the one thing a spend ledger is for.
//
// The rule now, and the four cases below:
//
//	(a) a round that STARTS grants both players a round and bills no provider;
//	(b) a round that reaches JUDGING bills the provider, once, and the players not
//	    again;
//	(c) a round that is forfeited or abandoned bills no provider at all;
//	(d) a re-fired judging attempt bills the provider AGAIN, and the players still
//	    exactly once — they got one round, however many times the judge was asked.
//
// This lives in package game, not beside the rest of the budget's DB suite in
// internal/aibudget: (d) has to drive refireJudging, which is unexported, and the
// three judging sites it is asserting about are all in here.
//
// Needs a migrated + seeded DATABASE_URL (docker compose up + goose up, including
// migration 00007); skips otherwise, with the same pool-close-via-t.Cleanup
// ordering as the rest of the DB suites (Cleanup is LIFO: the pool.Close
// registered FIRST runs LAST).
func TestDuelBudgetBilling_DB(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB-backed duel-billing test")
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
	logger := slog.New(slog.DiscardHandler)
	// A real (stub) renderer and a real (fake) judge, because case (b) submits for
	// both players and that dispatches an actual judging pass. A nil renderer would
	// panic in the goroutine and take the test binary with it.
	svc := NewService(pool, q, render.NewStubRenderer(), judge.NewFakeJudge(), logger)

	// A provider name nothing else in the world spends, so every count below is an
	// exact number rather than a delta against whatever else touches this database.
	ns := time.Now().UnixNano()
	provider := aibudget.Provider(fmt.Sprintf("test-duel-billing-%d", ns))

	// Caps high enough that nothing here is ever refused: this suite is about WHERE
	// rows are written, not about when the ceiling binds (that is aibudget's own
	// DB suite).
	budget := aibudget.New(q, map[aibudget.Kind]aibudget.Policy{
		aibudget.KindDuel: {Provider: provider, PerUser: 1000},
	}, 1_000_000, logger)
	check, _ := budget.For(aibudget.KindDuel)
	billPlayers, billProvider := budget.ForSplit(aibudget.KindDuel)
	svc.SetBudget(check, billPlayers, billProvider)

	var userIDs, matchIDs []string
	t.Cleanup(func() {
		for _, mid := range matchIDs {
			_, _ = pool.Exec(ctx, "delete from match_players where match_id = $1", mid)
			_, _ = pool.Exec(ctx, "delete from drawings where match_id = $1", mid)
			_, _ = pool.Exec(ctx, "delete from matches where id = $1", mid)
		}
		// Ledger rows before the users they reference (ai_calls.user_id is a FK). The
		// provider rows name no user, so they come out by the private provider name.
		_, _ = pool.Exec(ctx, "delete from ai_calls where provider = $1", string(provider))
		for _, uid := range userIDs {
			_, _ = pool.Exec(ctx, "delete from ai_calls where user_id = $1", uid)
			_, _ = pool.Exec(ctx, "delete from users where id = $1", uid)
		}
	})

	seq := 0
	mkUser := func(tag string) string {
		seq++
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Login:        fmt.Sprintf("bill-%s-%d-%d", tag, ns, seq),
			PasswordHash: "test-only-not-a-real-hash",
		})
		if err != nil {
			t.Fatalf("create user %s: %v", tag, err)
		}
		userIDs = append(userIDs, u.ID)
		return u.ID
	}
	track := func(mid string) {
		if mid != "" {
			matchIDs = append(matchIDs, mid)
		}
	}

	prompt, err := q.PickRandomActivePrompt(ctx)
	if err != nil {
		t.Fatalf("pick prompt (is the DB migrated + seeded? migration 00002): %v", err)
	}
	promptID := prompt.ID

	// providerRows is the global half: how many requests the ledger says we made.
	providerRows := func() int {
		var n int
		if err := pool.QueryRow(ctx,
			"select count(*) from ai_calls where provider = $1", string(provider)).Scan(&n); err != nil {
			t.Fatalf("count provider rows: %v", err)
		}
		return n
	}
	// playerRows is one player's half: how many rounds the ledger says they got.
	playerRows := func(uid string) int {
		var n int
		if err := pool.QueryRow(ctx,
			"select count(*) from ai_calls where user_id = $1 and kind = $2",
			uid, string(aibudget.KindDuel)).Scan(&n); err != nil {
			t.Fatalf("count player rows: %v", err)
		}
		return n
	}

	// startRound puts two fresh players through the REAL join: a seeded open match,
	// backdated so matchmaking provably lands on THIS one (it takes the OLDEST open
	// match the caller is not in, and a shared database holds others), then a
	// CreateOrJoin that fills the roster — the one site that starts a round.
	// Returns the match and its two players.
	startRound := func(t *testing.T, tag string) (string, string, string) {
		t.Helper()
		host, joiner := mkUser(tag+"-host"), mkUser(tag+"-joiner")

		var joinView MatchView
		withMatchmaking(t, ctx, pool, func() {
			m, err := q.CreateMatch(ctx, promptID)
			if err != nil {
				t.Fatalf("create match: %v", err)
			}
			track(m.ID)
			if err := q.AddMatchPlayer(ctx, db.AddMatchPlayerParams{MatchID: m.ID, UserID: host}); err != nil {
				t.Fatalf("seat host: %v", err)
			}
			if _, err := pool.Exec(ctx,
				"update matches set created_at = now() - make_interval(secs => $2::int) where id = $1",
				m.ID, int32(10*365*24*time.Hour/time.Second)); err != nil {
				t.Fatalf("backdate match: %v", err)
			}

			joinView, err = svc.CreateOrJoin(ctx, joiner)
			if err != nil {
				t.Fatalf("CreateOrJoin (joiner): %v", err)
			}
			track(joinView.ID)
			if joinView.ID != m.ID {
				t.Fatalf("joined match %s, want the seeded %s", joinView.ID, m.ID)
			}
		})
		if joinView.Status != statusDrawing {
			t.Fatalf("status = %q, want %q — no round, nothing granted", joinView.Status, statusDrawing)
		}
		return joinView.ID, host, joiner
	}

	submit := func(t *testing.T, matchID, uid string) {
		t.Helper()
		raw := []byte(markedDocJSON("mark-" + uid))
		doc, err := document.ParseAndValidate(raw)
		if err != nil {
			t.Fatalf("fixture doc rejected: %v", err)
		}
		if _, err := svc.Submit(ctx, uid, matchID, doc, raw); err != nil {
			t.Fatalf("Submit: %v", err)
		}
	}

	// expireRound moves the deadline into the past so the next resolve sees a round
	// whose time is up, then resolves it the way the sweeper does.
	expireRound := func(t *testing.T, matchID string) resolveOutcome {
		t.Helper()
		if _, err := pool.Exec(ctx,
			"update matches set drawing_deadline = now() - make_interval(secs => 1) where id = $1",
			matchID); err != nil {
			t.Fatalf("expire deadline: %v", err)
		}
		outcome, err := svc.resolveExpiredMatch(ctx, matchID)
		if err != nil {
			t.Fatalf("resolveExpiredMatch: %v", err)
		}
		return outcome
	}

	// waitForStatus polls until the out-of-band judging pass has resolved the match.
	// The pass is dispatched post-commit by Submit, so without this the cleanup could
	// delete rows from under it.
	waitForStatus := func(t *testing.T, matchID, want string) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for {
			m, err := q.GetMatch(ctx, matchID)
			if err != nil {
				t.Fatalf("get match: %v", err)
			}
			if m.Status == want {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("match stuck at %q, want %q", m.Status, want)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}

	// --- (a) a round that starts grants the players and bills nobody else ----
	t.Run("a round that starts bills both players and no provider", func(t *testing.T) {
		before := providerRows()
		_, host, joiner := startRound(t, "start")

		for _, uid := range []string{host, joiner} {
			if got := playerRows(uid); got != 1 {
				t.Errorf("player %s has %d duel rows after the round started, want 1", uid, got)
			}
		}
		// The line this whole change is about. A started round is not a judge call:
		// it becomes one only if both players submit, and it may never become one at
		// all.
		if got := providerRows(); got != before {
			t.Errorf("starting a round wrote %d provider rows, want 0 — no request has been made yet", got-before)
		}
	})

	// --- (b) the provider is billed at judging -------------------------------
	t.Run("the provider is billed when the match enters judging", func(t *testing.T) {
		before := providerRows()
		mid, host, joiner := startRound(t, "judge")

		submit(t, mid, host)
		if got := providerRows(); got != before {
			t.Fatalf("the first submission wrote %d provider rows, want 0 — the round is still waiting", got-before)
		}

		// The last submission flips the match to judging, and THAT is the request.
		submit(t, mid, joiner)
		if got := providerRows() - before; got != 1 {
			t.Errorf("entering judging wrote %d provider rows, want exactly 1", got)
		}
		for _, uid := range []string{host, joiner} {
			if got := playerRows(uid); got != 1 {
				t.Errorf("player %s has %d duel rows, want 1 — submitting is not a second round", uid, got)
			}
		}
		// Let the out-of-band pass finish before the cleanup pulls the rows out from
		// under it. It does not touch the ledger; it is only here so the teardown does
		// not race a live transaction.
		waitForStatus(t, mid, statusDone)
	})

	// --- (c) rounds that never reach a judge ---------------------------------
	for _, tc := range []struct {
		name        string
		submitters  int
		wantOutcome resolveOutcome
	}{
		{name: "an abandoned round bills no provider", submitters: 0, wantOutcome: outcomeAbandoned},
		{name: "a forfeited round bills no provider", submitters: 1, wantOutcome: outcomeForfeit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := providerRows()
			mid, host, joiner := startRound(t, "expire")
			if tc.submitters == 1 {
				submit(t, mid, host)
			}

			if got := expireRound(t, mid); got != tc.wantOutcome {
				t.Fatalf("outcome = %v, want %v", got, tc.wantOutcome)
			}
			// The measured bug, from the other side: this used to be 1. Nobody was ever
			// asked anything, so the ledger must say nothing was.
			if got := providerRows() - before; got != 0 {
				t.Errorf("a round that reached no judge wrote %d provider rows, want 0", got)
			}
			// The players still had their round — they were granted one and they drew
			// (or failed to), which is theirs to spend either way.
			for _, uid := range []string{host, joiner} {
				if got := playerRows(uid); got != 1 {
					t.Errorf("player %s has %d duel rows, want 1", uid, got)
				}
			}
		})
	}

	// --- (d) a re-fire is another request ------------------------------------
	t.Run("a re-fired judging attempt bills the provider again and the players once", func(t *testing.T) {
		before := providerRows()
		mid, host, joiner := startRound(t, "refire")

		// Enter judging through the shared helper every judging site uses, rather than
		// through Submit, so no out-of-band pass resolves the match before the re-fire
		// can be tested against it.
		if err := svc.enterJudging(ctx, q, mid); err != nil {
			t.Fatalf("enterJudging: %v", err)
		}
		if got := providerRows() - before; got != 1 {
			t.Fatalf("the first pass wrote %d provider rows, want 1", got)
		}

		// What the stuck-judging sweep does to a wedged attempt. refireJudging is its
		// only caller; calling it directly keeps the assertion off the sweep's own
		// listing, which in a shared database may see rows this test did not create.
		refired, err := svc.refireJudging(ctx, mid)
		if err != nil {
			t.Fatalf("refireJudging: %v", err)
		}
		if !refired {
			t.Fatal("refireJudging reported no re-entry, so nothing was billed either")
		}

		if got := providerRows() - before; got != 2 {
			t.Errorf("provider rows after a re-fire = %d, want 2 — a second attempt is a second request", got)
		}
		for _, uid := range []string{host, joiner} {
			if got := playerRows(uid); got != 1 {
				t.Errorf("player %s has %d duel rows after a re-fire, want 1 — a hung judge is not a second round", uid, got)
			}
		}
	})
}
