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
	// The work is one indexed DELETE of rows nobody can reach, so the only thing a
	// faster tick would buy is more queries, and a slower one costs at most a few
	// hours of rows that were going to be deleted anyway.
	DefaultSweepInterval = time.Hour
)

// RunSweeper deletes ledger rows past the retention horizon until ctx is
// cancelled. Start it once at boot: `go budget.RunSweeper(ctx,
// aibudget.DefaultSweepInterval)`. A non-positive interval is clamped to
// DefaultSweepInterval, so a misconfigured zero cannot spin the loop.
//
// It mirrors ratelimit.Limiter.RunSweeper — a bare ticker — rather than
// game.Service.RunSweeper, which opens with a boot drain. The drain is there
// because a missed match deadline leaves two players waiting on a round that will
// never resolve, so the backlog must be cleared before the first tick. Nothing
// waits on a retention sweep: a week-old row deleted an hour late, or a day late
// after a weekend of downtime, costs exactly nothing, and the next tick collects
// it along with everything else.
//
// It runs even when every kind is unbudgeted: rows may survive a config change
// that turned a real provider back into a fake one, and they should still age out.
func (b *Budget) RunSweeper(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultSweepInterval
	}
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
