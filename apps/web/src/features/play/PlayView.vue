<script lang="ts" setup>
/**
 * The AI-judged duel (`/play`) on the shared EditorShell, against the async-duel API
 * (docs/API.md §8). A WebSocket (docs/API.md §9) pushes the same transitions and only
 * demotes the poll loop, so the round still runs with the socket down.
 */
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { OriBadge, OriButton, OriSurface } from '@oriui/vue'
import { useQueryClient } from '@tanstack/vue-query'
import { blankDocument, renderToPNG } from '@justpaint/editor'
import {
    useSessionStore,
    useAuthGate,
    useCreateMatch,
    useSubmitMatch,
    matches,
    icons,
    isAuthError,
    isBudgetExhausted,
    toApiError,
    leaderboardKeys
} from '@core'
import type { Match, MatchResultDone, WsFrame } from '@core'
import ConfirmDialog from '../../components/ConfirmDialog.vue'
import ModeNav from '../../components/ModeNav.vue'
import EditorShell from '../editor/EditorShell.vue'
import FloatingToolbar from '../editor/FloatingToolbar.vue'
import ZoomControls from '../editor/ZoomControls.vue'
import { useBackdrop } from '../editor/useBackdrop'
import { useEditorHost } from '../editor/useEditorHost'
import { useLeaveGuard } from '../editor/useLeaveGuard'
import { GAME_CANVAS } from '../game/canvas'
import GamePromptBanner from '../game/GamePromptBanner.vue'
import JudgingOverlay from '../game/JudgingOverlay.vue'
import SubmitButton from '../game/SubmitButton.vue'
import OpponentStatusChip from './OpponentStatusChip.vue'
import type { OpponentStatus } from './OpponentStatusChip.vue'
import ResultReveal from './ResultReveal.vue'
import type { DuelResult } from './ResultReveal.vue'
import RoundTimerBar from './RoundTimerBar.vue'
import { useMatchSocket } from './useMatchSocket'
import { useRoundClock } from './useRoundClock'

// The poll cadence without a socket, demoted while the socket is live (docs/API.md §9.5).
const POLL_MS = 2000
const WS_POLL_MS = 15000

// Early enough to land before the server's deadline: a late submit is a 409 and a forfeit.
const AUTO_SUBMIT_MARGIN_MS = 3000

const session = useSessionStore()
const gate = useAuthGate()
const createMatch = useCreateMatch()
const submitMatch = useSubmitMatch()
const router = useRouter()
const queryClient = useQueryClient()

const blankGameDocument = () => blankDocument(GAME_CANVAS, GAME_CANVAS)

const {
    shell,
    editor,
    ui,
    canUndo,
    canRedo,
    zoomPercent,
    pickTool,
    setColor,
    setWidth,
    toggleFill,
    setFill,
    undo,
    redo,
    zoomIn,
    zoomOut,
    fitView,
    load,
    toPNG
} = useEditorHost({
    initialDocument: blankGameDocument,
    commands: { enter: () => submit() },
    // Tool keys only while drawing.
    beforeToolKeys: () => phase.value !== 'drawing' || leavePending.value !== null
})
useBackdrop(editor, { judged: true })

/**
 * A client view over GAME.md's match states: `connecting` (POST /matches), `waiting`
 * (roster filling, prompt redacted), `drawing`, `submitting` (transient), `judging`
 * (awaiting the opponent and the verdict), `done` and `error`.
 */
type Phase = 'connecting' | 'waiting' | 'drawing' | 'submitting' | 'judging' | 'done' | 'error'
const phase = ref<Phase>('connecting')

// Leaving never cancels the match on the server: the round runs on, and an opponent
// who submits alone wins by forfeit (docs/GAME.md §4.1).
const {
    pending: leavePending,
    leave,
    stay
} = useLeaveGuard(() => {
    if (phase.value === 'drawing')
        return {
            title: 'Leave the duel?',
            message: 'The round keeps running without you. If your opponent submits and you don’t, they win.',
            confirmText: 'Leave'
        }
    if (phase.value === 'waiting')
        return {
            title: 'Leave the queue?',
            message: 'If someone joins, the round starts without you, and they win if they submit.',
            confirmText: 'Leave'
        }
    return null
})

