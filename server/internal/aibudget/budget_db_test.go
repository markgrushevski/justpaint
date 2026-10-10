// This file is the external test package (aibudget_test), not the in-package
// one aibudget_test.go uses. It has to be: exercising the duel path means
// calling internal/game, which imports this package for its two port types,
// and an in-package test file would make that an import cycle.
package aibudget_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/markgrushevski/justpaint/server/internal/aibudget"
	"github.com/markgrushevski/justpaint/server/internal/db"
	"github.com/markgrushevski/justpaint/server/internal/game"
)

// budgetWindow mirrors the rolling window the budget counts over. The
// constant is unexported in aibudget — the budget's rules live with the
// budget — so this suite restates it, like internal/practice's DB suite does.
const budgetWindow = 24 * time.Hour

// statusDrawing is game's own status string for a started round, unexported
// over there. A duel's players are billed at open→drawing, so the suite needs
// to say the round really started.
const statusDrawing = "drawing"

// matchmakingLockID guards the seed-an-open-match-then-join window against
// other DB suites that drive CreateOrJoin, which `go test ./...` runs
// concurrently against this same database (internal/game's budget_db_test.go
// holds the same id; the two must stay equal to be worth anything).
//
// Matchmaking is global by design: FindOpenMatchToJoin takes the oldest open
// async match the caller is not in, anywhere in the table. Two suites that
// each seed a backdated open match and join it will steal each other's,
// whenever their windows overlap.
const matchmakingLockID int64 = 20260920

// withMatchmaking runs fn holding matchmakingLockID. The lock is
// session-scoped, so it is taken and released on one pooled connection; a
// t.Fatal inside fn still releases it, because Goexit runs deferred calls.
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

