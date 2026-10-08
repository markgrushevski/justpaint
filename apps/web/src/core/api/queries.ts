import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { computed, toValue } from 'vue'
import type { MaybeRefOrGetter } from 'vue'
import type { Document } from '@justpaint/editor'
import { isAuthError, toApiError } from './http'
import { drawings } from './drawings'
import type { DrawingFull, DrawingList, DrawingMeta } from './drawings'
import { practice } from './practice'
import type { PracticePrompt, PracticeRun } from './practice'
import { leaderboard } from './leaderboard'
import type { LeaderboardPage } from './leaderboard'
import { assist } from './assist'
import type { AssistOpsRequest, AssistOpsResponse } from './assist'
import { guess } from './guess'
import type { Guess } from './guess'

/**
 * TanStack Query bindings for the drawings/practice/leaderboard/assist/guess
 * APIs. TanStack Query owns server data; Pinia owns session/UI state; the
 * editor owns its own document/view state; the duel runs on its own loop
 * (features/play/useDuel). Save/load and the imperative practice actions are mutations, giving the view standardized
 * `isPending`/`error` and cache invalidation for free; the saved-drawings reads
 * and the leaderboard are the cached queries. The typed fetch clients stay the
 * single source of the request shapes; these only wrap them.
 */

/** Query keys for the drawings cache. `all` is the invalidation root for every write. */
export const drawingsKeys = {
    all: ['drawings'] as const,
    list: ['drawings', 'list'] as const,
    item: (id: string) => ['drawings', 'item', id] as const
}

/** Query keys for the leaderboard cache. `all` is the invalidation root (PlayView
 *  invalidates it after a rating moves); `list(limit)` scopes the cached page so
 *  different `limit`s don't collide. */
export const leaderboardKeys = {
    all: ['leaderboard'] as const,
    list: (limit: number) => ['leaderboard', 'list', limit] as const
}

export interface SaveDrawingVars {
    /** Existing drawing id to update, or undefined to create a new one. */
    id?: string
    document: Document
    /** Drawing name; omitted = server default on create / keep the current name on update. */
    name?: string
}

/** Create-or-update the current drawing; invalidates every cached drawing on success, so a
 *  re-saved drawing's list entry, document and gallery preview all refresh. */
export function useSaveDrawing() {
    const qc = useQueryClient()
    return useMutation({
        mutationFn: ({ id, document, name }: SaveDrawingVars): Promise<DrawingMeta> =>
            id ? drawings.update(id, document, name) : drawings.create(document, name),
        onSuccess: () => {
            qc.invalidateQueries({ queryKey: drawingsKeys.all })
        }
    })
}

/** Drawings per gallery page. */
const GALLERY_PAGE_SIZE = 24

/** How long a fetched document is trusted before a read fetches it again. */
const DRAWING_STALE_MS = 60_000

/** 401 and 404 are answers, not faults: retrying them only delays the sign-in or gone state. */
function retryTransient(count: number, err: unknown): boolean {
    return !isAuthError(err) && toApiError(err)?.status !== 404 && count < 3
}

/**
 * The caller's free drawings, newest first (docs/API.md §7), one cursor page at a time.
 * `owner` is the signed-in user's id: the query stays off while it is undefined, and it
 * keys the cache so one account's list is never served to the next one in the same tab.
 */
export function useDrawingsList(owner: MaybeRefOrGetter<string | undefined>) {
    return useInfiniteQuery({
        queryKey: computed(() => [...drawingsKeys.list, toValue(owner) ?? null, 'free']),
        queryFn: ({ pageParam }): Promise<DrawingList> =>
            drawings.list({ kind: 'free', limit: GALLERY_PAGE_SIZE, cursor: pageParam }),
        initialPageParam: undefined as string | undefined,
        getNextPageParam: (last) => last.nextCursor ?? undefined,
        enabled: () => Boolean(toValue(owner)),
        retry: retryTransient
    })
}

/**
 * One drawing with its document. `shallow` keeps the document raw: it can hold a hundred
 * thousand points, and only the renderer reads it. A gallery mounts one of these per card,
 * so a refetch on focus would re-download every document; a save invalidates them instead.
 */
