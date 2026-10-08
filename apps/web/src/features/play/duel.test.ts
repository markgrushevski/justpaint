import { describe, expect, it } from 'vitest'
import type { Match, MatchPlayer, MatchResultDone, MatchStatus, ResultPlayer, SubmitMatch } from '@core'
import { failureKind, initialDuel, opponentChip, readableReason, reduceDuel, toDuelResult } from './duel'
import type { DuelEvent, DuelState } from './duel'

const ME = 'me'
const BO = 'bo'
const DEADLINE = '2026-10-08T12:01:30.000Z'
const NOW = '2026-10-08T12:00:00.000Z'

function player(userId: string, submitted = false, displayName: string | null = null): MatchPlayer {
    return { userId, displayName, submitted }
}

function match(status: MatchStatus, players: MatchPlayer[], id = 'm1'): Match {
    const open = status === 'open'
    return {
        id,
        mode: 'async',
        status,
        prompt: { id: open ? null : 'p1', text: open ? null : 'a fox' },
        canvas: { width: 1080, height: 1080 },
        players,
        drawingDeadline: open ? null : DEADLINE,
        serverTime: NOW,
        createdAt: NOW,
        updatedAt: NOW
    }
}

const openMatch = (): Match => match('open', [player(ME)])
const drawing = (meIn = false, boIn = false): Match => match('drawing', [player(ME, meIn), player(BO, boIn, 'Bo')])

function resultPlayer(userId: string, over: Partial<ResultPlayer> = {}): ResultPlayer {
    return {
        userId,
        displayName: userId === BO ? 'Bo' : 'Me',
        drawingId: 'd-' + userId,
        score: 0.5,
        ratingBefore: 1200,
        ratingAfter: 1200,
        judgedImageUrl: null,
        ...over
    }
}

function result(over: Partial<MatchResultDone> = {}): MatchResultDone {
    return {
        status: 'done',
        ready: true,
        prompt: { id: 'p1', text: 'a fox' },
        winnerUserId: ME,
        isTie: false,
        reason: 'A covers 30% of the canvas vs B’s 10% — A wins on ink coverage (fake judge).',
        resolution: 'judged',
        players: [
            resultPlayer(ME, { score: 0.3, ratingAfter: 1216 }),
            resultPlayer(BO, { score: 0.1, ratingAfter: 1184 })
        ],
        ...over
    }
}

function reply(status: MatchStatus): SubmitMatch {
    return { id: 'm1', status, you: { submitted: true, drawingId: 'd1' }, drawingDeadline: DEADLINE, serverTime: NOW }
}

function run(...events: DuelEvent[]): DuelState {
    return events.reduce(reduceDuel, reduceDuel(initialDuel(), { type: 'start', me: ME }))
}

describe('reduceDuel: into the round', () => {
    it('starts connecting with nothing known', () => {
        const s = run()
        expect(s.phase).toBe('connecting')
        expect(s.me).toBe(ME)
        expect(s.matchId).toBeNull()
    })

    it('a lone creator waits with the prompt hidden and no opponent', () => {
        const s = run({ type: 'match', match: openMatch() })
        expect(s.phase).toBe('waiting')
        expect(s.matchId).toBe('m1')
        expect(s.prompt).toBe('')
        expect(s.opponent).toBeNull()
        expect(s.clock).toEqual({ drawingDeadline: null, serverTime: NOW })
    })

    it('a joiner draws at once, with the prompt and the deadline', () => {
        const s = run({ type: 'match', match: drawing() })
        expect(s.phase).toBe('drawing')
        expect(s.prompt).toBe('a fox')
        expect(s.opponent).toEqual({ userId: BO, name: 'Bo', submitted: false })
        expect(s.clock?.drawingDeadline).toBe(DEADLINE)
    })

    it('the waiting player moves to drawing when the roster fills', () => {
        const s = run({ type: 'match', match: openMatch() }, { type: 'match', match: drawing() })
        expect(s.phase).toBe('drawing')
    })

    it('a resume into a round where my drawing is in is `submitted`', () => {
        expect(run({ type: 'match', match: drawing(true) }).phase).toBe('submitted')
    })

    it('a snapshot for another match is ignored', () => {
        const s = run({ type: 'match', match: openMatch() }, { type: 'match', match: match('drawing', [], 'm2') })
        expect(s.phase).toBe('waiting')
        expect(s.matchId).toBe('m1')
    })
})

