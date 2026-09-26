package render

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/markgrushevski/justpaint/server/internal/document"
)

// aDoc is the smallest valid document; nothing here renders it, because nothing
// here gets past the semaphore.
func aDoc() document.Document {
	return document.Document{Version: 1, Width: 10, Height: 10}
}

// TestNodeRenderer_ConcurrencyBound pins that Render blocks for a free slot
// rather than forking past it. The binary doesn't exist, so a call that reached
// the fork would fail with an exec error instead of the context's deadline —
// that distinction is the whole assertion, and it needs no node on PATH to run
// in CI.
func TestNodeRenderer_ConcurrencyBound(t *testing.T) {
	r := NewNodeRenderer("definitely-not-a-real-binary-for-tests", "nowhere.mjs", 1)
	if got := cap(r.slots); got != 1 {
		t.Fatalf("slots = %d, want 1", got)
	}

	// Occupy the only slot, as a render in flight would.
	r.slots <- struct{}{}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := r.Render(ctx, aDoc())
	if err == nil {
		t.Fatal("Render returned nil with no slot free — it forked past the bound")
	}
	// It waited, rather than refusing immediately or forking anyway.
	if waited := time.Since(start); waited < 40*time.Millisecond {
		t.Errorf("waited %s before giving up, want the caller's full context (~50ms) — blocking, not refusing", waited)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want the caller's own context error", err)
	}
	// It also says why it waited, so a slow render isn't mistaken for a broken worker.
	if !strings.Contains(err.Error(), "worker slot") {
		t.Errorf("err = %q, want it to name the wait for a slot", err)
	}

	// Freed, the next caller gets through as far as the fork, where the fake
	// binary fails — proving the slot was returned.
	<-r.slots
	if _, err := r.Render(context.Background(), aDoc()); err == nil {
		t.Fatal("Render succeeded against a nonexistent binary")
	} else if strings.Contains(err.Error(), "worker slot") {
		t.Errorf("still waiting on a slot after the in-flight render finished: %v", err)
	}
}

// A non-positive bound is clamped rather than honoured: a renderer that can run
// nothing is not a safer renderer, it is a service with no judge.
func TestNewNodeRenderer_ClampsConcurrency(t *testing.T) {
	for _, in := range []int{0, -1} {
		if got := cap(NewNodeRenderer("node", "x.mjs", in).slots); got != 1 {
			t.Errorf("NewNodeRenderer(concurrency=%d) has %d slots, want 1", in, got)
		}
	}
}
