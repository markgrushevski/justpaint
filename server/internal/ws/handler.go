package ws

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"github.com/markgrushevski/justpaint/server/internal/auth"
	"github.com/markgrushevski/justpaint/server/internal/game"
	"github.com/markgrushevski/justpaint/server/internal/platform/web"
)

const (
	// wsStatusSessionExpired is the private (4000–4999) close code sent when a socket
	// outlives its session's JWT exp. The client treats it as "re-authenticate", not a
	// transient drop (docs/DESIGN-PHASE3-LIVE.md §3.4).
	wsStatusSessionExpired = websocket.StatusCode(4001)
	// wsStatusIdleTimeout is the private close code sent when heartbeatLoop evicts a
	// connection that has gone read-idle for Limits.ReadIdleTimeout (docs/API.md §9.1).
	// Unlike 4001, this is a transient condition — the client may simply reconnect.
	wsStatusIdleTimeout = websocket.StatusCode(4002)
)

// Limits bundles the pre-launch WS hardening knobs (docs/IDEAS.md "Realtime (WS
// hub)"): the per-connection read-idle timeout + heartbeat cadence (conn.go
// heartbeatLoop) and the process-wide / per-IP connection caps (limiter.go). Grouped
// into one struct, rather than four positional constructor params, so a call site
// names each value instead of relying on positional order between two same-typed
// durations and two same-typed ints. All four are required explicit values — nothing
// here is a magic number buried in code; config.Load owns picking/validating them.
type Limits struct {
	// ReadIdleTimeout is how long a connection may go without ANY proof of life (an
	// inbound frame, or a successful heartbeat pong) before the server evicts it.
	// Must clear the client's own ping cadence (apps/web PlayView.vue WS_PING_MS,
	// currently 25s) with real margin for jitter/background-tab delay.
	ReadIdleTimeout time.Duration
	// HeartbeatInterval is how often the server proactively pings a connection,
	// independent of client behavior. Must be well under ReadIdleTimeout.
	HeartbeatInterval time.Duration
	// MaxConns is the process-wide concurrent WS connection cap. <= 0 means
	// unlimited — config.Load should refuse to load an unlimited value in
	// production; this struct only carries whatever it is given.
	MaxConns int
	// MaxConnsPerIP is the per-remote-IP concurrent WS connection cap (same <= 0 =
	// unlimited caveat as MaxConns).
	MaxConnsPerIP int
	// TrustProxy mirrors the HTTP side: it decides whether the per-IP cap keys on
	// X-Forwarded-For or on the direct peer. It must agree with the rate limiter,
	// or the two disagree about who a client IS — and behind a proxy, keying on the
	// peer collapses MaxConnsPerIP into a second, much lower global cap, because
	// every visitor arrives as the proxy.
	TrustProxy bool
}

// Handler upgrades GET /api/matches/{id}/ws to a WebSocket after the SAME auth +
// membership gates the REST match routes use, then hands the socket to the hub.
type Handler struct {
	hub *Hub
	svc *game.Service
	// originPatterns authorize cross-Host origins on the handshake (the split-host dev
	// proxy). The request Host is always authorized; never "*" (docs/DESIGN-PHASE3-LIVE.md §3.4).
	originPatterns []string
	logger         *slog.Logger
	limits         Limits
	limiter        *connLimiter
}

func NewHandler(hub *Hub, svc *game.Service, originPatterns []string, logger *slog.Logger, limits Limits) *Handler {
	return &Handler{
		hub:            hub,
		svc:            svc,
		originPatterns: originPatterns,
		logger:         logger,
		limits:         limits,
		limiter:        newConnLimiter(limits.MaxConns, limits.MaxConnsPerIP),
	}
}

// Routes mounts the WS route behind protect (the same RequireAuth the match routes use),
// so auth.UserID / auth.SessionExpiry are populated before Connect runs.
func (h *Handler) Routes(mux *http.ServeMux, protect func(http.Handler) http.Handler) {
	mux.Handle("GET /api/matches/{id}/ws", protect(http.HandlerFunc(h.Connect)))
}

