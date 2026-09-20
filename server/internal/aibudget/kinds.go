package aibudget

// Kind names the feature that spent a call — the ledger's `kind` column
// (migration 00007). It is a named string type rather than a plain string for
// one concrete reason: the migration deliberately declines a check constraint or
// an enum on that column, on the grounds that "the value is a Go constant bound
// once at the composition root and never reaches here from a request, so the
// compiler already rejects a typo". These four constants are what makes that
// claim true. Adding a fifth AI feature costs a constant here and a line in
// DefaultPerUser — no migration, which is the friction the ledger exists to
// remove.
type Kind string

const (
	// KindDuel is one judged duel (docs/GAME.md §4.3). It is the one kind that
	// bills TWO players for a single provider request.
	KindDuel Kind = "duel"
	// KindPractice is one scored solo run (docs/GAME.md §10).
	KindPractice Kind = "practice"
	// KindGuess is "ask the AI what I drew" on /draw — no row anywhere else, since
	// the drawing may never be saved, which is half the reason the ledger exists.
	KindGuess Kind = "guess"
	// KindAssist is one AI-assist Op batch (docs/ASSIST.md). Its old ceiling was an
	// in-process token bucket that the host reset on every deploy and every wake
	// from idle, so it had never actually held (migration 00007).
	KindAssist Kind = "assist"
)

// Provider names whose free tier a call spends — the ledger's `provider` column.
// The global half of the ceiling is counted PER PROVIDER, never service-wide:
// Google running dry must not throttle an Anthropic-backed feature that still has
// quota, or the reverse (migration 00007).
type Provider string

const (
	// ProviderGoogle backs the Gemini judge and critic (JUDGE_MODE=gemini).
	ProviderGoogle Provider = "google"
	// ProviderAnthropic backs AI assist (ASSIST_MODE=anthropic).
	ProviderAnthropic Provider = "anthropic"
	// ProviderCollaborator is the ML collaborator's own service (JUDGE_MODE=http).
	// Its quota is not ours and we cannot see it, which is exactly why we keep a
	// ceiling under it rather than waiting to be told we exceeded one.
	ProviderCollaborator Provider = "collaborator"
)

// DefaultPerUser is the per-kind daily allowance one player gets when the
// operator configures none. The allowance is now per KIND, not one pot shared
// across every AI feature: spending a day's duels no longer costs a player their
// assists, because the two are not substitutes for each other and a single shared
// number could only ever be wrong for one of them.
//
// The shape of the numbers, not just the numbers: duel and practice keep the old
// shared default of 20 (JUDGE_DAILY_PER_USER) because they are the same act with
// and without an opponent. Guess is deliberately tiny — it is a novelty question
// about a drawing that may never be saved, and it would otherwise be the cheapest
// way to spend the whole provider quota. Assist is the loosest because it is a
// working tool: one honest drawing session makes several requests, and a ceiling
// that interrupts the work mid-drawing is a ceiling that breaks the feature
// rather than bounding it.
//
// This is a package-level map, so treat it as read-only; callers that need to
// vary a value should build their own map[Kind]Policy from it.
var DefaultPerUser = map[Kind]int{
	KindDuel:     20,
	KindPractice: 20,
	KindGuess:    2,
	KindAssist:   40,
}

// AllKinds lists every kind, in declaration order so an error message built from
// it is stable. It returns a fresh slice per call — the caller may sort or filter
// it without editing the package's idea of what exists.
func AllKinds() []Kind {
	return []Kind{KindDuel, KindPractice, KindGuess, KindAssist}
}

// ParseKind resolves a configured name to a Kind, reporting whether it is one.
// The composition root uses it to refuse a typo'd env key at BOOT rather than at
// the first request: a misspelled kind would otherwise configure an allowance
// nothing ever reads, and the feature it was meant to bound would run unbudgeted
// and look fine.
//
// The match is exact. Every other mode env in this service is lowercased by
// config.Load before it is compared (RENDER_MODE, JUDGE_MODE, ASSIST_MODE), so
// normalization is the caller's job here too — and keeping it out of ParseKind is
// what lets the boot error name the valid set honestly.
func ParseKind(s string) (Kind, bool) {
	for _, k := range AllKinds() {
		if string(k) == s {
			return k, true
		}
	}
	return "", false
}
