package assist

import (
	"sync"
	"time"
)

// Rate-limit defaults for /api/assist/ops (docs/ASSIST.md §3.4): each request can
// cost real API money, so a per-user token bucket shipped WITH the feature rather
// than waiting for the general 429 work, which has since landed beside it
// (internal/platform/ratelimit, wired in cmd/server/main.go). The two still bound
// different things: that one is keyed per IP and sized so a page load never
// notices, this one is keyed per USER and sized for a quota somebody else meters.
const (
	// DefaultBurst is how many assist requests one user may make back-to-back.
	DefaultBurst = 5
	// DefaultRefillInterval is how often one token is restored (≈5/min sustained).
	DefaultRefillInterval = 12 * time.Second
)

// RateLimiter is a per-user token-bucket limiter. Safe for concurrent use.
//
// Note: buckets are keyed by user id and never evicted, so the map grows with
// every account that has ever used assist and never shrinks. That was fine for a
// bounded set of demo users; the deployment is public now, so the only bound is
// how many accounts exist. internal/platform/ratelimit is this same token bucket
// WITH a TTL sweep and a map cap — folding assist into it (or adding eviction
// here) is docs/IDEAS.md "Rate-limit buckets are never evicted", still open.
type RateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	burst    float64
	interval time.Duration
	now      func() time.Time // injectable for deterministic tests
}

type bucket struct {
	tokens float64
	last   time.Time
}

// NewRateLimiter builds a limiter giving each user burst tokens, refilling one
// token every interval.
func NewRateLimiter(burst int, interval time.Duration) *RateLimiter {
	return &RateLimiter{
		buckets:  make(map[string]*bucket),
		burst:    float64(burst),
		interval: interval,
		now:      time.Now,
	}
}

// Allow reports whether userID may make a request now, consuming a token if so.
// A brand-new user starts with a full bucket; existing buckets refill lazily by
// the elapsed time since their last request.
func (rl *RateLimiter) Allow(userID string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := rl.now()
	b := rl.buckets[userID]
	if b == nil {
		b = &bucket{tokens: rl.burst, last: now}
		rl.buckets[userID] = b
	} else if rl.interval > 0 {
		if elapsed := now.Sub(b.last); elapsed > 0 {
			b.tokens += float64(elapsed) / float64(rl.interval)
			if b.tokens > rl.burst {
				b.tokens = rl.burst
			}
		}
	}
	b.last = now

	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// RetryAfter is the advisory wait a throttled user should honor before retrying —
// one refill interval. The handler renders it into the Retry-After header.
func (rl *RateLimiter) RetryAfter() time.Duration {
	return rl.interval
}
