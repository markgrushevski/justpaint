package aibudget

import (
	"context"
	"time"
)

const (
	// retention is how long a spent-call row is kept. Nothing is ever read past
	// the 24h window — a week is so "why did the ceiling refuse me last Tuesday"
	// has an answer, not for the budget itself. A week at the hard ceiling is a
	// few thousand rows; an unbounded ledger eventually is a problem, on a
	// free-tier disk.
	retention = 7 * 24 * time.Hour
	// DefaultSweepInterval is the cadence the composition root should use:
	// hourly. The DELETE is a sequential scan — both indexes on the table are
	// partial and lead on user_id/provider, not created_at — over a table that
	// holds at most a week's rows. Faster only buys more scans; slower costs at
	// most a few hours of rows that were going to be deleted anyway.
	DefaultSweepInterval = time.Hour
)

// RunSweeper deletes ledger rows past the retention horizon until ctx is
// cancelled. Start it once at boot: `go budget.RunSweeper(ctx,
// aibudget.DefaultSweepInterval)`. A non-positive interval is clamped to
// DefaultSweepInterval.
//
// It runs one pass before the ticker, the way game.Service.RunSweeper opens
// with its drain: a free-tier instance sleeps when idle and restarts more
// often than once an hour, so a sweep whose first pass is always an hour away
// would never run at all.
//
// One pass, not a drain-to-empty loop, because the DELETE is unpaged and
// already removes everything past the horizon in one go.
//
// Runs even when every kind is unbudgeted: rows may survive a config change
// that turned a real provider back into a fake one, and should still age out.
func (b *Budget) RunSweeper(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultSweepInterval
	}
	b.sweep(ctx)

	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			b.sweep(ctx)
		}
	}
}

// sweep runs one retention pass. A failure is logged and dropped: the next
// tick retries, and there is no caller to return an error to.
func (b *Budget) sweep(ctx context.Context) {
	// The app clock, not the counting window's Postgres now() — skew is
	// irrelevant at a week's distance, though it would be a bug at 24 hours.
	cutoff := time.Now().Add(-retention)

	removed, err := b.q.DeleteAICallsBefore(ctx, cutoff)
	if err != nil {
		b.logger.Error("ai-call retention sweep", "err", err)
		return
	}
	if removed == 0 {
		// Steady state on a quiet service: nothing to remove, every hour, forever.
		// Debug, not Info — an hourly line saying nothing happened is how logs
		// stop being read.
		b.logger.Debug("ai-call retention sweep: nothing past the horizon", "cutoff", cutoff)
		return
	}
	b.logger.Info("ai-call retention sweep", "removed", removed, "olderThan", retention)
}
