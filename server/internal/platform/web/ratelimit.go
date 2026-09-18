package web

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/markgrushevski/justpaint/server/internal/platform/ratelimit"
)

// RatePolicy is one row of a rate-limit policy table (see RateLimit): a
// request matching Match draws a token from Limiter. Give each tier (a
// "strict" auth policy, a "moderate" write policy, a "generous" catch-all
// default, …) its own *ratelimit.Limiter — sharing one Limiter across policies
// would let one tier's traffic drain another tier's budget.
type RatePolicy struct {
	Name    string                   // identifies the tier in logs only
	Match   func(*http.Request) bool // which requests this row governs
	Limiter *ratelimit.Limiter
}

// MethodPrefix builds a RatePolicy.Match: true when the request path starts
// with prefix and, if methods is given, its method is one of them (any method
// matches when methods is empty — e.g. omit it for the "strict" auth tier,
// which governs POST only, or pass none for a catch-all default). Two rows
// can share one Limiter to cover several prefixes under the same budget (e.g.
// a "moderate" write tier over both /api/matches and /api/drawings).
func MethodPrefix(prefix string, methods ...string) func(*http.Request) bool {
	return func(r *http.Request) bool {
		if len(methods) > 0 {
			matched := false
			for _, m := range methods {
				if r.Method == m {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}
		return strings.HasPrefix(r.URL.Path, prefix)
	}
}

// RateLimit builds a rate-limiting middleware keyed by client IP
// (ClientIP(r, trustProxy)) — the abuse-protection pass docs/DECISIONS.md and
// docs/IDEAS.md record as required before any public deploy: credential
// stuffing / bcrypt-cost DoS against auth, and unthrottled writes elsewhere.
//
// Policies are tried IN ORDER; the first matching row is authoritative for
// that request — put narrow/strict rows before broad/generous ones. A
// request matching NO row is never throttled, so a table meaning "everything
// gets at least a generous ceiling" must end with a catch-all row
// (Match: func(*http.Request) bool { return true }). Read-only GETs stay
// cheap by simply not being matched by a strict/moderate row (Allow is O(1)
// regardless), so route them to the generous default (or no policy at all).
//
// A throttled request gets 429 through the standard error envelope
// (CodeRateLimited) with a Retry-After header (seconds — the matched policy's
// refill interval) and a logged warning (policy/client_ip/method/path), so
// abuse is visible without needing to cross-reference a request id.
func RateLimit(trustProxy bool, policies []RatePolicy, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, p := range policies {
				if !p.Match(r) {
					continue
				}
				ip := ClientIP(r, trustProxy)
				if !p.Limiter.Allow(ip) {
					secs := int(p.Limiter.RetryAfter().Seconds())
					w.Header().Set("Retry-After", strconv.Itoa(max(1, secs)))
					logger.Warn("rate limited",
						"policy", p.Name,
						"client_ip", ip,
						"method", r.Method,
						"path", r.URL.Path,
					)
					Error(w, http.StatusTooManyRequests, CodeRateLimited, "too many requests")
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
