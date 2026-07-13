package assist

import (
	"testing"
	"time"
)

// TestRateLimiter_Allow pins the token-bucket behavior: a fresh user gets burst
// tokens, the (burst+1)th request is throttled, and each user has an independent
// bucket.
func TestRateLimiter_Allow(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)

	if !rl.Allow("u1") {
		t.Fatal("request 1 should pass (full bucket)")
	}
	if !rl.Allow("u1") {
		t.Fatal("request 2 should pass (burst=2)")
	}
	if rl.Allow("u1") {
		t.Fatal("request 3 should be throttled")
	}

	// A different user is unaffected.
	if !rl.Allow("u2") {
		t.Fatal("a different user has an independent bucket")
	}
}

// TestRateLimiter_Refill confirms tokens come back over time: with a frozen clock
// we exhaust the bucket, then advance past one refill interval and get one more.
func TestRateLimiter_Refill(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	now := time.Unix(0, 0)
	rl.now = func() time.Time { return now }

	if !rl.Allow("u1") {
		t.Fatal("request 1 should pass")
	}
	if rl.Allow("u1") {
		t.Fatal("request 2 should be throttled (bucket empty)")
	}

	now = now.Add(90 * time.Second) // > one refill interval
	if !rl.Allow("u1") {
		t.Fatal("request after a refill interval should pass again")
	}
}
