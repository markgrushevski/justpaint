/**
 * The duel round on the client (docs/API.md §8–§9): feeds the reducer in `duel.ts` from
 * one poll loop, the match socket and the submit, and runs what a state change implies.
 * The view owns the editor, the countdown and what the player sees.
 */
import { onBeforeUnmount, onMounted, ref, shallowRef } from 'vue'
import type { Document } from '@justpaint/editor'
import { matches, toApiError, useAuthGate, useSessionStore } from '@core'
import type { WsFrame } from '@core'
import { failureKind, initialDuel, isTerminal, reduceDuel } from './duel'
import type { DuelEvent, DuelState, Failure } from './duel'
import { useMatchSocket } from './useMatchSocket'

// Without a socket the poll carries the round. With one it only reconciles, and speeds
// up once the verdict is near, since a frame lost in a deploy would otherwise hold the
// result back. The waiting poll is also the queue's pulse: under the server's 45 s.
const POLL_MS = 2000
const LIVE_POLL_MS = 15000
const LIVE_VERDICT_POLL_MS = 4000

// After a failed poll; capped at the last step.
const RETRY_MS = [1000, 2000, 4000, 8000, 15000]

const SIGN_IN_REASON = 'Sign in to play a duel.'

function failureOf(err: unknown): Failure {
    const api = toApiError(err)
    return failureKind(api?.status ?? 0, api?.retryAfter ?? null)
}

