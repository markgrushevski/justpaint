// Package ws is the live-realtime layer for the async duel: a single-process,
// in-memory hub that pushes match-room state (presence, "opponent submitted", the
// deadline, judging, the verdict) to both duelists over WebSockets, while Postgres
// stays the sole source of truth and the REST poll loop remains a throttled fallback.
//
// The hub owns ZERO authoritative state and never mediates a mutation — it only fans
// out read-only snapshots of transitions internal/game already committed, reaching it
// through the game.Publisher seam (game defines it, ws implements it: the dependency
// runs ws→game only, so there is no import cycle). Per-viewer visibility (GAME.md §4.2)
// is preserved by rebuilding match_state / result PER RECIPIENT via the game read seam,
// never by a marshal-once broadcast (docs/DESIGN-PHASE3-LIVE.md §3).
package ws

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/markgrushevski/justpaint/server/internal/game"
)

// wsMaxConnsPerUser caps a single user's live sockets PER MATCH (duplicate tabs are
// allowed up to this). Authenticated is not unbounded: without a cap one valid member
// could open thousands of sockets (2 goroutines + buffers each) via a reconnect loop.
// On exceed, the OLDEST is force-closed before the newcomer is admitted
// (docs/DESIGN-PHASE3-LIVE.md §3.3).
const wsMaxConnsPerUser = 5

// wsPublishBuffer sizes the hub's inbound event channel. A committed game transition
// enqueues here via a NON-BLOCKING send; if it is ever full (a wedged hub loop), the
// event is dropped + logged rather than blocking the committed path
// (docs/DESIGN-PHASE3-LIVE.md §3.2).
const wsPublishBuffer = 64

// stateSource is the game read seam the hub needs to rebuild per-viewer frames. Kept an
// interface (not a bare *game.Service) so the hub/room logic is unit-testable with a
// fake. *game.Service satisfies it.
type stateSource interface {
	MatchStateJSON(ctx context.Context, viewerID, matchID string) (json.RawMessage, error)
	ResultJSON(ctx context.Context, viewerID, matchID string) (json.RawMessage, error)
}

// registration is a register/unregister request handed to the hub loop over a channel,
// so the rooms map is mutated only inside the loop (no mutex).
type registration struct {
	matchID string
	client  *client
}

// Hub is the actor: ONE goroutine (Run) owns rooms with no mutex, servicing register /
// unregister / publish over channels. It implements game.Publisher (each method is a
// non-blocking enqueue). Connections are dumb read/write pumps that reach the hub only
// through these channels and their own send buffers.
type Hub struct {
	svc    stateSource
	logger *slog.Logger

	rooms map[string]*room // matchID → room; touched ONLY in the Run loop

	register   chan registration
	unregister chan registration
	publish    chan event
	done       chan struct{} // closed when Run returns, so senders never block post-shutdown

	seq uint64 // monotonic registration order for the per-user cap; loop-local
}

// compile-time proof the hub satisfies the seam game calls.
var _ game.Publisher = (*Hub)(nil)

// NewHub builds a hub over the game service (the per-viewer read seam). Wire it in
// main.go: hub := ws.NewHub(gameSvc, logger); go hub.Run(ctx); gameSvc.SetPublisher(hub).
func NewHub(svc *game.Service, logger *slog.Logger) *Hub {
	return newHub(svc, logger)
}

// newHub is the interface-typed constructor tests use to inject a fake stateSource.
func newHub(svc stateSource, logger *slog.Logger) *Hub {
	return &Hub{
		svc:        svc,
		logger:     logger,
		rooms:      make(map[string]*room),
		register:   make(chan registration),
		unregister: make(chan registration),
		publish:    make(chan event, wsPublishBuffer),
		done:       make(chan struct{}),
	}
}

// Run is the single owning goroutine. It services the three channels until ctx is
// cancelled (server shutdown), then force-closes every live client and returns. main.go
// starts it with `go hub.Run(ctx)` on the shutdown context and does NOT await it — ctx
// cancel drains it (the same accepted pattern as the sweeper / judgeMatch).
func (h *Hub) Run(ctx context.Context) {
	defer close(h.done)
	for {
		select {
		case <-ctx.Done():
			h.shutdown()
			return
		case r := <-h.register:
			h.handleRegister(r)
		case r := <-h.unregister:
			h.handleUnregister(r)
		case ev := <-h.publish:
			h.handlePublish(ctx, ev)
		}
	}
}

// shutdown force-closes every client in every room. It does not drain unregisters —
// the handlers return once their pumps exit; the process is going away.
func (h *Hub) shutdown() {
	for _, rm := range h.rooms {
		for _, set := range rm.conns {
			for c := range set {
				c.forceClose()
			}
		}
	}
	h.rooms = make(map[string]*room)
}

// --- game.Publisher: each method NON-BLOCKING-enqueues onto publish; a full channel
// (a wedged loop) drops + logs, never blocking a committed transition. ---

func (h *Hub) MatchChanged(matchID string) {
	h.enqueue(event{kind: evMatchChanged, matchID: matchID})
}

func (h *Hub) PlayerSubmitted(matchID, userID string) {
	h.enqueue(event{kind: evPlayerSubmitted, matchID: matchID, userID: userID})
}

