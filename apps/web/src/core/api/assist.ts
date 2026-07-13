import type { DocSummary, Op } from '@justpaint/document'
import { request } from './http'

/**
 * Typed client for the AI-assist endpoint (docs/ASSIST.md §5), on the same native
 * `fetch` plumbing as the drawings client (`./http` — BASE, the `ApiError`
 * envelope, `request`). Stateless: a prompt + a compact doc summary go up, a
 * validated Op batch comes back. The full document is NEVER sent (token thrift);
 * the server sees only `docSummary` (§4). Session STATE stays in `useSessionStore`.
 */

/* ------------------------------------------------------------------ */
/* Wire types (camelCase, exact from the Go DTO structs).             */
/* ------------------------------------------------------------------ */

export interface AssistOpsRequest {
    prompt: string
    docSummary: DocSummary
    /** Optional: bias generation onto this layer. */
    targetLayerId?: string
}

export interface AssistOpsResponse {
    ops: Op[]
    /** Optional one-line explanation of the generated batch, surfaced in the UI. */
    note?: string
}

export const assist = {
    async ops(req: AssistOpsRequest): Promise<AssistOpsResponse> {
        return await request<AssistOpsResponse>('/assist/ops', { method: 'POST', body: req })
    }
}
