import type { Document } from '@justpaint/editor'
import { roundDocument } from '@justpaint/editor'
import { request } from './http'

/**
 * Typed client for the async drawing-duel API (docs/API.md §8, docs/GAME.md), on
 * the shared `fetch` plumbing (`./http`). Every route is under `/api/matches`
 * and auth-required.
 *
 * Lifecycle: create/auto-join (`POST /matches`) -> both draw the same prompt ->
 * submit the vector document (`POST /matches/:id/submit`) -> poll the verdict
 * (`GET /matches/:id/result`) until `ready`. `GET /matches/:id` is the roster
 * poll used while waiting for the opponent to join or submit. The judged raster
 * is rendered server-side — the client never sends a scored PNG (trust
 * boundary, docs/DOCUMENT-FORMAT.md §10).
 */

/** Match lifecycle states (`matches.status`, docs/GAME.md §3). */
export type MatchStatus = 'open' | 'drawing' | 'judging' | 'done' | 'abandoned'

/** The pinned prompt. Both fields are null until the match leaves `open` (reveal timing,
 *  docs/GAME.md §5) — a lone creator waiting must not pre-draw. */
export interface MatchPrompt {
    id: string | null
    text: string | null
}

/** The canonical square game canvas echoed by the server (1080×1080). */
export interface MatchCanvas {
    width: number
    height: number
}

/** One roster slot. `displayName` is a safe label (never a login); `drawingId` is
 *  the viewer's own once submitted, and the opponent's only once `done`. */
export interface MatchPlayer {
    userId: string
    displayName: string | null
    submitted: boolean
    drawingId?: string
}

/** Full (redacted-per-viewer) match state — `POST /matches`, `GET /matches/:id`. */
export interface Match {
    id: string
    mode: string
    status: MatchStatus
    prompt: MatchPrompt
    canvas: MatchCanvas
    players: MatchPlayer[]
    /** RFC3339Nano UTC, null while `open` (docs/API.md §8). */
    drawingDeadline: string | null
    /** Response-build instant; reconciles client clock skew for the countdown. */
    serverTime: string
    createdAt: string
    updatedAt: string
}

/** The compact post-submit echo (`POST /matches/:id/submit`, 202). `status` is
 *  `drawing` while the opponent is still out, `judging` once both are in. */
export interface SubmitMatch {
    id: string
    status: MatchStatus
    you: { submitted: boolean; drawingId: string }
    /** Same deadline/clock pair as `Match` (docs/API.md §8, submit). */
    drawingDeadline: string | null
    serverTime: string
}

/** One player's revealed outcome on the result screen (both shown once `done`). */
export interface ResultPlayer {
    userId: string
    displayName: string | null
    drawingId: string | null
    /** Judge similarity score in 0..1 (null until scored). */
    score: number | null
    ratingBefore: number | null
    ratingAfter: number | null
    /** Server-rendered authoritative raster URL — null until object storage lands. */
    judgedImageUrl: string | null
}

/** The in-flight result body: `status` echoes the live match state, `ready` false. */
export interface MatchResultPending {
    status: MatchStatus
    ready: false
}

/** The decided result body. Both canvases revealed. */
export interface MatchResultDone {
    status: 'done'
    ready: true
    prompt: MatchPrompt
    /** Resolved winner player id, or null on a tie (ties allowed — docs/JUDGE.md). */
    winnerUserId: string | null
    isTie: boolean
    reason: string | null
    /** `judged` / `forfeit` / `aborted` (docs/API.md §8, result). Branch on this,
     *  never on the free-text `reason`. */
    resolution: 'judged' | 'forfeit' | 'aborted'
    players: ResultPlayer[]
}

/** `GET /matches/:id/result` — a discriminated union on `ready`. */
export type MatchResult = MatchResultPending | MatchResultDone

interface MatchEnvelope {
    match: Match
}
interface SubmitEnvelope {
    match: SubmitMatch
}
interface ResultEnvelope {
    result: MatchResult
}
interface PlayerDrawingEnvelope {
    document: Document
}

