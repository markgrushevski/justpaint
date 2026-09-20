import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import type { Document } from '@justpaint/document'
import { isAuthError } from './http'
import { drawings } from './drawings'
import type { DrawingFull, DrawingMeta } from './drawings'
import { matches } from './matches'
import type { Match, SubmitMatch } from './matches'
import { practice } from './practice'
import type { PracticePrompt, PracticeRun } from './practice'
import { leaderboard } from './leaderboard'
import type { LeaderboardPage } from './leaderboard'
import { assist } from './assist'
import type { AssistOpsRequest, AssistOpsResponse } from './assist'
import { guess } from './guess'
import type { Guess } from './guess'

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
 * Practice mutations. Both are imperative button actions with no cache to own, so
 * they take the store-free mutation shape rather than `useQuery` — and the prompt
 * fetch in particular MUST NOT be cached: "New prompt" means give me a different
 * one, which a cached read would refuse to do.
 */

/** Fetch a prompt to draw. */
export function usePracticePrompt() {
    return useMutation({
        mutationFn: (): Promise<PracticePrompt> => practice.prompt()
    })
}

/** Submit a practice drawing and wait on the judge. Slow by nature (the server
 *  renders the raster and calls a vision model in-request) — the caller shows a
 *  judging state for the several seconds this takes. */
export function useSubmitPractice() {
    return useMutation({
        mutationFn: ({ promptId, document }: { promptId: string; document: Document }): Promise<PracticeRun> =>
            practice.run(promptId, document)
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

/**
 * Ask the AI what the current drawing is. Same store-free mutation shape as
 * `useAssist` — a button action whose answer is read once and thrown away, so
 * there is no cache to own and nothing to invalidate. Slow by nature (the server
 * renders the raster and calls a vision model in-request), so the caller shows a
 * pending card for the several seconds this takes rather than a frozen button.
 */
export function useGuess() {
    return useMutation({
        mutationFn: (doc: Document): Promise<Guess> => guess.ask(doc)
    })
}
