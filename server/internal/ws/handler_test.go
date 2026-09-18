package ws

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/markgrushevski/justpaint/server/internal/platform/web"
)

// TestConnectRefusesUpgradeAtGlobalCap asserts the WS upgrade is refused with a plain
// (pre-Accept) HTTP response, using the existing rate_limited error code (docs/API.md
// §3), once the process-wide cap is saturated. hub/svc are deliberately nil: the cap
// check runs before either is ever touched, so this test proves that ordering too —
// if it didn't, this would panic on a nil dereference instead of returning 429.
func TestConnectRefusesUpgradeAtGlobalCap(t *testing.T) {
	h := NewHandler(nil, nil, nil, testLogger(), Limits{MaxConns: 1, MaxConnsPerIP: 5})

	release, ok := h.limiter.tryAcquire("9.9.9.9") // saturate the global cap from a different IP
	if !ok {
		t.Fatal("setup acquire should succeed")
	}
	defer release()

	req := httptest.NewRequest(http.MethodGet, "/api/matches/m1/ws", nil)
	req.RemoteAddr = "1.2.3.4:5555"
	rec := httptest.NewRecorder()

	h.Connect(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusTooManyRequests, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), web.CodeRateLimited) {
		t.Fatalf("body = %s, want error code %q", rec.Body.String(), web.CodeRateLimited)
	}
}

// TestConnectRefusesUpgradeAtPerIPCap asserts the SAME refusal for a single IP that
// has hit its own per-IP cap, even though the global cap has room.
func TestConnectRefusesUpgradeAtPerIPCap(t *testing.T) {
	h := NewHandler(nil, nil, nil, testLogger(), Limits{MaxConns: 10, MaxConnsPerIP: 1})

	release, ok := h.limiter.tryAcquire("1.2.3.4")
	if !ok {
		t.Fatal("setup acquire should succeed")
	}
	defer release()

	req := httptest.NewRequest(http.MethodGet, "/api/matches/m1/ws", nil)
	req.RemoteAddr = "1.2.3.4:6666" // same IP, different port — must still count as one IP
	rec := httptest.NewRecorder()

	h.Connect(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusTooManyRequests, rec.Body.String())
	}
}

// TestConnectAllowsDifferentIPUnderPerIPCap proves the per-IP cap keys on IP, not a
// single shared slot: a DIFFERENT ip must be admitted past the cap gate even while
// another IP is saturated. It can't observe a full upgrade (that needs a live
// hub/svc/websocket handshake, out of scope for a unit test), but admission is proven
// by reaching the NEXT gate instead of being refused at this one — the request uses a
// malformed match id, so passing the cap check surfaces as the existing, unrelated 404
// rather than 429.
func TestConnectAllowsDifferentIPUnderPerIPCap(t *testing.T) {
	h := NewHandler(nil, nil, nil, testLogger(), Limits{MaxConns: 10, MaxConnsPerIP: 1})

	release, ok := h.limiter.tryAcquire("1.2.3.4")
	if !ok {
		t.Fatal("setup acquire should succeed")
	}
	defer release()

	req := httptest.NewRequest(http.MethodGet, "/api/matches/not-a-uuid/ws", nil)
	req.SetPathValue("id", "not-a-uuid")
	req.RemoteAddr = "5.6.7.8:7777"
	rec := httptest.NewRecorder()

	h.Connect(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (should clear the cap gate and fail on the bad id instead); body=%s",
			rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// TestConnectReleasesCapSlotOnEarlyReturn asserts the deferred release actually runs
// when Connect exits early (here, the existing bad-match-id 404 path) — the cap slot
// acquired at the top of Connect must not be held past that single request's handling,
// or a burst of well-formed-but-nonexistent-match requests would slowly exhaust the
// per-IP cap for a legitimate user reusing the same address.
func TestConnectReleasesCapSlotOnEarlyReturn(t *testing.T) {
	h := NewHandler(nil, nil, nil, testLogger(), Limits{MaxConns: 1, MaxConnsPerIP: 1})

	req := httptest.NewRequest(http.MethodGet, "/api/matches/not-a-uuid/ws", nil)
	req.SetPathValue("id", "not-a-uuid")
	req.RemoteAddr = "1.2.3.4:5555"
	rec := httptest.NewRecorder()

	h.Connect(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("first request status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	// If the slot from the first request leaked, this one is refused at the cap
	// (429) instead of reaching the same bad-id 404.
	req2 := httptest.NewRequest(http.MethodGet, "/api/matches/not-a-uuid/ws", nil)
	req2.SetPathValue("id", "not-a-uuid")
	req2.RemoteAddr = "1.2.3.4:6666"
	rec2 := httptest.NewRecorder()

	h.Connect(rec2, req2)
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("second request status = %d, want %d (cap slot leaked from the first request); body=%s",
			rec2.Code, http.StatusNotFound, rec2.Body.String())
	}
}
