package game

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/markgrushevski/justpaint/server/internal/db"
)

// Sweeper tunables (docs/DESIGN-PHASE3-LIVE.md §2.4, §2.6). Constants for now; a
// per-prompt round length or telemetry-tuned staleness is an additive change.
const (
	// sweepBatch is the page size per phase per tick. A batch that comes back full
	// means there may be more, so the boot pass keeps draining.
	sweepBatch = 256
	// judgeStaleSecs is how long a judging attempt may run before the watchdog
	// re-fires it — set well above p99 judge latency so it recovers crashes/hangs,
	// not healthy in-flight attempts.
	judgeStaleSecs = 45
	// maxJudgeAttempts caps stuck-judging retries so a genuinely wedged judge does
	// not spin forever (docs/DESIGN-PHASE3-LIVE.md §2.6, §5 Q5).
	maxJudgeAttempts = 3
	// openTTLSecs reaps open matches nobody joined, so a ghost can't later ambush a
	// fresh joiner (docs/DESIGN-PHASE3-LIVE.md §2.6, §5 Q9).
	openTTLSecs = 600
)

// RunSweeper drives the background deadline sweeps so resolution never depends on a
// client polling. It first drains each phase's backlog to empty (a boot pass that
// recovers every deadline missed while the process was down — including rows the
// migration backfilled), then ticks at interval doing one batch per phase. It
// returns when ctx is cancelled (server shutdown) (docs/DESIGN-PHASE3-LIVE.md §2.4,
// §2.5). Start it once: `go svc.RunSweeper(ctx, 3*time.Second)`.
func (s *Service) RunSweeper(ctx context.Context, interval time.Duration) {
	s.drain(ctx, s.sweepExpiredDrawing)
	s.drain(ctx, s.sweepStuckJudging)
	s.drain(ctx, s.sweepStaleOpen)

	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.sweepExpiredDrawing(ctx)
			s.sweepStuckJudging(ctx)
			s.sweepStaleOpen(ctx)
		}
	}
}

// drain runs one phase repeatedly until it handles fewer than sweepBatch rows in a
// pass — the backlog is drained OR the batch stopped making full progress. Because a
// phase returns its handled-without-error count (not the fetched count), a
// persistently-failing full batch returns < sweepBatch and drain EXITS rather than
// hot-looping a retry storm against an unhealthy DB (the 3s ticker still retries the
// tail, bounded). Honors ctx cancellation between batches. Boot pass only.
func (s *Service) drain(ctx context.Context, phase func(context.Context) int) {
	for {
		if ctx.Err() != nil {
			return
		}
		if n := phase(ctx); n < sweepBatch {
			return
		}
	}
}

// sweepExpiredDrawing resolves one batch of expired drawing rounds, each in its own
// tx (lock → resolveExpiry → commit), then fires judging after commit only for the
// defensive both-submitted case. A failing row is logged and skipped — it stays
// expired and is retried next tick, so one bad row can't stall the batch. Returns
// the count of rows HANDLED WITHOUT ERROR (drain's progress signal — NOT len(ids)):
// if a full batch all persistently fails, this returns < sweepBatch so the boot drain
// stops instead of hot-looping a retry storm against an unhealthy DB (the ticker still
// retries at its bounded 3s cadence).
func (s *Service) sweepExpiredDrawing(ctx context.Context) int {
	ids, err := s.q.ListExpiredDrawingMatches(ctx, sweepBatch)
	if err != nil {
		s.logger.Error("sweep expired drawing: list", "err", err)
		return 0
	}
	handled := 0
	for _, id := range ids {
		outcome, err := s.resolveExpiredMatch(ctx, id)
		if err != nil {
			s.logger.Error("sweep expired drawing: resolve", "matchID", id, "err", err)
			continue
		}
		handled++
		if outcome == outcomeJudging {
			go s.judgeMatch(id)
		}
	}
	return handled
}

// resolveExpiredMatch locks one match, resolves its expiry, and commits — the
// per-row tx the sweep (and, in spirit, Submit) share. Separated so one bad row
// can't stall the batch.
func (s *Service) resolveExpiredMatch(ctx context.Context, matchID string) (resolveOutcome, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return outcomeNone, fmt.Errorf("game: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	row, err := qtx.GetMatchForUpdate(ctx, matchID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return outcomeNone, nil // vanished between list and lock — nothing to do
		}
		return outcomeNone, fmt.Errorf("game: lock match: %w", err)
	}
	outcome, err := s.resolveExpiry(ctx, qtx, row)
	if err != nil {
		return outcomeNone, err
	}
	if err := tx.Commit(ctx); err != nil {
		return outcomeNone, fmt.Errorf("game: commit tx: %w", err)
	}
	return outcome, nil
}

