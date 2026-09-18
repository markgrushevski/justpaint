package ws

import "sync"

// connLimiter bounds concurrent WS connections: a process-wide maximum and a per-IP
// maximum, so a single host can't open sockets until the process (a memory-constrained
// single instance) runs out of memory (docs/IDEAS.md "A global connection semaphore /
// per-IP cap, beyond the per-user-per-match cap"). The only pre-existing bound
// (hub.go's wsMaxConnsPerUser) is per user PER MATCH — nothing previously stopped one
// authenticated user, or one IP fanning out across many accounts, from exhausting the
// process across ALL matches.
//
// This is a plain mutex-protected counter, deliberately NOT routed through the hub's
// single-goroutine actor loop: admission control is a pure counting decision made
// before a match room (or even a valid match id) is known, with no dependency on room
// or game state, so folding it into the hub would serialize an unrelated concern
// through the same channel for no benefit. A dedicated mutex here cannot introduce a
// second lock-ordering hazard against the hub: neither ever calls into or waits on the
// other, and this mutex is held only for O(1) map/counter arithmetic — never across a
// channel send, I/O, or a call into hub/game code.
type connLimiter struct {
	maxGlobal int // <= 0 means unlimited
	maxPerIP  int // <= 0 means unlimited

	mu    sync.Mutex
	total int
	perIP map[string]int
}

// newConnLimiter builds a limiter with the given process-wide and per-IP caps. Either
// cap <= 0 disables that specific bound (used by callers/tests that only care about
// the other one, or don't exercise capping at all) — config.Load is where a real
// deployment should refuse to load an unlimited value (docs/NOTES.md).
func newConnLimiter(maxGlobal, maxPerIP int) *connLimiter {
	return &connLimiter{
		maxGlobal: maxGlobal,
		maxPerIP:  maxPerIP,
		perIP:     make(map[string]int),
	}
}

// tryAcquire admits one connection from ip if neither cap is currently exceeded. On
// success it returns a release func the caller MUST invoke exactly once when the
// connection ends — release is idempotent (a second call is a safe no-op) so a caller
// that defers it can never double-decrement even if some other path also calls it.
//
// The caller is expected to defer release() immediately upon a successful acquire,
// before doing anything else that could panic or return early — Go runs deferred
// funcs during a panicking goroutine's unwind same as a normal return, so that
// placement is what makes "decremented on every exit path, panic included" hold
// without any special-casing here.
func (l *connLimiter) tryAcquire(ip string) (release func(), ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.maxGlobal > 0 && l.total >= l.maxGlobal {
		return nil, false
	}
	if l.maxPerIP > 0 && l.perIP[ip] >= l.maxPerIP {
		return nil, false
	}

	l.total++
	l.perIP[ip]++

	var once sync.Once
	release = func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			l.total--
			l.perIP[ip]--
			if l.perIP[ip] <= 0 {
				delete(l.perIP, ip) // don't leak a zero-count entry per distinct IP ever seen
			}
		})
	}
	return release, true
}

// snapshot returns the current global total and the count for one IP — test-only
// visibility into the counters (production code only ever needs tryAcquire/release).
func (l *connLimiter) snapshot(ip string) (total, forIP int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.total, l.perIP[ip]
}
