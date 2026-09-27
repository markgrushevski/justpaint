// Package aibudget is the daily ceiling on every AI call (duels, practice, /draw
// guesses, assist), counted over one ledger table. The rule, the row shapes and why a
// duel bills its player and provider halves apart (ForSplit): docs/GAME.md §4.3.
//
// A kind whose Policy has no Provider is never checked or recorded: a fake impl spends
// no external quota, so dev and CI never hit a ceiling.
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

// window is a rolling 24h, measured by Postgres against its own now(), so the app and
// DB clocks cannot skew it.
const window = 24 * time.Hour

// writeTimeout bounds one ledger write. http.Server.WriteTimeout does not cancel the
// request context, so without it a stalled insert outlives the response.
const writeTimeout = 5 * time.Second

func windowSecs() int32 { return int32(window / time.Second) }

// Policy is one kind's ceiling. An empty Provider means the kind's impl makes no
// external calls and is unbudgeted; with a Provider set, Policies rejects PerUser < 1
// at boot. Noun names the kind in a refused player's message.
type Policy struct {
	Provider Provider
	PerUser  int
	Noun     string
}

// Budget counts and records AI calls against the per-kind and per-provider ceilings.
// Consumers hold only the funcs For or ForSplit return, never this type.
type Budget struct {
	q        *db.Queries
	policies map[Kind]Policy
	global   int // calls one provider may serve per window, across every kind
	logger   *slog.Logger
}

// New builds the budget; a kind absent from policies is unbudgeted. The map is copied,
// so the ceilings cannot move after boot.
func New(q *db.Queries, policies map[Kind]Policy, global int, logger *slog.Logger) *Budget {
	if logger == nil {
		logger = slog.Default()
	}
	return &Budget{q: q, policies: maps.Clone(policies), global: global, logger: logger}
}

// Check asks whether this user may spend a call of some kind right now. A nil Check
// means unbudgeted.
type Check func(ctx context.Context, userID string) error

// Spend records one user's call before it is made: the player row and the provider
// row, in one conditional insert that writes both or neither. At cap it returns the
// same *KindSpentError as Check. It is not transactional with the call on purpose: a
// failed provider call still spent the quota.
type Spend func(ctx context.Context, userID string) error

// BillPlayers records that these users were granted a round, through the caller's
// tx-scoped queries, so the rows commit or roll back with the grant.
type BillPlayers func(ctx context.Context, q *db.Queries, userIDs ...string) error

// BillProvider records one request about to be made to the provider, through the
// caller's tx-scoped queries. It counts judging passes, not the retries inside one
// (docs/JUDGE.md §7).
type BillProvider func(ctx context.Context, q *db.Queries) error

// For binds a kind at the composition root and returns the check/spend pair a
// single-actor consumer holds as func fields.
func (b *Budget) For(k Kind) (Check, Spend) {
	return func(ctx context.Context, userID string) error {
			return b.check(ctx, k, userID)
		}, func(ctx context.Context, userID string) error {
			return b.spend(ctx, k, userID)
		}
}

// ForSplit binds a kind whose player and provider halves happen at different moments,
// in transactions the caller owns: the duel, alongside For's Check.
func (b *Budget) ForSplit(k Kind) (BillPlayers, BillProvider) {
	return func(ctx context.Context, q *db.Queries, userIDs ...string) error {
			return b.billPlayers(ctx, q, k, userIDs)
		}, func(ctx context.Context, q *db.Queries) error {
			return b.billProvider(ctx, q, k)
		}
}

// check is the advisory read, run before any expensive work; spend's conditional
// insert enforces the per-user cap again at write time.
func (b *Budget) check(ctx context.Context, k Kind, userID string) error {
	p, ok := b.policies[k]
	if !ok || p.Provider == "" {
		return nil // unbudgeted
	}

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
		// Nothing else reports this, and it turns the provider off for everyone.
		b.logger.Warn("daily AI budget exhausted — nothing backed by this provider runs until the rolling window frees a slot",
			"provider", p.Provider, "kind", k, "spent", spent, "budget", b.global, "window", window)
		return ErrGlobalSpent
	}
	return nil
}

func (b *Budget) spend(ctx context.Context, k Kind, userID string) error {
	p, ok := b.policies[k]
	if !ok || p.Provider == "" {
		return nil // unbudgeted
	}
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()

	// The query takes int32; clamp, since a wrapped negative cap would refuse every call.
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
		// Already at cap: the advisory check ran a render ago.
		return &KindSpentError{Kind: k, Cap: p.PerUser, Noun: p.Noun}
	}
	return nil
}

func (b *Budget) billPlayers(ctx context.Context, q *db.Queries, k Kind, userIDs []string) error {
	p, ok := b.policies[k]
	if !ok || p.Provider == "" {
		return nil // unbudgeted
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

func (b *Budget) billProvider(ctx context.Context, q *db.Queries, k Kind) error {
	p, ok := b.policies[k]
	if !ok || p.Provider == "" {
		return nil // unbudgeted
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
