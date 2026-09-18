package game

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestJudgeLimiterBound pins the semaphore's contract: it admits exactly `limit`
// concurrent passes, refuses (never blocks, never queues) the one that would exceed
// it, and hands the slot back when a pass returns. Pure — no DB, no pool.
func TestJudgeLimiterBound(t *testing.T) {
	t.Run("admits exactly the bound, then refuses", func(t *testing.T) {
		cases := []struct {
			name  string
			given int
			limit int // the effective bound after clamping
		}{
			{"bound of one", 1, 1},
			{"the default bound", defaultJudgeConcurrency, 2},
			{"a larger bound", 4, 4},
			{"zero clamps to one (never admit-nothing)", 0, 1},
			{"negative clamps to one", -3, 1},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				l := newJudgeLimiter(tc.given)
				if l.limit() != tc.limit {
					t.Fatalf("limit() = %d, want %d", l.limit(), tc.limit)
				}
				for i := range tc.limit {
					if !l.tryAcquire() {
						t.Fatalf("tryAcquire #%d refused below the bound %d", i+1, tc.limit)
					}
				}
				if l.tryAcquire() {
					t.Fatalf("tryAcquire #%d admitted past the bound %d", tc.limit+1, tc.limit)
				}
				// A release frees exactly one slot — no more, no less.
				l.release()
				if !l.tryAcquire() {
					t.Error("tryAcquire refused after a release freed a slot")
				}
				if l.tryAcquire() {
					t.Error("tryAcquire admitted twice off a single release")
				}
			})
		}
	})

	t.Run("tryGo never exceeds the bound and frees the slot when the pass returns", func(t *testing.T) {
		const limit = 2
		l := newJudgeLimiter(limit)

		var inFlight, peak atomic.Int32
		entered := make(chan struct{}, limit)
		release := make(chan struct{})
		var wg sync.WaitGroup

		// Park `limit` passes inside the limiter so every slot is held.
		for range limit {
			wg.Add(1)
			started := l.tryGo(func() {
				defer wg.Done()
				n := inFlight.Add(1)
				for {
					p := peak.Load()
					if n <= p || peak.CompareAndSwap(p, n) {
						break
					}
				}
				entered <- struct{}{} // signal BEFORE parking, so the test can wait for a full house
				<-release
				inFlight.Add(-1)
			})
			if !started {
				wg.Done()
				t.Fatal("tryGo refused a pass below the bound")
			}
		}
		// Wait until every dispatched pass is actually running; otherwise the peak
		// below would just be measuring the scheduler, not the bound.
		for i := range limit {
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				close(release)
				t.Fatalf("only %d of %d dispatched passes started", i, limit)
			}
		}

		// The next dispatch must be refused, not queued: it returns immediately.
		refusedAt := make(chan time.Duration, 1)
		go func() {
			start := time.Now()
			started := l.tryGo(func() { t.Error("a refused pass must not run") })
			if started {
				t.Error("tryGo admitted a pass past the bound")
			}
			refusedAt <- time.Since(start)
		}()
		select {
		case <-refusedAt:
		case <-time.After(2 * time.Second):
			close(release)
			t.Fatal("tryGo blocked at the bound — it must refuse, never park the caller")
		}

		close(release)
		wg.Wait()

		if got := peak.Load(); got != limit {
			t.Errorf("peak concurrency = %d, want %d", got, limit)
		}
		// Every finished pass returned its slot, so the limiter is fully free again.
		for i := range limit {
			if !l.tryAcquire() {
				t.Fatalf("slot %d not released after its pass returned", i+1)
			}
		}
	})

	t.Run("goHeld releases the caller-acquired slot, even if the pass panics", func(t *testing.T) {
		l := newJudgeLimiter(1)
		if !l.tryAcquire() {
			t.Fatal("tryAcquire refused on a free limiter")
		}

		done := make(chan struct{})
		l.goHeld(func() {
			defer func() {
				_ = recover() // the pass swallows its own panic; the slot must still come back
				close(done)
			}()
			panic("render worker exploded")
		})
		<-done

		// The deferred release runs after the function returns; give it a moment.
		deadline := time.Now().Add(2 * time.Second)
		for !l.tryAcquire() {
			if time.Now().After(deadline) {
				t.Fatal("goHeld leaked the slot — a crashed pass must not permanently shrink the bound")
			}
			time.Sleep(time.Millisecond)
		}
	})
}

// TestNewServiceJudgeLimiter pins the constructor wiring: the bound is an explicit
// parameter with a default shorthand, and it is never nil (a nil limiter would make
// every dispatch look saturated and silently stop all judging).
func TestNewServiceJudgeLimiter(t *testing.T) {
	cases := []struct {
		name string
		svc  *Service
		want int
	}{
		{"NewService uses the default bound", NewService(nil, nil, nil, nil, nil), defaultJudgeConcurrency},
		{"explicit bound is honored", NewServiceWithConcurrency(nil, nil, nil, nil, nil, 5), 5},
		{"non-positive bound clamps to one", NewServiceWithConcurrency(nil, nil, nil, nil, nil, 0), 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.svc.judging == nil {
				t.Fatal("judging limiter is nil — dispatch would refuse every pass")
			}
			if got := tc.svc.judging.limit(); got != tc.want {
				t.Errorf("limit() = %d, want %d", got, tc.want)
			}
		})
	}
}
