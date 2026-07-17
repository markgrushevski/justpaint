import { request } from './http'

/**
 * Typed client for the ratings leaderboard (docs/API.md §11), on the shared
 * cookie-session `fetch` plumbing (`./http`). Mirrors the drawings/matches house
 * style: the Go DTO is the source of truth for the shape (camelCase, exact;
 * `displayName` nullable), and this module carries only the wire types + the
 * endpoint. Unlike drawings' imperative CRUD this is a cached READ — the query
 * binding lives in `./queries` (`useLeaderboard`), not a mutation. Store-free
 * (no api⇄store cycle).
 */

/* ------------------------------------------------------------------ */
/* Wire types (camelCase, exact from the Go DTO struct).              */
/* ------------------------------------------------------------------ */

/** One ranked player row (docs/API.md §11). `rank` is 1-based, server-assigned. */
export interface LeaderboardEntry {
    rank: number
    userId: string
    displayName: string | null
    rating: number
    gamesPlayed: number
    wins: number
    losses: number
}

/** A page of the ladder — the entries plus the effective `limit` the server
 *  applied (may be smaller than requested if capped). */
export interface LeaderboardPage {
    leaderboard: LeaderboardEntry[]
    limit: number
}

/** Raw `GET /api/leaderboard` body. Structurally identical to the public
 *  {@link LeaderboardPage} today; kept as a private wire type so this module owns
 *  the single seam to adapt if the server ever wraps the page in an envelope
 *  (mirrors the `*Envelope` types in `./drawings`). */
interface LeaderboardResponse {
    leaderboard: LeaderboardEntry[]
    limit: number
}

export const leaderboard = {
    /** Fetch the top ranked players by rating (`?limit`, default server-side). */
    async list(opts: { limit?: number } = {}): Promise<LeaderboardPage> {
        return await request<LeaderboardResponse>('/leaderboard', { query: { limit: opts.limit } })
    }
}