describe('reduceDuel: a waiting player who missed the round moves forward', () => {
    it('to judging', () => {
        const s = run({ type: 'match', match: openMatch() }, { type: 'match', match: match('judging', []) })
        expect(s.phase).toBe('judging')
    })

    it('to the result: `done` keeps the phase and marks the result to fetch', () => {
        const waited = run({ type: 'match', match: openMatch() }, { type: 'match', match: match('done', []) })
        expect(waited.phase).toBe('waiting')
        expect(waited.status).toBe('done')
        const s = reduceDuel(waited, { type: 'result', result: result({ resolution: 'forfeit', winnerUserId: BO }) })
        expect(s.phase).toBe('done')
    })

    it('a `judging` result poll from drawing moves to judging', () => {
        const s = run({ type: 'match', match: drawing() }, { type: 'pending', status: 'judging' })
        expect(s.phase).toBe('judging')
        expect(s.opponent?.submitted).toBe(true)
    })
})

describe('reduceDuel: submitting', () => {
    const drawn = (): DuelState => run({ type: 'match', match: drawing() })

    it('opponent still drawing → submitted, with the clock re-anchored', () => {
        const s = [{ type: 'submit' } as const, { type: 'submit-ok', reply: reply('drawing') } as const].reduce(
            reduceDuel,
            drawn()
        )
        expect(s.phase).toBe('submitted')
        expect(s.clock?.drawingDeadline).toBe(DEADLINE)
    })

    it('the final submit → judging', () => {
        const s = [{ type: 'submit' } as const, { type: 'submit-ok', reply: reply('judging') } as const].reduce(
            reduceDuel,
            drawn()
        )
        expect(s.phase).toBe('judging')
    })

    it('a failed submit goes back to drawing with the inline error, and a retry clears it', () => {
        let s = reduceDuel(reduceDuel(drawn(), { type: 'submit' }), { type: 'submit-failed' })
        expect(s.phase).toBe('drawing')
        expect(s.submitFailed).toBe(true)
        s = reduceDuel(s, { type: 'submit' })
        expect(s.phase).toBe('submitting')
        expect(s.submitFailed).toBe(false)
    })

    it('a 409 waits for the server to say what happened', () => {
        const s = reduceDuel(reduceDuel(drawn(), { type: 'submit' }), { type: 'submit-conflict' })
        expect(s.phase).toBe('submitted')
    })

    it('only from drawing', () => {
        const waiting = run({ type: 'match', match: openMatch() })
        expect(reduceDuel(waiting, { type: 'submit' }).phase).toBe('waiting')
    })

    it('a late 409 does not replace an abandoned round with "judging"', () => {
        const s = [{ type: 'submit' }, { type: 'abandoned' }, { type: 'submit-conflict' }].reduce(
            (acc, e) => reduceDuel(acc, e as DuelEvent),
            drawn()
        )
        expect(s.phase).toBe('ended')
        expect(s.end).toBe('nobody-submitted')
    })

    it('a slow 202 does not replace a result already shown', () => {
        const s = [
            { type: 'submit' } as DuelEvent,
            { type: 'result', result: result() } as DuelEvent,
            { type: 'submit-ok', reply: reply('judging') } as DuelEvent
        ].reduce(reduceDuel, drawn())
        expect(s.phase).toBe('done')
    })

    it('a reply after a frame already moved on is dropped', () => {
        const s = [
            { type: 'submit' } as DuelEvent,
            { type: 'judging' } as DuelEvent,
            { type: 'submit-ok', reply: reply('drawing') } as DuelEvent
        ].reduce(reduceDuel, drawn())
        expect(s.phase).toBe('judging')
    })
})

