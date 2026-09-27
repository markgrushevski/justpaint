// Package ratings is the read-only leaderboard slice of Phase 4 (docs/GAME.md §8,
// docs/API.md §11): one endpoint returning the top-rated players and their
// win/loss records. It mirrors internal/assist's small single-route shape — a
// Service over *db.Queries plus an HTTP Handler — rather than living on
// internal/game, since the leaderboard is a pure global read sharing nothing
// with the match lifecycle.
package ratings

import (
	"context"
	"fmt"

	"github.com/markgrushevski/justpaint/server/internal/db"
)

// Service serves the leaderboard from a read-only query handle.
type Service struct {
	q *db.Queries
}

// NewService builds the leaderboard service over the shared queries. It takes only
// *db.Queries — no pool — because every read is a single non-transactional query.
func NewService(q *db.Queries) *Service {
	return &Service{q: q}
}

// Top returns the top `limit` players by rating (desc), each with games/wins/losses.
// Callers clamp `limit` before calling; the query orders by (rating desc, id asc)
// and hides players with zero finished matches (docs/GAME.md §8).
func (s *Service) Top(ctx context.Context, limit int32) ([]db.ListTopRatingsRow, error) {
	rows, err := s.q.ListTopRatings(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("ratings: list top: %w", err)
	}
	return rows, nil
}
