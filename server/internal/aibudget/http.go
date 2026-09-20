package aibudget

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/markgrushevski/justpaint/server/internal/platform/web"
)

// The player-facing refusal copy, in one place because the decision about what to
// disclose is one decision.
//
// Two rules govern it. The first: a player out of their own allowance and a
// service out of budget are not the same news, so they never share a sentence —
// one is "you have had your turn", the other is "the service has had its turn"
// and nothing the player does today will change it. The second: the global
// message says nothing about how large the budget is or how much of it is left.
// That is operator information. A per-kind message is free to name its cap,
// because that number is the player's OWN: they could have counted it themselves,
// and being told "all 2 of your AI guesses" rather than "all of them" is the
// difference between a limit and a shrug.
const (
	// msgKindSpentFmt takes the cap and the kind's noun (Kind.Noun). ONE template
	// rather than one constant per kind: the sentence never varied, only the noun
	// did, and one constant per kind meant one chance per kind for it to drift — which is how
	// `guess` ended up the only kind that named its own number.
	msgKindSpentFmt = "you have used all %d of your %s for today — new ones unlock as the day rolls over"
	// msgAnySpent is the refusal with no kind attached, so no cap to name either.
	// Check never returns that bare sentinel, but a caller that re-created it still
	// deserves plain language rather than a blank where a number should be.
	msgAnySpent    = "you have used all of your AI requests for today — new ones unlock as the day rolls over"
	msgGlobalSpent = "the AI budget for today is spent — this feature resumes tomorrow"
)

// WriteRefusal turns a budget refusal into the HTTP response, and reports whether
// err was one — so callers keep their own switch for everything else and only the
// budget's own statuses and copy live here.
//
// Both refusals are 429 + rate_limited (docs/API.md §3, §8, §12). Neither carries
// a Retry-After header, unlike the per-IP tiers: the window rolls continuously, so
// there is no fixed reset instant to name, and inventing one would be a promise
// the budget cannot keep (docs/API.md §3.1).
//
// It writes nothing and returns false for a nil error or any non-budget error.
func WriteRefusal(w http.ResponseWriter, err error) bool {
	var spent *KindSpentError
	switch {
	case errors.As(err, &spent):
		web.Error(w, http.StatusTooManyRequests, web.CodeRateLimited, perKindMessage(spent))
	case errors.Is(err, ErrPerUserSpent):
		// The sentinel without its kind. Check never returns this bare, but a caller
		// that wrapped or re-created it still deserves the right status rather than a
		// 500 dressed up as an internal fault.
		web.Error(w, http.StatusTooManyRequests, web.CodeRateLimited, msgAnySpent)
	case errors.Is(err, ErrGlobalSpent):
		web.Error(w, http.StatusTooManyRequests, web.CodeRateLimited, msgGlobalSpent)
	default:
		return false
	}
	return true
}

// perKindMessage fills the one template. The noun comes from the policy when the
// composition root set one and from Kind.Noun otherwise — the same table either
// way, so a hand-built Policy reads exactly like a configured one.
func perKindMessage(e *KindSpentError) string {
	noun := e.Noun
	if noun == "" {
		noun = e.Kind.Noun()
	}
	return fmt.Sprintf(msgKindSpentFmt, e.Cap, noun)
}