// sweepStuckJudging re-fires judging for matches wedged in 'judging' past the stale
// window with retries left (a crashed/hung judge attempt). Each is re-locked and
// re-checked status=='judging' before re-stamping, so it can never revert a match
// that just committed to 'done' (which would double-apply Elo). Returns the count of
// rows handled without error (drain's progress signal, not len(ids)).
func (s *Service) sweepStuckJudging(ctx context.Context) int {
	ids, err := s.q.ListStuckJudgingMatches(ctx, db.ListStuckJudgingMatchesParams{
		StaleSecs: judgeStaleSecs, MaxAttempts: maxJudgeAttempts, Lim: sweepBatch,
	})
	if err != nil {
		s.logger.Error("sweep stuck judging: list", "err", err)
		return 0
	}
	handled := 0
	for _, id := range ids {
		refired, err := s.refireJudging(ctx, id)
		if err != nil {
			s.logger.Error("sweep stuck judging: refire", "matchID", id, "err", err)
			continue
		}
		handled++
		if refired {
			go s.judgeMatch(id)
		}
	}
	return handled
}

// refireJudging re-stamps a stuck judging attempt (bumping judge_attempts +
// judging_started_at) under the row lock, rechecking status=='judging' first so a
// match that resolved to 'done' between the list and the lock is left alone rather
// than reverted. Returns whether it actually re-entered judging (⇒ fire judgeMatch).
// The lock+recheck is a deliberate strengthening of the design's bare SetMatchJudging:
// SetMatchJudging has no status guard, so an unlocked call could revert a just-done
// match to judging and re-apply Elo (docs/DESIGN-PHASE3-LIVE.md §2.6).
func (s *Service) refireJudging(ctx context.Context, matchID string) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("game: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	row, err := qtx.GetMatchForUpdate(ctx, matchID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("game: lock match: %w", err)
	}
	if row.Status != statusJudging {
		return false, nil // resolved (or moved on) between list and lock — don't revert
	}
	// Re-check the retry cap under the lock too. The list query filters
	// judge_attempts < maxJudgeAttempts, but its FOR UPDATE SKIP LOCKED lock releases
	// when the SELECT returns — so a (future) multi-instance deployment could have two
	// sweeps list the same row and refire past the cap. Single-instance this is
	// belt-and-suspenders (ARCHITECTURE §9 multi-instance trigger).
	if row.JudgeAttempts >= maxJudgeAttempts {
		return false, nil
	}
	if _, err := qtx.SetMatchJudging(ctx, matchID); err != nil {
		return false, fmt.Errorf("game: re-stamp judging: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("game: commit tx: %w", err)
	}
	return true, nil
}

// sweepStaleOpen reaps open matches nobody joined within the TTL to abandoned, so a
// ghost open match can't later pair a fresh player against a creator long gone. Each
// is locked and re-checked status=='open' before abandoning. Returns the count of
// rows handled without error (drain's progress signal, not len(ids)).
func (s *Service) sweepStaleOpen(ctx context.Context) int {
	ids, err := s.q.ListStaleOpenMatches(ctx, db.ListStaleOpenMatchesParams{
		TtlSecs: openTTLSecs, Lim: sweepBatch,
	})
	if err != nil {
		s.logger.Error("sweep stale open: list", "err", err)
		return 0
	}
	handled := 0
	for _, id := range ids {
		if err := s.reapOpenMatch(ctx, id); err != nil {
			s.logger.Error("sweep stale open: reap", "matchID", id, "err", err)
			continue
		}
		handled++
	}
	return handled
}

// reapOpenMatch locks one open match and abandons it, rechecking status=='open'
// under the lock so a match someone joined between the list and the lock is left
// alone.
func (s *Service) reapOpenMatch(ctx context.Context, matchID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("game: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	row, err := qtx.GetMatchForUpdate(ctx, matchID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("game: lock match: %w", err)
	}
	if row.Status != statusOpen {
		return nil // someone joined between list and lock — leave it
	}
	if _, err := qtx.SetMatchAbandoned(ctx, matchID); err != nil {
		return fmt.Errorf("game: abandon open match: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("game: commit tx: %w", err)
	}
	return nil
}
