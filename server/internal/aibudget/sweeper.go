package aibudget

import (
	"context"
	"time"
)

const (
	// retention is how long a spent-call row is kept. Nothing is ever READ past the
	// 24h window, so a week is not for the budget — it is so "why did the ceiling
	// refuse me last Tuesday" has an answer. A week at the hard ceiling is a few
	// thousand rows, which is not a storage problem; an unbounded ledger eventually
	// is, on a free tier with a small disk.
	retention = 7 * 24 * time.Hour
	// DefaultSweepInterval is the cadence the composition root should use: hourly.
	// The work is one DELETE of rows nobody can reach — a sequential scan, since
	// both indexes on the table are partial and lead on user_id/provider rather
	// than created_at — over a table that holds a week at the hard ceiling, which
	// is a few thousand rows. A faster tick would only buy more scans; a slower one
	// costs at most a few hours of rows that were going to be deleted anyway.
	DefaultSweepInterval = time.Hour
)

// RunSweeper deletes ledger rows past the retention horizon until ctx is
// cancelled. Start it once at boot: `go budget.RunSweeper(ctx,
// aibudget.DefaultSweepInterval)`. A non-positive interval is clamped to
// DefaultSweepInterval, so a misconfigured zero cannot spin the loop.
//
// It opens with ONE pass before the ticker, the way game.Service.RunSweeper opens
// with its drain. Not because anybody is waiting on it — nobody is, and a
// week-old row deleted an hour late costs exactly nothing — but because of the
// host: a free-tier instance sleeps when idle and restarts on the next request,
// regularly more often than once an hour, and a sweep whose first pass is always
// an hour away would then never run at all. The tick is the steady state; the
// boot pass is what makes the tick reachable.
//
// One pass, not game's drain-to-empty loop: the DELETE is unpaged, so a single
// pass already removes everything past the horizon.
//
// It runs even when every kind is unbudgeted: rows may survive a config change
// that turned a real provider back into a fake one, and they should still age out.
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

// sweep runs one retention pass. A failure is logged and dropped: the next tick
// retries, and there is no caller to return an error to.
func (b *Budget) sweep(ctx context.Context) {
	// The app clock, unlike the counting window, which Postgres measures against
	// its own now(). Skew between the two clocks is irrelevant at a week's distance
	// and would be a correctness bug at 24 hours — hence the different treatment.
	cutoff := time.Now().Add(-retention)

	removed, err := b.q.DeleteAICallsBefore(ctx, cutoff)
	if err != nil {
		b.logger.Error("ai-call retention sweep", "err", err)
		return
	}
	if removed == 0 {
		// The steady state on a quiet service is "nothing to remove", every hour,
		// forever. That belongs at Debug; an Info line per hour saying nothing
		// happened is how logs stop being read.
		b.logger.Debug("ai-call retention sweep: nothing past the horizon", "cutoff", cutoff)
		return
	}
	b.logger.Info("ai-call retention sweep", "removed", removed, "olderThan", retention)
}
