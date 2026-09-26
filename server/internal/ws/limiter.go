package ws

import "sync"

// connLimiter bounds concurrent WS connections with a process-wide maximum and a per-IP
// maximum, so a single host can't open sockets until the process runs out of memory.
// hub.go's wsMaxConnsPerUser only bounds one user per match — nothing else stops one
// account, or one IP across many accounts, from exhausting the process across all
// matches.
//
// A plain mutex-protected counter, deliberately not routed through the hub's actor
// loop: admission control is a pure counting decision made before a match room (or even
// a valid match id) is known, with no dependency on room or game state. The mutex is
// held only for O(1) arithmetic, never across a channel send or I/O, so it introduces
// no lock-ordering hazard against the hub — neither ever calls into or waits on the
// other.
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
// success it returns a release func the caller must invoke exactly once when the
// connection ends — release is idempotent, so double-calling it (an error path plus a
// deferred cleanup, say) can never double-decrement. Defer it immediately after a
// successful acquire so every exit path, panic included, frees the slot.
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
