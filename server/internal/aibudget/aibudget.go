// Package aibudget is the daily ceiling on every AI call this service makes —
// duels, practice runs, /draw guesses and AI assist — counted over one ledger
// table (migration 00007). Why one ledger instead of one table per feature:
// docs/DECISIONS.md "One AI-call ledger…"; the full rule, including ordering
// and row shapes: docs/GAME.md §4.3.
//
// A ledger row is either a PLAYER row ("this player was granted a round of
// this kind") or a PROVIDER row ("one request to the provider is about to be
// made"). For practice, guess and assist those are simultaneous, so one
// statement writes both. For a duel they are not: players are billed when the
// second seat fills, the provider is billed when the match enters judging —
// possibly more than once (a stuck-judging re-fire) or never (an abandoned
// round). Each half is written where it is true; that is what ForSplit is for.
//
// A kind whose Policy has no Provider is never checked and never recorded —
// the fake-impl case (JUDGE_MODE=fake, ASSIST_MODE=fake): no external quota to
// protect, so a dev loop or CI run must never hit a ceiling. Expressed as the
// absence of a provider rather than a separate "enforced" flag, because the
// two facts are one: you cannot spend a quota you have no provider for.
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

// window is the rolling 24h span both counts look back over — not a calendar
// day, so there is no timezone to get wrong and no fixed instant that hands
// out a fresh allowance right when a burst could empty it (docs/GAME.md §4.3).
// Postgres measures it against its own now() (the queries pass it as a
// param), so the app clock and the DB clock cannot skew it.
const window = 24 * time.Hour

// writeTimeout bounds one ledger write, on the caller's request context.
// http.Server.WriteTimeout does not cancel that context — it only stops the
// response being written — so a stalled insert would otherwise run past it
// with the caller watching a dead socket. Why 5s, and why this runs outside
// the caller's own work budget: docs/GAME.md §4.3.
const writeTimeout = 5 * time.Second

// windowSecs is the rolling window in the unit the queries take.
func windowSecs() int32 { return int32(window / time.Second) }

// Policy is one kind's ceiling.
//
// Provider says whose quota this kind spends (the global count is per
// provider) and, when empty, that this kind's impl makes no external calls —
// never enforced, never recorded (see the package comment). PerUser must be
// >= 1 when Provider is set — Policies rejects less at boot — so a zero here
// alongside a real provider is a wiring bug, not an operator mistake. Noun is
// the word for this kind in a refused player's message; Policies fills it
// from Kind.Noun, and an empty Noun still reads correctly since http.go falls
// back to the same table.
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
	// global caps how many calls one provider may serve inside the window,
	// across every kind — the number that keeps a free tier from being
	// emptied. The per-user caps keep one abuser from emptying it at everyone
	// else.
	global int
	logger *slog.Logger
}

// New builds the budget from the policies the composition root resolved. A
// kind absent from policies is unbudgeted, the same as an impl with no
// external calls.
//
// The map is copied: the ceiling is decided at boot, so a caller that kept a
// reference cannot move it afterwards.
func New(q *db.Queries, policies map[Kind]Policy, global int, logger *slog.Logger) *Budget {
	if logger == nil {
		// A refusal path is the worst place to panic, and the global refusal logs.
		logger = slog.Default()
	}
	return &Budget{q: q, policies: maps.Clone(policies), global: global, logger: logger}
}

// Check asks whether this user may spend a call of some kind right now. It is
// a plain func — the narrowest possible port — so a consumer module holds a
// field of this type and never imports the budget's own shape or its kinds.
//
// A nil Check means unbudgeted, so a caller may hold nil and skip the call.
type Check func(ctx context.Context, userID string) error

// Spend records a call one user is about to make: their player row and the
// provider row, in one statement that writes both or neither.
//
// It takes ONE user because the conditional insert behind it weighs the call
// against one allowance; the duel bills two players for a single request and
// uses ForSplit instead, since its two halves are not spent at the same
// moment.
//
// It returns the same *KindSpentError Check returns when the insert finds the
// caller already at their cap, so a caller needs no second branch for it.
//
// Spend is NOT transactional: a transaction would roll the record back when
// the provider call failed, which is backwards — that quota was spent, and a
// failure the budget cannot see is exactly when a broken provider drains it.
type Spend func(ctx context.Context, userID string) error

// BillPlayers records that these users were granted a round of some kind,
// through the caller's own tx-scoped *db.Queries (q.WithTx(tx)), so the rows
// commit or roll back with the work that granted the round.
//
// It writes no provider row: see BillProvider for why those are separate.
type BillPlayers func(ctx context.Context, q *db.Queries, userIDs ...string) error

// BillProvider records that one request to the provider is about to be made,
// through the caller's own tx-scoped *db.Queries.
//
// It writes no player row: a caller holding both ports calls them at
// different moments and counts — a duel's players are billed once, at their
// round's start, while the provider is billed at each entry into judging,
// including a stuck-judging re-fire, and possibly never, for a round that is
// abandoned or forfeited.
//
// This counts one row per judging pass, not per provider retry inside it —
// docs/JUDGE.md §7 covers why those stay uncounted.
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

// check is the advisory half of the rule; For's Check closure binds it.
//
// For a duel this runs at match creation: a player told "you are out of
// duels" before they pick up the brush has lost nothing, while the same
// message after a submit would be worse than the bug it guards against. Every
// kind checks before its expensive work for the same reason — a ceiling
// should refuse before the call, not after.
//
// It still earns its place beside the conditional insert that enforces the
// per-user half at write time: this refuses before the render, the insert
// refuses after it.
func (b *Budget) check(ctx context.Context, k Kind, userID string) error {
	p, ok := b.policies[k]
	if !ok || p.Provider == "" {
		return nil // unbudgeted — see the package comment
	}

	// Per-user first, and per kind: this player's duels do not eat their assists.
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
		// The one event here nobody else can see: a provider running dry takes
		// every feature backed by it down for everyone until the window rolls,
		// and without this line the only symptom is 429s the operator never sees.
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
		return nil // unbudgeted: an impl with no external call must not count
		// against a real provider's ceiling the day one is configured.
	}
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()

	// The query takes int32. An allowance beyond that is an operator typo, but
	// letting it wrap negative would refuse EVERY call, silently and backwards
	// from what they asked for.
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
		// The insert refused: this caller was already at cap, which the advisory
		// check a render ago could not have known. Same error Check returns, so
		// the caller's existing 429 branch handles it.
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
