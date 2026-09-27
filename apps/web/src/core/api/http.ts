/**
 * The shared `fetch` plumbing every typed API client is built on. Cookie
 * session (`jp_session`, HttpOnly) via `credentials: 'include'`, and the base
 * is the relative `/api` so every environment is same-origin (dev proxy, prod:
 * the Go binary serves the SPA) — which is also why {@link ApiError} can read
 * the non-CORS-safelisted `Retry-After` header at all. Store-free: session
 * state lives in `useSessionStore`.
 */

const BASE = '/api'

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
 * throws this for every non-2xx too, carrying the Go error `code`, HTTP status,
 * and the one response header that carries meaning the envelope does not.
 */
export class ApiError extends Error {
    readonly code: ClientErrorCode
    readonly status: number
    /** `Retry-After` in seconds, or null if absent/unparseable — the only way to
     *  tell the two 429s apart, see {@link isBudgetExhausted}. */
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
 * Our server only sends delta-seconds, but RFC 9110 also allows an HTTP-date,
 * and a platform edge or CDN in front of us may answer its own 429 that way —
 * both spellings are parsed so neither is misread as the daily budget.
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
 * True when the server refused because some ceiling is spent — a refusal, not a
 * fault, so a caller can keep a red toast off an expected outcome. Deliberately
 * broad: `rate_limited` covers two unrelated limits (docs/API.md §3.1), so this
 * does not answer "is retrying pointless" — ask {@link isBudgetExhausted} for that.
 */
export function isRateLimited(err: unknown): boolean {
    return err instanceof ApiError && (err.code === 'rate_limited' || err.status === 429)
}

/**
 * True for the refusal a retry cannot get round: the daily AI-call budget
 * (docs/GAME.md §4.3) — the caller's own allowance, or the provider's whole
 * budget — which does not come back until the day rolls over. Discriminated by
 * the missing `Retry-After`: every per-IP tier sends one, the budget never does.
 * Mistaking a tier's 429 for the budget's tells someone they're out for the day
 * when a two-second retry would have worked.
 */
export function isBudgetExhausted(err: unknown): boolean {
    return err instanceof ApiError && isRateLimited(err) && err.retryAfter === null
}

/**
 * Called whenever the server answers "no session" — the one place that sees
 * every 401, so no caller has to remember to forget a rejected session.
 * Forgetting is transport knowledge; ASKING for a new one is not and stays out
 * of here, since a background poll or WS reconnect must never throw a sign-in
 * modal in the player's face (docs/DECISIONS.md). `main.ts` wires this, keeping
 * the module store-free.
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