describe('reduceDuel: two tabs of one player', () => {
    it('my own `opponent_submitted` means my drawing is in', () => {
        const s = reduceDuel(run({ type: 'match', match: drawing() }), { type: 'player-submitted', userId: ME })
        expect(s.phase).toBe('submitted')
        expect(s.opponent?.submitted).toBe(false)
    })

    it('the opponent’s marks them submitted and keeps me drawing', () => {
        const s = reduceDuel(run({ type: 'match', match: drawing() }), { type: 'player-submitted', userId: BO })
        expect(s.phase).toBe('drawing')
        expect(s.opponent?.submitted).toBe(true)
    })

    it('a stale snapshot does not undo either submit', () => {
        const s = [
            { type: 'player-submitted', userId: ME } as DuelEvent,
            { type: 'player-submitted', userId: BO } as DuelEvent,
            { type: 'match', match: drawing(false, false) } as DuelEvent
        ].reduce(reduceDuel, run({ type: 'match', match: drawing() }))
        expect(s.phase).toBe('submitted')
        expect(s.opponent?.submitted).toBe(true)
    })
})

describe('reduceDuel: endings', () => {
    it('abandoned before the round: the queue ended', () => {
        const s = run({ type: 'match', match: openMatch() }, { type: 'abandoned' })
        expect(s.phase).toBe('ended')
        expect(s.end).toBe('queue-ended')
    })

    it('an abandoned snapshot of an open match is the queue ending too', () => {
        const reaped = { ...openMatch(), status: 'abandoned' as const }
        expect(run({ type: 'match', match: openMatch() }, { type: 'match', match: reaped }).end).toBe('queue-ended')
    })

    it('abandoned after it started: nobody submitted', () => {
        const s = run({ type: 'match', match: drawing() }, { type: 'pending', status: 'abandoned' })
        expect(s.end).toBe('nobody-submitted')
    })

    it('terminal phases never change', () => {
        const done = run({ type: 'match', match: drawing() }, { type: 'result', result: result() })
        for (const e of [
            { type: 'match', match: drawing() },
            { type: 'abandoned' },
            { type: 'judging' },
            { type: 'offline' },
            { type: 'end', end: 'gone' }
        ] as DuelEvent[]) {
            expect(reduceDuel(done, e)).toBe(done)
        }
        const ended = run({ type: 'end', end: 'budget' })
        expect(reduceDuel(ended, { type: 'result', result: result() })).toBe(ended)
    })

    it('start resets from any phase', () => {
        const s = reduceDuel(run({ type: 'end', end: 'gone' }), { type: 'start', me: ME })
        expect(s).toEqual({ ...initialDuel(ME) })
    })
})

describe('reduceDuel: blips and presence', () => {
    it('a failed poll keeps the phase and flags it; the next answer clears it', () => {
        let s = reduceDuel(run({ type: 'match', match: drawing() }), { type: 'offline' })
        expect(s.phase).toBe('drawing')
        expect(s.offline).toBe(true)
        s = reduceDuel(s, { type: 'match', match: drawing() })
        expect(s.offline).toBe(false)
    })

    it('presence is the opponent’s only', () => {
        const s = run({ type: 'match', match: drawing() })
        expect(reduceDuel(s, { type: 'presence', userId: ME, online: false })).toBe(s)
        expect(reduceDuel(s, { type: 'presence', userId: BO, online: false }).opponentOnline).toBe(false)
    })

    it('the status never walks back', () => {
        const s = run({ type: 'match', match: drawing() }, { type: 'judging' }, { type: 'match', match: drawing() })
        expect(s.status).toBe('judging')
        expect(s.phase).toBe('judging')
    })
})

describe('opponentChip', () => {
    it('no chip while nobody is seated, or once the round is over', () => {
        expect(opponentChip(run({ type: 'match', match: openMatch() }))).toBeNull()
        expect(opponentChip(run({ type: 'match', match: drawing() }, { type: 'result', result: result() }))).toBeNull()
    })

    it('drawing, offline in words, submitted; never "judging" while they draw', () => {
        const s = run({ type: 'match', match: drawing() })
        expect(opponentChip(s)).toEqual({ name: 'Bo', status: 'drawing' })
        expect(opponentChip(reduceDuel(s, { type: 'presence', userId: BO, online: false }))?.status).toBe('offline')
        expect(opponentChip(reduceDuel(s, { type: 'player-submitted', userId: BO }))?.status).toBe('submitted')
        const mineIn = [{ type: 'submit' } as const, { type: 'submit-ok', reply: reply('drawing') } as const].reduce(
            reduceDuel,
            s
        )
        expect(opponentChip(mineIn)?.status).toBe('drawing')
    })

    it('a seat without a display name reads "Player 2"', () => {
        const s = run({ type: 'match', match: match('drawing', [player(ME), player(BO)]) })
        expect(opponentChip(s)?.name).toBe('Player 2')
    })
})

