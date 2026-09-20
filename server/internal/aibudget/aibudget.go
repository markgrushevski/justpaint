// Package aibudget is the daily ceiling on every AI call this service makes —
// duels, practice runs, /draw guesses and AI assist — counted over one ledger
// table (migration 00007) by one rule.
//
// # Why one package instead of one per feature
//
// The ceiling used to live in internal/game and be counted by unioning whichever
// tables a feature happened to write, which made the game module the owner of
// every other feature's quota and made each new feature re-derive a "which column
// means spent" judgement (see the migration for the full argument). The rule that
// has not changed is the one that mattered: features that spend the SAME external
// quota must be counted by ONE rule, never two that drift (docs/GAME.md §4.3).
// This package is that rule, with the tables it counts replaced by a ledger.
//
// # The two halves
//
// A call must clear both, in this order (asserted by docs/API.md §8 and §12):
// the caller's own per-kind allowance FIRST, the provider's global budget only
// once they have cleared it. Per-user is the cheaper read, it is the case that
// actually fires, and checking it first means a player over their own cap learns
// that — rather than being told about the service's global state, which is none
// of their business.
//
// # Advisory, not transactional
//
// The counts are reads with no lock: two simultaneous callers can both read a
// count one below the ceiling and both pass. That is deliberate — serializing
// every match creation on a budget row would cost more than the handful of calls
// a burst can overshoot by, and the ceilings are set BELOW the external quota
// precisely so a small overshoot stays inside it.
//
// # The escape hatch
//
// A kind whose Policy has no Provider is never checked and never recorded. That
// is the fake-impl case (JUDGE_MODE=fake, ASSIST_MODE=fake): there is no external
// quota to protect, and a developer running the loop locally or a CI run must
// never hit a ceiling. It is expressed as the absence of a provider rather than a
// separate "enforced" flag because the two facts are the same fact — you cannot
// spend a quota you have no provider for — and the decision is then made once, at
// the composition root, where the impl is actually chosen.
package aibudget

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"time"

	"github.com/markgrushevski/justpaint/server/internal/db"
)

// window is the span both counts look back over. It ROLLS — "the last 24 hours",
// not "today". A calendar day would need a timezone to be meaningful, and every
// choice of one is wrong for somebody; worse, it hands out a full fresh allowance
// at a fixed instant, which is exactly when a burst empties the quota. A rolling
// window has no timezone at all, and since our ceiling sits below the provider's
// own per-day quota, never exceeding N in ANY 24h also never exceeds N in
// whatever calendar day the provider is counting.
//
// It is measured by POSTGRES, not here: the queries pass it as a second count and
// subtract from the database's own now(), so the window cannot skew when the app
// clock and the DB clock disagree.
const window = 24 * time.Hour

// Policy is one kind's ceiling.
//
// Provider is load-bearing twice over: it says WHOSE quota this kind spends (the
// global count is per provider) and, when empty, that this kind's impl is a fake
// — never enforced, never recorded. See the package comment.
//
// PerUser is expected to be >= 1 when a Provider is set (config rejects less at
// boot). A zero reaching here alongside a real provider would refuse every call,
// which is a programming error at the wiring site, not an operator mistake.
type Policy struct {
	Provider Provider
	PerUser  int
}

// Budget counts and records AI calls against the per-kind and per-provider
// ceilings. One instance serves every feature; each consumer holds only the two
// funcs For hands it, never this type.
type Budget struct {
	q        *db.Queries
	policies map[Kind]Policy
	// global caps how many calls ONE provider may serve inside the window, across
	// every kind. It is the number that keeps a free tier from being emptied; the
	// per-user caps are what keep one abuser from emptying it AT everyone else.
	global int
	logger *slog.Logger
}

// New builds the budget from the policies the composition root resolved. A kind
// absent from policies is unbudgeted, which is the same thing as a fake impl.
//
// The map is copied: the ceiling is decided at boot and a caller that kept a
// reference should not be able to move it afterwards.
func New(q *db.Queries, policies map[Kind]Policy, global int, logger *slog.Logger) *Budget {
	if logger == nil {
		// A refusal path is the worst possible place to panic, and the global refusal
		// logs. Cheaper to have a logger than to require one.
		logger = slog.Default()
	}
	return &Budget{q: q, policies: maps.Clone(policies), global: global, logger: logger}
}

// Check asks whether this user may spend a call of some kind right now. It is a
// plain func — the narrowest possible port — so a consumer module holds a field
// of this type and never imports the budget's own shape or its kinds (the pattern
// internal/practice already uses for BudgetCheck).
//
// A nil Check means unbudgeted, matching what a fake-configured kind returns
// anyway, so a caller may hold nil and skip the call.
type Check func(ctx context.Context, userID string) error

// Spend records a call that was made: one ledger row for the provider request
// plus one for each billed player (migration 00007).
//
// It is variadic because the duel is the one kind that bills two players for a
// single provider request — rolling both facts into per-player rows would make
// the global count charge a duel twice, halving the real ceiling for the
// product's main mode.
//
// Spend is NOT transactional on its own, and deliberately does not decide whether
// it should be: the duel spends inside its matchmaking transaction (see SpendTx),
// while practice spends outside one on purpose, because a transaction would roll
// the record back when the provider call failed — which is exactly backwards.
// That call was made, that quota was spent, and a failure that costs nothing is a
// failure the budget cannot see, precisely when a broken provider is draining it.
type Spend func(ctx context.Context, userIDs ...string) error

