// This file is the EXTERNAL test package (aibudget_test), not the in-package one
// its neighbour aibudget_test.go uses. It has to be: the duel path is the reason
// the ledger's row shape is the shape it is, and exercising it means calling
// internal/game — which imports this package for the two port types. An
// in-package test file would make that an import cycle ("import cycle not allowed
// in test"); an external test package is the supported way out and costs nothing
// here, because everything this suite touches is already exported.
package aibudget_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/markgrushevski/justpaint/server/internal/aibudget"
	"github.com/markgrushevski/justpaint/server/internal/db"
	"github.com/markgrushevski/justpaint/server/internal/game"
)

// budgetWindow mirrors the rolling window the budget counts over. The constant
// itself is unexported in aibudget and deliberately so — the budget's rules live
// with the budget — so this suite restates it, the way internal/practice's DB
// suite does.
const budgetWindow = 24 * time.Hour

// statusDrawing is game's own status string for a started round. Unexported over
// there; a duel is billed at open→drawing, so the suite has to be able to say
// that the round really started.
const statusDrawing = "drawing"

// TestAIBudget_DB proves the daily AI-call budget against a real Postgres: both
// counts are SQL over the ai_calls ledger (migration 00007), so only a database
// can say whether they count the right rows.
//
//	(a) a player under their cap starts a duel, through the real CreateOrJoin —
//	    and the three rows it writes are exactly the three the ledger's two counts
//	    depend on;
//	(b) a player AT their cap is refused — and a different player is not, which is
//	    the whole point of having a per-player half: one abuser must not deny
//	    service to everyone;
//	(c) a match nobody ever joined costs the player nothing;
//	(d) once the provider's global budget is reached, everybody is refused;
//	(e) an unbudgeted kind (a fake impl, so no Provider) skips both halves, even
//	    with caps that would otherwise refuse every duel;
//	(f) the window ROLLS: a call older than 24h counts against nobody, on either
//	    half;
//	(g) two KINDS do not share a per-user cap — a player out of duels can still
//	    practice;
//	(h) two PROVIDERS do not share the global cap — Google running dry must not
//	    throttle an Anthropic-backed kind.
//
// (g) and (h) are the two behaviours the ledger bought. Before it, per-user was
// one pot across every AI feature and the global count was service-wide, so both
// of these read as "refused" and nobody could see it.
//
// Needs a migrated + seeded DATABASE_URL (docker compose up + goose up, including
// migration 00007); skips otherwise, matching rating_db_test.go — including its
// pool-close-via-t.Cleanup ordering so no fixture leaks (Cleanup is LIFO: the
// pool.Close registered FIRST runs LAST).
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
	// No renderer/judge: nothing here reaches a judging pass — the budget refuses
	// (or allows) a duel long before anyone draws.
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
		// The ledger rows come out BEFORE the users they reference (ai_calls.user_id
		// is a FK). The provider-side rows name no user at all, so they are swept by
		// the test-private provider names instead — which is the other thing those
		// names are for.
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

	// mkProvider hands out a provider name nothing else in the world spends. The
	// global half counts by PROVIDER now, so a private name turns every global
	// assertion below from "a delta against whatever internal/practice is spending
	// in parallel" into an exact number — the isolation the old union-of-tables
	// count could not have, because there was only ever one global pot.
	mkProvider := func(tag string) aibudget.Provider {
		p := aibudget.Provider(fmt.Sprintf("test-%s-%d", tag, ns))
		providers = append(providers, string(p))
		return p
	}

	prompt, err := q.PickRandomActivePrompt(ctx)
	if err != nil {
		t.Fatalf("pick prompt (is the DB migrated + seeded? migration 00002): %v", err)
	}

	// partner fills the second seat of every fixture duel: a duel is billed when the
	// roster fills, so the duels have to be real two-seat matches.
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

	// joinableMatch seeds an open match and backdates it to the beginning of time, so
	// a later CreateOrJoin provably lands on THIS row: the matchmaking query takes the
	// OLDEST open match the caller is not in, and a shared dev database may well hold
	// open matches this test must not touch.
	joinableMatch := func(host string) string {
		mid := newMatch(host)
		if _, err := pool.Exec(ctx,
			"update matches set created_at = now() - make_interval(secs => $2::int) where id = $1",
			mid, int32(10*365*24*time.Hour/time.Second),
		); err != nil {
			t.Fatalf("backdate match: %v", err)
		}
		return mid
	}

	// useBudget installs a budget into the game service and hands it back, so a
	// subtest that also needs another kind's ports can ask the same instance. It is
	// the same post-construction wiring main.go does — check and the TRANSACTIONAL
	// spend, both bound to KindDuel once.
	useBudget := func(global int, policies map[aibudget.Kind]aibudget.Policy) *aibudget.Budget {
		b := aibudget.New(q, policies, global, logger)
		check, _ := b.For(aibudget.KindDuel)
		svc.SetBudget(check, b.ForTx(aibudget.KindDuel))
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
	// mineSpent is the per-user half — per KIND, which is the headline change: it
	// takes a kind because a player's duels are no longer the same pot as their
	// practice runs.
	mineSpent := func(uid string, k aibudget.Kind) int64 {
		n, err := q.CountUserKindCallsInWindow(ctx, db.CountUserKindCallsInWindowParams{
			UserID: uid, Kind: string(k), WindowSecs: windowSecs,
		})
		if err != nil {
			t.Fatalf("count %s calls for user: %v", k, err)
		}
		return n
	}
	// ledgerRows is every row the ledger holds for one player, of any kind — what
	// "this cost the player nothing" actually means.
	ledgerRows := func(uid string) int64 {
		var n int64
		if err := pool.QueryRow(ctx, "select count(*) from ai_calls where user_id = $1", uid).Scan(&n); err != nil {
			t.Fatalf("count ledger rows: %v", err)
		}
		return n
	}

	// ageUserCalls / ageProviderCalls backdate ledger rows so the rolling window can
	// be exercised without waiting a day. The window is counted over
	// ai_calls.created_at now — there is no lifecycle column to move instead, which
	// is most of why the ledger exists.
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
	// match. open→drawing, inside the matchmaking transaction, is now the ONLY site
	// that bills a duel, so a duel that counts has to be a duel that actually
	// started — there is no lifecycle column left to backdate one into existence.
	playDuel := func(t *testing.T, uid string) game.MatchView {
		t.Helper()
		want := joinableMatch(partner)
		view, err := svc.CreateOrJoin(ctx, uid)
		if err != nil {
			t.Fatalf("CreateOrJoin: %v", err)
		}
		track(view.ID)
		if view.ID != want {
			t.Fatalf("joined match %s, want the seeded %s", view.ID, want)
		}
		if view.Status != statusDrawing {
			t.Fatalf("status = %q, want %q — no round, no judge call, nothing billed", view.Status, statusDrawing)
		}
		return view
	}

	// --- (a) under the cap, a duel starts ---------------------------------
	t.Run("a player under their cap starts a duel", func(t *testing.T) {
		prov := mkProvider("under")
		useBudget(100, duelOnly(prov, 2))
		joiner := mkUser("under-joiner")

		view := playDuel(t, joiner)
		if len(view.Players) != 2 {
			t.Fatalf("roster has %d players, want 2", len(view.Players))
		}

		// The row shape the two counts depend on (migration 00007): ONE provider row
		// for the single request the duel costs the provider, ONE row per billed
		// player, and never both facts on one row. Rolling them together would make
		// the global count charge a duel twice — silently halving the real ceiling for
		// the product's main mode — and no other assertion in this suite would notice.
		var billsNobody, billsSomebody int64
		if err := pool.QueryRow(ctx, `select
			count(*) filter (where user_id is null),
			count(*) filter (where user_id is not null)
			from ai_calls where provider = $1`, string(prov),
		).Scan(&billsNobody, &billsSomebody); err != nil {
			t.Fatalf("read the provider rows: %v", err)
		}
		if billsNobody != 1 {
			t.Errorf("provider rows = %d, want exactly 1 — one duel is one request", billsNobody)
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
			// Both seats are billed for the one request. That is not a tax on being
			// joined: nobody is drawn into a duel passively, and both sides get the round.
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

		// Seeded through the real entry point under a cap that cannot bind, because
		// there is no other way to write a duel's rows honestly any more.
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
		// The exact shape the open-match reaper leaves behind: a match the player
		// created, nobody joined, swept to `abandoned`.
		//
		// This is now true by CONSTRUCTION rather than by a query's choice of anchor
		// column. The old count had to pick a lifecycle column that meant "a call was
		// actually spent" (`drawing_deadline is not null`) and get it right, so that a
		// `status <> 'open'` reading would not bill this player for having waited —
		// a cap on patience, not on judge calls. Now nothing derives anything: only
		// the join branch calls Spend, and the create and reuse branches write no
		// ledger row at all, so there is no row here to miscount.
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

		// Spend one call on this provider, the honest way. The count is exact rather
		// than a delta against the rest of the suite because the provider is ours
		// alone — the global half is per provider now, so isolation is a name.
		useBudget(1000, duelOnly(prov, 1000))
		playDuel(t, mkUser("global-first"))
		spent := providerSpent(prov)
		if spent != 1 {
			t.Fatalf("one duel wrote %d provider rows, want 1", spent)
		}

		// Fresh players, each miles under any per-player cap: only the global half can
		// refuse them.
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
		b := useBudget(int(spent)+10, duelOnly(prov, 1000))
		check, _ := b.For(aibudget.KindDuel)
		if err := check(ctx, mkUser("bystander-3")); err != nil {
			t.Errorf("with budget left the duel was still refused: %v", err)
		}
	})

	// --- (e) an unbudgeted kind skips both checks -------------------------
	t.Run("an unbudgeted kind skips both halves", func(t *testing.T) {
		prov := mkProvider("fake")
		joiner := mkUser("fake-joiner")

		// One real duel first, so the player is already over the cap installed below.
		useBudget(1000, duelOnly(prov, 1000))
		playDuel(t, joiner)
		before := ledgerRows(joiner)

		// Caps that would refuse every duel on earth — the missing Provider is the
		// only thing standing between them and this call, which is exactly the local
		// JUDGE_MODE=fake story: no external quota, so no ceiling, and nothing worth
		// recording either. The absence of a provider IS the off switch; there is no
		// separate `enforced` flag to get out of step with it.
		useBudget(0, map[aibudget.Kind]aibudget.Policy{aibudget.KindDuel: {PerUser: 0}})

		playDuel(t, joiner)

		// And a fake impl's calls must not count against a real provider's ceiling the
		// day one is configured.
		if after := ledgerRows(joiner); after != before {
			t.Errorf("an unbudgeted duel wrote %d ledger rows, want none", after-before)
		}
		if n := providerSpent(prov); n != 1 {
			t.Errorf("provider rows = %d, want 1 — only the budgeted duel may be there", n)
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
		// A duel inside the window does count — otherwise the case below would pass
		// for a count that is simply broken.
		if err := check(ctx, roller); !errors.Is(err, aibudget.ErrPerUserSpent) {
			t.Fatalf("a duel inside the window did not count: err = %v, want %v", err, aibudget.ErrPerUserSpent)
		}

		ageUserCalls(roller, stale)
		if err := check(ctx, roller); err != nil {
			t.Errorf("a call older than %s still counted against its player: %v", budgetWindow, err)
		}

		// And the global half rolls on the same clock. This provider is private to
		// this subtest, so backdating every one of its rows moves nothing else.
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
		// The headline behaviour change. Before the ledger, per-user was ONE pot
		// across every AI feature, so spending the day's duels also spent the day's
		// practice runs and the day's assists — which could only ever be the wrong
		// number for one of them, since the two are not substitutes for each other.
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
		// The line this whole subtest exists for.
		if err := practiceCheck(ctx, uid); err != nil {
			t.Errorf("a player out of duels was refused a practice run too (%v) — the two are separate allowances", err)
		}

		// And practice runs out on its OWN terms, naming its own kind, so the player
		// is told about the feature they actually asked for.
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
		// The second thing the ledger bought. One global pot meant an exhausted
		// Google quota refused an Anthropic-backed feature that still had plenty —
		// and the reverse — for no reason other than that they were counted together.
		google := mkProvider("google-ish")
		anthropic := mkProvider("anthropic-ish")
		b := aibudget.New(q, map[aibudget.Kind]aibudget.Policy{
			aibudget.KindDuel:     {Provider: google, PerUser: 1000},
			aibudget.KindPractice: {Provider: anthropic, PerUser: 1000},
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
		if n := providerSpent(anthropic); n != 0 {
			t.Errorf("the other provider was charged %d calls for a duel it never served", n)
		}
	})
}
