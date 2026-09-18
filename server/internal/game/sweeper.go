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
	// not spin forever (docs/DESIGN-PHASE3-LIVE.md §2.6, §5 Q5). Hitting the cap is
	// not the end of the line: sweepExhaustedJudging then closes the match out as
	// `done` + resolution 'aborted' (docs/GAME.md §4.1).
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
// Phase order matters: sweepStuckJudging runs BEFORE sweepExhaustedJudging, so a row
// it re-fires (re-stamping judging_started_at to now()) is no longer stale and cannot
// be aborted by the very same tick.
//
// RunSweeper returns as soon as ctx is cancelled; it never waits on the judging
// goroutines it dispatched (they hold a limiter slot and run on their own background
// context), so shutdown cannot deadlock against an in-flight render.
func (s *Service) RunSweeper(ctx context.Context, interval time.Duration) {
	s.drain(ctx, s.sweepExpiredDrawing)
	s.drain(ctx, s.sweepStuckJudging)
	s.drain(ctx, s.sweepExhaustedJudging)
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
			s.sweepExhaustedJudging(ctx)
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
			s.dispatchJudging(id)
		}
		// Uniform post-commit tail (same as the Submit late-path): forfeit→result,
		// abandoned→abandoned, judging→judging frame (docs/DESIGN-PHASE3-LIVE.md §2.4, §3.2).
		s.publishOutcome(id, outcome)
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
//
// The concurrency slot is taken BEFORE refireJudging, not after: refiring bumps
// judge_attempts, and a retry spent on a pass that never ran would walk the match
// toward the abort cap for no reason. If no slot is free the whole pass stops early
// — the rows are untouched, still `judging`, and the next tick lists them again.
func (s *Service) sweepStuckJudging(ctx context.Context) int {
	ids, err := s.q.ListStuckJudgingMatches(ctx, db.ListStuckJudgingMatchesParams{
		StaleSecs: judgeStaleSecs, MaxAttempts: maxJudgeAttempts, Lim: sweepBatch,
	})
	if err != nil {
		s.logger.Error("sweep stuck judging: list", "err", err)
		return 0
	}
	handled := 0
	for i, id := range ids {
		if !s.judging.tryAcquire() {
			// Saturated: leave the rest of the batch for a later tick. handled stays
			// short, so the boot drain stops here instead of spinning on rows it
			// cannot judge yet.
			s.logger.Warn("sweep stuck judging: concurrency limit reached, deferring the rest of the batch",
				"limit", s.judging.limit(), "deferred", len(ids)-i)
			break
		}
		refired, err := s.refireJudging(ctx, id)
		if err != nil {
			s.judging.release()
			s.logger.Error("sweep stuck judging: refire", "matchID", id, "err", err)
			continue
		}
		if !refired {
			s.judging.release() // resolved between list and lock — nothing to run
			handled++
			continue
		}
		handled++
		s.judging.goHeld(func() { s.judgeMatch(id) })
	}
	return handled
}

// sweepExhaustedJudging is the terminal fallback for a match whose judging retries
// are used up: it closes the round as `done` with resolution 'aborted' — no winner,
// no Elo, a player-facing reason — instead of leaving it wedged in `judging` forever
// with `{ready:false}` and no recourse short of manual DB surgery (docs/GAME.md
// §4.1, docs/DECISIONS.md 2026-09-18). Its list query is the exact complement of
// sweepStuckJudging's, so the retry cap hides no row from the sweeper. Returns the
// count of rows handled without error (drain's progress signal, not len(ids)).
func (s *Service) sweepExhaustedJudging(ctx context.Context) int {
	ids, err := s.q.ListExhaustedJudgingMatches(ctx, db.ListExhaustedJudgingMatchesParams{
		StaleSecs: judgeStaleSecs, MaxAttempts: maxJudgeAttempts, Lim: sweepBatch,
	})
	if err != nil {
		s.logger.Error("sweep exhausted judging: list", "err", err)
		return 0
	}
	handled := 0
	for _, id := range ids {
		aborted, err := s.abortJudging(ctx, id)
		if err != nil {
			s.logger.Error("sweep exhausted judging: abort", "matchID", id, "err", err)
			continue
		}
		handled++
		if aborted {
			s.logger.Error("match aborted: judging exhausted its retries — no verdict, no rating change",
				"matchID", id, "attempts", maxJudgeAttempts)
			// Post-commit: both duelists' sockets get the (unscored) terminal result,
			// the same frame a judged/forfeit resolution publishes
			// (docs/DESIGN-PHASE3-LIVE.md §3.2).
			s.publisher.Resolved(id)
		}
	}
	return handled
}

// abortJudging writes the terminal `done` + 'aborted' state for one match under the
// SAME row lock the rest of the lifecycle uses, rechecking both guards inside it:
// status is still 'judging' (a pass that committed a real verdict between the list
// and the lock must win) and judge_attempts is still at the cap (the list's FOR
// UPDATE SKIP LOCKED lock is released when the SELECT returns, so a racing re-fire
// could have reset the picture). Reports whether it actually aborted the match.
//
// It writes through SetMatchResult — status/winner/reason/resolution only — and
// deliberately NOT through writeFinalResult: no judge ran, so there is no score and
// no Elo to apply. match_players keeps its null score/rating_before/rating_after,
// exactly like an abandoned round (docs/GAME.md §8).
func (s *Service) abortJudging(ctx context.Context, matchID string) (bool, error) {
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
		return false, nil // a verdict landed between list and lock — never overwrite it
	}
	if row.JudgeAttempts < maxJudgeAttempts {
		return false, nil // retries left — sweepStuckJudging owns this row, not us
	}

	reason, resolution := abortedReason, resolutionAborted
	if _, err := qtx.SetMatchResult(ctx, db.SetMatchResultParams{
		ID: matchID, WinnerPlayerID: nil, JudgeReason: &reason, Resolution: &resolution,
	}); err != nil {
		return false, fmt.Errorf("game: abort judging: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("game: commit tx: %w", err)
	}
	return true, nil
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
	// Post-commit: a lone creator watching the "searching…" socket learns the match
	// was reaped (docs/DESIGN-PHASE3-LIVE.md §2.4, §3.2).
	s.publisher.Abandoned(matchID)
	return nil
}