export function useDuel() {
    const session = useSessionStore()
    const gate = useAuthGate()

    const state = shallowRef<DuelState>(initialDuel())
    /** The sign-in modal is up: nothing may be sent into the dead session. */
    const signingIn = ref(false)

    let disposed = false
    // Bumped by every reset, so a reply to the previous round is dropped.
    let generation = 0
    // The poll loop: one timer, one request in flight, or neither.
    let timer: number | null = null
    let inFlight = false
    let pollAgain = false
    let failures = 0

    const socket = useMatchSocket({
        onFrame,
        onSessionExpired: () => recoverAuth(),
        onDrop: () => pollNow(),
        shouldReconnect: () => !disposed && !isTerminal(state.value.phase) && state.value.matchId !== null
    })

    function dispatch(event: DuelEvent): void {
        const before = state.value
        const after = reduceDuel(before, event)
        if (after === before) return
        state.value = after
        if (before.matchId === null && after.matchId !== null) socket.open(after.matchId)
        if (isTerminal(after.phase)) {
            stopPolling()
            socket.close()
            return
        }
        if (after.status === 'done' && before.status !== 'done') pollNow()
        else if (after.phase !== before.phase) reschedule()
    }

    function onFrame(frame: WsFrame): void {
        if (disposed) return
        switch (frame.type) {
            case 'match_state':
                dispatch({ type: 'match', match: frame.match })
                break
            case 'opponent_submitted':
                dispatch({ type: 'player-submitted', userId: frame.userId })
                break
            case 'judging':
                dispatch({ type: 'judging' })
                break
            case 'result':
                dispatch({ type: 'result', result: frame.result })
                break
            case 'abandoned':
                dispatch({ type: 'abandoned' })
                break
            case 'opponent_connected':
                dispatch({ type: 'presence', userId: frame.userId, online: true })
                break
            case 'opponent_disconnected':
                dispatch({ type: 'presence', userId: frame.userId, online: false })
                break
            case 'pong':
                break
        }
    }

    function cadence(): number {
        if (!socket.live.value) return POLL_MS
        const { phase } = state.value
        return phase === 'submitted' || phase === 'judging' ? LIVE_VERDICT_POLL_MS : LIVE_POLL_MS
    }

    function schedule(ms: number): void {
        if (timer !== null) clearTimeout(timer)
        timer = window.setTimeout(tick, ms)
    }

    function stopPolling(): void {
        if (timer !== null) clearTimeout(timer)
        timer = null
        pollAgain = false
    }

    /** Polls at once, or right after the request in flight. */
    function pollNow(): void {
        if (disposed || isTerminal(state.value.phase) || signingIn.value) return
        if (inFlight) pollAgain = true
        else schedule(0)
    }

    /** A new phase can change the cadence: the next poll counts from now. */
    function reschedule(): void {
        if (!inFlight && timer !== null) schedule(cadence())
    }

    async function tick(): Promise<void> {
        timer = null
        const s = state.value
        if (disposed || isTerminal(s.phase)) return
        if (inFlight) {
            pollAgain = true
            return
        }
        inFlight = true
        const gen = generation
        try {
            if (s.matchId === null) {
                const m = await matches.create()
                if (gen !== generation) return
                dispatch({ type: 'match', match: m })
            } else if (s.status === 'judging' || s.status === 'done') {
                const r = await matches.result(s.matchId)
                if (gen !== generation) return
                dispatch(r.ready ? { type: 'result', result: r } : { type: 'pending', status: r.status })
            } else {
                const m = await matches.get(s.matchId)
                if (gen !== generation) return
                dispatch({ type: 'match', match: m })
            }
        } catch (err) {
            if (gen !== generation) return
            inFlight = false
            const kind = failureOf(err)
            if (kind === 'auth') {
                recoverAuth()
            } else if (kind === 'transient' || kind === 'conflict') {
                failures += 1
                dispatch({ type: 'offline' })
                schedule(RETRY_MS[Math.min(failures, RETRY_MS.length) - 1] ?? POLL_MS)
            } else {
                dispatch({ type: 'end', end: kind })
            }
            return
        }
        inFlight = false
        failures = 0
        if (isTerminal(state.value.phase)) return
        if (pollAgain) {
            pollAgain = false
            schedule(0)
        } else {
            schedule(cadence())
        }
    }

    /** The one reset: a new round, "Play again", a re-sign-in and unmount all go through it. */
    function reset(): void {
        generation += 1
        stopPolling()
        inFlight = false
        failures = 0
        socket.reset()
    }

    /** Starts a round, or resumes the one still in play: the server hands that one back. */
    function start(): void {
        reset()
        const me = session.user?.id
        if (!me) {
            dispatch({ type: 'end', end: 'auth' })
            return
        }
        dispatch({ type: 'start', me })
        schedule(0)
    }

    /** Asks for a session first; the gate waits for the cookie restore. */
    async function begin(): Promise<void> {
        signingIn.value = true
        const signedIn = await gate.ensure(SIGN_IN_REASON)
        signingIn.value = false
        if (disposed) return
        if (signedIn && session.user) start()
        else {
            reset()
            dispatch({ type: 'end', end: 'auth' })
        }
    }

    /**
     * A 401 from a poll or the submit, or the socket's session-expired close: sign back in
     * and resume. One recovery however many of them notice.
     */
    function recoverAuth(): void {
        if (signingIn.value || disposed) return
        stopPolling()
        begin().catch(() => {
            // ensure() does not reject; a thrown modal leaves the round where it was
        })
    }

    async function submit(doc: Document): Promise<void> {
        const s = state.value
        if (s.phase !== 'drawing' || s.matchId === null || signingIn.value) return
        dispatch({ type: 'submit' })
        const gen = generation
        try {
            const reply = await matches.submit(s.matchId, doc)
            if (gen !== generation) return
            dispatch({ type: 'submit-ok', reply })
        } catch (err) {
            if (gen !== generation) return
            const kind = failureOf(err)
            if (kind === 'conflict') {
                // The round moved on, or another tab sent a drawing: ask the server which.
                dispatch({ type: 'submit-conflict' })
                pollNow()
                return
            }
            if (kind === 'gone') {
                dispatch({ type: 'end', end: 'gone' })
                return
            }
            dispatch({ type: 'submit-failed' })
            if (kind === 'auth') recoverAuth()
        }
    }

    /** Leaving the queue on purpose: the open match is cancelled so nobody is seated against it. */
    function leaveQueue(): void {
        const { phase, matchId } = state.value
        if (phase !== 'waiting' || matchId === null) return
        dispatch({ type: 'end', end: 'left' })
        // A 409 means someone joined first: that round runs on without us.
        matches.cancel(matchId).catch(() => {})
    }

    // A fetch may not leave a page being unloaded; a beacon does.
    function onPageHide(): void {
        const { phase, matchId } = state.value
        if (phase === 'waiting' && matchId !== null) matches.cancelOnUnload(matchId)
    }

    // Back online: no need to sit out the retry backoff.
    function onOnline(): void {
        pollNow()
    }

    onMounted(() => {
        window.addEventListener('pagehide', onPageHide)
        window.addEventListener('online', onOnline)
    })

    onBeforeUnmount(() => {
        window.removeEventListener('pagehide', onPageHide)
        window.removeEventListener('online', onOnline)
        leaveQueue()
        disposed = true
        reset()
    })

    return { state, signingIn, begin, start, submit, leaveQueue }
}