// Connect performs, in order and ALL before websocket.Accept: a connection-cap check
// (global + per-IP, refused cleanly — docs/API.md §9.1), then parse {id} (non-UUID →
// hidden 404), then a MEMBERSHIP check via the viewer-scoped Get (a non-member → hidden
// 404, never 403 — docs/API.md §8). Only then does it upgrade with strict same-origin
// verification (never InsecureSkipVerify — a WS handshake bypasses CORS, so without this
// a cross-site page could open a socket riding the victim's auto-attached cookie). On
// accept it arms a session-expiry close, registers with the hub, sends the immediate
// per-viewer snapshot, and runs the pumps until the socket closes.
func (h *Handler) Connect(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserID(r.Context()) // RequireAuth guarantees presence

	// Admission control FIRST — cheapest possible check, shedding load before the
	// membership check's DB round-trip when the process (or this IP) is already at
	// capacity (docs/IDEAS.md "A global connection semaphore / per-IP cap"). release
	// is deferred immediately so EVERY subsequent exit path — the 404s below, a
	// failed Accept, a hub-shutdown refusal, or normal pump completion (panic
	// included, via the pumps' own recover) — decrements it exactly once.
	release, ok := h.limiter.tryAcquire(web.ClientIP(r, h.limits.TrustProxy))
	if !ok {
		web.Error(w, http.StatusTooManyRequests, web.CodeRateLimited, "too many connections")
		return
	}
	defer release()

	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		web.Error(w, http.StatusNotFound, web.CodeNotFound, "not found")
		return
	}

	// Membership gate BEFORE Accept: reuse the viewer-scoped read, so a non-player is a
	// hidden 404 (identical to REST) and never learns the match exists.
	if _, err := h.svc.Get(r.Context(), uid, id); err != nil {
		if errors.Is(err, game.ErrNotFound) {
			web.Error(w, http.StatusNotFound, web.CodeNotFound, "not found")
			return
		}
		h.logger.Error("ws: membership check", "matchID", id, "err", err)
		web.Error(w, http.StatusInternalServerError, web.CodeInternal, "internal error")
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: h.originPatterns,
		// InsecureSkipVerify is deliberately NOT set — same-origin is enforced.
	})
	if err != nil {
		// Accept has already written the handshake failure (e.g. 403 on a bad origin);
		// the response is spent — just log.
		h.logger.Warn("ws: accept", "matchID", id, "err", err)
		return
	}
	// The response is hijacked past this point — never call web.Error. CloseNow is the
	// final teardown (runs LAST; a no-op if the pumps already closed the conn).
	defer conn.CloseNow()

	// The pumps run on a background-derived context (the socket outlives the request),
	// cancelled by forceClose to abort a blocked Read/Write on teardown.
	connCtx, cancel := context.WithCancel(context.Background())
	c := newClient(uid, conn, cancel, h.logger, h.limits.ReadIdleTimeout, h.limits.HeartbeatInterval)

	// Nothing re-validates the cookie mid-connection, so close the socket at the JWT exp
	// with a 4001 the client reads as "re-authenticate". RequireAuth already rejected an
	// expired token, so exp is in the future.
	if exp, ok := auth.SessionExpiry(r.Context()); ok && !exp.IsZero() {
		timer := time.AfterFunc(time.Until(exp), func() {
			_ = conn.Close(wsStatusSessionExpired, "session expired")
			c.forceClose()
		})
		defer timer.Stop()
	}

	if !h.hub.registerClient(id, c) {
		// Hub is already shutting down; close cleanly instead of leaking the socket.
		_ = conn.Close(websocket.StatusGoingAway, "server shutting down")
		return
	}
	defer h.hub.unregisterClient(id, c)

	// Immediate per-viewer snapshot — the same MatchStateJSON REST would return to this
	// viewer, so reconnect is correct with no replay buffer. A benign race (the match
	// vanished between the gate and here) just skips it; the client still has REST.
	snapCtx, snapCancel := context.WithTimeout(connCtx, wsBuildTimeout)
	payload, snapErr := h.svc.MatchStateJSON(snapCtx, uid, id)
	snapCancel()
	if snapErr == nil {
		c.trySend(h.hub.matchStateFrameBytes(payload))
	} else if !errors.Is(snapErr, game.ErrNotFound) {
		h.logger.Error("ws: initial snapshot", "matchID", id, "err", snapErr)
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); c.readPump(connCtx); c.forceClose() }()
	go func() { defer wg.Done(); c.writePump(connCtx); c.forceClose() }()
	go func() { defer wg.Done(); c.heartbeatLoop(connCtx); c.forceClose() }()
	wg.Wait()
}
