package game

import (
	"context"
	"fmt"

	"github.com/markgrushevski/justpaint/server/internal/db"
)

// resolveOutcome is what resolveExpiry decided for a locked match row, so the
// caller knows whether to fire judging after committing (docs/GAME.md §4.1).
type resolveOutcome int

const (
	outcomeNone      resolveOutcome = iota // not expired / not drawing → no transition
	outcomeAbandoned                       // 0 submitted → abandoned (no result)
	outcomeForfeit                         // exactly 1 submitted → done (submitter wins)
	outcomeJudging                         // 2 submitted (defensive) → judging
)

// forfeitReason is the human-prose judge_reason stamped on a forfeit. The client
// branches on resolution == 'forfeit', never on this text (docs/API.md §8, result).
const forfeitReason = "opponent did not submit before the deadline"

// isExpiredDrawing reports whether a locked match row is a drawing round past
// its deadline on the DB clock — the guard both resolveExpiry and Submit share,
// so it can't drift. serverNow == deadline counts as expired.
func isExpiredDrawing(row db.GetMatchForUpdateRow) bool {
	return row.Status == statusDrawing &&
		row.DrawingDeadline != nil &&
		!row.ServerNow.Before(*row.DrawingDeadline)
}

// decideExpiry is the pure decision an expired round makes: partition the roster
// by who submitted, map to an outcome plus (for a forfeit) the seat-independent
// finalResult; other outcomes carry a zero value the caller ignores.
func decideExpiry(players []db.GetMatchPlayersForResolveRow) (resolveOutcome, finalResult) {
	var submitted, missing []db.GetMatchPlayersForResolveRow
	for _, p := range players {
		if p.SubmittedAt != nil {
			submitted = append(submitted, p)
		} else {
			missing = append(missing, p)
		}
	}

	switch {
	case len(submitted) == 0:
		return outcomeAbandoned, finalResult{}
	case len(submitted) == 1 && len(missing) == 1:
		return outcomeForfeit, forfeitResult(submitted[0], missing[0])
	default:
		// Both submitted (unreachable in steady state: Submit flips to judging in-tx
		// and the sweep's WHERE excludes non-drawing rows) or a malformed roster
		// that isn't exactly two players. Neither mis-forfeits: flip to judging and
		// let runJudging surface it.
		return outcomeJudging, finalResult{}
	}
}

// forfeitResult builds the seat-independent terminal result for a forfeit: the
// submitter wins by default, Elo from the same computeElo the judged path uses
// (K=32); neither carries a judge score (nil — no judge ran). Pure, so the seat
// mapping is table-testable.
func forfeitResult(submitter, forfeiter db.GetMatchPlayersForResolveRow) finalResult {
	afterWin, afterLose := computeElo(int(submitter.Rating), int(forfeiter.Rating), scoreWin)
	winner := submitter.UserID
	return finalResult{
		winner: &winner,
		players: []playerResult{
			{userID: submitter.UserID, score: nil, before: int(submitter.Rating), after: afterWin},
			{userID: forfeiter.UserID, score: nil, before: int(forfeiter.Rating), after: afterLose},
		},
		reason:     forfeitReason,
		resolution: resolutionForfeit,
	}
}

// resolveExpiry resolves one expired drawing round inside the caller's tx, with
// the match row already locked (ServerNow carries the DB clock). It
// rechecks-then-acts, so it's idempotent: a submit that just committed, or a
// second sweeper, sees a non-drawing status or an unexpired deadline and returns
// outcomeNone. Commits nothing and fires no judging — the caller does both.
func (s *Service) resolveExpiry(ctx context.Context, qtx *db.Queries, row db.GetMatchForUpdateRow) (resolveOutcome, error) {
	if !isExpiredDrawing(row) {
		return outcomeNone, nil
	}

	players, err := qtx.GetMatchPlayersForResolve(ctx, row.ID)
	if err != nil {
		return outcomeNone, fmt.Errorf("game: load players for resolve: %w", err)
	}

	outcome, fr := decideExpiry(players)
	switch outcome {
	case outcomeAbandoned:
		// Nobody drew → terminal, no result, no Elo.
		if _, err := qtx.SetMatchAbandoned(ctx, row.ID); err != nil {
			return outcomeNone, fmt.Errorf("game: abandon match: %w", err)
		}
	case outcomeForfeit:
		// One drew → forfeit: the submitter wins by default; the judge does not run
		// (only one image). Elo written via the shared, seat-safe writer.
		if err := s.writeFinalResult(ctx, qtx, row.ID, fr); err != nil {
			return outcomeNone, err
		}
	case outcomeJudging:
		s.logger.Warn("resolveExpiry: expired drawing round with both submitted — flipping to judging",
			"matchID", row.ID)
		// enterJudging, not a bare SetMatchJudging: this is one of the three sites
		// that cause a judge request and so must bill the provider for it
		// (docs/GAME.md §4.3). The two branches above reach no judge and bill
		// nothing.
		if err := s.enterJudging(ctx, qtx, row.ID); err != nil {
			return outcomeNone, err
		}
	}
	return outcome, nil
}
