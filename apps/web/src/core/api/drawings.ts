import type { Document } from '@justpaint/document'
import { parseDocument } from '@justpaint/document'

/**
 * Typed client for the Go backend's auth + drawings API (docs/API.md), on native
 * `fetch` (no axios). Cookie-based session (`jp_session`, HttpOnly) via
 * `credentials: 'include'` — no Authorization header, no localStorage. In dev the
 * vite proxy forwards same-origin `/api` to the Go server (:8080), so the cookie
 * is first-party. Session STATE lives in `useSessionStore`; this module is
 * store-free (no api⇄store cycle).
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

interface RequestOptions {
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
async function request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
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

/* ------------------------------------------------------------------ */
/* Wire types (camelCase, exact from the Go DTO structs).             */
/* ------------------------------------------------------------------ */

export interface User {
    id: string
    login: string
    displayName: string | null
    rating: number
    createdAt: string
}

export interface DrawingMeta {
    id: string
    ownerId: string
    matchId: string | null
    name: string
    docVersion: number
    width: number
    height: number
    thumbnailUrl: string | null
    createdAt: string
    updatedAt: string
}

export type DrawingFull = DrawingMeta & { document: Document }

export interface ListParams {
    limit?: number
    cursor?: string
    kind?: 'free' | 'duel' | 'all'
}

export interface DrawingList {
    drawings: DrawingMeta[]
    nextCursor: string | null
    limit: number
}

interface UserEnvelope {
    user: User
}
interface DrawingMetaEnvelope {
    drawing: DrawingMeta
}
interface DrawingFullEnvelope {
    drawing: DrawingFull
}

/* ------------------------------------------------------------------ */
/* Auth.                                                              */
/* ------------------------------------------------------------------ */

export interface RegisterBody {
    login: string
    password: string
    displayName?: string | null
}

export interface LoginBody {
    login: string
    password: string
}

export const auth = {
    async register(body: RegisterBody): Promise<User> {
        return (await request<UserEnvelope>('/auth/register', { method: 'POST', body })).user
    },
    async login(body: LoginBody): Promise<User> {
        return (await request<UserEnvelope>('/auth/login', { method: 'POST', body })).user
    },
    async logout(): Promise<void> {
        await request<void>('/auth/logout', { method: 'POST' })
    },
    async me(): Promise<User> {
        return (await request<UserEnvelope>('/auth/me')).user
    }
}

/* ------------------------------------------------------------------ */
/* Drawings CRUD. Body is `{ document: <vector doc>, name? }` — name  */
/* omitted when undefined (server defaults create to 'new art' and    */
/* keeps the existing name on update).                                */
/* ------------------------------------------------------------------ */

export const drawings = {
    async create(doc: Document, name?: string): Promise<DrawingMeta> {
        const body = name === undefined ? { document: doc } : { document: doc, name }
        return (await request<DrawingMetaEnvelope>('/drawings', { method: 'POST', body })).drawing
    },
    async get(id: string): Promise<DrawingFull> {
        const { drawing } = await request<DrawingFullEnvelope>('/drawings/' + id)
        // The server validated on write; re-validate on read before it reaches
        // the editor (loadDocument does NOT validate). Throws on a bad body.
        return { ...drawing, document: parseDocument(drawing.document) }
    },
    async update(id: string, doc: Document, name?: string): Promise<DrawingMeta> {
        const body = name === undefined ? { document: doc } : { document: doc, name }
        return (await request<DrawingMetaEnvelope>('/drawings/' + id, { method: 'PUT', body })).drawing
    },
    async remove(id: string): Promise<void> {
        await request<void>('/drawings/' + id, { method: 'DELETE' })
    },
    async list(params: ListParams = {}): Promise<DrawingList> {
        return await request<DrawingList>('/drawings', {
            query: { limit: params.limit, cursor: params.cursor, kind: params.kind }
        })
    }
}
