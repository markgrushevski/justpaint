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
// # A row is two facts, at two moments
//
// A ledger row is either a PLAYER row ("this player was granted a round of this
// kind") or a PROVIDER row ("one request to the provider is about to be made").
// For practice, guess and assist those two facts are simultaneous, so one
// statement writes both. For a duel they are not: the players are granted their
// round when the second seat fills, and the request is made when the match enters
// judging — which may be minutes later, may happen more than once (the
// stuck-judging re-fire), and may never happen at all (an abandoned or forfeited
// round makes no provider call). Each half is therefore written where it is true.
// That is what ForSplit is for.
//
// # How exact the two halves are
//
// The per-user half is enforced by the INSERT itself (RecordAICallUnderCap): the
// row is written only while the count is under the cap, so the ceiling no longer
// depends on a read taken a render and a network round trip earlier. It is still
// not exact — under READ COMMITTED two overlapping statements can each see the
// same pre-insert count — but the window shrank from seconds to the duration of
// one statement.
//
// The global half is deliberately advisory: it is read at Check and never gates
// a write. By the time a call is billed the expensive, human part has already
// happened (a duel's round has been played), and a refusal there would strand
// work rather than prevent it. The ceilings sit BELOW the provider's own quota
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
// the composition root, where the impl is actually chosen (see Policies).
package aibudget

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"math"
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

// writeTimeout bounds one ledger write.
//
// Every billing site runs on a caller context that nothing else will cut short in
// time: practice and guess run inside a request, and http.Server.WriteTimeout
// does not cancel the request context — it only stops the response being written.
// Their own RunBudget (25s) covers the render and the provider call, deliberately
// NOT the ledger write, which must not be lost to a nearly-spent work budget. A
// stalled insert would therefore sit past the 30s write timeout with a player
// watching a dead socket. Seconds are generous for a single-row insert on the
// same database the request already used; anything slower is a fault, not a wait.
const writeTimeout = 5 * time.Second

// windowSecs is the rolling window in the unit the queries take.
func windowSecs() int32 { return int32(window / time.Second) }

// Policy is one kind's ceiling.
//
// Provider is load-bearing twice over: it says WHOSE quota this kind spends (the
// global count is per provider) and, when empty, that this kind's impl makes no
// external calls — never enforced, never recorded. See the package comment.
//
// PerUser is expected to be >= 1 when a Provider is set (Policies rejects less at
// boot). A zero reaching here alongside a real provider would refuse every call,
// which is a programming error at the wiring site, not an operator mistake.
//
// Noun is what this kind is called in the sentence a refused player reads
// ("duels", "AI guesses"). It lives on the policy so the composition root decides
// player-facing copy in one place; Policies fills it from Kind.Noun, which is the
// one table of those words. An empty Noun still reads correctly — http.go falls
// back to the same table — so a hand-built Policy needs no ceremony.
type Policy struct {
	Provider Provider
	PerUser  int
	Noun     string
}

// Budget counts and records AI calls against the per-kind and per-provider
// ceilings. One instance serves every feature; each consumer holds only the funcs
// For or ForSplit hands it, never this type.
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
// absent from policies is unbudgeted, which is the same thing as an impl that
// makes no external calls.
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
// of this type and never imports the budget's own shape or its kinds.
//
// A nil Check means unbudgeted, matching what a kind with no provider returns
// anyway, so a caller may hold nil and skip the call.
type Check func(ctx context.Context, userID string) error

// Spend records a call that one user is about to make: their player row and the
// provider row, in one statement that writes both or neither.
//
// It takes ONE user, not a list, because the conditional insert behind it has one
// allowance to weigh the call against. The kind that bills two players for a
// single request — the duel — does not spend its two halves at the same moment
// anyway, and uses ForSplit instead.
//
// It returns the same *KindSpentError Check returns when the insert finds the
// caller already at their cap, so a caller needs no second branch: the refusal it
// already maps to a 429 is the refusal it gets.
//
// Spend is NOT transactional, deliberately: a transaction would roll the record
// back when the provider call failed, which is exactly backwards. That call was
// made, that quota was spent, and a failure that costs nothing is a failure the
// budget cannot see — precisely when a broken provider is draining it.
type Spend func(ctx context.Context, userID string) error

// BillPlayers records that these users were granted a round of some kind, through
// the caller's own tx-scoped *db.Queries (q.WithTx(tx)), so the rows commit or
// roll back with the work that granted the round.
//
// It writes no provider row: see BillProvider for why those are separate.
type BillPlayers func(ctx context.Context, q *db.Queries, userIDs ...string) error

// BillProvider records that one request to the provider is about to be made,
// through the caller's own tx-scoped *db.Queries.
//
// It writes no player row. A caller holding both ports calls them at different
// moments and a different number of times: a duel's players are billed once, when
// their round starts, while the provider is billed at each entry into judging —
// including a stuck-judging re-fire, which really is another request, and
// including never, for a round that is abandoned or forfeited and so never
// reaches a judge at all.
//
// The residual, stated plainly: this is one row per judging PASS, and the retries
// a judge impl makes INSIDE one Score call are not counted — three attempts, by
// docs/JUDGE.md §7. The only way to count them would be to hand the frozen Judge
// contract a database dependency (docs/JUDGE.md §2), which is not a trade worth
// making for a number that is already an exact count of passes and sits under a
// ceiling set below the provider's own.
type BillProvider func(ctx context.Context, q *db.Queries) error