// TestAIBudget_DB proves the daily AI-call budget against a real Postgres:
// both counts are SQL over the ai_calls ledger (migration 00007), so only a
// database can say whether they count the right rows.
//
//	(a) a player under their cap starts a duel, through the real CreateOrJoin;
//	(b) a player at their cap is refused, and a different player is not;
//	(c) a match nobody ever joined costs the player nothing;
//	(d) once the provider's global budget is reached, everybody is refused;
//	(e) an unbudgeted kind (a fake impl, so no Provider) skips both halves;
//	(f) the window rolls: a call older than 24h counts against nobody;
//	(g) two kinds do not share a per-user cap;
//	(h) two providers do not share the global cap.
//
// Needs a migrated + seeded DATABASE_URL (docker compose up + goose up,
// including migration 00007); skips otherwise, matching rating_db_test.go —
// including its pool-close-via-t.Cleanup ordering (Cleanup is LIFO: the
// pool.Close registered first runs last).
func TestAIBudget_DB(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB-backed AI-budget test")
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
	// No renderer/judge: the budget refuses or allows a duel long before anyone
	// draws, so nothing here reaches a judging pass.
	svc := game.NewService(pool, q, nil, nil, logger)

	windowSecs := int32(budgetWindow / time.Second)

	// --- fixtures ---------------------------------------------------------
	var userIDs, matchIDs, providers []string
	t.Cleanup(func() {
		for _, mid := range matchIDs {
			_, _ = pool.Exec(ctx, "delete from match_players where match_id = $1", mid)
			_, _ = pool.Exec(ctx, "delete from drawings where match_id = $1", mid)
			_, _ = pool.Exec(ctx, "delete from matches where id = $1", mid)
		}
		// Ledger rows come out before the users they reference (ai_calls.user_id
		// is a FK). Provider-side rows name no user, so they are swept by the
		// test-private provider names instead.
		for _, p := range providers {
			_, _ = pool.Exec(ctx, "delete from ai_calls where provider = $1", p)
		}
		for _, uid := range userIDs {
			_, _ = pool.Exec(ctx, "delete from ai_calls where user_id = $1", uid)
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

	// mkProvider hands out a provider name nothing else in the world spends, so
	// a global assertion below is an exact number rather than a delta against
	// whatever internal/practice is spending in parallel.
	mkProvider := func(tag string) aibudget.Provider {
		p := aibudget.Provider(fmt.Sprintf("test-%s-%d", tag, ns))
		providers = append(providers, string(p))
		return p
	}

	prompt, err := q.PickRandomActivePrompt(ctx)
	if err != nil {
		t.Fatalf("pick prompt (is the DB migrated + seeded? migration 00002): %v", err)
	}

	// partner fills the second seat of every fixture duel: a duel is billed
	// when the roster fills, so the duels have to be real two-seat matches.
	partner := mkUser("partner")

	// track hands a match id to the teardown, including from a subtest expecting
	// a refusal: a refusal returns no id, but if the gate ever stops refusing,
	// the match it silently created must still be torn down.
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

	// joinableMatch seeds an open match and backdates it, so a later
	// CreateOrJoin provably lands on this row: the matchmaking query takes the
	// oldest joinable open match the caller is not in, and a shared dev database
	// may well hold open matches this test must not touch.
	joinableMatch := func(host string) string {
		mid := newMatch(host)
		// The oldest joinable match: older than any other suite's, inside the open TTL.
		if _, err := pool.Exec(ctx,
			"update matches set created_at = now() - make_interval(secs => $2::int) where id = $1",
			mid, int32(90),
		); err != nil {
			t.Fatalf("backdate match: %v", err)
		}
		return mid
	}

	// useBudget installs a budget into the game service and hands it back, so a
	// subtest that also needs another kind's ports can ask the same instance —
	// the same post-construction wiring main.go does.
	useBudget := func(global int, policies map[aibudget.Kind]aibudget.Policy) *aibudget.Budget {
		b := aibudget.New(q, policies, global, logger)
		check, _ := b.For(aibudget.KindDuel)
		billPlayers, billProvider := b.ForSplit(aibudget.KindDuel)
		svc.SetBudget(check, billPlayers, billProvider)
		return b
	}
	// duelOnly is the common case: one kind, one provider, one per-user cap.
	duelOnly := func(p aibudget.Provider, perUser int) map[aibudget.Kind]aibudget.Policy {
		return map[aibudget.Kind]aibudget.Policy{aibudget.KindDuel: {Provider: p, PerUser: perUser}}
	}

	// providerSpent is the global half, read the way check reads it.
	providerSpent := func(p aibudget.Provider) int64 {
		n, err := q.CountProviderCallsInWindow(ctx, db.CountProviderCallsInWindowParams{
			Provider: string(p), WindowSecs: windowSecs,
		})
		if err != nil {
			t.Fatalf("count %s calls: %v", p, err)
		}
		return n
	}
	// mineSpent is the per-user half, per kind: a player's duels are not the
	// same pot as their practice runs.
	mineSpent := func(uid string, k aibudget.Kind) int64 {
		n, err := q.CountUserKindCallsInWindow(ctx, db.CountUserKindCallsInWindowParams{
			UserID: uid, Kind: string(k), WindowSecs: windowSecs,
		})
		if err != nil {
			t.Fatalf("count %s calls for user: %v", k, err)
		}
		return n
	}
	// ledgerRows is every row the ledger holds for one player, of any kind.
	ledgerRows := func(uid string) int64 {
		var n int64
		if err := pool.QueryRow(ctx, "select count(*) from ai_calls where user_id = $1", uid).Scan(&n); err != nil {
			t.Fatalf("count ledger rows: %v", err)
		}
		return n
	}

	// ageUserCalls / ageProviderCalls backdate ledger rows so the rolling
	// window can be exercised without waiting a day.
	ageUserCalls := func(uid string, d time.Duration) {
		if _, err := pool.Exec(ctx,
			"update ai_calls set created_at = now() - make_interval(secs => $2::int) where user_id = $1",
			uid, int32(d/time.Second),
		); err != nil {
			t.Fatalf("backdate player rows: %v", err)
		}
	}
	ageProviderCalls := func(p aibudget.Provider, d time.Duration) {
		if _, err := pool.Exec(ctx,
			"update ai_calls set created_at = now() - make_interval(secs => $2::int) where provider = $1",
			string(p), int32(d/time.Second),
		); err != nil {
			t.Fatalf("backdate provider rows: %v", err)
		}
	}

	// playDuel puts uid through the real entry point against a waiting partner
	// match. open→drawing, inside the matchmaking transaction, is the only
	// site that grants a duel to its players, so a duel counted against a
	// player has to be one that actually started.
	//
	// It writes no provider row: starting a round is not a request to the
	// judge (see aibudget.BillProvider). Subtests that need the global half
	// full bill it explicitly through the port the judging sites use.
	playDuel := func(t *testing.T, uid string) game.MatchView {
		t.Helper()
		var view game.MatchView
		// Seed and join under the lock: between those two statements this match
		// is the oldest open one in the table, which another suite's joiner
		// would otherwise take.
		withMatchmaking(t, ctx, pool, func() {
			want := joinableMatch(partner)
			var err error
			view, err = svc.CreateOrJoin(ctx, uid)
			if err != nil {
				t.Fatalf("CreateOrJoin: %v", err)
			}
			track(view.ID)
			if view.ID != want {
				t.Fatalf("joined match %s, want the seeded %s", view.ID, want)
			}
		})
		if view.Status != statusDrawing {
			t.Fatalf("status = %q, want %q — no round, nothing granted", view.Status, statusDrawing)
		}
		// A player in a live match is handed that match, not a new one, so a
		// subtest that plays the same player twice ends the round first.
		if _, err := pool.Exec(ctx, "update matches set status = 'done' where id = $1", view.ID); err != nil {
			t.Fatalf("end the round: %v", err)
		}
		return view
	}

	// --- (a) under the cap, a duel starts ---------------------------------
	t.Run("a player under their cap starts a duel", func(t *testing.T) {
		prov := mkProvider("under")
		b := useBudget(100, duelOnly(prov, 2))
		joiner := mkUser("under-joiner")

		view := playDuel(t, joiner)
		if len(view.Players) != 2 {
			t.Fatalf("roster has %d players, want 2", len(view.Players))
		}

		// Starting the round bills the players and nobody else: the request to
		// the judge has not happened and may never happen.
		if n := providerSpent(prov); n != 0 {
			t.Errorf("provider rows = %d after a round started, want 0 — no request has been made yet", n)
		}
		// The request, when it does happen, is billed through the port the
		// judging sites call — one row, naming no player.
		_, billProvider := b.ForSplit(aibudget.KindDuel)
		if err := billProvider(ctx, q); err != nil {
			t.Fatalf("bill the provider: %v", err)
		}

		// The row shape the two counts depend on: one provider row per request,
		// one row per billed player, never both facts on one row — rolling them
		// together would charge a duel twice against the global count.
		var billsNobody, billsSomebody int64
		if err := pool.QueryRow(ctx, `select
			count(*) filter (where user_id is null),
			count(*) filter (where user_id is not null)
			from ai_calls where provider = $1`, string(prov),
		).Scan(&billsNobody, &billsSomebody); err != nil {
			t.Fatalf("read the provider rows: %v", err)
		}
		if billsNobody != 1 {
			t.Errorf("provider rows = %d, want exactly 1 — one judging pass is one request", billsNobody)
		}
		if billsSomebody != 0 {
			t.Errorf("%d provider rows also name a player — the global count would then charge a duel twice", billsSomebody)
		}
		for _, p := range view.Players {
			var own, alsoProvider int64
			if err := pool.QueryRow(ctx, `select
				count(*) filter (where provider is null),
				count(*) filter (where provider is not null)
				from ai_calls where user_id = $1 and kind = $2`, p.UserID, string(aibudget.KindDuel),
			).Scan(&own, &alsoProvider); err != nil {
				t.Fatalf("read the player rows for %s: %v", p.UserID, err)
			}
			// Both seats are billed for the one request: nobody is drawn into a
			// duel passively, and both sides get the round.
			if own != 1 {
				t.Errorf("player %s has %d duel rows, want 1", p.UserID, own)
			}
			if alsoProvider != 0 {
				t.Errorf("player %s has %d rows that also name a provider", p.UserID, alsoProvider)
			}
		}
	})

	// --- (b) the per-player cap is per PLAYER -----------------------------
	t.Run("at their cap one player is refused and another still plays", func(t *testing.T) {
		const cap = 3
		prov := mkProvider("per-player")
		heavy := mkUser("heavy")
		innocent := mkUser("innocent")

		// Seeded through the real entry point under a cap that cannot bind: there
		// is no other way to write a duel's rows honestly.
		useBudget(1000, duelOnly(prov, cap+10))
		for i := 0; i < cap; i++ {
			playDuel(t, heavy)
		}
		if got := mineSpent(heavy, aibudget.KindDuel); got != cap {
			t.Fatalf("seeded %d duels for the heavy player, ledger says %d", cap, got)
		}

		b := useBudget(1000, duelOnly(prov, cap))
		check, _ := b.For(aibudget.KindDuel)

		// Through the real entry point: a refusal returns before the matchmaking
		// transaction, so this cannot disturb a row it did not create.
		view, err := svc.CreateOrJoin(ctx, heavy)
		if !errors.Is(err, aibudget.ErrPerUserSpent) {
			track(view.ID)
			t.Errorf("CreateOrJoin at the cap: err = %v, want %v", err, aibudget.ErrPerUserSpent)
		}
		// And it says WHICH allowance ran out, which is what lets one HTTP helper
		// write the duel's sentence rather than a generic one.
		var spent *aibudget.KindSpentError
		if !errors.As(err, &spent) {
			t.Errorf("refusal %v does not carry a *KindSpentError", err)
		} else if spent.Kind != aibudget.KindDuel || spent.Cap != cap {
			t.Errorf("refusal named %q/%d, want %q/%d", spent.Kind, spent.Cap, aibudget.KindDuel, cap)
		}

		// The property that makes a per-player cap worth having.
		if err := check(ctx, innocent); err != nil {
			t.Errorf("a different player was refused too (%v) — one abuser must not deny service to everyone", err)
		}

		// One duel short of the cap is still allowed, so the boundary is `>=`, not `>`.
		relaxed := useBudget(1000, duelOnly(prov, cap+1))
		check, _ = relaxed.For(aibudget.KindDuel)
		if err := check(ctx, heavy); err != nil {
			t.Errorf("one duel under the cap was refused: %v", err)
		}
	})

	// --- (c) waiting alone is free ----------------------------------------
	t.Run("a match nobody ever joined costs the player nothing", func(t *testing.T) {
		prov := mkProvider("lonely")
		lonely := mkUser("lonely")
		// The exact shape the open-match reaper leaves behind: a match the
		// player created, nobody joined, swept to `abandoned`. Only the join
		// branch calls Spend; the create and reuse branches write no ledger row
		// at all, so there is no row here to miscount.
		reaped := newMatch(lonely)
		if _, err := q.SetMatchAbandoned(ctx, reaped); err != nil {
			t.Fatalf("reap open match: %v", err)
		}

		if n := ledgerRows(lonely); n != 0 {
			t.Errorf("a never-joined match wrote %d ledger rows for its creator, want 0", n)
		}
		b := useBudget(1000, duelOnly(prov, 1))
		check, _ := b.For(aibudget.KindDuel)
		if err := check(ctx, lonely); err != nil {
			t.Errorf("a never-joined match counted against its creator: %v", err)
		}
	})

	// --- (d) the global budget refuses everyone ---------------------------
	t.Run("the global budget refuses everyone once reached", func(t *testing.T) {
		prov := mkProvider("global")

		// Spend one call the honest way: a round that starts and then reaches
		// judging, the only thing that bills a provider.
		b := useBudget(1000, duelOnly(prov, 1000))
		playDuel(t, mkUser("global-first"))
		_, billProvider := b.ForSplit(aibudget.KindDuel)
		if err := billProvider(ctx, q); err != nil {
			t.Fatalf("bill the provider: %v", err)
		}
		spent := providerSpent(prov)
		if spent != 1 {
			t.Fatalf("one judging pass wrote %d provider rows, want 1", spent)
		}

		// Fresh players, each miles under any per-player cap: only the global
		// half can refuse them.
		useBudget(int(spent), duelOnly(prov, 1000))
		for _, tag := range []string{"bystander-1", "bystander-2"} {
			uid := mkUser(tag)
			if view, err := svc.CreateOrJoin(ctx, uid); !errors.Is(err, aibudget.ErrGlobalSpent) {
				track(view.ID)
				t.Errorf("%s: err = %v, want %v", tag, err, aibudget.ErrGlobalSpent)
			}
		}

		// Headroom and the same players are welcome again — the boundary is `>=`, not
		// `>`, pinned by the refusal above.
		roomy := useBudget(int(spent)+10, duelOnly(prov, 1000))
		check, _ := roomy.For(aibudget.KindDuel)
		if err := check(ctx, mkUser("bystander-3")); err != nil {
			t.Errorf("with budget left the duel was still refused: %v", err)
		}
	})

	// --- (e) an unbudgeted kind skips both checks -------------------------
	t.Run("an unbudgeted kind skips both halves", func(t *testing.T) {
		prov := mkProvider("fake")
		joiner := mkUser("fake-joiner")

		// One real duel first, so the player is already over the cap installed
		// below, and one real judging pass so the provider has a row of its own.
		b := useBudget(1000, duelOnly(prov, 1000))
		playDuel(t, joiner)
		_, billProvider := b.ForSplit(aibudget.KindDuel)
		if err := billProvider(ctx, q); err != nil {
			t.Fatalf("bill the provider: %v", err)
		}
		before := ledgerRows(joiner)

		// Caps that would refuse every duel on earth — the missing Provider is
		// the off switch, the JUDGE_MODE=fake story: no external quota, so no
		// ceiling and nothing worth recording.
		fake := useBudget(0, map[aibudget.Kind]aibudget.Policy{aibudget.KindDuel: {PerUser: 0}})

		playDuel(t, joiner)
		// Both halves are off, not just the player one: an unbudgeted judging
		// pass must not write a provider row either.
		_, billFakeProvider := fake.ForSplit(aibudget.KindDuel)
		if err := billFakeProvider(ctx, q); err != nil {
			t.Fatalf("an unbudgeted provider bill errored: %v", err)
		}

		if after := ledgerRows(joiner); after != before {
			t.Errorf("an unbudgeted duel wrote %d ledger rows, want none", after-before)
		}
		if n := providerSpent(prov); n != 1 {
			t.Errorf("provider rows = %d, want 1 — only the budgeted pass may be there", n)
		}
	})

	// --- (f) the window rolls ---------------------------------------------
	t.Run("a call older than the window counts against nobody", func(t *testing.T) {
		stale := budgetWindow + time.Hour
		prov := mkProvider("rolling")
		roller := mkUser("roller")

		b := useBudget(1000, duelOnly(prov, 1))
		check, _ := b.For(aibudget.KindDuel)

		playDuel(t, roller)
		// A duel inside the window does count — otherwise the case below would
		// pass for a count that is simply broken.
		if err := check(ctx, roller); !errors.Is(err, aibudget.ErrPerUserSpent) {
			t.Fatalf("a duel inside the window did not count: err = %v, want %v", err, aibudget.ErrPerUserSpent)
		}

		ageUserCalls(roller, stale)
		if err := check(ctx, roller); err != nil {
			t.Errorf("a call older than %s still counted against its player: %v", budgetWindow, err)
		}

		// The global half rolls on the same clock. This provider is private to
		// this subtest, so backdating every one of its rows moves nothing else.
		_, billProvider := b.ForSplit(aibudget.KindDuel)
		if err := billProvider(ctx, q); err != nil {
			t.Fatalf("bill the provider: %v", err)
		}
		if n := providerSpent(prov); n != 1 {
			t.Fatalf("provider rows inside the window = %d, want 1", n)
		}
		ageProviderCalls(prov, stale)
		if n := providerSpent(prov); n != 0 {
			t.Errorf("provider rows = %d, want 0 — a call older than %s must have rolled out", n, budgetWindow)
		}
	})

	// --- (g) per-user is per KIND -----------------------------------------
	t.Run("two kinds do not share a per-user cap", func(t *testing.T) {
		prov := mkProvider("per-kind")
		b := useBudget(1000, map[aibudget.Kind]aibudget.Policy{
			// Same provider on purpose: it is the KIND that separates the two
			// allowances here, not the quota they spend.
			aibudget.KindDuel:     {Provider: prov, PerUser: 1},
			aibudget.KindPractice: {Provider: prov, PerUser: 1},
		})
		uid := mkUser("per-kind")

		playDuel(t, uid)

		duelCheck, _ := b.For(aibudget.KindDuel)
		practiceCheck, practiceSpend := b.For(aibudget.KindPractice)

		var spent *aibudget.KindSpentError
		if err := duelCheck(ctx, uid); !errors.As(err, &spent) || spent.Kind != aibudget.KindDuel {
			t.Fatalf("at the duel cap: err = %v, want a duel *KindSpentError", err)
		}
		if err := practiceCheck(ctx, uid); err != nil {
			t.Errorf("a player out of duels was refused a practice run too (%v) — the two are separate allowances", err)
		}

		// Practice runs out on its own terms, naming its own kind.
		if err := practiceSpend(ctx, uid); err != nil {
			t.Fatalf("spend a practice call: %v", err)
		}
		if err := practiceCheck(ctx, uid); !errors.As(err, &spent) || spent.Kind != aibudget.KindPractice {
			t.Errorf("at the practice cap: err = %v, want a practice *KindSpentError", err)
		}
		if got := mineSpent(uid, aibudget.KindDuel); got != 1 {
			t.Errorf("duel count = %d, want 1 — a practice run must not land in the duel pot", got)
		}
	})

	// --- (h) global is per PROVIDER ---------------------------------------
	t.Run("two providers do not share the global cap", func(t *testing.T) {
		google := mkProvider("google-ish")
		other := mkProvider("collaborator-ish")
		b := aibudget.New(q, map[aibudget.Kind]aibudget.Policy{
			aibudget.KindDuel:     {Provider: google, PerUser: 1000},
			aibudget.KindPractice: {Provider: other, PerUser: 1000},
		}, 1, logger)
		uid := mkUser("two-providers")

		duelCheck, duelSpend := b.For(aibudget.KindDuel)
		practiceCheck, _ := b.For(aibudget.KindPractice)

		// One call empties the ceiling of 1 — for the provider that served it.
		if err := duelSpend(ctx, uid); err != nil {
			t.Fatalf("spend a duel call: %v", err)
		}
		if err := duelCheck(ctx, uid); !errors.Is(err, aibudget.ErrGlobalSpent) {
			t.Fatalf("with its provider's budget spent: err = %v, want %v", err, aibudget.ErrGlobalSpent)
		}
		if err := practiceCheck(ctx, uid); err != nil {
			t.Errorf("a kind on a different provider was refused too (%v) — one free tier running dry must not throttle another", err)
		}
		if n := providerSpent(other); n != 0 {
			t.Errorf("the other provider was charged %d calls for a duel it never served", n)
		}
	})

	// --- (i) the INSERT enforces the cap, not just the check ----------------
	t.Run("the ledger refuses at the cap even after the check passed", func(t *testing.T) {
		// Check is an unlocked read taken before the expensive work, so under a
		// burst every caller can read the same count and pass. The cap also
		// lives in the INSERT, which sees the count as it stands at the moment
		// of writing rather than a render and a round trip earlier.
		const cap = 2
		prov := mkProvider("insert-cap")
		uid := mkUser("insert-cap")
		b := aibudget.New(q, map[aibudget.Kind]aibudget.Policy{
			aibudget.KindGuess: {Provider: prov, PerUser: cap},
		}, 1000, logger)
		check, spend := b.For(aibudget.KindGuess)

		// The check this caller passed, at zero.
		if err := check(ctx, uid); err != nil {
			t.Fatalf("check at zero: %v", err)
		}

		// The ledger fills while they render, simulated by earlier calls landing.
		for i := 0; i < cap; i++ {
			if err := spend(ctx, uid); err != nil {
				t.Fatalf("spend %d of %d: %v", i+1, cap, err)
			}
		}
		// One spend is TWO rows, written by one statement: the player's and the
		// provider's. That is the atomicity the single statement buys — there is no
		// interleaving that can leave a provider row billing a request the refusal
		// means nobody will make.
		if got := mineSpent(uid, aibudget.KindGuess); got != cap {
			t.Fatalf("player rows = %d, want %d", got, cap)
		}
		if got := providerSpent(prov); got != cap {
			t.Fatalf("provider rows = %d, want %d — each spend writes exactly one of each", got, cap)
		}

		// The write refuses, though the check that preceded it did not.
		err := spend(ctx, uid)
		var refusal *aibudget.KindSpentError
		if !errors.As(err, &refusal) {
			t.Fatalf("spend at the cap: err = %v, want a *KindSpentError", err)
		}
		if refusal.Kind != aibudget.KindGuess || refusal.Cap != cap {
			t.Errorf("refusal named %q/%d, want %q/%d", refusal.Kind, refusal.Cap, aibudget.KindGuess, cap)
		}
		// The same error Check returns, so every caller's existing 429 branch
		// already handles it.
		if !errors.Is(err, aibudget.ErrPerUserSpent) {
			t.Errorf("the insert's refusal does not satisfy errors.Is(err, ErrPerUserSpent)")
		}
		// A refused spend writes nothing — not the player row, and not a
		// provider row for a request that will not be made.
		if got := mineSpent(uid, aibudget.KindGuess); got != cap {
			t.Errorf("player rows = %d after a refusal, want %d", got, cap)
		}
		if got := providerSpent(prov); got != cap {
			t.Errorf("provider rows = %d after a refusal, want %d — a refusal must not bill a request", got, cap)
		}
	})

	// --- (j) …and it holds when they all arrive at once ---------------------
	t.Run("simultaneous spends at the cap all refuse and bill nothing", func(t *testing.T) {
		// Deterministic on purpose: every statement here reads a count already
		// >= cap from committed rows, and no concurrent statement can lower it,
		// so all callers must refuse. This does not claim exactness below the
		// cap: two statements that overlap from under it can still see the same
		// pre-insert count under READ COMMITTED and each insert. The window for
		// that shrank from a check-render-round-trip to one statement — smaller,
		// not closed. Closing it would take SERIALIZABLE or a per-user lock, for
		// a ceiling that already sits below the provider's own.
		const (
			cap     = 2
			callers = 25
		)
		prov := mkProvider("burst")
		uid := mkUser("burst")
		b := aibudget.New(q, map[aibudget.Kind]aibudget.Policy{
			aibudget.KindGuess: {Provider: prov, PerUser: cap},
		}, 1000, logger)
		check, spend := b.For(aibudget.KindGuess)

		for i := 0; i < cap; i++ {
			if err := spend(ctx, uid); err != nil {
				t.Fatalf("seed spend %d: %v", i+1, err)
			}
		}

		var wg sync.WaitGroup
		errs := make([]error, callers)
		start := make(chan struct{})
		for i := range callers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				// check first, exactly as every caller does — and it may well pass,
				// because it is a read with no lock. The insert is what must hold.
				_ = check(ctx, uid)
				errs[i] = spend(ctx, uid)
			}()
		}
		close(start)
		wg.Wait()

		billed := 0
		for i, err := range errs {
			if err == nil {
				billed++
				continue
			}
			var refusal *aibudget.KindSpentError
			if !errors.As(err, &refusal) {
				t.Errorf("caller %d: err = %v, want a *KindSpentError", i, err)
			}
		}
		if billed != 0 {
			t.Errorf("%d of %d simultaneous callers were billed past the cap of %d", billed, callers, cap)
		}
		if got := mineSpent(uid, aibudget.KindGuess); got != cap {
			t.Errorf("player rows = %d, want %d", got, cap)
		}
		if got := providerSpent(prov); got != cap {
			t.Errorf("provider rows = %d, want %d — a refused burst must bill no requests", got, cap)
		}
	})
}
