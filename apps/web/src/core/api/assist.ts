import type { Document, Op } from '@justpaint/editor'
import { roundDocument } from '@justpaint/editor'
import { request } from './http'

/**
 * Typed client for the AI-assist endpoint (docs/ASSIST.md §5), on the shared
 * `fetch` plumbing (`./http`). Stateless: a prompt and the current document go up,
 * a validated Op batch comes back. The server renders the document for the model
 * (docs/ASSIST.md §4).
 */

// Wire types (camelCase, exact from the Go DTO structs).

export interface AssistOpsRequest {
    prompt: string
    document: Document
    /** Bias generation onto this layer. */
    targetLayerId?: string
}

export interface AssistOpsResponse {
    ops: Op[]
    /** One-line explanation of the generated batch, surfaced in the UI. */
    note?: string
}

export const assist = {
    async ops(req: AssistOpsRequest): Promise<AssistOpsResponse> {
        const body = { ...req, document: roundDocument(req.document) }
        return request<AssistOpsResponse>('/assist/ops', { method: 'POST', body })
    }
}
