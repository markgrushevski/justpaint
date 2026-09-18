package ws

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// fakeConn is a minimal wsConn test double for exercising heartbeatLoop/readPump
// without a real socket. Read blocks until either a frame is pushed on frames or ctx
// is done — by default (frames left nil) it never delivers a frame, standing in for a
// client that sends nothing at the app level. Ping defers to pingFn, set per test.
type fakeConn struct {
	frames chan fakeFrame // optional: readPump delivers these as they arrive
	pingFn func(ctx context.Context) error

	mu          sync.Mutex
	closed      bool
	closeCode   websocket.StatusCode
	closeReason string
	pingCalls   int
}

type fakeFrame struct {
	typ  websocket.MessageType
	data []byte
}

func (f *fakeConn) Read(ctx context.Context) (websocket.MessageType, []byte, error) {
	select {
	case fr, ok := <-f.frames:
		if !ok {
			<-ctx.Done()
			return 0, nil, ctx.Err()
		}
		return fr.typ, fr.data, nil
	case <-ctx.Done():
		return 0, nil, ctx.Err()
	}
}

func (f *fakeConn) Write(ctx context.Context, typ websocket.MessageType, p []byte) error {
	return nil
}

func (f *fakeConn) Close(code websocket.StatusCode, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.closed {
		f.closed = true
		f.closeCode = code
		f.closeReason = reason
	}
	return nil
}

func (f *fakeConn) CloseNow() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *fakeConn) SetReadLimit(n int64) {}

func (f *fakeConn) Ping(ctx context.Context) error {
	f.mu.Lock()
	f.pingCalls++
	f.mu.Unlock()
	return f.pingFn(ctx)
}

func (f *fakeConn) closedWith() (code websocket.StatusCode, reason string, closed bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closeCode, f.closeReason, f.closed
}

func (f *fakeConn) pingCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pingCalls
}

// waitFor polls cond until it's true or the deadline (a safety net against a hung
// test, exactly like the existing hub_test.go time.After(2*time.Second) pattern) —
// never the mechanism driving the timing under test, which is the injected few-
// millisecond readIdleTimeout/heartbeatInterval below.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	if !cond() {
		t.Fatalf("condition not met within %s", timeout)
	}
}

// TestHeartbeatEvictsIdleConnection asserts that a connection whose heartbeat probes
// NEVER succeed (a black-holed peer that answers neither app-level pings nor
// protocol-level ones) is evicted once readIdleTimeout elapses, closed with the
// idle-timeout code, and that readPump unblocks as a result (proving forceClose really
// tore the connection down, not just heartbeatLoop's own view of it).
func TestHeartbeatEvictsIdleConnection(t *testing.T) {
	conn := &fakeConn{pingFn: func(ctx context.Context) error {
		return errors.New("peer unresponsive")
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const readIdleTimeout = 30 * time.Millisecond
	const heartbeatInterval = 10 * time.Millisecond
	c := newClient("alice", conn, cancel, testLogger(), readIdleTimeout, heartbeatInterval)

	readPumpDone := make(chan struct{})
	go func() { c.readPump(ctx); close(readPumpDone) }()
	go c.heartbeatLoop(ctx)

	waitFor(t, 2*time.Second, c.isClosed)

	code, _, closed := conn.closedWith()
	if !closed {
		t.Fatal("conn.Close was never called on idle eviction")
	}
	if code != wsStatusIdleTimeout {
		t.Fatalf("close code = %v, want %v", code, wsStatusIdleTimeout)
	}
	if conn.pingCount() == 0 {
		t.Fatal("heartbeatLoop should have probed at least once before evicting")
	}

	select {
	case <-readPumpDone:
	case <-time.After(2 * time.Second):
		t.Fatal("readPump did not unblock after idle eviction (forceClose should cancel ctx)")
	}
}

// TestHeartbeatKeepsQuietHealthyConnectionAlive asserts that a connection which sends
// no app-level frames at all, but whose protocol-level pings always succeed (a
// healthy peer, e.g. a duelist mid-drawing with a long silent stretch), is NOT evicted
// even after several multiples of readIdleTimeout have elapsed — proving the server
// heartbeat, not just client traffic, keeps a quiet connection alive.
func TestHeartbeatKeepsQuietHealthyConnectionAlive(t *testing.T) {
	conn := &fakeConn{pingFn: func(ctx context.Context) error {
		return nil // every probe succeeds
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const readIdleTimeout = 30 * time.Millisecond
	const heartbeatInterval = 10 * time.Millisecond
	c := newClient("alice", conn, cancel, testLogger(), readIdleTimeout, heartbeatInterval)

	go c.readPump(ctx)
	go c.heartbeatLoop(ctx)

	// Outlast readIdleTimeout several times over (a "long silent stretch") — a
	// successful heartbeat must keep refreshing lastActive so eviction never fires.
	time.Sleep(6 * readIdleTimeout)

	if c.isClosed() {
		t.Fatal("a healthy, ping-answering connection was evicted")
	}
	if _, _, closed := conn.closedWith(); closed {
		t.Fatal("conn.Close was called despite a healthy heartbeat")
	}
	if conn.pingCount() < 3 {
		t.Fatalf("expected multiple heartbeat probes over %s, got %d", 6*readIdleTimeout, conn.pingCount())
	}
}

// TestInboundFrameActivityPreventsIdleEviction asserts that real inbound app-level
// frames (the client's own {"type":"ping"}, or anything else it might send) refresh
// lastActive on their own, even when the heartbeat probe itself never succeeds —
// covering readPump's touch() call, the other half of "refresh on every inbound
// frame" alongside the heartbeat-driven half above.
func TestInboundFrameActivityPreventsIdleEviction(t *testing.T) {
	conn := &fakeConn{
		frames: make(chan fakeFrame),
		pingFn: func(ctx context.Context) error { return errors.New("no answer") },
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const readIdleTimeout = 30 * time.Millisecond
	const heartbeatInterval = 10 * time.Millisecond
	c := newClient("alice", conn, cancel, testLogger(), readIdleTimeout, heartbeatInterval)

	go c.readPump(ctx)
	go c.heartbeatLoop(ctx)

	stop := make(chan struct{})
	defer close(stop)
	go func() {
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				select {
				case conn.frames <- fakeFrame{typ: websocket.MessageText, data: []byte(`{"type":"ping"}`)}:
				case <-stop:
					return
				}
			}
		}
	}()

	time.Sleep(6 * readIdleTimeout)

	if c.isClosed() {
		t.Fatal("a connection with ongoing inbound frames was evicted")
	}
}
