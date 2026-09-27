package aibudget

// Kind names the feature that spent a call — the ledger's `kind` column
// (migration 00007). A named string type, not a plain string, so the compiler
// catches a typo; why the column itself takes no check constraint or enum:
// docs/DECISIONS.md "One AI-call ledger…".
type Kind string

const (
	// KindDuel is one judged duel (docs/GAME.md §4.3). The one kind that bills
	// two players for a single provider request.
	KindDuel Kind = "duel"
	// KindPractice is one scored solo run (docs/GAME.md §10).
	KindPractice Kind = "practice"
	// KindGuess is "ask the AI what I drew" on /draw — no row anywhere else,
	// since the drawing may never be saved.
	KindGuess Kind = "guess"
	// KindAssist is one AI-assist Op batch (docs/ASSIST.md). Its old ceiling,
	// an in-process token bucket, never actually held (docs/DECISIONS.md "One
	// AI-call ledger…").
	KindAssist Kind = "assist"
)

// kindNouns is what each kind is called in the one sentence a refused player
// reads — player-facing words, not the ledger's. A table rather than one
// message constant per kind, since only the noun varies and four constants
// meant four chances for one to drift. See http.go.
var kindNouns = map[Kind]string{
	KindDuel:     "duels",
	KindPractice: "scored drawings",
	KindGuess:    "AI guesses",
	KindAssist:   "AI drawing requests",
}

// fallbackNoun names a kind with no word of its own, so a future feature that
// ships its Kind before its word still refuses in plain language.
const fallbackNoun = "AI requests"

// Noun is this kind in the player's words, for the refusal message. Every
// shipped kind has one (pinned by TestAllKindsIsComplete); an unknown one
// falls back to a generic AI request.
func (k Kind) Noun() string {
	if n, ok := kindNouns[k]; ok {
		return n
	}
	return fallbackNoun
}

// Provider names whose free tier a call spends — the ledger's `provider`
// column. The global half is counted per provider, and, narrowed by
// WithModel, per model too (docs/GAME.md §4.3).
//
// An opaque key, not a closed set: the constants below are the vendors we
// know, but WithModel produces a perfectly good provider none of them spell.
// The ledger column is plain `text` for exactly that reason.
type Provider string

const (
	// ProviderGoogle backs every Gemini seam (JUDGE_MODE=gemini,
	// ASSIST_MODE=gemini). Almost never used bare — see WithModel.
	ProviderGoogle Provider = "google"
	// ProviderCollaborator is the external ML judge's service (JUDGE_MODE=http).
	// Its quota is not ours and we cannot see it, which is why we keep a
	// ceiling under it rather than waiting to be told we exceeded one.
	ProviderCollaborator Provider = "collaborator"
)

// WithModel narrows a provider to one model, producing the key
// "<provider>:<model>" the global ceiling is actually counted on — because
// Google's free tier meters requests per model, not per account
// (docs/GAME.md §4.3). A provider not metered per model keeps its bare name.
//
// An empty model returns the provider unchanged rather than "google:" — a key
// with nothing after the colon would count a pool that does not exist.
func (p Provider) WithModel(model string) Provider {
	if p == "" || model == "" {
		return p
	}
	return p + ":" + Provider(model)
}

// DefaultPerUser is the per-kind daily allowance a player gets when the
// operator configures none — per KIND, not one pot shared across every AI
// feature. Why these specific numbers: docs/GAME.md §4.3.
//
// Read-only: a caller that needs to vary a value should build its own
// map[Kind]Policy from it.
var DefaultPerUser = map[Kind]int{
	KindDuel:     20,
	KindPractice: 20,
	KindGuess:    2,
	KindAssist:   40,
}

// AllKinds lists every kind, in declaration order so an error message built
// from it is stable. Returns a fresh slice per call, so a caller may sort or
// filter it without editing the package's idea of what exists.
func AllKinds() []Kind {
	return []Kind{KindDuel, KindPractice, KindGuess, KindAssist}
}

// ParseKind resolves a configured name to a Kind, reporting whether it is
// one. The composition root uses it to refuse a typo'd env key at boot rather
// than at the first request, so a misspelled kind can't configure an
// allowance nothing reads while the feature it was meant to bound runs
// unbudgeted and looks fine.
//
// The match is exact; every other mode env (RENDER_MODE, JUDGE_MODE,
// ASSIST_MODE) is lowercased by config.Load before comparison, so
// normalization is the caller's job here too.
func ParseKind(s string) (Kind, bool) {
	for _, k := range AllKinds() {
		if string(k) == s {
			return k, true
		}
	}
	return "", false
}
