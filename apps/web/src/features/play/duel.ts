/**
 * The duel round as a pure reducer over what the client learns: match snapshots (the
 * create and poll replies, `match_state` frames), results, the other frames and the
 * submit reply. `useDuel` feeds it; nothing here touches the network, the DOM or Vue.
 */
import type { Match, MatchResultDone, MatchStatus, SubmitMatch } from '@core'

/**
 * `waiting`: nobody is seated yet and the prompt is hidden. `submitted`: my drawing is
 * in and the opponent still draws. `ended`: over without a verdict, `end` says why.
 */
export type DuelPhase = 'connecting' | 'waiting' | 'drawing' | 'submitting' | 'submitted' | 'judging' | 'done' | 'ended'

/** Why a round ended without a verdict. */
export type DuelEnd =
    /** Nobody joined in time, or the queue was given up. */
    | 'queue-ended'
    /** The round ran out with no drawing in. */
    | 'nobody-submitted'
    /** The player left the queue. */
    | 'left'
    /** Signed out, and did not sign back in. */
    | 'auth'
    /** No duels left today. */
    | 'budget'
    /** The match is not there for this player (404). */
    | 'gone'
    /** A refusal a retry cannot fix. */
    | 'failed'

export interface DuelOpponent {
    userId: string
    /** A display name, never a login; null when they have none. */
    name: string | null
    submitted: boolean
}

export interface DuelState {
    phase: DuelPhase
    end: DuelEnd | null
    /** The signed-in player. */
    me: string
    matchId: string | null
    /** The furthest status the server has reported. */
    status: MatchStatus | null
    /** A deadline exists: tells an empty queue from a round nobody finished. */
    started: boolean
    /** Empty until the server reveals it. */
    prompt: string
    opponent: DuelOpponent | null
    /** Socket presence; undefined until a frame says. */
    opponentOnline: boolean | undefined
    /** The last deadline and server time heard, for the countdown. */
    clock: { drawingDeadline: string | null; serverTime: string } | null
    result: MatchResultDone | null
    /** The last submit never reached the server. */
    submitFailed: boolean
    /** Polls are failing; the round is kept and retried. */
    offline: boolean
}

export type DuelEvent =
    | { type: 'start'; me: string }
    | { type: 'match'; match: Match }
    | { type: 'result'; result: MatchResultDone }
    /** A result poll that is not ready yet. */
    | { type: 'pending'; status: MatchStatus }
    | { type: 'player-submitted'; userId: string }
    | { type: 'judging' }
    | { type: 'abandoned' }
    | { type: 'presence'; userId: string; online: boolean }
    | { type: 'submit' }
    | { type: 'submit-ok'; reply: SubmitMatch }
    /** A 409: the round moved on without this submit, or another tab sent one. */
    | { type: 'submit-conflict' }
    | { type: 'submit-failed' }
    | { type: 'offline' }
    | { type: 'end'; end: DuelEnd }

export function initialDuel(me = ''): DuelState {
    return {
        phase: 'connecting',
        end: null,
        me,
        matchId: null,
        status: null,
        started: false,
        prompt: '',
        opponent: null,
        opponentOnline: undefined,
        clock: null,
        result: null,
        submitFailed: false,
        offline: false
    }
}

export function isTerminal(phase: DuelPhase): boolean {
    return phase === 'done' || phase === 'ended'
}

// A snapshot may still move these into `waiting` or `drawing`; later phases never go back.
const BEFORE_ROUND = new Set<DuelPhase>(['connecting', 'waiting'])

const STATUS_ORDER: Record<MatchStatus, number> = { open: 0, drawing: 1, judging: 2, done: 3, abandoned: 3 }

/** A late reply must not walk the status back. */
function furthest(prev: MatchStatus | null, next: MatchStatus): MatchStatus {
    return prev !== null && STATUS_ORDER[prev] > STATUS_ORDER[next] ? prev : next
}

function abandon(s: DuelState): DuelState {
    return { ...s, phase: 'ended', end: s.started ? 'nobody-submitted' : 'queue-ended', status: 'abandoned' }
}

