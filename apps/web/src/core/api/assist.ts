import type { DocSummary, Op } from '@justpaint/document'
import { safeValidateOpBatch } from '@justpaint/document'
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
        const res = await request<AssistOpsResponse>('/assist/ops', { method: 'POST', body: req })
        // Trust edge (docs/REVIEW.md): the endpoint generated + validated the
        // batch, but it arrived over the network, so re-validate it against the
        // request's OWN docSummary before it reaches the editor — `previewOps`
        // does NOT validate. Mirrors how `drawings.get` re-parses a fetched
        // document. Throws a DocumentValidationError on a bad body, which the
        // mutation's `onError` surfaces.
        const result = safeValidateOpBatch(req.docSummary, res.ops)
        if (!result.ok) throw result.error
        return { ...res, ops: result.ops }
    }
}
