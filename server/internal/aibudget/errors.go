package aibudget

import (
	"errors"
	"fmt"
)

// Two sentinels, not one: a caller's own turn being up and the service's turn
// being up are different news, so http.go turns them into different messages
// and check logs only the global one.
var (
	// ErrPerUserSpent means this caller has spent their own allowance for a
	// kind inside the rolling window. Everyone else still plays. Check never
	// returns it bare; it returns a *KindSpentError that unwraps to it, so
	// errors.Is(err, ErrPerUserSpent) is the kind-agnostic test and
	// errors.As(&KindSpentError{}) is the one that names the feature.
	ErrPerUserSpent = errors.New("aibudget: per-user daily allowance spent")
	// ErrGlobalSpent means the provider's whole daily budget is gone, so
	// nothing backed by it can be scored until the window rolls. The response
	// must not disclose the budget's size or what is left — operator
	// information (docs/API.md §8, §12).
	ErrGlobalSpent = errors.New("aibudget: daily budget spent")
)

// KindSpentError is the per-user refusal: which feature ran out, the cap it
// ran out against, and the word for that feature in the player's language, so
// one sentence in http.go serves every kind. Noun may be empty — a hand-built
// Policy carries none — and http.go then falls back to Kind.Noun.
//
// Returned as a pointer so errors.As has a single obvious target type.
type KindSpentError struct {
	Kind Kind
	Cap  int
	Noun string
}

// Error renders the operator-facing form; player-facing copy lives in http.go.
func (e *KindSpentError) Error() string {
	return fmt.Sprintf("aibudget: %s allowance of %d spent", e.Kind, e.Cap)
}

// Unwrap makes errors.Is(err, ErrPerUserSpent) true for every kind, so a
// caller that only needs to know which half refused never enumerates kinds.
func (e *KindSpentError) Unwrap() error { return ErrPerUserSpent }
