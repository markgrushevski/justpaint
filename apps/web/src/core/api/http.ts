/**
 * The shared `fetch` plumbing every typed API client is built on (drawings,
 * matches, …). Cookie-based session (`jp_session`, HttpOnly) via
 * `credentials: 'include'` — no Authorization header, no localStorage. In dev the
 * vite proxy forwards same-origin `/api` to the Go server (:8080), so the cookie
 * is first-party. This module is store-free (no api⇄store cycle); session STATE
 * lives in `useSessionStore`.
 *
 * Same-origin in EVERY environment (`VITE_URL_API` is the relative `/api`, and the
 * Go service serves the SPA itself) is also what lets {@link ApiError} read a
 * response header at all: `Retry-After` is not CORS-safelisted, so cross-origin it
 * would need an `Access-Control-Expose-Headers` the server deliberately never
 * sends. Should the API ever move to its own origin, that is the thing to fix
 * before {@link isBudgetExhausted} silently starts calling every 429 a spent day.
 */

const BASE = import.meta.env.VITE_URL_API

/** Closed v1 error-code set from web.go, plus a client-only `network` code. */
export type ApiErrorCode =
    | 'validation_failed'
    | 'invalid_credentials'
    | 'unauthorized'
    | 'forbidden'
    | 'not_found'
    | 'conflict'
    | 'document_too_large'
    | 'rate_limited'
    | 'internal'

export type ClientErrorCode = ApiErrorCode | 'network'

/**
 * A failed API call. `fetch` only rejects on a network error, so {@link request}
 * throws this for every non-2xx too, carrying the Go error `code` + HTTP status
 * + the one response header that carries meaning the envelope does not.
 */
export class ApiError extends Error {
    readonly code: ClientErrorCode
    readonly status: number
    /**
     * `Retry-After` in seconds, or null when the response carried none (or one
     * nothing could be made of). It is the ONLY thing that tells the two 429s
     * apart — see {@link isBudgetExhausted} — so it is read here, at the
     * transport, rather than left on a `Response` every caller has thrown away.
     */
    readonly retryAfter: number | null
    constructor(message: string, code: ClientErrorCode, status: number, retryAfter: number | null = null) {
        super(message)
        this.name = 'ApiError'
        this.code = code
        this.status = status
        this.retryAfter = retryAfter
    }
}

/**
 * `Retry-After` as whole seconds from now, or null when there is nothing usable.
 *
 * Our own server only ever sends delta-seconds (`platform/web/ratelimit.go` writes
 * the matched tier's refill interval), but RFC 9110 also allows an HTTP-date, and
 * a platform edge or CDN in front of us may well answer its own 429 that way.
 * Returning null for one of those would file a throttle that clears in seconds as
 * a spent daily budget — precisely the confusion this field exists to end — so
 * both spellings are parsed and only a genuinely absent or unreadable header
 * becomes null.
 */
function parseRetryAfter(res: Response): number | null {
    const raw = res.headers.get('Retry-After')?.trim()
    if (!raw) return null
    const seconds = Number(raw)
    if (Number.isFinite(seconds)) return Math.max(0, Math.floor(seconds))
    const at = Date.parse(raw)
    if (Number.isNaN(at)) return null
    return Math.max(0, Math.round((at - Date.now()) / 1000))
}

export function isApiError(err: unknown): err is ApiError {
    return err instanceof ApiError
}

/** Narrow an unknown error to an {@link ApiError}, or null. */
export function toApiError(err: unknown): ApiError | null {
    return err instanceof ApiError ? err : null
}

/** True for the auth-failure cases the UI should surface as "sign in". */
export function isAuthError(err: unknown): boolean {
    return (
        err instanceof ApiError &&
        (err.code === 'unauthorized' || err.code === 'invalid_credentials' || err.status === 401)
    )
}

/**
 * True when the server refused because SOME ceiling is spent. Deliberately broad:
 * it answers "was this a refusal rather than a fault", which is what a caller
 * needs to keep a red toast off an expected outcome.
 *
 * It does NOT answer "is retrying pointless" — `rate_limited` is one code over
 * two unrelated limits (docs/API.md §3.1). Ask {@link isBudgetExhausted} for that.
 */
export function isRateLimited(err: unknown): boolean {
    return err instanceof ApiError && (err.code === 'rate_limited' || err.status === 429)
}

/**
 * True for the refusal a retry cannot get round: the **daily AI-call budget**
 * (`docs/GAME.md` §4.3) — the caller's own allowance, or the provider's whole
 * budget — which does not come back until the day rolls over.
 *
 * The discriminator is the header, and it is a deliberate one on the server's
 * part. Every per-IP tier (`platform/web/ratelimit.go`: burst 30, one token per
 * 2s across `/api/drawings`, `/api/matches`, `/api/practice`, `/api/guess`) sends
 * a `Retry-After`; the daily budget (`internal/aibudget/http.go`) sends none,
 * *because* its window rolls continuously and there is no reset instant it could
 * honestly name. So "429 with no `Retry-After`" is exactly "spent for today".
 *
 * Why it matters: both answer `429 rate_limited`, and treating the tier's as the
 * budget's tells someone behind a NAT that they are out for the day and takes
 * away the retry that would have worked two seconds later.
 */
export function isBudgetExhausted(err: unknown): boolean {
    return err instanceof ApiError && isRateLimited(err) && err.retryAfter === null
}

/**
 * Called whenever the server answers "no session". This is the ONE place that
 * sees every 401, so no caller has to remember to forget a session the server
 * has already rejected — the omission that used to leave the side menu showing
 * the profile of someone who was no longer signed in.
 *
 * Forgetting is transport knowledge. ASKING for a new session is not, and
 * deliberately stays out of here: a background poll tick or a WS reconnect must
 * never throw a sign-in modal in the player's face (docs/DECISIONS.md). The
 * caller decides that; the transport only stops lying about the session.
 *
 * `main.ts` wires it, keeping this module store-free (no api-store cycle).
 */
let onUnauthorized: (() => void) | null = null

export function setUnauthorizedHandler(fn: (() => void) | null): void {
    onUnauthorized = fn
}

export interface RequestOptions {
    method?: string
    body?: unknown
    query?: Record<string, string | number | undefined>
}

/**
 * Thin typed wrapper over `fetch`. Sends cookies, JSON-encodes a body, builds a
 * query string, and — since `fetch` does NOT reject on 4xx/5xx — throws an
 * {@link ApiError} parsed from the Go `{error:{code,message}}` envelope on any
 * non-2xx. Returns the parsed JSON body, or `undefined` for 204.
 */
export async function request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
    const { method = 'GET', body, query } = opts

    let url = BASE + path
    if (query) {
        const qs = new URLSearchParams()
        for (const [k, v] of Object.entries(query)) if (v !== undefined) qs.set(k, String(v))
        const s = qs.toString()
        if (s) url += '?' + s
    }

    let res: Response
    try {
        res = await fetch(url, {
            method,
            credentials: 'include',
            headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
            body: body !== undefined ? JSON.stringify(body) : undefined
        })
    } catch {
        throw new ApiError('Network error (is the server running?).', 'network', 0)
    }

    if (!res.ok) {
        const data = (await res.json().catch(() => null)) as {
            error?: { code?: ClientErrorCode; message?: string }
        } | null
        const code = data?.error?.code ?? 'internal'
        const message = data?.error?.message ?? res.statusText ?? 'request failed'
        const err = new ApiError(message, code, res.status, parseRetryAfter(res))
        if (isAuthError(err)) onUnauthorized?.()
        throw err
    }

    if (res.status === 204) return undefined as T
    return (await res.json()) as T
}
