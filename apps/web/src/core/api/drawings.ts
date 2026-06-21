import axios from 'axios'
import type { AxiosError } from 'axios'
import type { Document } from '@justpaint/document'
import { parseDocument } from '@justpaint/document'
import { useUserStore } from '../stores'

/**
 * Typed client for the Go backend's auth + drawings API (docs/API.md).
 *
 * Deliberately a FRESH axios instance, not the legacy `mainAPI` one:
 *  - cookie-based session (`jp_session`, HttpOnly) → `withCredentials: true`,
 *    no Authorization header, no localStorage (API.md §1/§2).
 *  - a corrected response interceptor that reads `error.response?.status`,
 *    flips `isLoggedIn` on 401, and ALWAYS rejects (the legacy one swallowed
 *    errors into `undefined`).
 */
const api = axios.create({
    baseURL: import.meta.env.VITE_URL_API,
    withCredentials: true
})

api.interceptors.response.use(
    (res) => res,
    (error: AxiosError) => {
        if (error.response?.status === 401) {
            useUserStore().isLoggedIn = false
        }
        return Promise.reject(error)
    }
)

/* ------------------------------------------------------------------ */
/* Wire types (camelCase, exact from the Go DTO structs).             */
/* ------------------------------------------------------------------ */

/** Closed v1 error-code set from web.go. Switch on `code`, never `message`. */
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

export interface ApiError {
    code: ApiErrorCode
    message: string
}

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

/* Response envelopes. */
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
/* Error helpers.                                                     */
/* ------------------------------------------------------------------ */

/** Extract the Go error envelope `{ error: { code, message } }`, if present. */
export function toApiError(err: unknown): ApiError | null {
    if (axios.isAxiosError(err)) {
        const data = err.response?.data as { error?: ApiError } | undefined
        if (data?.error && typeof data.error.code === 'string') {
            return data.error
        }
    }
    return null
}

/** True for the auth-failure codes the toolbar should surface as "sign in". */
export function isAuthError(err: unknown): boolean {
    const code = toApiError(err)?.code
    if (code === 'unauthorized' || code === 'invalid_credentials') return true
    // No envelope (e.g. connection refused — backend not up yet) but a 401.
    return axios.isAxiosError(err) && err.response?.status === 401
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
        const res = await api.post<UserEnvelope>('/auth/register', body)
        return res.data.user
    },
    async login(body: LoginBody): Promise<User> {
        const res = await api.post<UserEnvelope>('/auth/login', body)
        return res.data.user
    },
    async logout(): Promise<void> {
        await api.post('/auth/logout')
    },
    async me(): Promise<User> {
        const res = await api.get<UserEnvelope>('/auth/me')
        return res.data.user
    }
}

/* ------------------------------------------------------------------ */
/* Drawings CRUD. Body is `{ document: <vector doc> }`; axios JSON-      */
/* stringifies the live doc object directly.                          */
/* ------------------------------------------------------------------ */

export const drawings = {
    async create(doc: Document): Promise<DrawingMeta> {
        const res = await api.post<DrawingMetaEnvelope>('/drawings', { document: doc })
        return res.data.drawing
    },
    async get(id: string): Promise<DrawingFull> {
        const res = await api.get<DrawingFullEnvelope>(`/drawings/${id}`)
        // The server validated on write, but re-validate on read before it
        // reaches the editor (loadDocument does NOT validate). Throws
        // DocumentValidationError on a bad body.
        const document = parseDocument(res.data.drawing.document)
        return { ...res.data.drawing, document }
    },
    async update(id: string, doc: Document): Promise<DrawingMeta> {
        const res = await api.put<DrawingMetaEnvelope>(`/drawings/${id}`, { document: doc })
        return res.data.drawing
    },
    async remove(id: string): Promise<void> {
        await api.delete(`/drawings/${id}`)
    },
    async list(params?: ListParams): Promise<DrawingList> {
        const res = await api.get<DrawingList>('/drawings', { params })
        return res.data
    }
}