// Set true on unmount; every async continuation checks it before touching state.
let disposed = false

let matchId: string | null = null
let myUserId = ''

// Redacted by the server until `drawing`.
const prompt = ref('')
const REVEAL_PHASES = new Set<Phase>(['drawing', 'submitting', 'judging', 'done'])
const promptRevealed = computed(() => prompt.value !== '' && REVEAL_PHASES.has(phase.value))

// A display label, never a login (docs/GAME.md §4.2).
const opponent = reactive<{ name: string; status: OpponentStatus }>({
    name: 'Player 2',
    status: 'drawing'
})

// Best-effort presence; undefined until the first frame, and never load-bearing.
const opponentOnline = ref<boolean | undefined>(undefined)

const clock = useRoundClock({
    marginMs: AUTO_SUBMIT_MARGIN_MS,
    armed: () => phase.value === 'drawing',
    // Near the server cutoff: auto-submit whatever is on the canvas.
    onDeadline: () => submit()
})
const { deadlineMs, totalSeconds: roundTotalSeconds, remaining } = clock

const socket = useMatchSocket({
    onFrame: handleWsFrame,
    onSessionExpired: () => recoverFromAuthError(),
    shouldReconnect: () => !disposed && !isTerminalPhase() && matchId !== null
})
const wsReconnecting = socket.reconnecting
const pollCadence = computed(() => (socket.live.value ? WS_POLL_MS : POLL_MS))

const submitting = computed(() => phase.value === 'submitting')
const canSubmit = computed(() => phase.value === 'drawing')

const result = ref<DuelResult | null>(null)
const errorMsg = ref('')

// Object URLs for the rasters, revoked on reset and unmount.
let youImageUrl: string | null = null
let opponentImageUrl: string | null = null

// The poll loop's timeouts, cleared on reset and unmount.
const timeouts = new Set<number>()

function later(fn: () => void, ms: number): void {
    const id = window.setTimeout(() => {
        timeouts.delete(id)
        fn()
    }, ms)
    timeouts.add(id)
}

function clearTimers(): void {
    clock.stop()
    for (const id of timeouts) clearTimeout(id)
    timeouts.clear()
}

// A spent daily budget: the card offers the ladder instead of a doomed "Try again".
const exhausted = ref(false)

function toError(msg: string, spent = false): void {
    exhausted.value = spent
    errorMsg.value = msg
    phase.value = 'error'
    clock.stop()
}

// A poll tick and the socket's session-expired close can both notice one dead session;
// this joins them into one recovery.
let recovering = false

// Signing back in restarts with a fresh match; declining leaves the retry card, whose
// "Try again" raises the gate again on the next 401.
async function recoverFromAuthError(): Promise<void> {
    if (recovering) return
    recovering = true
    // Stop the clock first, or the auto-submit fires into the dead session, loops back
    // here and parks the player in `submitting`.
    clock.stop()
    try {
        const signedIn = await gate.ensure('Sign in to play a duel.')
        if (disposed) return
        if (signedIn && session.user) {
            // The visitor may have signed in as another account.
            myUserId = session.user.id
            startMatch()
        } else {
            toError('Sign in to play a duel.')
        }
    } finally {
        recovering = false
    }
}

function handleError(err: unknown): void {
    if (isAuthError(err)) {
        recoverFromAuthError()
        return
    }
    // Not isRateLimited: the per-IP 429 clears in seconds (docs/API.md §3.1).
    toError(toApiError(err)?.message ?? 'Something went wrong. Try again.', isBudgetExhausted(err))
}

// Keeps applyRoster, applyResult and frames monotonic: a slow response or a re-delivered
// verdict cannot regress a terminal UI. startMatch resets it by moving to `connecting`.
function isTerminalPhase(): boolean {
    return phase.value === 'done' || phase.value === 'error'
}

