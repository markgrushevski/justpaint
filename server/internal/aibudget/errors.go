package aibudget

import (
	"errors"
	"fmt"
)

// The two refusals, which are 429s for the same reason and different news.
//
// They are separate sentinels rather than one because the thing a caller does
// with them differs: one says the PLAYER has had their turn, the other says the
// SERVICE has had its turn and nothing the player does will change it today.
// http.go turns that distinction into two different messages, and aibudget.go logs
// only the second (see Check).
var (
	// ErrPerUserSpent means this caller has spent their own allowance for a kind
	// inside the rolling window. Their problem alone — everyone else still plays.
	// Check never returns it bare; it returns a *KindSpentError that unwraps to it,
	// so `errors.Is(err, ErrPerUserSpent)` is the kind-agnostic test and
	// `errors.As(&KindSpentError{})` is the one that can name the feature.
	ErrPerUserSpent = errors.New("aibudget: per-user daily allowance spent")
	// ErrGlobalSpent means the provider's whole daily budget is gone, so nothing
	// backed by it can be scored until the window rolls. The player did nothing
	// wrong, and the response they get must not disclose how large the budget was
	// or how much is left — that is operator information (docs/API.md §8, §12).
	ErrGlobalSpent = errors.New("aibudget: daily budget spent")
)

// KindSpentError is the per-user refusal, carrying WHICH feature ran out, the
// cap it ran out against, and the word for that feature in a player's language.
// Together they let ONE sentence in http.go serve every kind: a player out of
// duels is told about duels and not about "AI requests", and the number they are
// told is their own cap, which is theirs to know.
//
// Noun may be empty — a Policy built by hand rather than by Policies carries no
// noun — and http.go then falls back to Kind.Noun, which is the same table
// Policies fills it from.
//
// It is returned as a pointer so `errors.As` has a single obvious target type.
type KindSpentError struct {
	Kind Kind
	Cap  int
	Noun string
}

// Error renders the operator-facing form. It is not the text a player sees:
// player-facing copy lives in http.go, where the decision about what to disclose
// is made once.
func (e *KindSpentError) Error() string {
	return fmt.Sprintf("aibudget: %s allowance of %d spent", e.Kind, e.Cap)
}

// Unwrap makes `errors.Is(err, ErrPerUserSpent)` true for every kind, so a caller
// that only needs to know WHICH HALF of the budget refused it never has to
// enumerate the kinds.
func (e *KindSpentError) Unwrap() error { return ErrPerUserSpent }
