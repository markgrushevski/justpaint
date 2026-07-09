import type { Document } from '@justpaint/document'
import { parseDocument } from '@justpaint/document'
import { request } from './http'

/**
 * Typed client for the Go backend's auth + drawings API (docs/API.md), on native
 * `fetch` (no axios). The shared cookie-session `fetch` plumbing lives in
 * `./http` (BASE, the `ApiError` envelope, `request`); this module carries only
 * the drawings/auth wire types + endpoints. Session STATE lives in
 * `useSessionStore`; both are store-free (no api⇄store cycle).
 */

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
