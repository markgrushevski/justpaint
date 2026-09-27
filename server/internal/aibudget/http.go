package aibudget

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/markgrushevski/justpaint/server/internal/platform/web"
)

// The player-facing refusal copy lives here, in one place, because what to
// disclose is one decision: a per-user refusal and a global one are never the
// same sentence, and the global message never states the budget's size or
// what is left (docs/API.md §8, §12) — a per-kind message may state the cap,
// since that number is the player's own.
const (
	// msgKindSpentFmt takes the cap and the kind's noun (Kind.Noun). One
	// template, not one constant per kind, so the noun is the only thing that
	// can drift — which is how `guess` once ended up the only kind naming its
	// own number.
	msgKindSpentFmt = "you have used all %d of your %s for today — new ones unlock as the day rolls over"
	// msgAnySpent is the refusal with no kind attached, so no cap to name.
	// Check never returns this bare sentinel, but a caller that re-created it
	// still deserves plain language rather than a blank where a number should be.
	msgAnySpent    = "you have used all of your AI requests for today — new ones unlock as the day rolls over"
	msgGlobalSpent = "the AI budget for today is spent — this feature resumes tomorrow"
)

// WriteRefusal turns a budget refusal into the HTTP response and reports
// whether err was one, so callers keep their own switch for everything else.
//
// Both refusals are 429 rate_limited (docs/API.md §3, §8, §12), with no
// Retry-After: the window rolls continuously, so there is no fixed reset
// instant to name (docs/API.md §3.1).
//
// It writes nothing and returns false for a nil error or any non-budget error.
func WriteRefusal(w http.ResponseWriter, err error) bool {
	var spent *KindSpentError
	switch {
	case errors.As(err, &spent):
		web.Error(w, http.StatusTooManyRequests, web.CodeRateLimited, perKindMessage(spent))
	case errors.Is(err, ErrPerUserSpent):
		// The sentinel without its kind. Check never returns this bare, but a
		// caller that wrapped or re-created it still deserves the right status.
		web.Error(w, http.StatusTooManyRequests, web.CodeRateLimited, msgAnySpent)
	case errors.Is(err, ErrGlobalSpent):
		web.Error(w, http.StatusTooManyRequests, web.CodeRateLimited, msgGlobalSpent)
	default:
		return false
	}
	return true
}

// perKindMessage fills the one template. The noun comes from the policy when
// the composition root set one, and from Kind.Noun otherwise — the same
// table either way.
func perKindMessage(e *KindSpentError) string {
	noun := e.Noun
	if noun == "" {
		noun = e.Kind.Noun()
	}
	return fmt.Sprintf(msgKindSpentFmt, e.Cap, noun)
}
