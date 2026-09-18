package ws

import (
	"sync"
	"testing"
)

func TestConnLimiterCaps(t *testing.T) {
	tests := []struct {
		name        string
		maxGlobal   int
		maxPerIP    int
		setupIPs    []string // acquired before the probe, to saturate a cap
		probeIP     string
		wantProbeOK bool
	}{
		{
			name: "under both caps admits", maxGlobal: 5, maxPerIP: 5,
			probeIP: "1.1.1.1", wantProbeOK: true,
		},
		{
			name: "zero global cap means unlimited", maxGlobal: 0, maxPerIP: 1,
			probeIP: "1.1.1.1", wantProbeOK: true,
		},
		{
			name: "negative per-ip cap means unlimited", maxGlobal: 5, maxPerIP: -1,
			setupIPs: []string{"1.1.1.1"}, probeIP: "1.1.1.1", wantProbeOK: true,
		},
		{
			name: "global cap refuses even a fresh ip", maxGlobal: 1, maxPerIP: 5,
			setupIPs: []string{"1.1.1.1"}, probeIP: "2.2.2.2", wantProbeOK: false,
		},
		{
			name: "per-ip cap refuses the same ip", maxGlobal: 10, maxPerIP: 1,
			setupIPs: []string{"1.1.1.1"}, probeIP: "1.1.1.1", wantProbeOK: false,
		},
		{
			name: "per-ip cap allows a different ip", maxGlobal: 10, maxPerIP: 1,
			setupIPs: []string{"1.1.1.1"}, probeIP: "2.2.2.2", wantProbeOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newConnLimiter(tt.maxGlobal, tt.maxPerIP)
			for _, ip := range tt.setupIPs {
				if _, ok := l.tryAcquire(ip); !ok {
					t.Fatalf("setup acquire for %s should have succeeded", ip)
				}
			}

			_, ok := l.tryAcquire(tt.probeIP)
			if ok != tt.wantProbeOK {
				t.Fatalf("tryAcquire(%s) ok = %v, want %v", tt.probeIP, ok, tt.wantProbeOK)
			}
		})
	}
}

// TestConnLimiterReleaseFreesSlot asserts a released slot can be re-acquired — the cap
// bounds CONCURRENT connections, not lifetime connections.
func TestConnLimiterReleaseFreesSlot(t *testing.T) {
	l := newConnLimiter(1, 1)

	release, ok := l.tryAcquire("1.1.1.1")
	if !ok {
		t.Fatal("first acquire should succeed")
	}
	if _, ok := l.tryAcquire("1.1.1.1"); ok {
		t.Fatal("second acquire at the cap should be refused")
	}

	release()

	if _, ok := l.tryAcquire("1.1.1.1"); !ok {
		t.Fatal("acquire after release should succeed")
	}
}

// TestConnLimiterReleaseIsIdempotent asserts a release func called more than once
// (e.g. both an error path and a deferred cleanup) never double-decrements — a bug
// here would eventually drive the counters negative and jam the cap open.
func TestConnLimiterReleaseIsIdempotent(t *testing.T) {
	l := newConnLimiter(2, 2)

	release, ok := l.tryAcquire("1.1.1.1")
	if !ok {
		t.Fatal("acquire should succeed")
	}
	release()
	release()
	release()

	total, forIP := l.snapshot("1.1.1.1")
	if total != 0 || forIP != 0 {
		t.Fatalf("repeated release corrupted counters: total=%d forIP=%d, want 0/0", total, forIP)
	}
}

// TestReleaseRunsOnPanic proves the acquire/defer-release pattern Connect uses (defer
// release() immediately after a successful tryAcquire, before anything else) still
// decrements the counters when the code between acquire and return panics — the
// "panic included" exit path the task and docs/IDEAS.md call out, so a bug handling
// one connection can't leak a global/per-IP slot forever and eventually brick the
// server. This mirrors handler.go's Connect exactly: acquire, defer release, then
// arbitrary work that might panic (each pump already recovers its own panics, but the
// guarantee this test checks is Go's defer-runs-during-unwind semantics, which is what
// makes that recovery pattern safe for the limiter specifically).
func TestReleaseRunsOnPanic(t *testing.T) {
	l := newConnLimiter(1, 1)

	func() {
		defer func() { _ = recover() }() // stands in for a pump's own recover()
		release, ok := l.tryAcquire("1.1.1.1")
		if !ok {
			t.Fatal("acquire should succeed")
		}
		defer release() // the exact pattern Connect uses
		panic("simulated handler panic between acquire and normal return")
	}()

	total, forIP := l.snapshot("1.1.1.1")
	if total != 0 || forIP != 0 {
		t.Fatalf("release did not run on panic: total=%d forIP=%d, want 0/0", total, forIP)
	}
	if _, ok := l.tryAcquire("1.1.1.1"); !ok {
		t.Fatal("slot was not freed after a panicking handler")
	}
}

// TestConnLimiterConcurrentAcquireRelease exercises tryAcquire/release from many
// goroutines at once (go test -race is the real assertion here) and checks the
// counters land back at zero — a sanity check that the mutex actually serializes the
// map/counter mutations under concurrent connect/disconnect churn.
func TestConnLimiterConcurrentAcquireRelease(t *testing.T) {
	l := newConnLimiter(4, 4)
	const workers = 20
	const rounds = 50

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(id int) {
			defer wg.Done()
			ip := "10.0.0.1"
			if id%2 == 0 {
				ip = "10.0.0.2"
			}
			for r := 0; r < rounds; r++ {
				if release, ok := l.tryAcquire(ip); ok {
					release()
				}
			}
		}(i)
	}
	wg.Wait()

	total, _ := l.snapshot("10.0.0.1")
	if total != 0 {
		t.Fatalf("counters did not settle back to 0 after concurrent churn: total=%d", total)
	}
}
