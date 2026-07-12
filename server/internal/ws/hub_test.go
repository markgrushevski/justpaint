package ws

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeSource is an in-memory stateSource: it returns per-viewer bytes keyed by viewerID,
// so a room-level test can prove the hub sends each user THEIR OWN redacted frame without
// a database (the DB-backed proof of the redaction itself lives in internal/game).
type fakeSource struct {
	matchState map[string]json.RawMessage
	result     map[string]json.RawMessage
	err        error
}

func (f *fakeSource) MatchStateJSON(_ context.Context, viewerID, _ string) (json.RawMessage, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.matchState[viewerID], nil
}

func (f *fakeSource) ResultJSON(_ context.Context, viewerID, _ string) (json.RawMessage, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.result[viewerID], nil
}

// newTestClient builds a client with no socket (conn nil) and a no-op cancel — enough to
// exercise register/unregister/broadcast/fan-out, none of which touch the conn. The pumps
// are never started.
func newTestClient(id string) *client {
	return newClient(id, nil, func() {}, testLogger())
}

// drainFrames non-blocking-reads every queued frame from a client's send buffer.
func drainFrames(c *client) [][]byte {
	var out [][]byte
	for {
		select {
		case f := <-c.send:
			out = append(out, f)
		default:
			return out
		}
	}
}

// hasFrame reports whether any frame decodes to the given type (and, if userID != "",
// that userId).
func hasFrame(frames [][]byte, typ, userID string) bool {
	for _, f := range frames {
		var m struct {
			Type   string `json:"type"`
			UserID string `json:"userId"`
		}
		if json.Unmarshal(f, &m) != nil {
			continue
		}
		if m.Type == typ && (userID == "" || m.UserID == userID) {
			return true
		}
	}
	return false
}

// TestPresence asserts that registering a user broadcasts opponent_connected to the OTHER
// members, and unregistering the last of a user broadcasts opponent_disconnected.
func TestPresence(t *testing.T) {
	h := newHub(&fakeSource{}, testLogger())
	const match = "m1"

	bob := newTestClient("bob")
	h.handleRegister(registration{matchID: match, client: bob})
	// Bob is first in the room; his own connect is broadcast to himself.
	_ = drainFrames(bob) // clear so the next assertion is unambiguous

	alice := newTestClient("alice")
	h.handleRegister(registration{matchID: match, client: alice})

	if got := drainFrames(bob); !hasFrame(got, frameOpponentConnected, "alice") {
		t.Fatalf("bob did not receive opponent_connected{alice}; frames=%s", got)
	}

	// A second alice tab must NOT re-fire presence (alice already present).
	alice2 := newTestClient("alice")
	h.handleRegister(registration{matchID: match, client: alice2})
	if got := drainFrames(bob); hasFrame(got, frameOpponentConnected, "alice") {
		t.Fatalf("second alice tab wrongly re-fired opponent_connected; frames=%s", got)
	}

	// Disconnecting ONE alice tab (alice still has alice2) must NOT fire disconnect.
	h.handleUnregister(registration{matchID: match, client: alice})
	if got := drainFrames(bob); hasFrame(got, frameOpponentDisconnected, "alice") {
		t.Fatalf("disconnect fired while alice still had a tab; frames=%s", got)
	}

	// Disconnecting the LAST alice tab fires disconnect.
	h.handleUnregister(registration{matchID: match, client: alice2})
	if got := drainFrames(bob); !hasFrame(got, frameOpponentDisconnected, "alice") {
		t.Fatalf("bob did not receive opponent_disconnected{alice}; frames=%s", got)
	}
}

// TestPerUserCap asserts the (N+1)th connection for one user force-closes the OLDEST and
// keeps the set at the cap, with no presence churn.
func TestPerUserCap(t *testing.T) {
	h := newHub(&fakeSource{}, testLogger())
	const match = "m1"

	clients := make([]*client, 0, wsMaxConnsPerUser+1)
	for i := 0; i < wsMaxConnsPerUser; i++ {
		c := newTestClient("alice")
		h.handleRegister(registration{matchID: match, client: c})
		clients = append(clients, c)
	}
	rm := h.rooms[match]
	if got := rm.count("alice"); got != wsMaxConnsPerUser {
		t.Fatalf("count = %d, want %d", got, wsMaxConnsPerUser)
	}

	// The (cap+1)th admits and evicts the oldest (first registered).
	sixth := newTestClient("alice")
	h.handleRegister(registration{matchID: match, client: sixth})

	if got := rm.count("alice"); got != wsMaxConnsPerUser {
		t.Fatalf("after overflow count = %d, want %d", got, wsMaxConnsPerUser)
	}
	if !clients[0].isClosed() {
		t.Fatalf("oldest client was not force-closed on cap overflow")
	}
	if sixth.isClosed() {
		t.Fatalf("newest client should be admitted, not closed")
	}
	// The evicted client's later unregister must be a harmless no-op (already gone).
	h.handleUnregister(registration{matchID: match, client: clients[0]})
	if got := rm.count("alice"); got != wsMaxConnsPerUser {
		t.Fatalf("stale unregister changed the set: count = %d, want %d", got, wsMaxConnsPerUser)
	}
}

