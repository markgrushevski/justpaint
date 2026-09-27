package game

// judgeLimiter bounds how many judging passes (render + judge call) run at once:
// a plain counting semaphore, because under RENDER_MODE=node each pass spawns
// two OS child processes and an unbounded `go judgeMatch(id)` is a fork bomb
// under a boot drain or a burst of final submits.
//
// Non-blocking by design: a caller that can't get a slot is never parked or
// queued — the match stays `judging` for the stuck-judging sweep to re-claim
// (sweeper.go), so shutdown can never deadlock against an in-flight pass.
type judgeLimiter struct {
	slots chan struct{}
}

// newJudgeLimiter builds a limiter admitting n concurrent passes; a non-positive
// n clamps to 1 (a zero-cap channel would wedge judging entirely).
func newJudgeLimiter(n int) *judgeLimiter {
	if n < 1 {
		n = 1
	}
	return &judgeLimiter{slots: make(chan struct{}, n)}
}

// limit is the configured bound, for logging.
func (l *judgeLimiter) limit() int { return cap(l.slots) }

// tryAcquire takes a slot without blocking; false means every slot is busy. The
// caller MUST release exactly one slot per successful acquire.
func (l *judgeLimiter) tryAcquire() bool {
	select {
	case l.slots <- struct{}{}:
		return true
	default:
		return false
	}
}

// release returns a slot taken by tryAcquire.
func (l *judgeLimiter) release() { <-l.slots }

// goHeld runs fn in its own goroutine, releasing an already-acquired slot when it
// returns — for a caller (the stuck-judging sweep) that must acquire before
// committing the work it guards.
func (l *judgeLimiter) goHeld(fn func()) {
	go func() {
		defer l.release()
		fn()
	}()
}

// tryGo runs fn in its own goroutine if a slot is free, releasing it when fn
// returns; it reports whether the pass started. False means saturated — the caller
// leaves the match for the sweep instead of queueing.
func (l *judgeLimiter) tryGo(fn func()) bool {
	if !l.tryAcquire() {
		return false
	}
	l.goHeld(fn)
	return true
}
