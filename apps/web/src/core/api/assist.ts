import type { DocSummary, Op } from '@justpaint/editor'
import { request } from './http'

/**
 * Typed client for the AI-assist endpoint (docs/ASSIST.md §5), on the shared
 * `fetch` plumbing (`./http`). Stateless: a prompt + a compact doc summary go
 * up, a validated Op batch comes back. The full document is never sent (token
 * thrift) — the server sees only `docSummary` (docs/ASSIST.md §4).
 */

// Wire types (camelCase, exact from the Go DTO structs).

export interface AssistOpsRequest {
    prompt: string
    docSummary: DocSummary
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
        return request<AssistOpsResponse>('/assist/ops', { method: 'POST', body: req })
    }
}
