package game

import (
	"context"
	"fmt"
	"time"

	"github.com/markgrushevski/justpaint/server/internal/db"
)

// judgeBudgetWindow is the span both judge-call counts look back over. It ROLLS —
// "the last 24 hours", not "today". A calendar day would need a timezone to be
// meaningful, and every choice of one is wrong for somebody; worse, it hands out a
// full fresh allowance at a fixed instant, which is exactly when a burst empties
// the quota. A rolling window has no timezone at all, and since our ceiling sits
// below the external judge's own per-day quota, never exceeding N in ANY 24h also
// never exceeds N in whatever calendar day the provider is counting.
const judgeBudgetWindow = 24 * time.Hour

// JudgeBudget is the daily ceiling on judge calls, in two halves: PerUser caps how
// many duels one player may start inside judgeBudgetWindow, Global caps how many
// the whole service may. Both matter for different reasons — the global one is what
// keeps the external quota from being emptied, the per-user one is what keeps a
// single abuser from emptying it AT everyone else.
//
// Enforced is the fake-judge escape hatch, and it is a field rather than an
// inference so the decision is made once, at the composition root, where the judge
// impl is actually chosen: with JUDGE_MODE=fake there is no external quota to
// protect and a developer running the loop locally must never hit a ceiling.
// The zero value is therefore "no budget", which is what every test and every
// caller that never wires one gets.
//
// Global and PerUser are expected to be >= 1 (config.Load rejects anything less at
// boot); a zero reaching here while Enforced would refuse every duel, which is a
// programming error at the wiring site, not an operator mistake.
type JudgeBudget struct {
	Enforced bool
	Global   int
	PerUser  int
}

// SetJudgeBudget installs the daily judge-call ceiling after construction, the same
// post-construction wiring shape as SetPublisher: NewService's signature stays put,
// every existing test keeps its unbudgeted service, and main.go opts in once with
// the values config validated.
func (s *Service) SetJudgeBudget(b JudgeBudget) {
	s.budget = b
}

// checkJudgeBudget refuses a new duel when either half of the daily judge budget is
// spent. It runs at match CREATION, which is the only humane place for it: a player
// told "you are out of duels" before they pick up the brush has lost nothing, while
// the same message after two minutes of drawing and a submit would be worse than the
// bug this guards against.
//
// The counts are advisory, not transactional: two simultaneous creates can both read
// a count one below the ceiling and both pass. That is deliberate — serializing every
// match creation on a budget row would cost more than the handful of duels a burst
// can overshoot by, and the ceiling is already set below the external quota.
func (s *Service) checkJudgeBudget(ctx context.Context, userID string) error {
	if !s.budget.Enforced {
		return nil
	}
	windowSecs := int32(judgeBudgetWindow / time.Second)

	// Per-user first: it is the cheaper read (match_players is indexed by user_id),
	// it is the case that actually fires in practice, and checking it first means a
	// player over their own cap learns that — rather than being told about the
	// service's global state, which is none of their business.
	mine, err := s.q.CountPlayerDuelsInWindow(ctx, db.CountPlayerDuelsInWindowParams{
		UserID: userID, WindowSecs: windowSecs,
	})
	if err != nil {
		return fmt.Errorf("game: count player duels in window: %w", err)
	}
	if mine >= int64(s.budget.PerUser) {
		return ErrDailyDuelsSpent
	}

	spent, err := s.q.CountJudgeCallsInWindow(ctx, windowSecs)
	if err != nil {
		return fmt.Errorf("game: count judge calls in window: %w", err)
	}
	if spent >= int64(s.budget.Global) {
		// The one event here nobody else can see. A player hitting their own cap is
		// ordinary and stays quiet; the global budget running out means the product's
		// core feature is off for EVERYONE until the window rolls, and without this
		// line the only symptom is 429s the owner never receives.
		s.logger.Warn("daily judge budget exhausted — no new duels start until the rolling window frees a slot",
			"spent", spent, "budget", s.budget.Global, "window", judgeBudgetWindow)
		return ErrJudgeBudgetSpent
	}
	return nil
}