function applyRoster(m: Match): void {
    if (isTerminalPhase()) return
    clock.anchor(m.drawingDeadline, m.serverTime)
    if (m.prompt.text) prompt.value = m.prompt.text
    const opp = m.players.find((p) => p.userId !== myUserId)
    if (opp) {
        opponent.name = opp.displayName ?? 'Player 2'
        opponent.status = m.status === 'judging' ? 'judging' : opp.submitted ? 'submitted' : 'drawing'
    }
}

// Callers run applyRoster first, which anchors the deadline.
function beginDrawing(): void {
    phase.value = 'drawing'
    clock.start()
}

// The round's one poll loop (docs/NOTES.md "/play runs exactly one poll loop");
// `submitting` falls through untouched.
async function pollTick(): Promise<void> {
    if (disposed || matchId === null) return
    if (phase.value === 'done' || phase.value === 'error') return
    try {
        if (phase.value === 'waiting' || phase.value === 'drawing') {
            const m = await matches.get(matchId)
            if (disposed) return
            applyRoster(m)
            if (m.status === 'abandoned') {
                toError('This match was abandoned.')
                return
            }
            if (m.status === 'drawing' && phase.value === 'waiting') beginDrawing()
        } else if (phase.value === 'judging') {
            const r = await matches.result(matchId)
            if (disposed) return
            if (r.ready) {
                applyResult(r)
                return
            }
            if (r.status === 'abandoned') {
                toError('This match was abandoned.')
                return
            }
            opponent.status = r.status === 'judging' ? 'judging' : 'drawing'
        }
    } catch (err) {
        if (disposed) return
        handleError(err)
        return
    }
    scheduleNextPoll()
}

function scheduleNextPoll(): void {
    if (disposed) return
    later(() => pollTick(), pollCadence.value)
}

// Dispatches into the poll loop's own handlers; no second state machine.
function handleWsFrame(frame: WsFrame): void {
    if (disposed || isTerminalPhase()) return
    switch (frame.type) {
        case 'match_state':
            applyRoster(frame.match)
            if (frame.match.status === 'abandoned') {
                toError('This match was abandoned.')
            } else if (frame.match.status === 'drawing' && phase.value === 'waiting') {
                beginDrawing()
            }
            break
        case 'opponent_submitted':
            if (frame.userId !== myUserId) opponent.status = 'submitted'
            break
        case 'judging':
            opponent.status = 'judging'
            break
        case 'result':
            applyResult(frame.result)
            break
        case 'abandoned':
            toError('This match was abandoned.')
            break
        case 'opponent_connected':
            if (frame.userId !== myUserId) opponentOnline.value = true
            break
        case 'opponent_disconnected':
            if (frame.userId !== myUserId) opponentOnline.value = false
            break
        case 'pong':
            break
    }
}

// Advisory only: the judged raster is rendered server-side (docs/GAME.md §6).
async function captureYourRaster(): Promise<string | null> {
    try {
        const blob = await toPNG()
        return blob && URL.createObjectURL(blob)
    } catch {
        return null
    }
}

function revokeYourRaster(): void {
    if (youImageUrl) {
        URL.revokeObjectURL(youImageUrl)
        youImageUrl = null
    }
}

function revokeOpponentRaster(): void {
    if (opponentImageUrl) {
        URL.revokeObjectURL(opponentImageUrl)
        opponentImageUrl = null
    }
}

// Fetches the opponent's document through the membership-gated match route and renders
// it client-side. Any failure keeps the "No preview" placeholder.
async function renderOpponentRaster(id: string, userId: string): Promise<void> {
    try {
        const doc = await matches.playerDrawing(id, userId)
        if (disposed) return
        const blob = await renderToPNG(doc, { outWidth: doc.width, outHeight: doc.height, fit: 'contain' })
        if (disposed) return
        revokeOpponentRaster()
        opponentImageUrl = URL.createObjectURL(blob)
        if (result.value) result.value.opponent.image = opponentImageUrl
    } catch {
        // keep the placeholder
    }
}

