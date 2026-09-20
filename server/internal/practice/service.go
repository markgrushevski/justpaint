// Package practice is single-player mode: one prompt, one drawing, one score, no
// opponent (docs/API.md §8 covers the duel; this is its solo sibling).
//
// # Why it is not a match
//
// It does NOT reuse `matches`, and the reason is written into the schema
// (migration 00006) and visible in game/deadline.go: a single-seat drawing round
// falls into decideExpiry's "malformed roster" branch, gets flipped to judging,
// and runJudging demands exactly two submissions and wedges. The duel lifecycle —
// matchmaking, a shared deadline, forfeit, abandonment, the stuck-judging watchdog
// — is intricate precisely BECAUSE two people wait on each other. One player waits
// on nobody. So practice is a flat `practice_runs` row: no status, no deadline, no
// sweeper.
//
// # Why it is synchronous
//
// A duel judges out of band because the second player may still be drawing. Here
// the one player is already staring at a spinner, so the request does the render
// and the critique and returns the verdict; there is nothing to come back for.
//
// # Module shape
//
// Like internal/ratings, this is a small module over the shared *db.Queries plus
// an HTTP handler, wired in main.go next to the game handler. It holds NO pool and
// opens no transaction — see Run for why the two writes must not be one.
package practice

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/markgrushevski/justpaint/server/internal/db"
	"github.com/markgrushevski/justpaint/server/internal/document"
	"github.com/markgrushevski/justpaint/server/internal/judge"
	"github.com/markgrushevski/justpaint/server/internal/render"
)

// RunBudget bounds one practice run end to end: the authoritative render plus the
// critique, retries included.
//
// Unlike the duel's JudgePassBudget (60s, on a background context) this one has a
// hard ceiling it did not choose: the run happens INSIDE the request, and the HTTP
// server's WriteTimeout is 30s (cmd/server/main.go). Overrun there is not an error
// the player can read — it is a response cut off mid-write. 25s leaves the margin,
// and a clean 500 beats a dead socket.
//
// The arithmetic worth knowing: with the default JUDGE_TIMEOUT of 10s and the
// JUDGE.md §7 policy of 3 attempts, a critic that keeps timing out needs ~30s and
// will be cut short here. That is the honest trade — a player waiting half a
// minute for a practice score has already lost the round of feedback.
const RunBudget = 25 * time.Second

// Sentinel errors the handler maps onto HTTP responses.
var (
	// ErrPromptNotFound: the promptId names no ACTIVE prompt → 404. A retired
	// prompt answers exactly like a made-up one, so deactivation is not detectable
	// by trying to draw for it.
	ErrPromptNotFound = errors.New("practice: prompt not found")
	// ErrNotConfigured: no critic is wired, which today means JUDGE_MODE=http — the
	// collaborator's service implements the two-image Judge contract and has no
	// critique endpoint (docs/JUDGE.md §2 is frozen). → 500.
	//
	// It is deliberately NOT a silent fallback to FakeCritic. The fake scores ink
	// coverage and has never read a prompt; presenting its number as a real
	// critique is a lie the player cannot detect, and a feature that looks like it
	// works is worse than one that says it does not.
	ErrNotConfigured = errors.New("practice: no critic configured")
)

// BudgetCheck asks whether this player may spend an AI call right now, and
// BudgetSpend records the one they just spent. Both are plain funcs — the
// narrowest possible ports — bound to aibudget.KindPractice at the composition
// root, so this package never learns its own kind's name or the budget's shape.
//
// Practice has its OWN per-player allowance now, separate from the duel's; what
// it still shares with every other feature is the provider's global ceiling, and
// that is counted by ONE rule for all of them rather than by per-feature copies
// that drift (docs/GAME.md §4.3). Before the ledger this port was literally
// game.Service.CheckJudgeBudget, which made the game module the owner of solo
// mode's quota — a dependency practice no longer has.
//
// Nil means unbudgeted, which is what every test and every fake-configured
// deployment gets.
type BudgetCheck func(ctx context.Context, userID string) error

// BudgetSpend records one spent AI call against the caller's allowance. See
// BudgetCheck; it is a separate port because refusing and billing are separate
// decisions made at separate moments.
type BudgetSpend func(ctx context.Context, userIDs ...string) error

// Service runs the practice loop. It takes *db.Queries and no pool: the two writes
// here must NOT share a transaction (see Run).
type Service struct {
	q        *db.Queries
	renderer render.Renderer
	// critic is the seam (judge.Critic — ours, not the collaborator's frozen Judge).
	// Nil means practice is not configured; see ErrNotConfigured.
	critic judge.Critic
	budget BudgetCheck
	spend  BudgetSpend
	logger *slog.Logger
}

// NewService builds the practice service. A nil critic is a legitimate,
// deliberate state — the endpoints then refuse honestly instead of inventing
// scores — so it is not an error here; main.go logs it at boot.
func NewService(q *db.Queries, renderer render.Renderer, critic judge.Critic, budget BudgetCheck, spend BudgetSpend, logger *slog.Logger) *Service {
	return &Service{q: q, renderer: renderer, critic: critic, budget: budget, spend: spend, logger: logger}
}

// PromptView is one prompt offered for a practice run.
type PromptView struct {
	ID   string
	Text string
}

// RunView is a finished practice run: the score, the feedback, and the prompt it
// was scored against (echoed so the client renders a result without re-fetching).
type RunView struct {
	ID       string
	Score    float64
	Feedback string
	Prompt   PromptView
}

