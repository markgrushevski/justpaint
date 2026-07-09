import type { Document } from '@justpaint/document'
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
    createdAt: string
    updatedAt: string
}

/** The compact post-submit echo (`POST /matches/:id/submit`, 202). `status` is
 *  `drawing` while the opponent is still out, `judging` once both are in. */
export interface SubmitMatch {
    id: string
    status: MatchStatus
    you: { submitted: boolean; drawingId: string }
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
            await request<SubmitEnvelope>('/matches/' + id + '/submit', { method: 'POST', body: { document: doc } })
        ).match
    },
    /** The end-of-round verdict; poll until `ready` (WS push replaces this later). */
    async result(id: string): Promise<MatchResult> {
        return (await request<ResultEnvelope>('/matches/' + id + '/result')).result
    }
}
