package game

// judgeLimiter bounds how many judging passes (authoritative render + judge call)
// run at once. It is a plain counting semaphore — a buffered channel, no extra
// dependency — because a judging pass is the single most expensive thing this
// service does: under RENDER_MODE=node each pass spawns TWO OS child processes,
// each rasterizing a 1024² canvas through node-canvas. Unbounded `go judgeMatch(id)`
// is a fork bomb on a small instance the moment a boot drain dispatches a backlog
// (up to sweepBatch matches) or a burst of final submits lands together.
//
// Non-blocking by design: a caller that cannot get a slot is never parked and never
// queues. The work is not lost — the match stays in `judging` and the stuck-judging
// sweep re-claims it (sweeper.go), which is also why nothing here needs a shutdown
// handshake: no goroutine ever waits on a slot, so cancelling the server context can
// never deadlock against an in-flight pass.
type judgeLimiter struct {
	slots chan struct{}
}

// newJudgeLimiter builds a limiter admitting n concurrent passes. A non-positive n
// is clamped to 1 rather than producing a limiter that admits nothing (a zero-cap
// channel would wedge judging entirely) — the caller validates the configured bound.
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

// goHeld runs fn in its own goroutine and releases an ALREADY-ACQUIRED slot when fn
// returns (panic included, via defer). It exists for the caller that must take the
// slot BEFORE committing the work it guards — the stuck-judging sweep acquires first
// so it never burns a judge_attempts retry it cannot actually spend.
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