async function startMatch(): Promise<void> {
    phase.value = 'connecting'
    errorMsg.value = ''
    prompt.value = ''
    opponent.name = 'Player 2'
    opponent.status = 'drawing'
    opponentOnline.value = undefined
    // "Try again" re-enters here directly too, so reset all socket and clock state.
    socket.reset()
    clock.reset()
    try {
        const m = await createMatch.mutateAsync()
        if (disposed) return
        matchId = m.id
        applyRoster(m)
        // Open now, so a waiting player hears the instant the opponent joins.
        socket.open(matchId)
        if (m.status === 'drawing') beginDrawing()
        else phase.value = 'waiting'
        scheduleNextPoll()
    } catch (err) {
        if (disposed) return
        handleError(err)
    }
}

async function submit(): Promise<void> {
    if (phase.value !== 'drawing' || matchId === null || !editor.value) return
    phase.value = 'submitting'
    clock.stop()
    revokeYourRaster()
    youImageUrl = await captureYourRaster()
    if (disposed || !editor.value) return
    try {
        const res = await submitMatch.mutateAsync({ id: matchId, document: editor.value.getDocument() })
        if (disposed) return
        clock.anchor(res.drawingDeadline, res.serverTime)
        opponent.status = 'judging'
        phase.value = 'judging'
    } catch (err) {
        if (disposed) return
        // A 409 means the round already moved on, so poll the verdict (docs/API.md §8, submit).
        if (toApiError(err)?.status === 409) {
            phase.value = 'judging'
            return
        }
        handleError(err)
    }
}

// Monotonic, so a re-delivered verdict cannot clobber the opponent image patched in later.
function applyResult(r: MatchResultDone): void {
    if (isTerminalPhase()) return
    const me = r.players.find((p) => p.userId === myUserId)
    const opp = r.players.find((p) => p.userId !== myUserId)
    const before = me?.ratingBefore ?? session.user?.rating ?? 1200
    const after = me?.ratingAfter ?? before
    result.value = {
        // Scores are 0..1, the bar 0..100. Both are null when no judge ran, and
        // `resolution` then hides the score row.
        you: { score: (me?.score ?? 0) * 100, image: youImageUrl },
        opponent: {
            name: opp?.displayName ?? opponent.name,
            score: (opp?.score ?? 0) * 100,
            image: null
        },
        // The server's isTie, not a null winner: an aborted round has a null winner too.
        winner: r.isTie
            ? 'tie'
            : r.resolution === 'aborted'
              ? 'none'
              : r.winnerUserId === myUserId
                ? 'you'
                : 'opponent',
        reason: r.reason ?? '',
        resolution: r.resolution,
        eloDelta: after - before,
        ratingBefore: before
    }
    // The session store only refreshes user.rating at login or restore.
    if (session.user && me) session.user.rating = after
    queryClient.invalidateQueries({ queryKey: leaderboardKeys.all })
    phase.value = 'done'
    clock.stop()
    // Off the critical path; skipped for a forfeiter with no drawing.
    if (matchId !== null && opp?.drawingId) renderOpponentRaster(matchId, opp.userId)
}

function playAgain(): void {
    clearTimers()
    socket.close()
    revokeYourRaster()
    revokeOpponentRaster()
    result.value = null
    load(blankGameDocument())
    startMatch()
}

function viewLeaderboard(): void {
    router.push('/leaderboard')
}

onMounted(async () => {
    // `ensure` waits for the cookie restore, then raises the shared sign-in modal.
    const signedIn = await gate.ensure('Sign in to play a duel.')
    if (disposed) return
    if (!signedIn) {
        toError('Sign in to play a duel.')
        return
    }
    if (!session.user) return // gate only resolves true once a session exists
    myUserId = session.user.id
    startMatch()
})

onBeforeUnmount(() => {
    disposed = true
    clearTimers()
    socket.close()
    revokeYourRaster()
    revokeOpponentRaster()
})
</script>