// TestFullBufferForceClosesNotBlocks asserts a client whose send buffer is full is
// force-closed by a broadcast rather than blocking the hub.
func TestFullBufferForceClosesNotBlocks(t *testing.T) {
	h := newHub(&fakeSource{}, testLogger())
	const match = "m1"

	c := newTestClient("alice")
	h.handleRegister(registration{matchID: match, client: c})
	// Fill the send buffer to capacity so the next send cannot enqueue.
	for c.trySend([]byte(`{"type":"filler"}`)) {
	}

	done := make(chan struct{})
	go func() {
		h.handlePublish(context.Background(), event{kind: evJudging, matchID: match})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handlePublish blocked on a full-buffer client")
	}
	if !c.isClosed() {
		t.Fatalf("full-buffer client was not force-closed")
	}
}

// TestFanoutPerViewerRedaction asserts the hub sends each user their OWN per-viewer bytes:
// alice's frame carries alice's payload and never bob's, and vice versa (the wire-level
// mirror of GAME.md §4.2, driven with a fake source).
func TestFanoutPerViewerRedaction(t *testing.T) {
	src := &fakeSource{matchState: map[string]json.RawMessage{
		"alice": json.RawMessage(`{"players":[{"userId":"alice","drawingId":"DRAW_ALICE"}]}`),
		"bob":   json.RawMessage(`{"players":[{"userId":"bob","drawingId":"DRAW_BOB"}]}`),
	}}
	h := newHub(src, testLogger())
	const match = "m1"

	alice := newTestClient("alice")
	bob := newTestClient("bob")
	h.handleRegister(registration{matchID: match, client: alice})
	h.handleRegister(registration{matchID: match, client: bob})
	drainFrames(alice) // clear presence frames
	drainFrames(bob)

	rm := h.rooms[match]
	h.fanoutPerViewer(context.Background(), match, rm.snapshotUsers(), src.MatchStateJSON, h.matchStateFrameBytes)

	aFrames := drainFrames(alice)
	if !frameContains(aFrames, frameMatchState, "DRAW_ALICE") || frameContains(aFrames, frameMatchState, "DRAW_BOB") {
		t.Fatalf("alice frame leaked/omitted a drawingId: %s", aFrames)
	}
	bFrames := drainFrames(bob)
	if !frameContains(bFrames, frameMatchState, "DRAW_BOB") || frameContains(bFrames, frameMatchState, "DRAW_ALICE") {
		t.Fatalf("bob frame leaked/omitted a drawingId: %s", bFrames)
	}
}

func frameContains(frames [][]byte, typ, needle string) bool {
	for _, f := range frames {
		var m struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(f, &m) == nil && m.Type == typ && bytes.Contains(f, []byte(needle)) {
			return true
		}
	}
	return false
}

// TestRunReturnsOnCancel asserts Run drains on ctx cancel, closes its done channel, and
// force-closes live clients; and that registerClient refuses once the hub has stopped.
func TestRunReturnsOnCancel(t *testing.T) {
	h := newHub(&fakeSource{}, testLogger())
	ctx, cancel := context.WithCancel(context.Background())

	returned := make(chan struct{})
	go func() { h.Run(ctx); close(returned) }()

	// Register through the live channel now that Run is looping.
	c := newTestClient("bob")
	if !h.registerClient("m1", c) {
		t.Fatal("registerClient returned false while hub is running")
	}

	cancel()
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return on ctx cancel")
	}
	select {
	case <-h.done:
	default:
		t.Fatal("Run did not close done")
	}
	if !c.isClosed() {
		t.Fatal("shutdown did not force-close a live client")
	}
	if h.registerClient("m1", newTestClient("carol")) {
		t.Fatal("registerClient admitted a client after shutdown")
	}
}

// TestEnqueueDropsWhenFull asserts a full publish buffer drops (never blocks) a publish —
// a committed game transition must never be held up by the realtime layer.
func TestEnqueueDropsWhenFull(t *testing.T) {
	h := newHub(&fakeSource{}, testLogger())
	// Fill the publish channel to capacity without a running loop to drain it.
	for i := 0; i < wsPublishBuffer; i++ {
		h.publish <- event{kind: evJudging, matchID: "m"}
	}
	done := make(chan struct{})
	go func() {
		h.MatchChanged("m") // must not block even though the buffer is full
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("enqueue blocked on a full publish buffer")
	}
}
