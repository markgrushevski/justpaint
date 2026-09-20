package practice

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/markgrushevski/justpaint/server/internal/aibudget"
	"github.com/markgrushevski/justpaint/server/internal/db"
	"github.com/markgrushevski/justpaint/server/internal/game"
	"github.com/markgrushevski/justpaint/server/internal/judge"
	"github.com/markgrushevski/justpaint/server/internal/render"
)

// budgetWindowSecs mirrors the budget's rolling window (24h) for the count
// assertions below. The constant itself is unexported in internal/aibudget and
// deliberately so — the budget's rules live with the budget.
const budgetWindowSecs = int32(24 * 60 * 60)

// quotaCritic is the provider saying the thing our own ceiling says: its daily
// budget is gone. Wrapped the way judge's Gemini client wraps it, because the
// handler's mapping is an errors.Is through a service wrap and a client wrap.
type quotaCritic struct{}

func (quotaCritic) Critique(context.Context, judge.CritiqueRequest) (judge.Critique, error) {
	return judge.Critique{}, fmt.Errorf("judge: %w (429): daily limit", judge.ErrQuotaExhausted)
}

// failingCritic is a critic that always fails, for the case the ledger comment
// is actually about: a judge call that was spent and produced nothing.
type failingCritic struct{}

func (failingCritic) Critique(context.Context, judge.CritiqueRequest) (judge.Critique, error) {
	return judge.Critique{}, errors.New("the critic is having a day")
}

