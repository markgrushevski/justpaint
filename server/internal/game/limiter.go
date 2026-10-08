package game

// judgeLimiter bounds how many judging passes (render + judge call) run at once: a
// counting semaphore sized to what one instance's memory holds (JUDGE_CONCURRENCY).
//
// Non-blocking for callers: one that can't get a slot never waits — its match stays
// queued in `judging` for the sweeper to start (sweeper.go).
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

// drain waits until every pass in flight has returned its slot, and keeps the slots
// so no new pass starts. Shutdown calls it; the caller bounds the wait.
func (l *judgeLimiter) drain() {
	for range cap(l.slots) {
		l.slots <- struct{}{}
	}
}