// Prompt hands out one random active prompt to draw. Unlike a duel — where the
// prompt is pinned server-side and hidden until an opponent arrives, so nobody can
// pre-draw (docs/GAME.md §5) — there is nothing to hide from a solo player and
// nobody to gain an advantage over, so the text comes back immediately.
//
// It refuses when practice is unconfigured: handing out a prompt we cannot score
// would walk the player through a whole drawing before admitting it.
func (s *Service) Prompt(ctx context.Context) (PromptView, error) {
	if s.critic == nil {
		return PromptView{}, ErrNotConfigured
	}
	p, err := s.q.PickRandomActivePrompt(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Same fault as the duel's ErrNoPrompts: the seed migration (00002) never
			// ran. A server problem, not a client one.
			return PromptView{}, fmt.Errorf("practice: no active prompts to offer: %w", err)
		}
		return PromptView{}, fmt.Errorf("practice: pick prompt: %w", err)
	}
	return PromptView{ID: p.ID, Text: p.Text}, nil
}

// Run scores one drawing against the prompt it claims to answer. The document is
// already validated by the handler (game.ValidateSubmission — one validator for
// both modes); doc is the parsed form the renderer needs.
//
// Order is deliberate:
//
//  1. the budget, before any expensive work — the whole point of a ceiling is to
//     refuse before the call, not after;
//  2. the prompt, which must be a live one (a foreign/retired id is a 404);
//  3. the ATTEMPT row, written BEFORE the critic is called;
//  4. render, then critique;
//  5. the verdict, stamped onto the row from (3).
//
// Steps 3 and 5 are separate statements and NOT one transaction, deliberately. A
// transaction would roll the attempt back when the critic failed, which is exactly
// backwards: that call was made, that quota was spent, and a failure that costs
// nothing is a failure the budget cannot see — precisely when a broken critic is
// draining it (migration 00006, docs/GAME.md §4.3).
func (s *Service) Run(ctx context.Context, userID, promptID string, doc document.Document) (RunView, error) {
	if s.critic == nil {
		return RunView{}, ErrNotConfigured
	}
	if s.budget != nil {
		if err := s.budget(ctx, userID); err != nil {
			return RunView{}, err // an aibudget refusal; the handler maps it
		}
	}

	prompt, err := s.q.GetActivePromptByID(ctx, promptID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RunView{}, ErrPromptNotFound
		}
		return RunView{}, fmt.Errorf("practice: get prompt: %w", err)
	}

	run, err := s.q.CreatePracticeRun(ctx, db.CreatePracticeRunParams{UserID: userID, PromptID: prompt.ID})
	if err != nil {
		return RunView{}, fmt.Errorf("practice: record attempt: %w", err)
	}
	// Billed here, beside the attempt row and for the same reason: BEFORE the
	// critic is called, and outside any transaction. A call recorded only on
	// success is a call the budget stops seeing exactly when a broken critic is
	// draining it — the failure that costs nothing is the one that costs the most.
	if s.spend != nil {
		if err := s.spend(ctx, userID); err != nil {
			return RunView{}, fmt.Errorf("practice: bill run: %w", err)
		}
	}

	// Bound the expensive half only. The budget check and the attempt row are
	// already committed, so a timeout here leaves exactly the state we want: a
	// spent call, counted.
	workCtx, cancel := context.WithTimeout(ctx, RunBudget)
	defer cancel()

	// The judged raster is derived server-side from the vector document, never taken
	// from the client — the same trust boundary as a duel (docs/GAME.md §6).
	img, err := s.renderer.Render(workCtx, doc)
	if err != nil {
		return RunView{}, fmt.Errorf("practice: render: %w", err)
	}

	startedAt := time.Now()
	critique, err := s.critic.Critique(workCtx, judge.CritiqueRequest{Prompt: prompt.Text, Image: img})
	if err != nil {
		return RunView{}, fmt.Errorf("practice: critique: %w", err)
	}
	if err := critique.Validate(); err != nil {
		return RunView{}, fmt.Errorf("practice: critique result: %w", err)
	}
	// The line that says the feature worked, mirroring "match judged": without it a
	// live critic is unobservable — you cannot tell a sane score from a degenerate
	// one (every run a 0), or notice latency creeping toward RunBudget. The score
	// and the latency, never the feedback: that is player-facing text which can
	// carry whatever a drawing provoked, and logs are not the place for it.
	s.logger.Info("practice run scored",
		"runID", run.ID,
		"prompt", prompt.Text,
		"score", critique.Score,
		"critique_ms", time.Since(startedAt).Milliseconds(),
		"render_bytes", len(img),
	)

	// Scoped by user_id as well as id: the row was created for this caller a few
	// lines up, so no row back is a bug in here, not a client's doing.
	stamped, err := s.q.SetPracticeRunVerdict(ctx, db.SetPracticeRunVerdictParams{
		ID: run.ID, UserID: userID, Score: critique.Score, Feedback: critique.Feedback,
	})
	if err != nil {
		return RunView{}, fmt.Errorf("practice: store verdict: %w", err)
	}

	return RunView{
		ID:       stamped.ID,
		Score:    critique.Score,
		Feedback: critique.Feedback,
		Prompt:   PromptView{ID: prompt.ID, Text: prompt.Text},
	}, nil
}