// SpendTx is Spend for a caller that already has a transaction open: it writes
// the ledger rows through the caller's own tx-scoped *db.Queries (q.WithTx(tx)),
// so the rows commit or roll back with the work that spent them.
//
// It exists as a second port rather than as a *Budget clone (a WithTx(q) *Budget)
// because the kind is bound ONCE, at the composition root: a clone would have to
// be re-asked for its kind at the call site, deep inside the transaction, which is
// the one place the consumer module should not have to know its own kind's name.
// A consumer that spends inside a transaction holds a SpendTx field exactly the
// way practice holds a Spend field — same shape, one extra argument.
type SpendTx func(ctx context.Context, q *db.Queries, userIDs ...string) error

// For binds a kind once, at the composition root, and returns the two
// one-question ports a consumer module holds as plain func fields. The consumer
// then cannot ask about another feature's budget, cannot mis-name its own kind,
// and needs no import of this package at all.
func (b *Budget) For(k Kind) (Check, Spend) {
	return func(ctx context.Context, userID string) error {
			return b.check(ctx, k, userID)
		}, func(ctx context.Context, userIDs ...string) error {
			return b.record(ctx, b.q, k, userIDs)
		}
}

// ForTx binds a kind to the transactional spend port. Used alongside For by a
// consumer whose spend belongs inside its own transaction — the duel, whose call
// is decided by the same commit that starts the round.
func (b *Budget) ForTx(k Kind) SpendTx {
	return func(ctx context.Context, q *db.Queries, userIDs ...string) error {
		return b.record(ctx, q, k, userIDs)
	}
}

// check is the whole rule; For's Check closure is a binding of it.
//
// For a duel this runs at match CREATION, which is the only humane place for it:
// a player told "you are out of duels" before they pick up the brush has lost
// nothing, while the same message after two minutes of drawing and a submit would
// be worse than the bug it guards against. Every other kind checks before the
// expensive work for the same reason — the point of a ceiling is to refuse before
// the call, not after.
func (b *Budget) check(ctx context.Context, k Kind, userID string) error {
	p, ok := b.policies[k]
	if !ok || p.Provider == "" {
		return nil // unbudgeted — see the package comment
	}
	windowSecs := int32(window / time.Second)

	// Per-user first, and per KIND: this player's duels do not eat their assists.
	mine, err := b.q.CountUserKindCallsInWindow(ctx, db.CountUserKindCallsInWindowParams{
		UserID: userID, Kind: string(k), WindowSecs: windowSecs,
	})
	if err != nil {
		return fmt.Errorf("aibudget: count %s calls for user in window: %w", k, err)
	}
	if mine >= int64(p.PerUser) {
		return &KindSpentError{Kind: k, Cap: p.PerUser}
	}

	spent, err := b.q.CountProviderCallsInWindow(ctx, db.CountProviderCallsInWindowParams{
		Provider: string(p.Provider), WindowSecs: windowSecs,
	})
	if err != nil {
		return fmt.Errorf("aibudget: count %s calls in window: %w", p.Provider, err)
	}
	if spent >= int64(b.global) {
		// The one event here nobody else can see. A player hitting their own cap is
		// ordinary and stays quiet; a provider's budget running out means every
		// feature backed by it is off for EVERYONE until the window rolls, and
		// without this line the only symptom is 429s the owner never receives.
		b.logger.Warn("daily AI budget exhausted — nothing backed by this provider runs until the rolling window frees a slot",
			"provider", p.Provider, "kind", k, "spent", spent, "budget", b.global, "window", window)
		return ErrGlobalSpent
	}
	return nil
}

// record writes one call's ledger rows through q. Both spend ports are bindings
// of it; q is the only thing that differs between them.
func (b *Budget) record(ctx context.Context, q *db.Queries, k Kind, userIDs []string) error {
	p, ok := b.policies[k]
	if !ok || p.Provider == "" {
		return nil // unbudgeted: a fake impl's calls must not count against a real
		// provider's ceiling the day one is configured.
	}
	if q == nil {
		// A backstop, not an interface: every caller passes either its own tx-scoped
		// Queries or (via For) the budget's. A nil here means a wiring slip, and a
		// row written outside the intended transaction is far better than a panic in
		// a path whose entire job is to be unobtrusive.
		q = b.q
	}

	// The provider row first, and unconditionally: the request was made and that
	// cost is real even when there is no player to attribute it to.
	provider := string(p.Provider)
	if err := q.RecordAICall(ctx, db.RecordAICallParams{Kind: string(k), Provider: &provider}); err != nil {
		return fmt.Errorf("aibudget: record %s provider call: %w", k, err)
	}
	for _, uid := range userIDs {
		if err := q.RecordAICall(ctx, db.RecordAICallParams{UserID: &uid, Kind: string(k)}); err != nil {
			return fmt.Errorf("aibudget: record %s call for player: %w", k, err)
		}
	}
	return nil
}
