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
// That is operator information. The per-kind messages are free to name a cap
// where it is the player's OWN and small enough that they could have counted it
// themselves, which is why guess names its number and the others do not.
const (
	msgDuelSpent     = "you have used all of your duels for today — new ones unlock as the day rolls over"
	msgPracticeSpent = "you have used all of your scored drawings for today — new ones unlock as the day rolls over"
	msgGuessSpentFmt = "you have used all %d of your AI guesses for today — new ones unlock as the day rolls over"
	msgAssistSpent   = "you have used all of your AI drawing requests for today — new ones unlock as the day rolls over"
	// msgAnySpent covers a kind with no copy of its own. A future feature that ships
	// its Kind before its sentence should still refuse a player in plain language
	// rather than fall through to a 500 — the ceiling worked, only the wording is
	// missing.
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
		web.Error(w, http.StatusTooManyRequests, web.CodeRateLimited, perKindMessage(spent.Kind, spent.Cap))
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

// perKindMessage picks the sentence for the feature that ran out. Go cannot make
// this switch exhaustive, so the default is written to be true of any AI feature
// rather than to be unreachable.
func perKindMessage(k Kind, perUserCap int) string {
	switch k {
	case KindDuel:
		return msgDuelSpent
	case KindPractice:
		return msgPracticeSpent
	case KindGuess:
		return fmt.Sprintf(msgGuessSpentFmt, perUserCap)
	case KindAssist:
		return msgAssistSpent
	default:
		return msgAnySpent
	}
}
