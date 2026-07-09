/**
 * The shared `fetch` plumbing every typed API client is built on (drawings,
 * matches, …). Cookie-based session (`jp_session`, HttpOnly) via
 * `credentials: 'include'` — no Authorization header, no localStorage. In dev the
 * vite proxy forwards same-origin `/api` to the Go server (:8080), so the cookie
 * is first-party. This module is store-free (no api⇄store cycle); session STATE
 * lives in `useSessionStore`.
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
 * throws this for every non-2xx too, carrying the Go error `code` + HTTP status.
 */
export class ApiError extends Error {
    readonly code: ClientErrorCode
    readonly status: number
    constructor(message: string, code: ClientErrorCode, status: number) {
        super(message)
        this.name = 'ApiError'
        this.code = code
        this.status = status
    }
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
        throw new ApiError(message, code, res.status)
    }

    if (res.status === 204) return undefined as T
    return (await res.json()) as T
}
