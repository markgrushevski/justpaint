// Package practice is single-player mode: one prompt, one drawing, one score, no
// opponent — a flat practice_runs row scored synchronously, not a match
// (docs/GAME.md §10, docs/DECISIONS.md 2026-09-20).
package practice

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/markgrushevski/justpaint/server/internal/aibudget"
	"github.com/markgrushevski/justpaint/server/internal/db"
	"github.com/markgrushevski/justpaint/server/internal/document"
	"github.com/markgrushevski/justpaint/server/internal/judge"
	"github.com/markgrushevski/justpaint/server/internal/render"
)

// RunBudget bounds one practice run end to end (render plus critique, retries
// included). The work happens inside the request and the HTTP server's
// WriteTimeout is 30s (cmd/server/main.go), so 25s leaves margin for a clean 500
// instead of a response cut off mid-write.
const RunBudget = 25 * time.Second

// Sentinel errors the handler maps onto HTTP responses.
var (
	// ErrPromptNotFound means promptId names no active prompt, → 404. A retired
	// prompt answers exactly like a made-up one, so deactivation is not detectable
	// by trying to draw for it.
	ErrPromptNotFound = errors.New("practice: prompt not found")
	// ErrNotConfigured means no critic is wired (JUDGE_MODE=http has none), → 500.
	// Deliberately not a fallback to FakeCritic: see docs/JUDGE.md §8.2 for why a
	// fabricated score is worse than an honest refusal.
	ErrNotConfigured = errors.New("practice: no critic configured")
)

// budget and spend are aibudget's func types bound to aibudget.KindPractice at
// the composition root; nil means unbudgeted, which every test and
// fake-configured deployment gets. Practice has its own per-player allowance but
// shares the provider's global ceiling with every other AI feature (docs/GAME.md
// §4.3).

// Service runs the practice loop. It takes *db.Queries and no pool: the two writes
// here must NOT share a transaction (see Run).
type Service struct {
	q        *db.Queries
	renderer render.Renderer
	// critic is judge.Critic — ours, not the external judge's frozen Judge. Nil
	// means practice is unconfigured; see ErrNotConfigured.
	critic judge.Critic
	budget aibudget.Check
	spend  aibudget.Spend
	logger *slog.Logger
}

// NewService builds the practice service. A nil critic is a legitimate,
// deliberate state — the endpoints then refuse honestly instead of inventing
// scores — so it is not an error here; main.go logs it at boot.
func NewService(q *db.Queries, renderer render.Renderer, critic judge.Critic, budget aibudget.Check, spend aibudget.Spend, logger *slog.Logger) *Service {
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

// Prompt hands out one random active prompt to draw. Unlike a duel, it is never
// redacted — there is no opponent to be fair to (docs/GAME.md §5) — and it
// refuses when practice is unconfigured, rather than walking the player through
// a drawing we cannot score.
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

// Run scores one drawing against the prompt it claims to answer. doc is already
// validated by the handler (document.ValidateScored, as in a duel); raw is the
// same document as sent, stored with the attempt.
//
// Order matters: budget checked first, then the prompt looked up (a foreign or
// retired id is a 404), then an attempt row written before the critic runs, then
// render, then the ledger spent immediately before the critique (it re-checks the
// cap at write time, tighter than the check above), then the verdict stamped onto
// the attempt row.
//
// The attempt row and the verdict update are separate statements, not one
// transaction: a transaction would roll the attempt back when the critic failed,
// which is backwards — that call was made and that quota was spent (docs/GAME.md
// §4.3).
func (s *Service) Run(ctx context.Context, userID, promptID string, doc document.Document, raw []byte) (RunView, error) {
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

	run, err := s.q.CreatePracticeRun(ctx, db.CreatePracticeRunParams{UserID: userID, PromptID: prompt.ID, Document: raw})
	if err != nil {
		return RunView{}, fmt.Errorf("practice: record attempt: %w", err)
	}

	// Bounds the render and the critique only; the budget check and the attempt row
	// are already committed, so a timeout here still leaves a spent call, counted.
	workCtx, cancel := context.WithTimeout(ctx, RunBudget)
	defer cancel()

	// The judged raster is derived server-side from the vector document, never taken
	// from the client — the same trust boundary as a duel (docs/GAME.md §6).
	img, err := s.renderer.Render(workCtx, doc)
	if err != nil {
		return RunView{}, fmt.Errorf("practice: render: %w", err)
	}

	// Billed here, between the render and the critique: a critique that fails still
	// spent the provider's quota, and the render is ours and free (docs/GAME.md
	// §4.3, same rule as internal/guess). This write also re-checks the cap at the
	// instant of writing, tighter than the check at the top of Run.
	if s.spend != nil {
		if err := s.spend(ctx, userID); err != nil {
			return RunView{}, fmt.Errorf("practice: bill run: %w", err)
		}
	}

	startedAt := time.Now()
	critique, err := s.critic.Critique(workCtx, judge.CritiqueRequest{Prompt: prompt.Text, Image: img})
	if err != nil {
		return RunView{}, fmt.Errorf("practice: critique: %w", err)
	}
	if err := critique.Validate(); err != nil {
		return RunView{}, fmt.Errorf("practice: critique result: %w", err)
	}
	// Confirms the feature works and would surface latency creeping toward
	// RunBudget. Never the feedback: that is player-facing text that can carry
	// whatever the drawing provoked.
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
