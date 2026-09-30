import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import type { Document } from '@justpaint/editor'
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
 * TanStack Query bindings for the drawings/matches/practice/leaderboard/assist
 * APIs. TanStack Query owns server data; Pinia owns session/UI state; the
 * editor owns its own document/view state. Save/load and the imperative duel
 * and practice actions are mutations, giving the view standardized
 * `isPending`/`error` and cache invalidation for free; the leaderboard below is
 * the one cached `useQuery` read. The typed fetch clients stay the single
 * source of the request shapes; these only wrap them.
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
            qc.invalidateQueries({ queryKey: drawingsKeys.list })
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
 * Match mutations for the imperative duel actions. The reads that drive the
 * flow — the roster poll (`matches.get`) and verdict poll (`matches.result`) —
 * are called directly from the /play phase machine, an ephemeral flow with no
 * shared cache to own. The live WS push (docs/API.md §9) carries those same
 * transitions, demoting the polling to a reconciliation fallback.
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