func (h *Hub) Judging(matchID string) {
	h.enqueue(event{kind: evJudging, matchID: matchID})
}

func (h *Hub) Resolved(matchID string) {
	h.enqueue(event{kind: evResolved, matchID: matchID})
}

func (h *Hub) Abandoned(matchID string) {
	h.enqueue(event{kind: evAbandoned, matchID: matchID})
}

func (h *Hub) enqueue(ev event) {
	select {
	case h.publish <- ev:
	case <-h.done:
		// Hub has stopped; nothing to deliver.
	default:
		h.logger.Warn("ws: publish buffer full, dropping event", "kind", ev.kind, "matchID", ev.matchID)
	}
}

// --- connection lifecycle (called by the handler; block only until the loop accepts,
// or the hub has stopped) ---

// registerClient enrolls a client in its match room. It returns false if the hub has
// already stopped (so the handler tears the socket down instead of leaking it).
func (h *Hub) registerClient(matchID string, c *client) bool {
	select {
	case h.register <- registration{matchID: matchID, client: c}:
		return true
	case <-h.done:
		return false
	}
}

// unregisterClient removes a client from its room. A no-op if the hub has stopped.
func (h *Hub) unregisterClient(matchID string, c *client) {
	select {
	case h.unregister <- registration{matchID: matchID, client: c}:
	case <-h.done:
	}
}

// --- loop handlers (synchronous; called only from Run, so they own the rooms map
// exclusively. Unit tests call them directly for determinism.) ---

func (h *Hub) handleRegister(r registration) {
	rm := h.rooms[r.matchID]
	if rm == nil {
		rm = newRoom(r.matchID)
		h.rooms[r.matchID] = rm
	}

	// Per-user cap: force-close the oldest before admitting the newcomer, so a runaway
	// reconnect loop can't accumulate sockets. Evicting one of >=cap leaves the user
	// still present, so no presence churn fires.
	if rm.count(r.client.id) >= wsMaxConnsPerUser {
		if victim := rm.oldest(r.client.id); victim != nil {
			rm.remove(victim)
			victim.forceClose()
		}
	}

	h.seq++
	r.client.seq = h.seq
	if firstForUser := rm.add(r.client); firstForUser {
		// This user just became present → tell the room's other members. The frame
		// carries {userId}; a client ignores its own.
		rm.broadcast(h.userFrameBytes(frameOpponentConnected, r.client.id))
	}
}

func (h *Hub) handleUnregister(r registration) {
	rm := h.rooms[r.matchID]
	if rm == nil {
		return
	}
	lastForUser := rm.remove(r.client)
	if lastForUser {
		rm.broadcast(h.userFrameBytes(frameOpponentDisconnected, r.client.id))
	}
	if rm.empty() {
		delete(h.rooms, r.matchID)
	}
}

func (h *Hub) handlePublish(ctx context.Context, ev event) {
	rm := h.rooms[ev.matchID]
	if rm == nil {
		return // no live room for this match — REST is still authoritative
	}
	switch ev.kind {
	case evPlayerSubmitted:
		rm.broadcast(h.userFrameBytes(frameOpponentSubmitted, ev.userID))
	case evJudging:
		rm.broadcast(h.typedFrameBytes(frameJudging))
	case evAbandoned:
		rm.broadcast(h.typedFrameBytes(frameAbandoned))
	case evMatchChanged:
		// Per-viewer: snapshot in the loop, build+send OFF the loop (a DB read must not
		// stall every other room).
		snap := rm.snapshotUsers()
		go h.fanoutPerViewer(ctx, ev.matchID, snap, h.svc.MatchStateJSON, h.matchStateFrameBytes)
	case evResolved:
		snap := rm.snapshotUsers()
		go h.fanoutPerViewer(ctx, ev.matchID, snap, h.svc.ResultJSON, h.resultFrameBytes)
	}
}

// fanoutPerViewer builds each recipient's OWN redacted frame — once per distinct userID
// via the game read seam — and non-blocking-sends it to that user's clients. This is
// where GAME.md §4.2 visibility is enforced on the wire: A's match_state carries A's
// drawingId and never B's mid-round (docs/DESIGN-PHASE3-LIVE.md §3.6). Runs off the hub
// loop (its own goroutine); it touches only the client snapshot, never the rooms map. A
// build that returns ErrNotFound (a user no longer a player) is skipped silently; any
// other error is logged and skipped. A full-buffer client is force-closed.
func (h *Hub) fanoutPerViewer(
	ctx context.Context,
	matchID string,
	users []userConns,
	build func(context.Context, string, string) (json.RawMessage, error),
	wrap func(json.RawMessage) []byte,
) {
	for _, uc := range users {
		payload, err := build(ctx, uc.userID, matchID)
		if err != nil {
			if !errors.Is(err, game.ErrNotFound) {
				h.logger.Error("ws: build per-viewer frame", "matchID", matchID, "userID", uc.userID, "err", err)
			}
			continue
		}
		frame := wrap(payload)
		for _, c := range uc.clients {
			if !c.trySend(frame) {
				c.forceClose()
			}
		}
	}
}