// TestPracticeRun_DB drives the whole practice loop against a real Postgres,
// because everything interesting about it is SQL: which rows count against the
// daily AI budget, whether an attempt survives a failed critique, and whether a
// retired prompt is reachable.
//
//	(a) the happy path end to end, through the fake critic;
//	(b) an unknown or deactivated promptId is a 404-shaped refusal, and costs
//	    nothing — the budget check runs before it, the spend after it;
//	(c) a practice run writes BOTH ledger rows — the player's and the provider's —
//	    and both halves of the ceiling refuse when they are out;
//	(d) an attempt that failed in the critic is still recorded, and still counts —
//	    a failure that costs nothing is a failure the budget cannot see;
//	(e) an unconfigured critic (JUDGE_MODE=http) refuses without touching anything.
//
// Needs a migrated + seeded DATABASE_URL (docker compose up + goose up, including
// migration 00007); skips otherwise, matching aibudget's budget_db_test.go —
// including its pool-close-via-t.Cleanup ordering so no fixture leaks (Cleanup is
// LIFO: the pool.Close registered FIRST runs LAST).
func TestPracticeRun_DB(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB-backed practice test")
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

	// --- fixtures ---------------------------------------------------------
	var userIDs, promptIDs, providers []string
	t.Cleanup(func() {
		for _, uid := range userIDs {
			// The ledger rows come out BEFORE the users they reference
			// (ai_calls.user_id is a FK).
			_, _ = pool.Exec(ctx, "delete from ai_calls where user_id = $1", uid)
			_, _ = pool.Exec(ctx, "delete from practice_runs where user_id = $1", uid)
			_, _ = pool.Exec(ctx, "delete from users where id = $1", uid)
		}
		// The provider-side rows name no user at all, so they are swept by the
		// test-private provider names instead.
		for _, p := range providers {
			_, _ = pool.Exec(ctx, "delete from ai_calls where provider = $1", p)
		}
		for _, pid := range promptIDs {
			_, _ = pool.Exec(ctx, "delete from practice_runs where prompt_id = $1", pid)
			_, _ = pool.Exec(ctx, "delete from prompts where id = $1", pid)
		}
	})

	ns := time.Now().UnixNano()
	seq := 0
	mkUser := func(tag string) string {
		seq++
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Login:        fmt.Sprintf("practice-%s-%d-%d", tag, ns, seq),
			PasswordHash: "test-only-not-a-real-hash",
		})
		if err != nil {
			t.Fatalf("create user %s: %v", tag, err)
		}
		userIDs = append(userIDs, u.ID)
		return u.ID
	}

	// mkProvider hands out a provider name nothing else in the world spends.
	//
	// This is how the old suite's flakiness stays fixed. `go test ./...` runs
	// packages in parallel, and internal/aibudget's DB suite spends AI calls into
	// the same rolling window, so the global count was never safe to assert as an
	// absolute — the old file worked around that by reading the total and its two
	// halves in one repeatable-read snapshot and asserting the identity between
	// them. The ledger has no halves to add up any more; what it has instead is a
	// global count scoped PER PROVIDER, so a name nothing else spends scopes every
	// assertion below to rows this test created, which is stronger than a delta and
	// deterministic besides.
	mkProvider := func(tag string) aibudget.Provider {
		p := aibudget.Provider(fmt.Sprintf("test-practice-%s-%d", tag, ns))
		providers = append(providers, string(p))
		return p
	}

	prompt, err := q.PickRandomActivePrompt(ctx)
	if err != nil {
		t.Fatalf("pick prompt (is the DB migrated + seeded? migration 00002): %v", err)
	}

	// The one document every case draws: minimal, valid, and on the game canvas —
	// through the SAME validator a duel submission goes through, so this test would
	// fail if the two ever stopped agreeing.
	doc, err := game.ValidateSubmission([]byte(docOfSize(game.GameCanvasSize, game.GameCanvasSize)))
	if err != nil {
		t.Fatalf("the shared submission validator rejected the fixture document: %v", err)
	}

	// newService wires the production shape: the stub renderer (the loop, not the
	// art), a critic, and the two budget ports bound ONCE to aibudget.KindPractice.
	// Practice holds them as plain funcs and never learns its own kind's name — the
	// port used to be game.Service.CheckJudgeBudget, which made the game module the
	// owner of solo mode's quota.
	//
	// The ports pass straight through now. They used to be converted to local
	// BudgetCheck/BudgetSpend types on the grounds that this package then never
	// imported aibudget — which was never true, since the handler imports it for
	// WriteRefusal, so the copies bought a second name for one contract and nothing
	// else.
	newService := func(critic judge.Critic, b *aibudget.Budget) *Service {
		check, spend := b.For(aibudget.KindPractice)
		return NewService(q, render.NewStubRenderer(), critic, check, spend, logger)
	}
	// practiceBudget is the production policy shape for one subtest: its own
	// provider, its own per-kind allowance.
	practiceBudget := func(p aibudget.Provider, perUser, global int) *aibudget.Budget {
		return aibudget.New(q, map[aibudget.Kind]aibudget.Policy{
			aibudget.KindPractice: {Provider: p, PerUser: perUser},
		}, global, logger)
	}
	// unbudgeted is the fake-impl case: no policy at all, so nothing is checked and
	// nothing is recorded (aibudget's escape hatch).
	unbudgeted := aibudget.New(q, nil, 0, logger)

	// providerSpent is the global half, read the way the budget reads it.
	providerSpent := func(p aibudget.Provider) int64 {
		n, err := q.CountProviderCallsInWindow(ctx, db.CountProviderCallsInWindowParams{
			Provider: string(p), WindowSecs: budgetWindowSecs,
		})
		if err != nil {
			t.Fatalf("count %s calls: %v", p, err)
		}
		return n
	}
	// mineSpent is the per-user half — per KIND, which is the change practice cares
	// about most: a player's duels no longer eat their practice runs.
	mineSpent := func(uid string) int64 {
		n, err := q.CountUserKindCallsInWindow(ctx, db.CountUserKindCallsInWindowParams{
			UserID: uid, Kind: string(aibudget.KindPractice), WindowSecs: budgetWindowSecs,
		})
		if err != nil {
			t.Fatalf("count practice calls for user: %v", err)
		}
		return n
	}
	// ledgerRows is every row the ledger holds for one player, of any kind — what
	// "this run cost the player nothing" actually means.
	ledgerRows := func(uid string) int64 {
		var n int64
		if err := pool.QueryRow(ctx, "select count(*) from ai_calls where user_id = $1", uid).Scan(&n); err != nil {
			t.Fatalf("count ledger rows: %v", err)
		}
		return n
	}
	storedRun := func(runID string) db.PracticeRun {
		var r db.PracticeRun
		if err := pool.QueryRow(ctx,
			"select id, user_id, prompt_id, score, feedback, created_at from practice_runs where id = $1", runID,
		).Scan(&r.ID, &r.UserID, &r.PromptID, &r.Score, &r.Feedback, &r.CreatedAt); err != nil {
			t.Fatalf("read back run %s: %v", runID, err)
		}
		return r
	}

	// --- (a) the happy path ------------------------------------------------
	t.Run("a drawing is rendered, critiqued and recorded", func(t *testing.T) {
		uid := mkUser("happy")
		svc := newService(judge.NewFakeCritic(), unbudgeted)

		view, err := svc.Run(ctx, uid, prompt.ID, doc)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if _, err := uuid.Parse(view.ID); err != nil {
			t.Errorf("run id %q is not a uuid", view.ID)
		}
		if view.Score < 0 || view.Score > 1 {
			t.Errorf("score = %v, want [0,1]", view.Score)
		}
		if view.Feedback == "" {
			t.Error("feedback is empty — the player is owed a sentence, not just a number")
		}
		// The prompt is echoed so the client renders a result without re-fetching.
		if view.Prompt.ID != prompt.ID || view.Prompt.Text != prompt.Text {
			t.Errorf("prompt = %+v, want %s/%q", view.Prompt, prompt.ID, prompt.Text)
		}

		// The verdict survives the response that produced it (migration 00006).
		row := storedRun(view.ID)
		if row.UserID != uid {
			t.Errorf("run recorded against %s, want %s", row.UserID, uid)
		}
		if row.Score == nil || *row.Score != view.Score {
			t.Errorf("stored score = %v, want %v", row.Score, view.Score)
		}
		if row.Feedback == nil || *row.Feedback != view.Feedback {
			t.Errorf("stored feedback = %v, want the returned one", row.Feedback)
		}
	})

	// --- (b) the prompt must be a live one ---------------------------------
	t.Run("an unknown or retired prompt is not found", func(t *testing.T) {
		uid := mkUser("prompt")
		// Budgeted on purpose: only then is "a refused run cost the player nothing" a
		// claim about the ORDER of the loop — the budget check clears first, the
		// prompt lookup then fails, and the spend site a few lines further down is
		// never reached. Against an unbudgeted service it would be true of a service
		// that bills nobody for anything.
		svc := newService(judge.NewFakeCritic(), practiceBudget(mkProvider("prompt"), 20, 200))

		// A deactivated prompt must answer exactly like a made-up one, so retiring a
		// prompt is not detectable by trying to draw for it.
		var retiredID string
		if err := pool.QueryRow(ctx,
			"insert into prompts (text, active) values ($1, false) returning id",
			fmt.Sprintf("a retired prompt %d", ns),
		).Scan(&retiredID); err != nil {
			t.Fatalf("seed a retired prompt: %v", err)
		}
		promptIDs = append(promptIDs, retiredID)

		for _, tc := range []struct {
			name string
			id   string
		}{
			{"a prompt that never existed", uuid.NewString()},
			{"a prompt that was deactivated", retiredID},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if _, err := svc.Run(ctx, uid, tc.id, doc); !errors.Is(err, ErrPromptNotFound) {
					t.Errorf("err = %v, want %v", err, ErrPromptNotFound)
				}
			})
		}

		// And nothing was recorded for a run that never happened.
		if n := ledgerRows(uid); n != 0 {
			t.Errorf("a refused run cost the player %d ledger rows, want 0", n)
		}
	})

	// --- (c) practice spends the daily AI budget ---------------------------
	t.Run("a practice run bills the player and the provider", func(t *testing.T) {
		prov := mkProvider("budget")
		uid := mkUser("budget")

		svc := newService(judge.NewFakeCritic(), practiceBudget(prov, 1, 200))
		if _, err := svc.Run(ctx, uid, prompt.ID, doc); err != nil {
			t.Fatalf("Run under the cap: %v", err)
		}

		// The extension this whole feature turns on, now read off the ledger rather
		// than off a union of whichever tables a feature happened to write: one run is
		// TWO rows — the player's allowance and the provider's quota. Without the
		// provider row the global ceiling would be no ceiling at all for practice,
		// which is one cheap request per call with no opponent to wait for; without
		// the player row the per-user cap would never bind.
		var mine, theirs int64
		if err := pool.QueryRow(ctx, `select
			(select count(*) from ai_calls where user_id = $1 and provider is null),
			(select count(*) from ai_calls where provider = $2 and user_id is null)`,
			uid, string(prov),
		).Scan(&mine, &theirs); err != nil {
			t.Fatalf("read the ledger rows: %v", err)
		}
		if mine != 1 {
			t.Errorf("player rows = %d, want 1 — a practice run must count against its player", mine)
		}
		if theirs != 1 {
			t.Errorf("provider rows = %d, want 1 — the global ceiling must see practice", theirs)
		}
		if got := mineSpent(uid); got != 1 {
			t.Errorf("per-player count = %d, want 1", got)
		}

		// Now at their own cap: refused, and told it is THEIR allowance — and told
		// which one, so the message names scored drawings rather than "AI requests".
		_, err := svc.Run(ctx, uid, prompt.ID, doc)
		if !errors.Is(err, aibudget.ErrPerUserSpent) {
			t.Errorf("at the per-player cap: err = %v, want %v", err, aibudget.ErrPerUserSpent)
		}
		var spent *aibudget.KindSpentError
		if !errors.As(err, &spent) {
			t.Errorf("refusal %v does not carry a *KindSpentError", err)
		} else if spent.Kind != aibudget.KindPractice {
			t.Errorf("refusal named %q, want %q", spent.Kind, aibudget.KindPractice)
		}
		// The property that makes a per-player half worth having: one heavy user must
		// not deny service to everyone else.
		innocent := mkUser("budget-innocent")
		if _, err := svc.Run(ctx, innocent, prompt.ID, doc); err != nil {
			t.Errorf("a different player was refused too (%v) — one player's cap is not everyone's", err)
		}

		// The global half, with a per-player cap nowhere near binding. The count is
		// exact rather than a delta against the rest of the suite because the provider
		// is this subtest's alone.
		spentNow := providerSpent(prov)
		if spentNow != 2 {
			t.Fatalf("two runs wrote %d provider rows, want 2", spentNow)
		}
		strapped := newService(judge.NewFakeCritic(), practiceBudget(prov, 1000, int(spentNow)))
		bystander := mkUser("budget-bystander")
		if _, err := strapped.Run(ctx, bystander, prompt.ID, doc); !errors.Is(err, aibudget.ErrGlobalSpent) {
			t.Errorf("with the global budget spent: err = %v, want %v", err, aibudget.ErrGlobalSpent)
		}
		// Headroom and the same player is welcome again — the boundary is `>=`, not
		// `>`, pinned by the refusal above.
		relaxed := newService(judge.NewFakeCritic(), practiceBudget(prov, 1000, int(spentNow)+10))
		if _, err := relaxed.Run(ctx, bystander, prompt.ID, doc); err != nil {
			t.Errorf("with budget left the run was still refused: %v", err)
		}
	})

	// --- (d) a failed critique still spent a judge call --------------------
	t.Run("an attempt that failed in the critic is recorded and counted", func(t *testing.T) {
		uid := mkUser("failing")
		// Budgeted, because the ledger row is the thing being asserted and an
		// unbudgeted kind writes none: no provider, no external quota, nothing to
		// record.
		svc := newService(failingCritic{}, practiceBudget(mkProvider("failing"), 20, 200))
		if _, err := svc.Run(ctx, uid, prompt.ID, doc); err == nil {
			t.Fatal("expected the critic's failure to surface, got a score")
		}

		// The ledger's own reasoning, unchanged by the move: the attempt row AND the
		// ledger row are written BEFORE the critic is called, so a failure cannot be
		// free. A transaction around them would have rolled both away — which is
		// exactly when a broken critic is draining the quota fastest.
		if got := mineSpent(uid); got != 1 {
			t.Errorf("per-player count = %d, want 1 — a spent-and-failed call must still count", got)
		}
		var score *float64
		if err := pool.QueryRow(ctx,
			"select score from practice_runs where user_id = $1 order by created_at desc limit 1", uid,
		).Scan(&score); err != nil {
			t.Fatalf("read back the failed attempt: %v", err)
		}
		if score != nil {
			t.Errorf("a failed attempt stored score %v, want null", *score)
		}
	})

	// --- (e) an unconfigured critic refuses --------------------------------
	t.Run("an unconfigured critic refuses without recording anything", func(t *testing.T) {
		uid := mkUser("unconfigured")
		// Real budget ports, nil critic: the claim is that the not-configured guard
		// runs before either of them, so the player is neither billed nor recorded for
		// a feature that cannot run.
		b := practiceBudget(mkProvider("unconfigured"), 20, 200)
		check, spend := b.For(aibudget.KindPractice)
		svc := NewService(q, render.NewStubRenderer(), nil, check, spend, logger)

		if _, err := svc.Run(ctx, uid, prompt.ID, doc); !errors.Is(err, ErrNotConfigured) {
			t.Errorf("Run: err = %v, want %v", err, ErrNotConfigured)
		}
		if _, err := svc.Prompt(ctx); !errors.Is(err, ErrNotConfigured) {
			t.Errorf("Prompt: err = %v, want %v", err, ErrNotConfigured)
		}
		if n := ledgerRows(uid); n != 0 {
			t.Errorf("a refused run cost the player %d ledger rows, want 0", n)
		}
	})

	// --- (f) the provider's own refusal reads like ours --------------------
	t.Run("a provider that is out of quota answers 429, not 500", func(t *testing.T) {
		// Two ceilings can refuse a run: ours, which knows the player's allowance, and
		// the provider's, which we only learn about from a 429 on the wire. They are
		// the same news to the player — "not today" — so they must read the same. As a
		// 500 this offered a retry that could not possibly succeed until the
		// provider's own window rolled.
		//
		// Driven through the HTTP handler, because the mapping IS the handler's: the
		// service only wraps the error, and what matters is that the wrap survives to
		// the branch.
		uid := mkUser("provider-quota")
		svc := newService(quotaCritic{}, practiceBudget(mkProvider("provider-quota"), 20, 200))

		mux := http.NewServeMux()
		NewHandler(svc, logger).Routes(mux, authMiddleware(t))

		body := `{"promptId":"` + prompt.ID + `","document":` +
			docOfSize(game.GameCanvasSize, game.GameCanvasSize) + `}`
		req := httptest.NewRequest(http.MethodPost, "/api/practice", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(mintCookie(t, uid))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusTooManyRequests {
			t.Fatalf("status = %d, want 429 (body %s)", rec.Code, rec.Body.String())
		}
		if code := errorCode(t, rec); code != "rate_limited" {
			t.Errorf("code = %q, want rate_limited", code)
		}
		// The same sentence the global half of our own ceiling writes — one decision
		// about what a refusal discloses, in aibudget.
		if !strings.Contains(rec.Body.String(), "resumes tomorrow") {
			t.Errorf("body %s should read like the budget's own global refusal", rec.Body.String())
		}
		// And the call really was billed: it reached the provider, which is what spent
		// the quota. Billing only on success is how a broken impl drains a budget that
		// cannot see it.
		if n := ledgerRows(uid); n != 1 {
			t.Errorf("player ledger rows = %d, want 1 — the request was made", n)
		}
	})

	// --- the prompt handout ------------------------------------------------
	t.Run("a configured practice hands out an active prompt with its text", func(t *testing.T) {
		svc := newService(judge.NewFakeCritic(), unbudgeted)
		got, err := svc.Prompt(ctx)
		if err != nil {
			t.Fatalf("Prompt: %v", err)
		}
		if got.ID == "" || got.Text == "" {
			t.Fatalf("prompt = %+v, want both an id and its text", got)
		}
		// Unlike a duel, the text comes back immediately: there is nobody to pre-draw
		// against (docs/GAME.md §5 hides it only while a match is `open`).
		p, err := q.GetActivePromptByID(ctx, got.ID)
		if err != nil {
			t.Fatalf("the offered prompt is not an active one: %v", err)
		}
		if p.Text != got.Text {
			t.Errorf("text = %q, want %q", got.Text, p.Text)
		}
	})
}
