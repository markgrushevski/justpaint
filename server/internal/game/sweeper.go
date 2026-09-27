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
	sweepBatch = 256 // rows per phase per tick
	// judgeStaleSecs sits well above p99 judge latency, so only a crashed or hung
	// pass is re-fired.
	judgeStaleSecs   = 45
	maxJudgeAttempts = 3 // then sweepExhaustedJudging aborts the match
	openTTLSecs      = 600
)

// RunSweeper drains each phase's backlog, then runs one batch per phase every
// interval until ctx is cancelled. Start it once.
//
// sweepStuckJudging runs before sweepExhaustedJudging, so a row it just re-fired is
// no longer stale and cannot be aborted in the same tick. It never waits on the
// judging goroutines it dispatched, so shutdown cannot deadlock on a render.
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

// drain runs one phase until it handles fewer than sweepBatch rows. Phases return
// the rows handled without error, not the rows fetched, so a failing batch comes back
// short and drain stops instead of hot-looping against an unhealthy DB.
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

// sweepExpiredDrawing resolves one batch of expired drawing rounds, one transaction
// per row; a failing row is logged and retried next tick.
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
		s.publishOutcome(id, outcome)
	}
	return handled
}

// resolveExpiredMatch locks one match, resolves its expiry and commits.
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

// sweepStuckJudging re-fires judging for matches stale in 'judging' with retries left.
//
// The concurrency slot is taken before refireJudging: a re-fire bumps
// judge_attempts, so one whose pass never ran would walk the match toward the abort
// cap for nothing. With no slot free, the rest waits for the next tick.
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
			// handled stays short, so the boot drain stops here too.
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

// sweepExhaustedJudging closes matches whose judging retries are used up as `done` /
// 'aborted', with no winner and no Elo (docs/GAME.md §4.1). Its list query is the
// exact complement of sweepStuckJudging's, so no row falls between them.
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
			s.publisher.Resolved(id)
		}
	}
	return handled
}

// abortJudging writes `done` / 'aborted' under the row lock, re-checking status and
// the retry cap there, since the list query only yields candidates
// (docs/NOTES.md "The sweeper's SKIP LOCKED lists are only candidates"). It uses
// SetMatchResult, not writeFinalResult: no judge ran, so there is no score or Elo.
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

// refireJudging re-stamps a stuck pass under the row lock and reports whether the
// caller should run judgeMatch. SetMatchJudging has no status guard, so the re-check
// here is what keeps a just-finished match from being reverted and rated twice.
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
	// The cap too: two sweeps on two instances could both pass the list query.
	if row.JudgeAttempts >= maxJudgeAttempts {
		return false, nil
	}
	// enterJudging bills the provider again for this second request; the players were
	// billed once, for the round.
	if err := s.enterJudging(ctx, qtx, matchID); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("game: commit tx: %w", err)
	}
	return true, nil
}

// sweepStaleOpen abandons open matches nobody joined within the TTL, so a fresh
// player is never paired with a creator long gone.
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

// reapOpenMatch abandons one open match, unless someone joined it before the lock.
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
	s.publisher.Abandoned(matchID)
	return nil
}
