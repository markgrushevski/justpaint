package ws

import (
	"encoding/json"
	"log/slog"
)

// Server→client frame types — the wire protocol the frontend socket wrapper mirrors
// EXACTLY (docs/DESIGN-PHASE3-LIVE.md §3.5). Client→server carries only {"type":"ping"};
// nothing authoritative crosses this channel (submit stays HTTP POST).
const (
	frameMatchState           = "match_state"           // per-viewer: full matchDTO under "match"
	frameOpponentSubmitted    = "opponent_submitted"    // shared: {userId} — clients ignore their own
	frameJudging              = "judging"               // shared
	frameResult               = "result"                // per-viewer: resultDone under "result"
	frameAbandoned            = "abandoned"             // shared
	frameOpponentConnected    = "opponent_connected"    // shared: {userId}
	frameOpponentDisconnected = "opponent_disconnected" // shared: {userId}
	framePong                 = "pong"                  // shared: reply to a client ping
)

// clientPing is the ONLY client→server payload (docs/DESIGN-PHASE3-LIVE.md §3.5).
const clientPing = "ping"

// eventKind tags an internal hub event (what a game.Publisher method enqueued). The
// hub loop turns each into the matching wire frame(s).
type eventKind int

const (
	evMatchChanged    eventKind = iota // → per-viewer match_state
	evPlayerSubmitted                  // → opponent_submitted (shared)
	evJudging                          // → judging (shared)
	evResolved                         // → per-viewer result
	evAbandoned                        // → abandoned (shared)
)

// event is the internal hub message the Publisher methods enqueue. It carries ONLY
// ids — never a ws or DTO type — so the hub rebuilds every per-viewer payload itself
// via the game read seam (docs/DESIGN-PHASE3-LIVE.md §3.2). Small and copyable, so it
// rides the buffered publish channel by value.
type event struct {
	kind    eventKind
	matchID string
	userID  string // set only for evPlayerSubmitted
}

// --- frame envelopes (marshaled to the outbound text frames) ---

type typedFrame struct {
	Type string `json:"type"`
}

type userFrame struct {
	Type   string `json:"type"`
	UserID string `json:"userId"`
}

type matchStateFrame struct {
	Type  string          `json:"type"`
	Match json.RawMessage `json:"match"`
}

type resultFrame struct {
	Type   string          `json:"type"`
	Result json.RawMessage `json:"result"`
}

// The frame builders below marshal a fixed struct (with, for the per-viewer frames, a
// pre-validated json.RawMessage produced by game's own json.Marshal) — so encoding
// cannot realistically fail. On the impossible error we log and return nil, which
// trySend treats as a no-op frame rather than shipping a corrupt one.

func marshalFrame(logger *slog.Logger, v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		logger.Error("ws: marshal frame", "err", err)
		return nil
	}
	return b
}

func (h *Hub) typedFrameBytes(t string) []byte {
	return marshalFrame(h.logger, typedFrame{Type: t})
}

func (h *Hub) userFrameBytes(t, userID string) []byte {
	return marshalFrame(h.logger, userFrame{Type: t, UserID: userID})
}

func (h *Hub) matchStateFrameBytes(payload json.RawMessage) []byte {
	return marshalFrame(h.logger, matchStateFrame{Type: frameMatchState, Match: payload})
}

func (h *Hub) resultFrameBytes(payload json.RawMessage) []byte {
	return marshalFrame(h.logger, resultFrame{Type: frameResult, Result: payload})
}
