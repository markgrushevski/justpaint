package game

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestDrain covers the boot-pass drain loop with an injected phase closure. drain
// never dereferences the Service, so a zero-value &Service{} with nil deps is enough
// (the closure never touches them). It pins the fix that a phase returns its
// handled-WITHOUT-error count: drain keeps passing while a batch comes back full,
// stops once a batch is short (drained) OR handles zero (a persistently-failing
// batch must not hot-loop), and honors ctx cancellation.
func TestDrain(t *testing.T) {
	svc := &Service{} // drain touches no fields

	t.Run("keeps draining on a full batch, stops on the first short batch", func(t *testing.T) {
		calls := 0
		phase := func(context.Context) int {
			calls++
			if calls <= 3 {
				return sweepBatch // full → there may be more, keep going
			}
			return sweepBatch - 1 // short → backlog drained
		}
		svc.drain(context.Background(), phase)
		if calls != 4 {
			t.Errorf("phase called %d times, want 4 (3 full batches + 1 short)", calls)
		}
	})

	t.Run("stops after one pass that handles zero (persistent failure, no hot loop)", func(t *testing.T) {
		calls := 0
		phase := func(context.Context) int {
			calls++
			return 0 // 0 < sweepBatch: a wholly-failing batch must break the drain
		}
		svc.drain(context.Background(), phase)
		if calls != 1 {
			t.Errorf("phase called %d times, want 1 (zero-handled must not spin)", calls)
		}
	})

	t.Run("returns promptly when ctx is already cancelled, without calling the phase", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var calls atomic.Int32
		phase := func(context.Context) int {
			calls.Add(1)
			return sweepBatch // a full batch would loop forever if ctx were ignored
		}

		done := make(chan struct{})
		go func() {
			svc.drain(ctx, phase)
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("drain did not return with a cancelled ctx — it ignored cancellation")
		}
		if n := calls.Load(); n != 0 {
			t.Errorf("phase called %d times, want 0 (ctx cancelled before the first pass)", n)
		}
	})
}
