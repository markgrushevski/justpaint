package ws

// room is one match's live membership: userID → set of that user's clients (duplicate
// tabs/devices allowed). It is a PLAIN struct with NO goroutine and NO mutex — every
// field is touched only inside the hub's single select loop, which is the whole point
// of the actor model (docs/DESIGN-PHASE3-LIVE.md §3.1, §3.3). The room never blocks:
// fan-out is non-blocking-send, and a client that can't keep up is force-closed.
type room struct {
	matchID string
	conns   map[string]map[*client]struct{}
}

func newRoom(matchID string) *room {
	return &room{matchID: matchID, conns: make(map[string]map[*client]struct{})}
}

// add seats a client under its userID and reports whether this made the user newly
// present in the room (its client set went from empty to non-empty → presence connect).
func (rm *room) add(c *client) (firstForUser bool) {
	set := rm.conns[c.id]
	if set == nil {
		set = make(map[*client]struct{})
		rm.conns[c.id] = set
		firstForUser = true
	}
	set[c] = struct{}{}
	return firstForUser
}

// remove drops a client and reports whether that emptied the user's set (user is now
// absent → presence disconnect). Removing a client not present is a no-op returning
// false — so a force-closed-then-unregistered client can't double-fire a disconnect.
func (rm *room) remove(c *client) (lastForUser bool) {
	set := rm.conns[c.id]
	if set == nil {
		return false
	}
	if _, ok := set[c]; !ok {
		return false
	}
	delete(set, c)
	if len(set) == 0 {
		delete(rm.conns, c.id)
		return true
	}
	return false
}

// count returns how many clients the given user has in this room (for the per-user cap).
func (rm *room) count(userID string) int {
	return len(rm.conns[userID])
}

// oldest returns the user's longest-lived client (smallest seq) — the one evicted when
// the per-user cap is exceeded. nil if the user has none.
func (rm *room) oldest(userID string) *client {
	var oldest *client
	for c := range rm.conns[userID] {
		if oldest == nil || c.seq < oldest.seq {
			oldest = c
		}
	}
	return oldest
}

func (rm *room) empty() bool {
	return len(rm.conns) == 0
}

// broadcast non-blocking-sends one already-marshaled SHARED frame (identical for both
// viewers: opponent_submitted / judging / abandoned / presence / pong) to every client.
// A client whose buffer is full is force-closed, never waited on — one slow socket must
// not stall the room (docs/DESIGN-PHASE3-LIVE.md §3.1). Called only inside the hub loop.
func (rm *room) broadcast(frame []byte) {
	for _, set := range rm.conns {
		for c := range set {
			if !c.trySend(frame) {
				c.forceClose()
			}
		}
	}
}

// userConns is a snapshot of one user's clients, taken in the hub loop so the per-viewer
// fan-out (which does a DB read to build that user's redacted DTO) can run OFF the loop
// without touching the rooms map.
type userConns struct {
	userID  string
	clients []*client
}

// snapshotUsers copies the room's userID→clients into a slice, so per-viewer fan-out can
// build+send outside the hub loop (a slow DB read must not stall every other room). The
// copy is shallow (client pointers), and the only client methods the fan-out calls —
// trySend and forceClose — are themselves concurrency-safe and never touch rooms.
func (rm *room) snapshotUsers() []userConns {
	out := make([]userConns, 0, len(rm.conns))
	for uid, set := range rm.conns {
		cs := make([]*client, 0, len(set))
		for c := range set {
			cs = append(cs, c)
		}
		out = append(out, userConns{userID: uid, clients: cs})
	}
	return out
}
