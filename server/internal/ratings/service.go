// Package ratings is the read-only leaderboard slice of Phase 4 (docs/GAME.md §8,
// docs/API.md §11): one endpoint that returns the top-rated players and their
// win/loss records. It mirrors the small single-route shape of internal/assist —
// a Service over *db.Queries plus an HTTP Handler — rather than living on
// internal/game, because the leaderboard is a pure global read that shares nothing
// with the match lifecycle.
//
// The Service is READ-ONLY: it takes only *db.Queries (never the pool), owns no
// transaction, and performs no write. The whole aggregate + sort happens in one
// query (ListTopRatings); there is no in-memory ranking beyond assigning the
// row-number rank in the handler.
package ratings

import (
	"context"

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
	return s.q.ListTopRatings(ctx, limit)
}
