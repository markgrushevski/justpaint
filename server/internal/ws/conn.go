package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	// wsSendBuffer is the per-client outbound queue depth. A client that can't drain
	// this many frames before it overflows is force-closed (non-blocking-send-or-kill,
	// docs/DESIGN-PHASE3-LIVE.md §3.1) — every frame is superseded by a later full
	// match_state, so a dropped slow client just falls back to REST + reconnect.
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
}

// client is one live socket in a room: a dumb read/write pair around a wsConn. It owns
// no room state. Two goroutines run it — readPump (drains inbound frames only to detect
// close and service ping→pong) and writePump (the sole socket writer, draining the
// buffered send channel). Fan-out reaches a client only through trySend (a non-blocking
// channel send); a client that can't keep up is forceClose'd, never waited on
// (docs/DESIGN-PHASE3-LIVE.md §3.3).
type client struct {
	id     string // the authenticated userID; duplicate tabs share it
	conn   wsConn
	logger *slog.Logger

	send chan []byte // buffered outbound frames; writePump is the ONLY reader

	// seq is a hub-assigned registration order used to evict the OLDEST connection
	// when a user exceeds the per-match cap. Written and read only inside the hub loop.
	seq uint64

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
func newClient(id string, conn wsConn, cancel context.CancelFunc, logger *slog.Logger) *client {
	return &client{
		id:     id,
		conn:   conn,
		logger: logger,
		send:   make(chan []byte, wsSendBuffer),
		done:   make(chan struct{}),
		cancel: cancel,
	}
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