function withOpponentSubmitted(s: DuelState): DuelState {
    return s.opponent ? { ...s, opponent: { ...s.opponent, submitted: true } } : s
}

function applyMatch(s: DuelState, m: Match): DuelState {
    if (s.matchId !== null && m.id !== s.matchId) return s
    const mine = m.players.find((p) => p.userId === s.me)
    const opp = m.players.find((p) => p.userId !== s.me)
    const next: DuelState = {
        ...s,
        matchId: m.id,
        status: furthest(s.status, m.status),
        started: s.started || m.drawingDeadline !== null,
        prompt: m.prompt.text ?? s.prompt,
        opponent: opp
            ? {
                  userId: opp.userId,
                  name: opp.displayName,
                  submitted: opp.submitted || (s.opponent?.userId === opp.userId && s.opponent.submitted)
              }
            : s.opponent,
        clock: { drawingDeadline: m.drawingDeadline, serverTime: m.serverTime },
        offline: false
    }
    switch (m.status) {
        case 'open':
            return BEFORE_ROUND.has(s.phase) ? { ...next, phase: 'waiting' } : next
        case 'drawing':
            // Another tab may have sent my drawing: the server is the one to ask.
            if (mine?.submitted)
                return s.phase === 'judging' ? next : { ...next, phase: 'submitted', submitFailed: false }
            return BEFORE_ROUND.has(s.phase) ? { ...next, phase: 'drawing' } : next
        case 'judging':
            return withOpponentSubmitted({ ...next, phase: 'judging' })
        case 'done':
            // The phase holds until the result is fetched.
            return next
        case 'abandoned':
            return abandon(next)
    }
}

/**
 * Terminal phases never change, so a late reply or a re-delivered frame cannot undo a
 * shown result; a submit reply counts only while that submit is still the news.
 */
export function reduceDuel(s: DuelState, e: DuelEvent): DuelState {
    if (e.type === 'start') return initialDuel(e.me)
    if (isTerminal(s.phase)) return s
    switch (e.type) {
        case 'match':
            return applyMatch(s, e.match)
        case 'result':
            return { ...s, phase: 'done', status: 'done', started: true, result: e.result, offline: false }
        case 'pending':
            if (e.status === 'abandoned') return abandon(s)
            if (e.status === 'judging')
                return withOpponentSubmitted({ ...s, phase: 'judging', status: 'judging', offline: false })
            return { ...s, status: furthest(s.status, e.status), offline: false }
        case 'player-submitted':
            if (e.userId !== s.me) return withOpponentSubmitted(s)
            return s.phase === 'drawing' || s.phase === 'submitting'
                ? { ...s, phase: 'submitted', submitFailed: false }
                : s
        case 'judging':
            return withOpponentSubmitted({ ...s, phase: 'judging', status: 'judging' })
        case 'abandoned':
            return abandon(s)
        case 'presence':
            return e.userId === s.me ? s : { ...s, opponentOnline: e.online }
        case 'submit':
            return s.phase === 'drawing' ? { ...s, phase: 'submitting', submitFailed: false } : s
        case 'submit-ok': {
            if (s.phase !== 'submitting') return s
            const clock = { drawingDeadline: e.reply.drawingDeadline, serverTime: e.reply.serverTime }
            const status = furthest(s.status, e.reply.status)
            const next = { ...s, status, clock, offline: false }
            return status === 'judging'
                ? withOpponentSubmitted({ ...next, phase: 'judging' })
                : { ...next, phase: 'submitted' }
        }
        case 'submit-conflict':
            return s.phase === 'submitting' ? { ...s, phase: 'submitted' } : s
        case 'submit-failed':
            return s.phase === 'submitting' ? { ...s, phase: 'drawing', submitFailed: true } : s
        case 'offline':
            return s.offline ? s : { ...s, offline: true }
        case 'end':
            return { ...s, phase: 'ended', end: e.end }
    }
}

/** What a failed request means for the round. */
export type Failure = 'auth' | 'transient' | 'conflict' | DuelEnd

/**
 * Network errors (status 0), 5xx and the per-IP 429 pass: the round is kept and retried.
 * The daily budget's 429 is the one without `Retry-After` (docs/API.md §3.1).
 */
