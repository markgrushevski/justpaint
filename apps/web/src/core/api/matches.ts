import type { Document } from '@justpaint/document'
import { roundDocument } from '@justpaint/document'
import { request } from './http'

/**
 * Typed client for the async drawing-duel API (docs/API.md §8, docs/GAME.md), on
 * the shared cookie-session `fetch` plumbing (`./http`). Every route is under
 * `/api/matches` and auth-required. The Go DTOs (`server/internal/game`) are the
 * source of truth for these shapes — camelCase, exact.
 *
 * The lifecycle: create/auto-join (`POST /matches`) → both draw the SAME prompt →
 * submit the vector document (`POST /matches/:id/submit`) → poll the verdict
 * (`GET /matches/:id/result`) until `ready`. `GET /matches/:id` is the roster poll
 * used while waiting for the opponent to join / submit. The authoritative judged
 * raster is rendered server-side — the client never sends a scored PNG (trust
 * boundary, DOCUMENT-FORMAT §10).
 */

/** Match lifecycle states (`matches.status`, docs/GAME.md §3). */
export type MatchStatus = 'open' | 'drawing' | 'judging' | 'done' | 'abandoned'

/** The pinned prompt. `text` is null until the match leaves `open` (reveal timing,
 *  docs/GAME.md §5) — a lone creator waiting must not pre-draw. */
export interface MatchPrompt {
    id: string
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
    /** Absolute round deadline (RFC3339Nano, UTC) — null while `open` (not stamped
     *  until the match enters `drawing`). The client counts down against this,
     *  reconciled with `serverTime` (docs/API.md §8). */
    drawingDeadline: string | null
    /** The response-build instant (RFC3339Nano, UTC), always present — lets the
     *  client correct clock skew before computing the countdown from
     *  `drawingDeadline`. */
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
    /** Same deadline/clock pair as `Match`, so the submit ack re-anchors the
     *  client countdown without a follow-up GET (docs/API.md §8.3). */
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

/** The decided result body (`status: 'done'`, `ready: true`). Both canvases revealed. */
export interface MatchResultDone {
    status: 'done'
    ready: true
    prompt: MatchPrompt
    /** Resolved winner player id, or null on a tie (ties allowed — docs/JUDGE.md). */
    winnerUserId: string | null
    isTie: boolean
    reason: string | null
    /** How the match was decided: `judged` (the ML judge ran), `forfeit` (one
     *  player never submitted before the deadline, default win, no judge run), or
     *  `aborted` (judging exhausted its retries — both players drew, nobody was
     *  scored, no rating moved; docs/GAME.md §3). The client branches its copy on
     *  this, never on the free-text `reason` (docs/API.md §8.4). */
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
    /** Create or auto-join an async match; the server pins one shared prompt. */
    async create(mode: 'async' = 'async'): Promise<Match> {
        return (await request<MatchEnvelope>('/matches', { method: 'POST', body: { mode } })).match
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
    /** The end-of-round verdict; poll until `ready`. The WS `result` frame carries the
     *  same body instantly, so this poll is the reconciliation fallback, not the path. */
    async result(id: string): Promise<MatchResult> {
        return (await request<ResultEnvelope>('/matches/' + id + '/result')).result
    },
    /**
     * A fellow participant's submitted vector document — how the reveal shows the
     * OPPONENT's canvas. Authorized by match membership + `done` status, since the
     * ownership-scoped `GET /drawings/:id` 404s a non-owner (docs/API.md §8). No
     * object storage: the client renders the returned document. `userId` may be the
     * caller's own too (a uniform participant-drawing read).
     */
    async playerDrawing(matchId: string, userId: string): Promise<Document> {
        return (await request<PlayerDrawingEnvelope>('/matches/' + matchId + '/players/' + userId + '/drawing'))
            .document
    }
}

/* --- WS realtime (docs/API.md §9 wire protocol) --- */

/**
 * The 8 server→client frames the WS hub (`server/internal/ws/events.go`) emits,
 * mirrored here EXACTLY, discriminated on `type`. `match` / `result` carry the
 * SAME DTOs as the equivalent REST responses (`Match` / `MatchResultDone`) — the
 * hub builds them through the identical viewer-scoped read the REST handlers use,
 * so there is one shape, two transports.
 */
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

/** Parse one WS text message into a {@link WsFrame}, or null for anything
 *  unparseable / not one of the known `type`s (silently dropped by the caller —
 *  never thrown, since a stray frame must not take down the socket). */
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
    onClose?(code: number, reason: string): void
    onError?(ev: Event): void
}

/** A thin transport handle — reconnect/backoff policy and frame dispatch live in
 *  the caller (PlayView), not here. */
export interface MatchSocketHandle {
    /** Close the socket. Safe to call more than once. */
    close(): void
    /** Send the ONE client→server frame the wire protocol allows (heartbeat).
     *  A no-op if the socket isn't currently open. */
    ping(): void
    readonly readyState: number
}

/**
 * Open the live match socket: same-origin `GET /api/matches/:id/ws` (the
 * `jp_session` cookie rides the handshake automatically — a WS handshake can't
 * carry a custom header, so cookie auth is the only mechanism, same as REST).
 * Built from `location.*` rather than the request base, which is equivalent: the
 * base is always the relative `/api`, so this is same-origin in dev (through the
 * Vite proxy) and in production (the Go binary serves the SPA) alike.
 *
 * A thin wrapper over native `WebSocket`: JSON-parses each message into a
 * {@link WsFrame} (dropping anything unparseable or of an unknown `type`) and
 * forwards open/close/error. No reconnect/backoff/dispatch policy here — the
 * caller owns all of that.
 */
export function openMatchSocket(matchId: string, handlers: MatchSocketHandlers): MatchSocketHandle {
    const scheme = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const socket = new WebSocket(`${scheme}//${location.host}/api/matches/${matchId}/ws`)

    socket.addEventListener('open', () => handlers.onOpen?.())
    socket.addEventListener('close', (ev) => handlers.onClose?.(ev.code, ev.reason))
    socket.addEventListener('error', (ev) => handlers.onError?.(ev))
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
        },
        get readyState(): number {
            return socket.readyState
        }
    }
}
