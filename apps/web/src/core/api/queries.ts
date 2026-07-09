import { useMutation, useQueryClient } from '@tanstack/vue-query'
import type { Document } from '@justpaint/document'
import { drawings } from './drawings'
import type { DrawingFull, DrawingMeta } from './drawings'
import { matches } from './matches'
import type { Match, SubmitMatch } from './matches'

/**
 * TanStack Query bindings for the drawings API (ROADMAP Phase 2 "state" pass:
 * TanStack Query owns server data; Pinia owns session/UI state; the editor owns
 * its own document/view state). Save + load are modelled as mutations (they're
 * imperative button actions), giving the view standardized `isPending`/`error`
 * and cache invalidation for free. The typed fetch client (`./drawings`) stays
 * the single source of the request shapes; these only wrap it.
 */

/** Query keys for the drawings cache (a future saved-drawings list reads these). */
export const drawingsKeys = {
    all: ['drawings'] as const,
    list: ['drawings', 'list'] as const
}

export interface SaveDrawingVars {
    /** Existing drawing id to update, or undefined to create a new one. */
    id?: string
    document: Document
    /** Drawing name; omitted = server default on create / keep the current name on update. */
    name?: string
}

/** Create-or-update the current drawing; invalidates the cached list on success. */
export function useSaveDrawing() {
    const qc = useQueryClient()
    return useMutation({
        mutationFn: ({ id, document, name }: SaveDrawingVars): Promise<DrawingMeta> =>
            id ? drawings.update(id, document, name) : drawings.create(document, name),
        onSuccess: () => {
            void qc.invalidateQueries({ queryKey: drawingsKeys.list })
        }
    })
}

/** Load the most recent free drawing; resolves null when nothing is saved yet. */
export function useLoadLatestDrawing() {
    return useMutation({
        mutationFn: async (): Promise<DrawingFull | null> => {
            const page = await drawings.list({ limit: 1, kind: 'free' })
            const first = page.drawings[0]
            return first ? drawings.get(first.id) : null
        }
    })
}

/**
 * Match mutations for the imperative duel actions (create/auto-join + submit).
 * The reads that DRIVE the flow — the roster poll (`matches.get`) and the verdict
 * poll (`matches.result`) — are called directly from the /play phase machine (an
 * ephemeral per-round flow with no shared cache to own, mirroring how
 * `useLoadLatestDrawing` reaches straight to `drawings.get`). WS push replaces the
 * polling later (docs/API.md §9, not-v1).
 */

/** Create or auto-join an async match. */
export function useCreateMatch() {
    return useMutation({
        mutationFn: (): Promise<Match> => matches.create()
    })
}

/** Submit the caller's vector document for a match. */
export function useSubmitMatch() {
    return useMutation({
        mutationFn: ({ id, document }: { id: string; document: Document }): Promise<SubmitMatch> =>
            matches.submit(id, document)
    })
}
