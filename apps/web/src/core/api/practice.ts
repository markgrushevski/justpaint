import type { Document } from '@justpaint/document'
import { roundDocument } from '@justpaint/document'
import { request } from './http'

/**
 * Typed client for single-player practice (`/api/practice`), on the shared
 * cookie-session `fetch` plumbing (`./http`). Practice is the duel's little
 * sibling: one player, one prompt, the SAME real judge — it exists because a duel
 * needs two people at once and there is nobody to match with yet.
 *
 * Two auth-required calls. `prompt()` hands out something to draw; `run()` posts
 * the vector document and comes back with the verdict. Unlike the duel there is
 * no lifecycle to poll: `run()` is SYNCHRONOUS and slow — the server renders the
 * authoritative raster and waits on a vision model — so callers must budget
 * several seconds and show a judging state, not a frozen button.
 *
 * The Go DTOs are the source of truth for these shapes (camelCase, exact). The
 * trust boundary is the duel's: only the vector document goes up, never a
 * client-rendered PNG (DOCUMENT-FORMAT §10).
 */

/** Something to draw. `text` is never redacted — there is no opponent to be fair
 *  to, so unlike a duel prompt it arrives revealed. */
export interface PracticePrompt {
    id: string
    text: string
}

/** One scored attempt. `score` is the judge's similarity in 0..1; `feedback` is
 *  plain text (up to 500 characters) and is the whole product here — in a duel
 *  the reason explains who won, in practice it is the reason to come back. */
export interface PracticeRun {
    id: string
    score: number
    feedback: string
    prompt: PracticePrompt
}

interface PromptEnvelope {
    prompt: PracticePrompt
}
interface RunEnvelope {
    run: PracticeRun
}

export const practice = {
    /** Fetch a prompt to draw. */
    async prompt(): Promise<PracticePrompt> {
        return (await request<PromptEnvelope>('/practice/prompt')).prompt
    },
    /** Submit a drawing against `promptId` and wait for the judge (seconds). */
    async run(promptId: string, doc: Document): Promise<PracticeRun> {
        return (
            await request<RunEnvelope>('/practice', {
                method: 'POST',
                body: { promptId, document: roundDocument(doc) }
            })
        ).run
    }
}
