import type { Document } from '@justpaint/editor'
import { roundDocument } from '@justpaint/editor'
import { request } from './http'

/**
 * Typed client for the drawing-guess endpoint (`POST /api/guess`), on the shared
 * cookie-session `fetch` plumbing (`./http`). One auth-required call: the current
 * vector document goes up, the AI's reading of it comes back.
 *
 * SLOW by nature, exactly like `practice.run` — the server renders the drawing to
 * a raster and only then asks a vision model, all in-request — so callers must
 * budget several seconds and show a pending state rather than a frozen button.
 *
 * Trust boundary: only the vector document is sent, never a client-rendered PNG
 * (DOCUMENT-FORMAT §10) — the server derives anything it looks at itself.
 *
 * The Go DTOs are the source of truth for these shapes (camelCase, exact).
 */

/** The AI's reading of a drawing. */
export interface Guess {
    /** What it thinks the drawing is, as a noun phrase ("a cat wearing a hat"). */
    label: string
    /** How sure it is, 0..1. Meant to be shown as a phrase, never as a raw number. */
    confidence: number
    /**
     * 0–2 runner-up guesses. Always an array, and the server already promises
     * that: the handler normalizes its nil slice to `[]` before marshalling
     * (`internal/guess/handler.go`), because an absent list and an empty one are
     * the same fact and the client should not have to know two spellings of it
     * (docs/API.md §13). The `?? []` below is belt-and-braces against a future
     * handler that forgets — one normalization at the edge is cheaper than every
     * consumer null-checking the fun part.
     */
    alternatives: string[]
}

/** The response body. Typed to tolerate an `alternatives` the server promises never
 *  to send (see above); {@link Guess} makes no such allowance. */
interface GuessEnvelope {
    guess: Omit<Guess, 'alternatives'> & { alternatives?: string[] | null }
}

export const guess = {
    /** Ask what the drawing is, and wait on the vision model (seconds). */
    async ask(doc: Document): Promise<Guess> {
        const body = await request<GuessEnvelope>('/guess', { method: 'POST', body: { document: roundDocument(doc) } })
        return { ...body.guess, alternatives: body.guess.alternatives ?? [] }
    }
}
