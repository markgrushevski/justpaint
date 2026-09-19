package practice

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/markgrushevski/justpaint/server/internal/db"
	"github.com/markgrushevski/justpaint/server/internal/game"
	"github.com/markgrushevski/justpaint/server/internal/judge"
	"github.com/markgrushevski/justpaint/server/internal/render"
)

// budgetWindowSecs mirrors the game's rolling window (24h) for the count assertions
// below. The constant itself is unexported over there and deliberately so — the
// budget's rules live with the budget.
const budgetWindowSecs = int32(24 * 60 * 60)

// failingCritic is a critic that always fails, for the case the migration comment
// is actually about: a judge call that was spent and produced nothing.
type failingCritic struct{}

func (failingCritic) Critique(context.Context, judge.CritiqueRequest) (judge.Critique, error) {
	return judge.Critique{}, errors.New("the critic is having a day")
}

// TestPracticeRun_DB drives the whole practice loop against a real Postgres,
// because everything interesting about it is SQL: which rows count against the
// judge budget, whether an attempt survives a failed critique, and whether a
// retired prompt is reachable.
//
//	(a) the happy path end to end, through the fake critic;
//	(b) an unknown or deactivated promptId is a 404-shaped refusal;
//	(c) a practice run counts against BOTH halves of the daily judge budget, and
//	    both halves refuse when they are out;
//	(d) an attempt that failed in the critic is still recorded, and still counts —
//	    a failure that costs nothing is a failure the budget cannot see;
//	(e) an unconfigured critic (JUDGE_MODE=http) refuses without touching anything.
//
// Needs a migrated + seeded DATABASE_URL (docker compose up + goose up); skips
// otherwise, matching budget_db_test.go — including its pool-close-via-t.Cleanup
// ordering so no fixture leaks (Cleanup is LIFO: the pool.Close registered FIRST
// runs LAST).
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
	var userIDs, promptIDs []string
	t.Cleanup(func() {
		for _, uid := range userIDs {
			_, _ = pool.Exec(ctx, "delete from practice_runs where user_id = $1", uid)
			_, _ = pool.Exec(ctx, "delete from users where id = $1", uid)
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
	// art), a critic, and the game's own budget check — practice must spend from the
	// same ceiling through the same code.
	newService := func(critic judge.Critic, budget game.JudgeBudget) *Service {
		gameSvc := game.NewService(pool, q, nil, nil, logger)
		gameSvc.SetJudgeBudget(budget)
		return NewService(q, render.NewStubRenderer(), critic, gameSvc.CheckJudgeBudget, logger)
	}
	unbudgeted := game.JudgeBudget{}

	globalSpent := func() int64 {
		n, err := q.CountJudgeCallsInWindow(ctx, budgetWindowSecs)
		if err != nil {
			t.Fatalf("count judge calls: %v", err)
		}
		return n
	}
	// judgeCalls reads the global count together with its two halves in ONE
	// repeatable-read snapshot. The total is global state and `go test ./...` runs
	// packages in parallel — internal/game's DB tests flip matches into judging
	// while this one runs — so a before/after delta on it is not deterministic. The
	// identity `total == duels + practice` under one snapshot is, and it is the
	// actual claim being made: the ceiling counts practice runs too.
	judgeCalls := func() (total, duels, practice int64) {
		t.Helper()
		tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		if err != nil {
			t.Fatalf("begin snapshot: %v", err)
		}
		defer tx.Rollback(ctx)
		if total, err = q.WithTx(tx).CountJudgeCallsInWindow(ctx, budgetWindowSecs); err != nil {
			t.Fatalf("count judge calls: %v", err)
		}
		if err = tx.QueryRow(ctx, `select
			(select count(*) from matches where judging_started_at > now() - make_interval(secs => $1::int)),
			(select count(*) from practice_runs where created_at > now() - make_interval(secs => $1::int))`,
			budgetWindowSecs).Scan(&duels, &practice); err != nil {
			t.Fatalf("count the halves: %v", err)
		}
		return total, duels, practice
	}
	mineSpent := func(uid string) int64 {
		n, err := q.CountPlayerJudgeCallsInWindow(ctx, db.CountPlayerJudgeCallsInWindowParams{
			UserID: uid, WindowSecs: budgetWindowSecs,
		})
		if err != nil {
			t.Fatalf("count player judge calls: %v", err)
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
		svc := newService(judge.NewFakeCritic(), unbudgeted)

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
		if n := mineSpent(uid); n != 0 {
			t.Errorf("a refused run cost the player %d judge calls, want 0", n)
		}
	})

	// --- (c) practice spends the daily judge budget ------------------------
	t.Run("a practice run counts against both halves of the judge budget", func(t *testing.T) {
		uid := mkUser("budget")
		beforeMine := mineSpent(uid)
		_, _, beforePractice := judgeCalls()

		svc := newService(judge.NewFakeCritic(), game.JudgeBudget{
			Enforced: true, Global: int(globalSpent()) + 10, PerUser: int(beforeMine) + 1,
		})
		if _, err := svc.Run(ctx, uid, prompt.ID, doc); err != nil {
			t.Fatalf("Run under the cap: %v", err)
		}

		// The extension this whole feature turns on: a practice run is a judge call,
		// on both counts. Without it the ceiling would be no ceiling at all — practice
		// is one cheap request per judge call, with no opponent to wait for.
		total, duels, practice := judgeCalls()
		if practice != beforePractice+1 {
			t.Errorf("practice half = %d, want %d — the run must be counted", practice, beforePractice+1)
		}
		if total != duels+practice {
			t.Errorf("global count = %d, want %d (%d duels + %d practice runs) — the global ceiling must see practice", total, duels+practice, duels, practice)
		}
		if got := mineSpent(uid); got != beforeMine+1 {
			t.Errorf("per-player count = %d, want %d — a practice run must count", got, beforeMine+1)
		}

		// Now at their own cap: refused, and told it is THEIR allowance.
		if _, err := svc.Run(ctx, uid, prompt.ID, doc); !errors.Is(err, game.ErrDailyDuelsSpent) {
			t.Errorf("at the per-player cap: err = %v, want %v", err, game.ErrDailyDuelsSpent)
		}
		// The property that makes a per-player half worth having: one heavy user must
		// not deny service to everyone else.
		innocent := mkUser("budget-innocent")
		if _, err := svc.Run(ctx, innocent, prompt.ID, doc); err != nil {
			t.Errorf("a different player was refused too (%v) — one player's cap is not everyone's", err)
		}

		// The global half, with a per-player cap nowhere near binding.
		spent := globalSpent()
		strapped := newService(judge.NewFakeCritic(), game.JudgeBudget{
			Enforced: true, Global: int(spent), PerUser: 1000,
		})
		bystander := mkUser("budget-bystander")
		if _, err := strapped.Run(ctx, bystander, prompt.ID, doc); !errors.Is(err, game.ErrJudgeBudgetSpent) {
			t.Errorf("with the global budget spent: err = %v, want %v", err, game.ErrJudgeBudgetSpent)
		}
		// Headroom and the same player is welcome again. Deliberately more than one
		// call of it: the boundary (`>=`, not `>`) is pinned by the refusal above, and
		// a parallel test package can legitimately spend a slot in between.
		relaxed := newService(judge.NewFakeCritic(), game.JudgeBudget{
			Enforced: true, Global: int(globalSpent()) + 10, PerUser: 1000,
		})
		if _, err := relaxed.Run(ctx, bystander, prompt.ID, doc); err != nil {
			t.Errorf("with budget left the run was still refused: %v", err)
		}
	})

	// --- (d) a failed critique still spent a judge call --------------------
	t.Run("an attempt that failed in the critic is recorded and counted", func(t *testing.T) {
		uid := mkUser("failing")
		before := mineSpent(uid)

		svc := newService(failingCritic{}, unbudgeted)
		if _, err := svc.Run(ctx, uid, prompt.ID, doc); err == nil {
			t.Fatal("expected the critic's failure to surface, got a score")
		}

		// The migration comment's own reasoning: the row is written BEFORE the critic
		// is called, so a failure cannot be free. A transaction around the two writes
		// would have rolled this away — which is exactly when a broken critic is
		// draining the quota fastest.
		if got := mineSpent(uid); got != before+1 {
			t.Errorf("per-player count = %d, want %d — a spent-and-failed call must still count", got, before+1)
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
		svc := NewService(q, render.NewStubRenderer(), nil, nil, logger)

		if _, err := svc.Run(ctx, uid, prompt.ID, doc); !errors.Is(err, ErrNotConfigured) {
			t.Errorf("Run: err = %v, want %v", err, ErrNotConfigured)
		}
		if _, err := svc.Prompt(ctx); !errors.Is(err, ErrNotConfigured) {
			t.Errorf("Prompt: err = %v, want %v", err, ErrNotConfigured)
		}
		if n := mineSpent(uid); n != 0 {
			t.Errorf("a refused run cost the player %d judge calls, want 0", n)
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
