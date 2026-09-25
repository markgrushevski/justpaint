package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

const (
	// wsSendBuffer is the per-client outbound queue depth. A client that can't drain
	// this many frames before it overflows is force-closed (non-blocking-send-or-kill)
	// — every frame is superseded by a later full match_state, so a dropped slow
	// client just falls back to REST + reconnect.
	wsSendBuffer = 32
	// wsWriteTimeout bounds a single frame write, so one stalled socket can't wedge its
	// write pump forever (the pump is the ONLY writer per coder/websocket).
	wsWriteTimeout = 10 * time.Second
	// wsReadLimit caps an inbound frame. The client sends only tiny {"type":"ping"}
	// control payloads; anything larger is abusive and trips the limit → close.
	wsReadLimit = 512
)

// pongBytes is the static reply to a client ping (no per-message marshal needed).
var pongBytes = []byte(`{"type":"` + framePong + `"}`)

// wsConn is the subset of *websocket.Conn the client uses — narrowed to an interface
// so the pumps can be exercised without a live socket if needed. *websocket.Conn
// satisfies it.
type wsConn interface {
	Read(ctx context.Context) (websocket.MessageType, []byte, error)
	Write(ctx context.Context, typ websocket.MessageType, p []byte) error
	Close(code websocket.StatusCode, reason string) error
	CloseNow() error
	SetReadLimit(n int64)
	// Ping sends a protocol-level ping and blocks until the peer's pong or ctx
	// expiry — the heartbeat pump's probe (heartbeatLoop). Per coder/websocket's
	// own contract it must be called while a Read/Reader loop is live on the same
	// connection (readPump), since a pong is only ever observed by that loop, not
	// by Ping itself (docs/NOTES.md "WS realtime").
	Ping(ctx context.Context) error
}

// client is one live socket in a room: a dumb read/write pair around a wsConn. It owns
// no room state. Two goroutines run it — readPump (drains inbound frames only to detect
// close and service ping→pong) and writePump (the sole socket writer, draining the
// buffered send channel). Fan-out reaches a client only through trySend (a non-blocking
// channel send); a client that can't keep up is forceClose'd, never waited on.
type client struct {
	id     string // the authenticated userID; duplicate tabs share it
	conn   wsConn
	logger *slog.Logger

	send chan []byte // buffered outbound frames; writePump is the ONLY reader

	// seq is a hub-assigned registration order used to evict the OLDEST connection
	// when a user exceeds the per-match cap. Written and read only inside the hub loop.
	seq uint64

	// readIdleTimeout and heartbeatInterval drive heartbeatLoop (pre-launch hardening,
	// docs/IDEAS.md "Realtime (WS hub)"): a connection silent for readIdleTimeout is
	// evicted, and a probe every heartbeatInterval (well under the timeout) keeps a
	// healthy-but-quiet peer — long stretches while someone draws — from being caught
	// by it. Explicit constructor params, never a package-level magic number; either
	// <= 0 disables the loop (used by callers/tests that don't exercise this).
	readIdleTimeout   time.Duration
	heartbeatInterval time.Duration
	// lastActive is the UnixNano of the last proof of life: an inbound frame
	// (readPump) or a successful heartbeat probe (heartbeatLoop). Read/written from
	// both pump goroutines, hence atomic.
	lastActive atomic.Int64

	// forceClose signals teardown exactly once: it closes done (waking writePump) and
	// cancels the pump context (aborting a blocked Read and any in-flight Write). It
	// touches NEITHER the socket's close handshake NOR the rooms map, so it is safe and
	// non-blocking to call from the hub loop (the real conn teardown — which can block
	// up to coder/websocket's 15s waitGoroutines — happens on the pump goroutines and
	// the handler, never on the loop).
	closeOnce sync.Once
	done      chan struct{}
	cancel    context.CancelFunc
}

// newClient wraps an accepted socket. cancel must cancel the context the pumps run on
// (so forceClose can abort a blocked Read/Write). conn may be nil in hub/room unit
// tests that never start the pumps — forceClose and trySend never touch it.
// readIdleTimeout/heartbeatInterval configure heartbeatLoop (see the client doc); pass
// 0 for both from a caller that never starts it.
func newClient(id string, conn wsConn, cancel context.CancelFunc, logger *slog.Logger, readIdleTimeout, heartbeatInterval time.Duration) *client {
	c := &client{
		id:                id,
		conn:              conn,
		logger:            logger,
		send:              make(chan []byte, wsSendBuffer),
		done:              make(chan struct{}),
		cancel:            cancel,
		readIdleTimeout:   readIdleTimeout,
		heartbeatInterval: heartbeatInterval,
	}
	c.touch() // seed so the first heartbeat tick doesn't see a zero-time, since-epoch gap
	return c
}

// touch records fresh proof of life (an inbound frame or a successful heartbeat pong).
func (c *client) touch() {
	c.lastActive.Store(time.Now().UnixNano())
}

// idleSince reports how long it has been since the last proof of life.
func (c *client) idleSince() time.Duration {
	return time.Since(time.Unix(0, c.lastActive.Load()))
}

// trySend enqueues a frame without blocking. It returns false when the buffer is full
// — the caller (hub loop or fan-out goroutine) then forceClose's this client. A nil
// frame (an impossible marshal failure upstream) is dropped as a no-op. Safe to call
// concurrently from the hub loop, a fan-out goroutine, and readPump (pong).
func (c *client) trySend(frame []byte) bool {
	if frame == nil {
		return true
	}
	select {
	case c.send <- frame:
		return true
	default:
		return false
	}
}