describe('failureKind', () => {
    it.each([
        [0, null, 'transient'],
        [500, null, 'transient'],
        [503, null, 'transient'],
        [429, 2, 'transient'],
        [429, null, 'budget'],
        [401, null, 'auth'],
        [404, null, 'gone'],
        [409, null, 'conflict'],
        [400, null, 'failed'],
        [403, null, 'failed']
    ] as const)('status %i, Retry-After %s → %s', (status, retryAfter, kind) => {
        expect(failureKind(status, retryAfter)).toBe(kind)
    })
})

describe('readableReason', () => {
    it('starts with a capital and leaves an A/B reason alone', () => {
        expect(readableReason('b covers more')).toBe('B covers more')
        expect(readableReason('A wins on ink coverage')).toBe('A wins on ink coverage')
        expect(readableReason(null)).toBe('')
    })
})

describe('toDuelResult', () => {
    it('a judged win for the first submitter: A is mine', () => {
        const r = toDuelResult(result(), ME, 1200)
        expect(r.headline).toBe('You win!')
        expect(r.you).toMatchObject({ name: 'You', letter: 'A', won: true, drew: true })
        expect(r.you.score).toBeCloseTo(30)
        expect(r.opponent).toMatchObject({ name: 'Bo', letter: 'B', won: false })
        expect(r.reason).toMatch(/^A covers/)
        expect(r.rating).toEqual({ before: 1200, after: 1216 })
        expect(r.scored).toBe(true)
    })

    it('the same round seen by the second submitter: "You" is still first, lettered B', () => {
        const r = toDuelResult(result(), BO, 1200)
        expect(r.headline).toBe('You lose')
        expect(r.you).toMatchObject({ letter: 'B', won: false })
        expect(r.opponent).toMatchObject({ name: 'Me', letter: 'A', won: true })
        expect(r.rating).toEqual({ before: 1200, after: 1184 })
    })

    it('a tie', () => {
        const r = toDuelResult(result({ isTie: true, winnerUserId: null }), ME, 1200)
        expect(r.headline).toBe('It’s a tie')
        expect(r.you.won || r.opponent.won).toBe(false)
    })

    it('a forfeit is worded for each side, without the server’s reason', () => {
        const forfeit = result({
            resolution: 'forfeit',
            winnerUserId: ME,
            reason: 'opponent did not submit before the deadline',
            players: [
                resultPlayer(ME, { score: null, ratingAfter: 1216 }),
                resultPlayer(BO, { score: null, drawingId: null, ratingAfter: 1184 })
            ]
        })
        const won = toDuelResult(forfeit, ME, 1200)
        expect(won.headline).toBe('You win — Bo didn’t submit in time')
        expect(won.reason).toBe('')
        expect(won.scored).toBe(false)
        expect(won.you.letter).toBeNull()
        expect(won.opponent.drew).toBe(false)
        const lost = toDuelResult(forfeit, BO, 1200)
        expect(lost.headline).toBe('You didn’t submit in time')
        expect(lost.reason).toBe('')
        expect(lost.you.drew).toBe(false)
        expect(lost.rating).toEqual({ before: 1200, after: 1184 })
    })

    it('an aborted round: plain words, no winner, no rating', () => {
        const r = toDuelResult(
            result({
                resolution: 'aborted',
                winnerUserId: null,
                reason: 'this round could not be scored — the judge did not answer',
                players: [
                    resultPlayer(ME, { score: null, ratingBefore: null, ratingAfter: null }),
                    resultPlayer(BO, { score: null, ratingBefore: null, ratingAfter: null })
                ]
            }),
            ME,
            1234
        )
        expect(r.headline).toBe('Round couldn’t be scored')
        expect(r.reason).toBe('The judge didn’t answer, so nobody wins and ratings stay the same.')
        expect(r.rating).toBeNull()
        expect(r.you.won || r.opponent.won).toBe(false)
    })
})