<template>
    <EditorShell ref="shell" mode="play">
        <template #top-left>
            <!-- Two rows, so the opponent never reaches the prompt centered on the first. -->
            <div class="play__top-left">
                <ModeNav :collapse-below="1200" />
                <div class="play__opponent">
                    <!-- Display name or "Player 2", never a login. -->
                    <OpponentStatusChip :name="opponent.name" :status="opponent.status" :online="opponentOnline" />
                    <OriBadge
                        v-if="wsReconnecting"
                        content="reconnecting…"
                        color="warning"
                        variant="soft"
                        label="Reconnecting to the match"
                    />
                </div>
            </div>
        </template>

        <template #top-center>
            <!-- Hidden until the deadline exists, rather than a misleading 0:00. -->
            <RoundTimerBar v-if="deadlineMs !== null" :remaining="remaining" :total="roundTotalSeconds" />
            <div class="play__prompt">
                <GamePromptBanner :prompt="prompt" :revealed="promptRevealed" />
            </div>
        </template>

        <!-- No drawer: SideMenu is /draw-specific; leaving goes through ModeNav.
             TODO(play-api): a play drawer (rematch/profile). -->
        <template #top-right>
            <SubmitButton :disabled="!canSubmit" :loading="submitting" @submit="submit" />
        </template>

        <template #bottom-center>
            <FloatingToolbar
                class="play__toolbar-item"
                :active-tool="ui.activeTool"
                :color="ui.color"
                :stroke-width="ui.strokeWidth"
                :fill-enabled="ui.fillEnabled"
                :fill="ui.fill"
                :can-undo="canUndo"
                :can-redo="canRedo"
                @pick-tool="pickTool"
                @set-color="setColor"
                @set-width="setWidth"
                @toggle-fill="toggleFill"
                @set-fill="setFill"
                @undo="undo"
                @redo="redo"
            />
        </template>

        <template #bottom-right>
            <ZoomControls :percent="zoomPercent" @zoom-in="zoomIn" @zoom-out="zoomOut" @fit="fitView" />
        </template>

        <template #overlay>
            <OriSurface v-if="phase === 'error'" class="play__notice" role="alert">
                <h2 class="play__notice-title">
                    {{ exhausted ? 'That’s your duels for today' : 'Can’t start the duel' }}
                </h2>
                <p class="play__notice-msg">{{ errorMsg }}</p>
                <OriButton
                    v-if="exhausted"
                    label="Leaderboard"
                    variant="outline"
                    radius="md"
                    :icon="icons.podium"
                    icon-position="left"
                    @click="viewLeaderboard"
                />
                <OriButton v-else label="Try again" variant="solid" color="primary" radius="md" @click="startMatch" />
            </OriSurface>
            <JudgingOverlay v-else-if="phase === 'judging' || phase === 'submitting'" :opponent-name="opponent.name" />
            <ResultReveal
                v-else-if="phase === 'done' && result"
                :result="result"
                @play-again="playAgain"
                @view-leaderboard="viewLeaderboard"
            />

            <ConfirmDialog
                :open="leavePending !== null"
                :title="leavePending?.title ?? ''"
                :message="leavePending?.message"
                :confirm-text="leavePending?.confirmText"
                cancel-text="Stay"
                danger
                @confirm="leave"
                @cancel="stay"
            />
        </template>
    </EditorShell>
</template>

<style scoped>
.play__top-left {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: var(--ori-size-gap_sm, 0.25rem);
}

.play__opponent {
    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_sm, 0.25rem);
}

/* Clears the timer chip and the corner islands on a narrow phone. */
.play__prompt {
    padding-top: 2.5rem;
    pointer-events: none;
}

/* The shell's strip is pointer-events:none; the toolbar opts back in. */
.play__toolbar-item {
    pointer-events: auto;
}

/* Opts back into pointer events inside the shell's passive overlay. */
.play__notice {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--ori-size-gap_sm, 0.25rem);

    width: min(92vw, 24rem);
    padding: var(--ori-size-gap_lg, 0.75rem) var(--ori-size-gap_xl, 1rem) var(--ori-size-gap_xl, 1rem);

    pointer-events: auto;
    text-align: center;
}

.play__notice-title {
    margin: 0;

    font-size: var(--ori-font-size_lg, 1.15rem);
    font-weight: 800;
    letter-spacing: -0.01em;
}

.play__notice-msg {
    margin: 0 0 var(--ori-size-gap_sm, 0.25rem);

    font-size: var(--ori-font-size_sm, 0.9rem);
    opacity: 0.8;
}
</style>
