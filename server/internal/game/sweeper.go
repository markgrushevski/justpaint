package game

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/markgrushevski/justpaint/server/internal/db"
)

// Sweeper tunables (docs/GAME.md §4.1).
const (
	// sweepBatch is the page size per phase per tick. A batch that comes back full
	// means there may be more, so the boot pass keeps draining.
	sweepBatch = 256
	// judgeStaleSecs is how long a judging attempt may run before the watchdog
	// re-fires it — set well above p99 judge latency so it recovers crashes/hangs,
	// not healthy in-flight attempts.
	judgeStaleSecs = 45
	// maxJudgeAttempts caps stuck-judging retries so a wedged judge doesn't spin
	// forever; at the cap, sweepExhaustedJudging closes the match as done/'aborted'
	// (docs/GAME.md §4.1).
	maxJudgeAttempts = 3
	// openTTLSecs reaps open matches nobody joined, so a ghost can't later ambush a
	// fresh joiner (docs/GAME.md §4.1).
	openTTLSecs = 600
)

// RunSweeper drives the background deadline sweeps so resolution never depends
// on a client polling. It first drains each phase's backlog to empty, then ticks
// at interval doing one batch per phase, until ctx is cancelled. Start it once:
// `go svc.RunSweeper(ctx, 3*time.Second)`.
//
// Phase order matters: sweepStuckJudging runs before sweepExhaustedJudging, so a
// row it just re-fired is no longer stale and can't be aborted in the same tick.
//
// Never waits on the judging goroutines it dispatched, so shutdown can't
// deadlock against an in-flight render.
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

// drain runs one phase repeatedly until it handles fewer than sweepBatch rows —
// the backlog is drained or progress stalled. A phase returns its
// handled-without-error count, not the fetched count, so a persistently-failing
// batch still returns short and drain exits rather than hot-looping against an
// unhealthy DB. Honors ctx cancellation between batches. Boot pass only.
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

// sweepExpiredDrawing resolves one batch of expired drawing rounds, each in its
// own tx (lock → resolveExpiry → commit), firing judging after commit only for
// the defensive both-submitted case. A failing row is logged and retried next
// tick, so one bad row can't stall the batch (drain's progress signal, see drain).
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
		// abandoned→abandoned, judging→judging frame (docs/API.md §9.2).
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

// sweepStuckJudging re-fires judging for matches wedged in 'judging' past the
// stale window with retries left. Each is re-locked and re-checked
// status=='judging' before re-stamping, so it can never revert a match that just
// committed to 'done' (double-applying Elo).
//
// The concurrency slot is taken before refireJudging, not after: refiring bumps
// judge_attempts, so a retry spent on a pass that never ran would walk the match
// toward the abort cap for nothing. If no slot is free the pass stops early —
// rows stay untouched, still `judging`, for the next tick.
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

// sweepExhaustedJudging is the terminal fallback for a match whose judging
// retries are used up: it closes the round as `done` with resolution 'aborted' —
// no winner, no Elo, a player-facing reason — instead of wedging in `judging`
// forever (docs/GAME.md §4.1, docs/DECISIONS.md 2026-09-18). Its list query is
// the exact complement of sweepStuckJudging's, so the retry cap hides no row.
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
			// the same frame a judged/forfeit resolution publishes (docs/API.md §9.2).
			s.publisher.Resolved(id)
		}
	}
	return handled
}

// abortJudging writes the terminal `done` + 'aborted' state under the same row
// lock the rest of the lifecycle uses, rechecking both guards inside it: status
// is still 'judging' (a pass that committed a verdict between list and lock must
// win) and judge_attempts is still at the cap (SKIP LOCKED releases when the
// SELECT returns, so a racing re-fire could have reset it). Reports whether it
// actually aborted.
//
// Writes through SetMatchResult only, not writeFinalResult: no judge ran, so
// there is no score or Elo — match_players stays null, like an abandoned round
// (docs/GAME.md §8).
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
// match resolved to 'done' between the list and the lock is left alone rather than
// reverted. Returns whether it actually re-entered judging (⇒ fire judgeMatch).
// This lock+recheck deliberately strengthens a bare SetMatchJudging, which has no
// status guard and could otherwise revert a just-done match and re-apply Elo.
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
	// Re-checks the retry cap under the lock too: the list query's SKIP LOCKED
	// releases when the SELECT returns, so a future multi-instance deployment
	// could have two sweeps refire past the cap (ARCHITECTURE.md §9);
	// single-instance this is belt-and-suspenders.
	if row.JudgeAttempts >= maxJudgeAttempts {
		return false, nil
	}
	// enterJudging, not a bare SetMatchJudging: a re-fire is a second request to the
	// judge, so it bills the provider a second time. The two players are not billed
	// again — they were granted one round when the match started, and a judge that
	// hung is not a round they got twice.
	if err := s.enterJudging(ctx, qtx, matchID); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("game: commit tx: %w", err)
	}
	return true, nil
}

// sweepStaleOpen reaps open matches nobody joined within the TTL to abandoned,
// so a ghost open match can't later pair a fresh player against a creator long
// gone. Each is locked and re-checked status=='open' before abandoning.
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
	// was reaped (docs/API.md §9.2).
	s.publisher.Abandoned(matchID)
	return nil
}
