import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import type { Document } from '@justpaint/document'
import { isAuthError } from './http'
import { drawings } from './drawings'
import type { DrawingFull, DrawingMeta } from './drawings'
import { matches } from './matches'
import type { Match, SubmitMatch } from './matches'
import { leaderboard } from './leaderboard'
import type { LeaderboardPage } from './leaderboard'
import { assist } from './assist'
import type { AssistOpsRequest, AssistOpsResponse } from './assist'

/**
 * TanStack Query bindings for the drawings API (ROADMAP Phase 2 "state" pass:
 * TanStack Query owns server data; Pinia owns session/UI state; the editor owns
 * its own document/view state). Save + load are modelled as mutations (they're
 * imperative button actions), giving the view standardized `isPending`/`error`
 * and cache invalidation for free. The typed fetch client (`./drawings`) stays
 * the single source of the request shapes; these only wrap it.
 *
 * The leaderboard is the FIRST genuine `useQuery` here (everything above is a
 * mutation): it's a cached READ the UI displays, not an imperative action a
 * button fires — so it wants Query's fetch-on-mount, dedupe, background refresh,
 * and `staleTime` window, none of which a mutation models. The duel result then
 * `invalidateQueries({ queryKey: leaderboardKeys.all })` so the ladder re-fetches
 * once a rating moves (see `useLeaderboard`).
 */

/** Query keys for the drawings cache (a future saved-drawings list reads these). */
export const drawingsKeys = {
    all: ['drawings'] as const,
    list: ['drawings', 'list'] as const
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
 * The ranked-players ladder (docs/API.md §11) — a cached read the leaderboard
 * page renders. `staleTime` holds the page fresh for 30s so navigating back to it
 * doesn't re-fetch on every visit, while a rating change still invalidates it
 * (PlayView, on the duel result) to force an immediate refresh. `limit` is fixed
 * for a page's lifetime, so a plain key is enough (no reactive key needed).
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

/**
 * Generate an AI-assist Op batch from a prompt (docs/ASSIST.md §5). Imperative
 * (a button action) with no cache to own — the returned ops are previewed as a
 * ghost and only enter the document on Accept — so it mirrors the store-free
 * `useLoadLatestDrawing` shape: a thin mutation over the fetch client, no
 * invalidation.
 */
export function useAssist() {
    return useMutation({
        mutationFn: (req: AssistOpsRequest): Promise<AssistOpsResponse> => assist.ops(req)
    })
}
