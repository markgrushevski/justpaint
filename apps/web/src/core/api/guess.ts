import type { Document } from '@justpaint/editor'
import { roundDocument } from '@justpaint/editor'
import { request } from './http'

/**
 * Typed client for the drawing-guess endpoint (`POST /api/guess`), on the shared
 * `fetch` plumbing (`./http`). Slow by nature, like `practice.run`: the server
 * renders the drawing to a raster and only then asks a vision model, so callers
 * must budget several seconds and show a pending state. Only the vector document
 * is sent, never a client-rendered PNG (docs/DOCUMENT-FORMAT.md §10).
 */

/** The AI's reading of a drawing. */
export interface Guess {
    /** What it thinks the drawing is, as a noun phrase ("a cat wearing a hat"). */
    label: string
    /** How sure it is, 0..1. Meant to be shown as a phrase, never as a raw number. */
    confidence: number
    /** 0-2 runner-up guesses. Always an array (docs/API.md §13); the `?? []`
     *  below is belt-and-braces against a handler that forgets to normalize. */
    alternatives: string[]
}

/** The response body, typed to tolerate an `alternatives` the server promises
 *  never to send (see above); {@link Guess} makes no such allowance. */
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