// For binds a kind once, at the composition root, and returns the two
// one-question ports a single-actor consumer holds as plain func fields. The
// consumer then cannot ask about another feature's budget, cannot mis-name its
// own kind, and needs no import of this package at all.
func (b *Budget) For(k Kind) (Check, Spend) {
	return func(ctx context.Context, userID string) error {
			return b.check(ctx, k, userID)
		}, func(ctx context.Context, userID string) error {
			return b.spend(ctx, k, userID)
		}
}

// ForSplit binds a kind whose two halves happen apart, and in a transaction the
// caller owns. Used with For's Check by the duel, the only kind where "the player
// got a round" and "a request is being made" are separate events.
func (b *Budget) ForSplit(k Kind) (BillPlayers, BillProvider) {
	return func(ctx context.Context, q *db.Queries, userIDs ...string) error {
			return b.billPlayers(ctx, q, k, userIDs)
		}, func(ctx context.Context, q *db.Queries) error {
			return b.billProvider(ctx, q, k)
		}
}

// check is the advisory half of the rule; For's Check closure is a binding of it.
//
// For a duel this runs at match CREATION, which is the only humane place for it:
// a player told "you are out of duels" before they pick up the brush has lost
// nothing, while the same message after two minutes of drawing and a submit would
// be worse than the bug it guards against. Every other kind checks before the
// expensive work for the same reason — the point of a ceiling is to refuse before
// the call, not after.
//
// It still earns its place beside the conditional insert that now enforces the
// per-user half: this refuses before the render, the insert refuses after it.
func (b *Budget) check(ctx context.Context, k Kind, userID string) error {
	p, ok := b.policies[k]
	if !ok || p.Provider == "" {
		return nil // unbudgeted — see the package comment
	}

	// Per-user first, and per KIND: this player's duels do not eat their assists.
	mine, err := b.q.CountUserKindCallsInWindow(ctx, db.CountUserKindCallsInWindowParams{
		UserID: userID, Kind: string(k), WindowSecs: windowSecs(),
	})
	if err != nil {
		return fmt.Errorf("aibudget: count %s calls for user in window: %w", k, err)
	}
	if mine >= int64(p.PerUser) {
		return &KindSpentError{Kind: k, Cap: p.PerUser, Noun: p.Noun}
	}

	spent, err := b.q.CountProviderCallsInWindow(ctx, db.CountProviderCallsInWindowParams{
		Provider: string(p.Provider), WindowSecs: windowSecs(),
	})
	if err != nil {
		return fmt.Errorf("aibudget: count %s calls in window: %w", p.Provider, err)
	}
	if spent >= int64(b.global) {
		// The one event here nobody else can see. A player hitting their own cap is
		// ordinary and stays quiet; a provider's budget running out means every
		// feature backed by it is off for EVERYONE until the window rolls, and
		// without this line the only symptom is 429s the operator never sees.
		b.logger.Warn("daily AI budget exhausted — nothing backed by this provider runs until the rolling window frees a slot",
			"provider", p.Provider, "kind", k, "spent", spent, "budget", b.global, "window", window)
		return ErrGlobalSpent
	}
	return nil
}

// spend writes both halves of one single-actor call, under the per-user cap.
func (b *Budget) spend(ctx context.Context, k Kind, userID string) error {
	p, ok := b.policies[k]
	if !ok || p.Provider == "" {
		return nil // unbudgeted: an impl that makes no external call must not count
		// against a real provider's ceiling the day one is configured.
	}
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()

	// The query takes int32. An allowance beyond that is absurd — an operator who
	// typed extra zeros — but letting it wrap negative would make this refuse EVERY
	// call, silently and in the exact opposite direction from what they asked for.
	allowance := int32(math.MaxInt32)
	if int64(p.PerUser) < math.MaxInt32 {
		allowance = int32(p.PerUser)
	}
	rows, err := b.q.RecordAICallUnderCap(ctx, db.RecordAICallUnderCapParams{
		UserID: userID, Kind: string(k), Provider: string(p.Provider),
		WindowSecs: windowSecs(), Cap: allowance,
	})
	if err != nil {
		return fmt.Errorf("aibudget: record %s call: %w", k, err)
	}
	if rows == 0 {
		// The ledger refused: this caller was at their cap when the insert ran, which
		// the advisory check a render ago could not have known. Same error the check
		// returns, so the caller's existing 429 branch handles it.
		return &KindSpentError{Kind: k, Cap: p.PerUser, Noun: p.Noun}
	}
	return nil
}

// billPlayers writes the player half only, through the caller's queries.
func (b *Budget) billPlayers(ctx context.Context, q *db.Queries, k Kind, userIDs []string) error {
	p, ok := b.policies[k]
	if !ok || p.Provider == "" {
		return nil // unbudgeted — see spend
	}
	if len(userIDs) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()

	if err := q.RecordPlayerAICalls(ctx, db.RecordPlayerAICallsParams{
		UserIds: userIDs, Kind: string(k),
	}); err != nil {
		return fmt.Errorf("aibudget: record %s calls for players: %w", k, err)
	}
	return nil
}

// billProvider writes the provider half only, through the caller's queries.
func (b *Budget) billProvider(ctx context.Context, q *db.Queries, k Kind) error {
	p, ok := b.policies[k]
	if !ok || p.Provider == "" {
		return nil // unbudgeted — see spend
	}
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()

	if err := q.RecordProviderAICall(ctx, db.RecordProviderAICallParams{
		Kind: string(k), Provider: string(p.Provider),
	}); err != nil {
		return fmt.Errorf("aibudget: record %s provider call: %w", k, err)
	}
	return nil
}
