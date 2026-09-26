// Package ratelimit provides an in-memory, per-key token-bucket rate limiter.
//
// It generalizes the token-bucket math already used for the per-user
// AI-assist limiter (internal/assist/ratelimit.go), which is keyed by user id
// and never evicts (docs/IDEAS.md). A limiter facing the public internet is
// keyed by client IP instead — an attacker-influenced keyspace — so this
// package also sweeps idle buckets and caps the map size.
package ratelimit

import (
	"context"
	"sync"
	"time"
)

// Defaults for the eviction knobs, used by New when idleTTL/maxBuckets are
// zero. Sized for an IP-keyed limiter: a bucket untouched for 10 minutes has
// either fully refilled or been forgotten either way, and 100,000 tracked
// keys is generously above realistic single-instance IP cardinality.
const (
	DefaultIdleTTL    = 10 * time.Minute
	DefaultMaxBuckets = 100_000
)

// bucket is one key's token-bucket state.
type bucket struct {
	tokens float64
	last   time.Time // last time this bucket was touched — refill reference AND idle-eviction clock
}

// Limiter is a per-key token-bucket rate limiter, safe for concurrent use.
// Every key shares the same burst/interval shape — construct one Limiter per
// policy tier (e.g. a strict tier for auth, a generous default for everything
// else) rather than parameterizing the shape per call.
type Limiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	burst    float64
	interval time.Duration
	now      func() time.Time // injectable for deterministic tests

	idleTTL    time.Duration // a bucket untouched this long is evictable
	maxBuckets int           // hard ceiling on map size (defense in depth — see Allow)
}

// New builds a Limiter giving each key burst tokens, refilling one token every
// interval. idleTTL is how long an untouched bucket may survive before a sweep
// (RunSweeper, or an opportunistic sweep in Allow) may evict it; maxBuckets is
// a hard ceiling on the number of tracked keys. A zero/negative idleTTL or
// maxBuckets uses DefaultIdleTTL / DefaultMaxBuckets.
func New(burst int, interval, idleTTL time.Duration, maxBuckets int) *Limiter {
	if idleTTL <= 0 {
		idleTTL = DefaultIdleTTL
	}
	if maxBuckets <= 0 {
		maxBuckets = DefaultMaxBuckets
	}
	return &Limiter{
		buckets:    make(map[string]*bucket),
		burst:      float64(burst),
		interval:   interval,
		now:        time.Now,
		idleTTL:    idleTTL,
		maxBuckets: maxBuckets,
	}
}

// Allow reports whether key may make a request now, consuming a token if so.
// A brand-new key starts with a full bucket; an existing one refills lazily by
// the elapsed time since it was last touched (the same math as
// internal/assist/ratelimit.go's RateLimiter.Allow).
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	b := l.buckets[key]
	if b == nil {
		if len(l.buckets) >= l.maxBuckets {
			l.evictIdleLocked(now)
		}
		if len(l.buckets) >= l.maxBuckets {
			// Still at capacity after sweeping idle entries: every tracked key
			// is recently active — genuine sustained load, not a stale map.
			// Fails open rather than closed: refusing to track a new key is
			// safer than making the limiter itself the outage, since a hard
			// deny here would let an attacker who can generate enough distinct
			// keys block every other caller's first request. This request
			// rides through untracked instead; it just isn't remembered.
			return true
		}
		l.buckets[key] = &bucket{tokens: l.burst, last: now}
		b = l.buckets[key]
	} else if l.interval > 0 {
		if elapsed := now.Sub(b.last); elapsed > 0 {
			b.tokens += float64(elapsed) / float64(l.interval)
			if b.tokens > l.burst {
				b.tokens = l.burst
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

// RetryAfter is the advisory wait a throttled caller should honor before
// retrying — one refill interval. Mirrors internal/assist/ratelimit.go.
func (l *Limiter) RetryAfter() time.Duration {
	return l.interval
}

// Len reports the number of currently tracked keys — for tests and diagnostics.
func (l *Limiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

// evictIdleLocked removes every bucket untouched for longer than idleTTL.
// Callers must hold l.mu.
func (l *Limiter) evictIdleLocked(now time.Time) {
	for k, b := range l.buckets {
		if now.Sub(b.last) > l.idleTTL {
			delete(l.buckets, k)
		}
	}
}

// RunSweeper periodically evicts idle buckets so a long-lived process's
// memory tracks only recently-active keys — Allow's capacity check is a
// backstop, not the primary eviction path. Call as `go
// limiter.RunSweeper(ctx, interval)` once per Limiter instance; it returns
// when ctx is cancelled.
func (l *Limiter) RunSweeper(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.mu.Lock()
			l.evictIdleLocked(l.now())
			l.mu.Unlock()
		}
	}
}