export function useDrawing(id: MaybeRefOrGetter<string>) {
    return useQuery({
        queryKey: computed(() => drawingsKeys.item(toValue(id))),
        queryFn: (): Promise<DrawingFull> => drawings.get(toValue(id)),
        staleTime: DRAWING_STALE_MS,
        shallow: true,
        refetchOnWindowFocus: false,
        retry: retryTransient
    })
}

/**
 * Rename a drawing. The API has no rename-only route (a PUT replaces the document), so
 * this reads the document fresh, never from the cache: a copy saved from another tab in
 * the meantime would be overwritten by the older one. Only the list is refetched: the
 * other drawings' documents did not change, and a gallery page holds one live document
 * query per card.
 */
export function useRenameDrawing() {
    const qc = useQueryClient()
    return useMutation({
        mutationFn: async ({ id, name }: { id: string; name: string }): Promise<DrawingMeta> => {
            const full = await drawings.get(id)
            return drawings.update(id, full.document, name)
        },
        onSuccess: (meta, { id }) => {
            qc.setQueryData<DrawingFull>(drawingsKeys.item(id), (old) => old && { ...old, ...meta })
            return qc.invalidateQueries({ queryKey: drawingsKeys.list })
        }
    })
}

/** Delete a drawing (409 while a live match references it). */
export function useDeleteDrawing() {
    const qc = useQueryClient()
    return useMutation({
        mutationFn: (id: string): Promise<void> => drawings.remove(id),
        onSuccess: (_, id) => {
            qc.removeQueries({ queryKey: drawingsKeys.item(id) })
        },
        // Also on failure: a 404 means the card shows a drawing that is already gone.
        onSettled: () => qc.invalidateQueries({ queryKey: drawingsKeys.list })
    })
}

/** Fetch a saved drawing to open in the editor; always fresh, since its next save overwrites the row. */
export function useOpenDrawing() {
    return useMutation({
        mutationFn: (id: string): Promise<DrawingFull> => drawings.get(id)
    })
}

/**
 * The ranked-players ladder (docs/API.md §11) — a cached read, unlike the
 * mutations above. `staleTime` holds it fresh for 30s; a rating change still
 * invalidates it (PlayView) for an immediate refresh. `limit` is fixed per
 * page, so a plain key suffices.
 */
export function useLeaderboard(limit = 20) {
    return useQuery({
        queryKey: leaderboardKeys.list(limit),
        queryFn: (): Promise<LeaderboardPage> => leaderboard.list({ limit }),
        staleTime: 30_000,
        // A 401 can't succeed while unauthenticated — surface it immediately for
        // the sign-in branch instead of burning the default 3 retries on a
        // request that will keep failing.
        retry: (count, err) => !isAuthError(err) && count < 3
    })
}

/**
 * Practice mutations: imperative button actions with no cache to own. The
 * prompt fetch in particular must not be cached — "New prompt" means a
 * different one, which a cached read would refuse to give.
 */

/** Fetch a prompt to draw. */
export function usePracticePrompt() {
    return useMutation({
        mutationFn: (): Promise<PracticePrompt> => practice.prompt()
    })
}

/** Submit a practice drawing and wait on the judge (seconds): the server renders
 *  the raster and calls a vision model in-request. */
export function useSubmitPractice() {
    return useMutation({
        mutationFn: ({ promptId, document }: { promptId: string; document: Document }): Promise<PracticeRun> =>
            practice.run(promptId, document)
    })
}

/**
 * Generate an AI-assist Op batch (docs/ASSIST.md §5). Imperative, no cache to
 * own — the returned ops preview as a ghost and only enter the document on
 * Accept.
 */
export function useAssist() {
    return useMutation({
        mutationFn: (req: AssistOpsRequest): Promise<AssistOpsResponse> => assist.ops(req)
    })
}

/**
 * Ask the AI what the current drawing is. Same shape as `useAssist`: no cache,
 * nothing to invalidate. Slow by nature (raster render + vision model
 * in-request), so the caller shows a pending card, not a frozen button.
 */
export function useGuess() {
    return useMutation({
        mutationFn: (doc: Document): Promise<Guess> => guess.ask(doc)
    })
}
