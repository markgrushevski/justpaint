package game

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Publisher is the seam internal/game calls to push a just-committed transition
// to the realtime layer, implemented by internal/ws.Hub — defined here so the
// dependency runs one way (ws imports game, never the reverse).
//
// Every method takes only ids: no ws types or DTOs cross this boundary. The hub
// rebuilds per-viewer payloads via MatchStateJSON / ResultJSON, so runtime
// redaction (docs/GAME.md §4.2) lives in one place.
type Publisher interface {
	MatchChanged(matchID string)            // roster/deadline changed → per-viewer match_state
	PlayerSubmitted(matchID, userID string) // a player submitted → opponent_submitted (room-broadcast)
	Judging(matchID string)                 // both submitted → judging (shared)
	Resolved(matchID string)                // done (judged OR forfeit) → per-viewer result
	Abandoned(matchID string)               // → abandoned (shared)
}

// NopPublisher is the default publisher: every method is a no-op, so the
// service runs unchanged with no realtime layer wired.
type NopPublisher struct{}

func (NopPublisher) MatchChanged(string)            {}
func (NopPublisher) PlayerSubmitted(string, string) {}
func (NopPublisher) Judging(string)                 {}
func (NopPublisher) Resolved(string)                {}
func (NopPublisher) Abandoned(string)               {}

// SetPublisher swaps in a real publisher (the ws hub) after construction, so
// NewService's signature stays stable for existing tests. A nil publisher resets
// to the no-op.
func (s *Service) SetPublisher(p Publisher) {
	if p == nil {
		p = NopPublisher{}
	}
	s.publisher = p
}

// publishOutcome is the uniform post-commit tail every resolveExpiry caller runs,
// so a resolution publishes no matter which path (Submit's late-expiry 409 or the
// sweeper) triggered it — notifying the winning opponent the instant the loser's
// late submit forfeits the round (docs/API.md §9.2). outcomeNone is a no-op.
func (s *Service) publishOutcome(matchID string, outcome resolveOutcome) {
	switch outcome {
	case outcomeForfeit:
		s.publisher.Resolved(matchID)
	case outcomeAbandoned:
		s.publisher.Abandoned(matchID)
	case outcomeJudging:
		s.publisher.Judging(matchID)
	}
}

// MatchStateJSON builds the per-viewer match_state payload — the same
// buildMatchDTO redaction GET /matches/{id} applies, marshaled to bytes. Returns
// ErrNotFound for a non-player (reusing Get). Called once per distinct userID in
// a room, so A's frame carries A's drawingId and never B's mid-round
// (docs/GAME.md §4.2).
func (s *Service) MatchStateJSON(ctx context.Context, viewerID, matchID string) (json.RawMessage, error) {
	view, err := s.Get(ctx, viewerID, matchID)
	if err != nil {
		return nil, err // ErrNotFound for a non-player, else already wrapped
	}
	b, err := json.Marshal(buildMatchDTO(view, viewerID, time.Now()))
	if err != nil {
		return nil, fmt.Errorf("game: marshal match state: %w", err)
	}
	return b, nil
}

// ResultJSON builds the per-viewer result payload (the same buildResultDTO the
// REST handler returns), or ErrNotFound for a non-player (reusing Result)
// (docs/API.md §9.2).
func (s *Service) ResultJSON(ctx context.Context, viewerID, matchID string) (json.RawMessage, error) {
	view, err := s.Result(ctx, viewerID, matchID)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(buildResultDTO(view))
	if err != nil {
		return nil, fmt.Errorf("game: marshal result: %w", err)
	}
	return b, nil
}
