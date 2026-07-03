import { useMutation, useQueryClient } from '@tanstack/vue-query'
import type { Document } from '@justpaint/document'
import { drawings } from './drawings'
import type { DrawingFull, DrawingMeta } from './drawings'

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
}

/** Create-or-update the current drawing; invalidates the cached list on success. */
export function useSaveDrawing() {
    const qc = useQueryClient()
    return useMutation({
        mutationFn: ({ id, document }: SaveDrawingVars): Promise<DrawingMeta> =>
            id ? drawings.update(id, document) : drawings.create(document),
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