export const matches = {
    /** Create or auto-join a match; the server pins one shared prompt. A caller with a match
     *  still in play gets that one back. */
    async create(): Promise<Match> {
        return (await request<MatchEnvelope>('/matches', { method: 'POST' })).match
    },
    /** Fetch (redacted) match state — the roster poll while waiting for the opponent. */
    async get(id: string): Promise<Match> {
        return (await request<MatchEnvelope>('/matches/' + id)).match
    },
    /** Submit the caller's vector document (validated + 1080²-checked server-side). */
    async submit(id: string, doc: Document): Promise<SubmitMatch> {
        return (
            await request<SubmitEnvelope>('/matches/' + id + '/submit', {
                method: 'POST',
                body: { document: roundDocument(doc) }
            })
        ).match
    },
    /** Abandon the caller's own open match on leaving the queue: 204, or 409 once the round
     *  has started (it then runs on). */
    async cancel(id: string): Promise<void> {
        await request<void>('/matches/' + id + '/cancel', { method: 'POST' })
    },
    /** The same cancel from a page being unloaded, where a fetch may never leave. A beacon is
     *  same-origin, so it carries the session cookie. */
    cancelOnUnload(id: string): void {
        navigator.sendBeacon('/api/matches/' + id + '/cancel')
    },
    /** The end-of-round verdict; poll until `ready`. The WS `result` frame carries the
     *  same body instantly, so this poll is the reconciliation fallback, not the path. */
    async result(id: string): Promise<MatchResult> {
        return (await request<ResultEnvelope>('/matches/' + id + '/result')).result
    },
    /**
     * A participant's submitted vector document — how the reveal shows the
     * opponent's canvas, since the ownership-scoped `GET /drawings/:id` 404s a
     * non-owner (docs/API.md §8). No object storage: the client renders it.
     */
    async playerDrawing(matchId: string, userId: string): Promise<Document> {
        return (await request<PlayerDrawingEnvelope>('/matches/' + matchId + '/players/' + userId + '/drawing'))
            .document
    }
}

// WS realtime (docs/API.md §9 wire protocol).

/** The 8 server→client frames (docs/API.md §9.2, `server/internal/ws/events.go`),
 *  discriminated on `type`. `match` / `result` carry the same DTOs as the
 *  equivalent REST responses, rebuilt per recipient. */
export type WsFrame =
    | { type: 'match_state'; match: Match }
    | { type: 'opponent_submitted'; userId: string }
    | { type: 'judging' }
    | { type: 'result'; result: MatchResultDone }
    | { type: 'abandoned' }
    | { type: 'opponent_connected'; userId: string }
    | { type: 'opponent_disconnected'; userId: string }
    | { type: 'pong' }

const WS_FRAME_TYPES = new Set<WsFrame['type']>([
    'match_state',
    'opponent_submitted',
    'judging',
    'result',
    'abandoned',
    'opponent_connected',
    'opponent_disconnected',
    'pong'
])

/** Parse one WS text message into a {@link WsFrame}, or null if it's unparseable
 *  or an unknown `type` — never thrown, since a stray frame must not take down
 *  the socket. */
function parseWsFrame(raw: string): WsFrame | null {
    let parsed: unknown
    try {
        parsed = JSON.parse(raw)
    } catch {
        return null
    }
    if (typeof parsed !== 'object' || parsed === null) return null
    const type = (parsed as { type?: unknown }).type
    if (typeof type !== 'string' || !WS_FRAME_TYPES.has(type as WsFrame['type'])) return null
    return parsed as WsFrame
}

export interface MatchSocketHandlers {
    /** Called for every frame that parses to a known {@link WsFrame}. */
    onFrame(frame: WsFrame): void
    onOpen?(): void
    onClose?(code: number): void
}

/** A thin transport handle — reconnect/backoff policy lives in `useMatchSocket`, frame
 *  dispatch in `useDuel`. */
export interface MatchSocketHandle {
    /** Close the socket. Safe to call more than once. */
    close(): void
    /** Send the one client→server frame the wire protocol allows (heartbeat).
     *  A no-op if the socket isn't currently open. */
    ping(): void
}

/**
 * Open the live match socket: same-origin `GET /api/matches/:id/ws` (cookie
 * auth rides the handshake automatically, docs/API.md §9.1). Built from
 * `location.*` rather than the request base, equivalent since the base is
 * always the relative `/api`.
 *
 * A thin wrapper over native `WebSocket`: parses each message into a
 * {@link WsFrame}, drops anything unparseable, and forwards open/close.
 * No reconnect/backoff/dispatch policy — the caller owns that.
 */
export function openMatchSocket(matchId: string, handlers: MatchSocketHandlers): MatchSocketHandle {
    const scheme = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const socket = new WebSocket(`${scheme}//${location.host}/api/matches/${matchId}/ws`)

    socket.addEventListener('open', () => handlers.onOpen?.())
    socket.addEventListener('close', (ev) => handlers.onClose?.(ev.code))
    socket.addEventListener('message', (ev) => {
        if (typeof ev.data !== 'string') return
        const frame = parseWsFrame(ev.data)
        if (frame) handlers.onFrame(frame)
    })

    return {
        close(): void {
            socket.close(1000, 'client disposed')
        },
        ping(): void {
            if (socket.readyState === WebSocket.OPEN) socket.send(JSON.stringify({ type: 'ping' }))
        }
    }
}