// forceClose tears the client down once: wake the write pump and cancel the pump ctx
// (which aborts the blocked Read and closes the underlying conn via the library). It is
// non-blocking and idempotent — the hub loop calls it directly on a full-buffer client.
func (c *client) forceClose() {
	c.closeOnce.Do(func() {
		close(c.done)
		if c.cancel != nil {
			c.cancel()
		}
	})
}

// isClosed reports whether forceClose has fired (used by tests and to short-circuit).
func (c *client) isClosed() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

// readPump drains inbound frames until the socket closes. The client sends no
// application data, so this exists to (a) notice the close/error promptly and (b)
// answer {"type":"ping"} with a pong. Any read error (normal close, timeout, abusive
// oversize) ends the pump; its own recover keeps a panic from taking down the process.
func (c *client) readPump(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			c.logger.Error("ws: readPump panic", "userID", c.id, "err", r)
		}
	}()
	c.conn.SetReadLimit(wsReadLimit)
	for {
		typ, data, err := c.conn.Read(ctx)
		if err != nil {
			return // close/timeout/limit — tear down (the handler forceClose's on return)
		}
		c.touch() // ANY inbound frame is proof of life, not just a recognized ping
		if typ != websocket.MessageText {
			continue
		}
		var msg struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(data, &msg) == nil && msg.Type == clientPing {
			// Route the pong through the write pump (the sole writer). Drop it if the
			// buffer is full — a client too backed up to receive a pong is already being
			// force-closed by fan-out.
			c.trySend(pongBytes)
		}
	}
}

// writePump is the ONLY goroutine that writes to the socket (required by
// coder/websocket). It drains the send buffer, bounding each frame by wsWriteTimeout so
// a stalled peer can't wedge it. It exits on ctx cancel, forceClose, or any write error.
func (c *client) writePump(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			c.logger.Error("ws: writePump panic", "userID", c.id, "err", r)
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		case frame := <-c.send:
			wctx, cancel := context.WithTimeout(ctx, wsWriteTimeout)
			err := c.conn.Write(wctx, websocket.MessageText, frame)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

// heartbeatLoop is the third per-connection pump (docs/IDEAS.md "Realtime (WS hub)"
// pre-launch hardening): on every tick it either evicts a connection that has gone
// read-idle for readIdleTimeout, or — if still within budget — proactively probes the
// peer with a protocol-level ping well before that budget runs out, so a
// healthy-but-quiet client (a duel has long silent stretches while someone draws) is
// never evicted just because the app-level protocol (client ping only, docs/API.md
// §9.3) happened to go quiet.
//
// The probe is a coder/websocket protocol ping/pong, answered by the BROWSER's network
// stack, not by any client JS — so it requires no frontend change and, unlike the
// client's own timer-driven ping (apps/web PlayView.vue WS_PING_MS), is not subject to
// background-tab timer throttling (docs/NOTES.md "WS realtime"). It cannot reset an
// in-flight Read's own bound (coder/websocket ties a Read's context expiry to closing
// the whole connection, not just failing that call — see docs/NOTES.md), which is why
// readPump's Read stays on the connection's lifetime context and eviction is driven
// from here instead, off a plain wall-clock lastActive check.
//
// Exits on ctx cancel, on forceClose, or once it evicts. A non-positive
// readIdleTimeout/heartbeatInterval disables it (callers/tests that don't care).
func (c *client) heartbeatLoop(ctx context.Context) {
	if c.readIdleTimeout <= 0 || c.heartbeatInterval <= 0 {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			c.logger.Error("ws: heartbeatLoop panic", "userID", c.id, "err", r)
		}
	}()
	ticker := time.NewTicker(c.heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		case <-ticker.C:
			if c.idleSince() >= c.readIdleTimeout {
				c.evictIdle()
				return
			}
			c.probe(ctx)
		}
	}
}

// probe sends one protocol-level ping, bounded by heartbeatInterval so a single
// stalled probe can't wedge this loop past its next tick, and refreshes lastActive on
// a timely pong. Best-effort: a failed/timed-out probe just skips the touch and lets a
// later tick's idleSince check decide, rather than evicting on one bad round trip.
//
// Calling Ping concurrently with readPump's in-flight Read is required, not just safe
// — coder/websocket only surfaces the reply pong to whichever goroutine is currently
// reading, so Ping itself never observes it (docs/NOTES.md "WS realtime").
func (c *client) probe(ctx context.Context) {
	pctx, cancel := context.WithTimeout(ctx, c.heartbeatInterval)
	defer cancel()
	if err := c.conn.Ping(pctx); err == nil {
		c.touch()
	}
}

// evictIdle closes the socket with the idle-timeout code and tears the client down.
// Mirrors handler.go's session-expiry path (Close, then forceClose): Close attempts a
// graceful handshake so the peer learns why (docs/API.md §9.1), and forceClose
// guarantees readPump/writePump exit — which in turn lets Connect's deferred limiter
// release run, freeing this connection's cap slot — even though the handshake wait
// will itself block briefly on the read side readPump already occupies.
func (c *client) evictIdle() {
	_ = c.conn.Close(wsStatusIdleTimeout, "idle timeout")
	c.forceClose()
}
