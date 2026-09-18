package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestLimiter_Burst pins the token-bucket shape across several burst sizes:
// exactly `burst` requests pass back-to-back, the next is throttled, and a
// different key has its own independent bucket (mirrors
// internal/assist/ratelimit.go's TestRateLimiter_Allow).
func TestLimiter_Burst(t *testing.T) {
	cases := []struct {
		name  string
		burst int
	}{
		{"burst of 1", 1},
		{"burst of 3", 3},
		{"burst of 10", 10},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l := New(c.burst, time.Minute, 0, 0)

			for i := 0; i < c.burst; i++ {
				if !l.Allow("k") {
					t.Fatalf("request %d/%d should pass (within burst)", i+1, c.burst)
				}
			}
			if l.Allow("k") {
				t.Fatalf("request %d should be throttled (burst of %d exhausted)", c.burst+1, c.burst)
			}

			// A different key is unaffected — independent bucket.
			if !l.Allow("other") {
				t.Error("a different key should have its own, unexhausted bucket")
			}
		})
	}
}

// TestLimiter_Refill confirms tokens come back over time: with a frozen clock
// we exhaust a burst-of-1 bucket, then advance by increasing amounts and check
// whether a refill has (or hasn't) restored a token.
func TestLimiter_Refill(t *testing.T) {
	cases := []struct {
		name    string
		elapsed time.Duration
		wantOK  bool
	}{
		{"no time passed: still empty", 0, false},
		{"less than one interval: still empty", 30 * time.Second, false},
		{"exactly one interval: refilled", time.Minute, true},
		{"well past one interval: refilled (capped at burst)", 10 * time.Minute, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l := New(1, time.Minute, 0, 0)
			now := time.Unix(0, 0)
			l.now = func() time.Time { return now }

			if !l.Allow("u1") {
				t.Fatal("first request should pass (full bucket)")
			}
			if l.Allow("u1") {
				t.Fatal("second immediate request should be throttled (bucket empty)")
			}

			now = now.Add(c.elapsed)
			if got := l.Allow("u1"); got != c.wantOK {
				t.Errorf("Allow() after %s = %v, want %v", c.elapsed, got, c.wantOK)
			}
		})
	}
}

// TestLimiter_RetryAfter pins the advisory Retry-After the HTTP middleware
// renders into the header: one refill interval, verbatim.
func TestLimiter_RetryAfter(t *testing.T) {
	l := New(1, 42*time.Second, 0, 0)
	if got := l.RetryAfter(); got != 42*time.Second {
		t.Errorf("RetryAfter() = %s, want 42s", got)
	}
}

// TestLimiter_Eviction covers the capacity guard Allow falls back on when the
// map is full of new keys: an idle bucket is swept to make room, and — the
// documented tradeoff — a key is let through untracked (fail OPEN) rather than
// denied when every existing bucket is still recently active.
func TestLimiter_Eviction(t *testing.T) {
	t.Run("idle buckets are swept to make room for a new key", func(t *testing.T) {
		l := New(1, time.Minute, time.Minute, 2) // maxBuckets=2, idleTTL=1m
		now := time.Unix(0, 0)
		l.now = func() time.Time { return now }

		l.Allow("a")
		l.Allow("b")
		if got := l.Len(); got != 2 {
			t.Fatalf("Len() = %d, want 2 (at capacity)", got)
		}

		// Past idleTTL: a and b are now evictable.
		now = now.Add(2 * time.Minute)
		if !l.Allow("c") {
			t.Fatal("Allow(c) should pass once idle buckets are swept for room")
		}
		if got := l.Len(); got != 1 {
			t.Errorf("Len() = %d, want 1 (a and b evicted, only c tracked)", got)
		}
	})

	t.Run("fails open when still full after a sweep (no idle buckets to evict)", func(t *testing.T) {
		l := New(1, time.Minute, time.Minute, 2) // maxBuckets=2, idleTTL=1m
		now := time.Unix(0, 0)
		l.now = func() time.Time { return now }

		l.Allow("a")
		l.Allow("b")
		if got := l.Len(); got != 2 {
			t.Fatalf("Len() = %d, want 2 (at capacity)", got)
		}

		// No time has passed — a and b are both still fresh, so a sweep finds
		// nothing to evict. A brand-new key must still be ALLOWED (fail open),
		// but the map must not grow past maxBuckets.
		if !l.Allow("c") {
			t.Fatal("Allow(c) should fail OPEN (allow) when the limiter is at capacity with no idle buckets")
		}
		if got := l.Len(); got != 2 {
			t.Errorf("Len() = %d, want 2 (an untracked fail-open key must not grow the map)", got)
		}
	})
}

// TestLimiter_RunSweeper drives the background sweeper end-to-end: buckets
// idle past idleTTL are evicted on the sweeper's own schedule (not merely as a
// side effect of Allow), and the goroutine returns promptly once ctx is
// cancelled (mirrors internal/game's sweeper tests, e.g. TestDrain's
// cancellation case).
func TestLimiter_RunSweeper(t *testing.T) {
	l := New(1, time.Minute, 20*time.Millisecond, 0) // idleTTL=20ms
	var clock atomic.Int64
	start := time.Unix(0, 0)
	clock.Store(start.UnixNano())
	l.now = func() time.Time { return time.Unix(0, clock.Load()) }

	l.Allow("a")
	l.Allow("b")
	if got := l.Len(); got != 2 {
		t.Fatalf("Len() = %d, want 2 before any sweep", got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		l.RunSweeper(ctx, 5*time.Millisecond)
		close(done)
	}()

	// Jump the clock well past idleTTL so the sweeper's next tick evicts both.
	clock.Store(start.Add(time.Second).UnixNano())

	deadline := time.After(2 * time.Second)
waitEvicted:
	for {
		if l.Len() == 0 {
			break waitEvicted
		}
		select {
		case <-deadline:
			t.Fatalf("buckets were not evicted within the deadline (Len()=%d)", l.Len())
		case <-time.After(10 * time.Millisecond):
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunSweeper did not return after ctx was cancelled")
	}
}

// TestLimiter_ConcurrentAccess exercises Allow from many goroutines against a
// handful of shared keys — run with `go test -race` to catch any unguarded
// access. The only correctness assertion (beyond the race detector) is that
// the bucket count stays bounded to the actual key cardinality used.
func TestLimiter_ConcurrentAccess(t *testing.T) {
	l := New(1000, time.Millisecond, 0, 0)
	const goroutines = 50
	const perGoroutine = 200
	const distinctKeys = 5

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", i%distinctKeys)
			for j := 0; j < perGoroutine; j++ {
				l.Allow(key)
			}
		}(i)
	}
	wg.Wait()

	if got := l.Len(); got > distinctKeys {
		t.Errorf("Len() = %d, want at most %d distinct keys", got, distinctKeys)
	}
}
