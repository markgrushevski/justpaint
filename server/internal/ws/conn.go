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
	// this many frames before it overflows is force-closed — every frame is superseded
	// by a later full match_state, so a dropped slow client just falls back to REST.
	wsSendBuffer = 32
	// wsWriteTimeout bounds a single frame write, so one stalled socket can't wedge its
	// write pump forever (the pump is the only writer per coder/websocket).
	wsWriteTimeout = 10 * time.Second
	// wsReadLimit caps an inbound frame. The client sends only tiny {"type":"ping"}
	// payloads; anything larger is abusive and trips the limit into a close.
	wsReadLimit = 512
)

// pongBytes is the static reply to a client ping (no per-message marshal needed).
var pongBytes = []byte(`{"type":"` + framePong + `"}`)

// wsConn is the subset of *websocket.Conn the client uses — narrowed to an interface so
// the pumps can be exercised without a live socket. *websocket.Conn satisfies it.
type wsConn interface {
	Read(ctx context.Context) (websocket.MessageType, []byte, error)
	Write(ctx context.Context, typ websocket.MessageType, p []byte) error
	Close(code websocket.StatusCode, reason string) error
	CloseNow() error
	SetReadLimit(n int64)
	// Ping blocks until the peer's pong or ctx expiry. coder/websocket only surfaces a
	// pong to a live Read/Reader loop on the same connection, never to Ping itself, so
	// this must be called while readPump is running (docs/NOTES.md "WS realtime").
	Ping(ctx context.Context) error
}

// client is one live socket in a room: a dumb read/write pair around a wsConn, owning no
// room state. readPump drains inbound frames (close detection, ping→pong) and writePump
// is the sole socket writer. Fan-out reaches a client only through trySend (a
// non-blocking channel send); a client that can't keep up is forceClose'd, never waited on.
type client struct {
	id     string // the authenticated userID; duplicate tabs share it
	conn   wsConn
	logger *slog.Logger

	send chan []byte // buffered outbound frames; writePump is the only reader

	// seq is a hub-assigned registration order used to evict the oldest connection
	// when a user exceeds the per-match cap. Written and read only inside the hub loop.
	seq uint64

	// readIdleTimeout and heartbeatInterval configure heartbeatLoop: a connection silent
	// for readIdleTimeout is evicted; a probe fires every heartbeatInterval. Explicit
	// constructor params, not a package constant, so a caller/test can disable the loop
	// by passing <= 0.
	readIdleTimeout   time.Duration
	heartbeatInterval time.Duration
	// lastActive is the UnixNano of the last proof of life: an inbound frame (readPump)
	// or a successful heartbeat probe (heartbeatLoop). Written from both pumps, hence atomic.
	lastActive atomic.Int64

	// forceClose signals teardown exactly once: closes done (waking writePump) and
	// cancels the pump context (aborting a blocked Read and any in-flight Write). It
	// touches neither the close handshake nor the rooms map, so it's safe to call
	// non-blocking from the hub loop — the real teardown, which can block up to
	// coder/websocket's 15s waitGoroutines, happens on the pump goroutines and the
	// handler instead.
	closeOnce sync.Once
	done      chan struct{}
	cancel    context.CancelFunc
}

// newClient wraps an accepted socket. cancel must cancel the pumps' context (so
// forceClose can abort a blocked Read/Write). conn may be nil in hub/room unit tests
// that never start the pumps. readIdleTimeout/heartbeatInterval configure heartbeatLoop;
// pass 0 for both to disable it.
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
		c.touch() // any inbound frame is proof of life, not just a recognized ping
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

// writePump is the only goroutine that writes to the socket (required by
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

// heartbeatLoop is the third per-connection pump: each tick either evicts a connection
// idle for readIdleTimeout, or probes the peer with a protocol-level ping so a
// healthy-but-quiet client isn't evicted just because the app-level ping (docs/API.md
// §9.3) went quiet. The probe is answered by the browser's network stack, not client JS,
// and eviction runs off a wall-clock check rather than the Read's own context — a
// coder/websocket quirk explained in docs/NOTES.md "WS realtime". Exits on ctx cancel,
// forceClose, or once it evicts. A non-positive readIdleTimeout/heartbeatInterval
// disables it.
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

// probe sends one protocol-level ping, bounded by heartbeatInterval, and refreshes
// lastActive on a timely pong. Best-effort: a failed/timed-out probe just skips the
// touch and lets a later tick's idleSince check decide. Must run concurrently with
// readPump's in-flight Read — coder/websocket only surfaces a pong to whichever
// goroutine is reading (docs/NOTES.md "WS realtime").
func (c *client) probe(ctx context.Context) {
	pctx, cancel := context.WithTimeout(ctx, c.heartbeatInterval)
	defer cancel()
	if err := c.conn.Ping(pctx); err == nil {
		c.touch()
	}
}

// evictIdle closes the socket with the idle-timeout code and tears the client down,
// mirroring handler.go's session-expiry path: Close attempts a graceful handshake
// (docs/API.md §9.1) and forceClose guarantees the pumps exit, which frees Connect's
// deferred limiter slot even though Close itself may block briefly on the read side
// readPump already occupies.
func (c *client) evictIdle() {
	_ = c.conn.Close(wsStatusIdleTimeout, "idle timeout")
	c.forceClose()
}