export function failureKind(status: number, retryAfter: number | null): Failure {
    if (status === 401) return 'auth'
    if (status === 429) return retryAfter === null ? 'budget' : 'transient'
    if (status === 0 || status >= 500) return 'transient'
    if (status === 404) return 'gone'
    if (status === 409) return 'conflict'
    return 'failed'
}

export type OpponentStatus = 'drawing' | 'submitted' | 'offline'

/** The chip's line; null while nobody is seated and once the round is over. */
export function opponentChip(s: DuelState): { name: string; status: OpponentStatus } | null {
    if (!s.opponent || isTerminal(s.phase)) return null
    const name = s.opponent.name ?? 'Player 2'
    if (s.opponent.submitted || s.phase === 'judging') return { name, status: 'submitted' }
    if (s.opponentOnline === false) return { name, status: 'offline' }
    return { name, status: 'drawing' }
}

/** The judge's reason names the drawings A and B, the letters the result cards carry. */
export function readableReason(reason: string | null): string {
    if (!reason) return ''
    return reason.charAt(0).toUpperCase() + reason.slice(1)
}

export interface DuelSide {
    userId: string | null
    name: string
    /** 0..100; null when no judge ran. */
    score: number | null
    /** The judge's name for this drawing on a judged round: A was submitted first. */
    letter: 'A' | 'B' | null
    won: boolean
    /** A drawing was submitted, so there is one to show. */
    drew: boolean
}

/** Everything the result card shows, worded for the viewer. */
export interface DuelResult {
    headline: string
    /** Empty when there is nothing to add to the headline. */
    reason: string
    you: DuelSide
    opponent: DuelSide
    /** A judge ran, so the scores mean something. */
    scored: boolean
    /** Null when the rating did not move because nothing was decided. */
    rating: { before: number; after: number } | null
}

/**
 * The result DTO lists players in the judge's A/B order (docs/GAME.md §7.1), while the
 * card always shows "You" first, so each side carries its letter.
 */
export function toDuelResult(r: MatchResultDone, me: string, fallbackRating: number): DuelResult {
    const mine = r.players.find((p) => p.userId === me)
    const opp = r.players.find((p) => p.userId !== me)
    const scored = r.resolution === 'judged'
    const oppName = opp?.displayName ?? 'Player 2'
    const youWon = !r.isTie && r.resolution !== 'aborted' && r.winnerUserId === me
    const oppWon = !r.isTie && r.resolution !== 'aborted' && r.winnerUserId !== null && r.winnerUserId !== me

    const letterOf = (userId: string | undefined): 'A' | 'B' | null => {
        if (!scored || userId === undefined) return null
        return r.players.findIndex((p) => p.userId === userId) === 0 ? 'A' : 'B'
    }
    const scoreOf = (score: number | null | undefined): number | null =>
        scored && score !== null && score !== undefined ? score * 100 : null

    let headline: string
    let reason: string
    if (r.resolution === 'aborted') {
        headline = 'Round couldn’t be scored'
        reason = 'The judge didn’t answer, so nobody wins and ratings stay the same.'
    } else if (r.resolution === 'forfeit') {
        headline = youWon ? `You win — ${oppName} didn’t submit in time` : 'You didn’t submit in time'
        reason = ''
    } else {
        headline = r.isTie ? 'It’s a tie' : youWon ? 'You win!' : 'You lose'
        reason = readableReason(r.reason)
    }

    const before = mine?.ratingBefore ?? fallbackRating
    const after = mine?.ratingAfter ?? before
    return {
        headline,
        reason,
        you: {
            userId: me,
            name: 'You',
            score: scoreOf(mine?.score),
            letter: letterOf(mine?.userId),
            won: youWon,
            drew: mine?.drawingId != null
        },
        opponent: {
            userId: opp?.userId ?? null,
            name: oppName,
            score: scoreOf(opp?.score),
            letter: letterOf(opp?.userId),
            won: oppWon,
            drew: opp?.drawingId != null
        },
        scored,
        rating: r.resolution === 'aborted' ? null : { before, after }
    }
}
