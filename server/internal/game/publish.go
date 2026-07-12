package game

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Publisher is the seam internal/game calls to push a just-committed transition to
// the realtime layer. It is defined HERE (game owns the interface it calls) and
// implemented by internal/ws.Hub, so the dependency runs one way — ws imports game,
// never the reverse — and there is no import cycle (docs/DESIGN-PHASE3-LIVE.md §3.2).
//
// Every method takes ONLY ids: no ws types, no DTOs cross this boundary. The hub
// rebuilds any per-viewer payload itself via MatchStateJSON / ResultJSON, so runtime
// per-viewer redaction (GAME.md §4.2) lives in exactly one place — the game service —
// and the hub never touches internal/document or a raw canvas (§3.6).
type Publisher interface {
	MatchChanged(matchID string)            // roster/deadline changed → per-viewer match_state
	PlayerSubmitted(matchID, userID string) // a player submitted → opponent_submitted (room-broadcast)
	Judging(matchID string)                 // both submitted → judging (shared)
	Resolved(matchID string)                // done (judged OR forfeit) → per-viewer result
	Abandoned(matchID string)               // → abandoned (shared)
}

// NopPublisher is the default publisher: every method is a no-op. It is what
// NewService installs, so the game service (and the whole round-deadline suite) runs
// unchanged with no realtime layer wired — the hub is strictly additive
// (docs/DESIGN-PHASE3-LIVE.md §3.2, §4.1).
type NopPublisher struct{}

func (NopPublisher) MatchChanged(string)            {}
func (NopPublisher) PlayerSubmitted(string, string) {}
func (NopPublisher) Judging(string)                 {}
func (NopPublisher) Resolved(string)                {}
func (NopPublisher) Abandoned(string)               {}

// SetPublisher swaps in a real publisher (the ws hub) after construction. This keeps
// NewService's signature stable — the round-deadline tests call NewService(pool,q,nil,
// nil,...) and must keep passing — while main.go wires the hub via gameSvc.SetPublisher(hub).
// A nil publisher resets to the no-op, so a caller can never install a nil that panics
// on the committed path.
func (s *Service) SetPublisher(p Publisher) {
	if p == nil {
		p = NopPublisher{}
	}
	s.publisher = p
}

// publishOutcome is the uniform post-commit tail every resolveExpiry caller runs, so a
// resolution is published no matter which path (the Submit late-expiry 409 or the
// sweeper) triggered it — this is what notifies the WINNING opponent the instant the
// loser's own late submit forfeits the round (docs/DESIGN-PHASE3-LIVE.md §2.4, §3.2).
// It maps the committed outcome to the matching frame; outcomeNone is a no-op.
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

// MatchStateJSON builds the per-viewer match_state payload for one viewer — the SAME
// buildMatchDTO redaction the REST GET /matches/{id} handler applies, marshaled to
// bytes. Returns ErrNotFound when the viewer is not a player (reusing Get), so the hub
// can never emit a frame to a non-member. The hub calls this ONCE PER DISTINCT userID
// in a room, so each recipient gets their own bytes: A's frame carries A's drawingId
// and never B's mid-round (docs/DESIGN-PHASE3-LIVE.md §3.3, §3.6, docs/GAME.md §4.2).
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

// ResultJSON builds the per-viewer result payload for one viewer (the same
// buildResultDTO the REST result handler returns), or ErrNotFound if they are not a
// player (reusing Result). Marshals the {status,ready:false} pending shape or the full
// resultDone verdict, whichever Result yields (docs/DESIGN-PHASE3-LIVE.md §3.5).
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
