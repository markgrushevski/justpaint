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
import { DEFAULT_STYLE, DOC_VERSION, Editor, newId, renderToPNG, TOOLS } from '@justpaint/editor'
import type { Document, ToolId } from '@justpaint/editor'
import { useThemeColor } from '@oriui/headless/vue'
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
    openMatchSocket,
    leaderboardKeys
} from '@core'
import type { Match, MatchResultDone, WsFrame, MatchSocketHandle } from '@core'
import EditorShell from '../editor/EditorShell.vue'
import FloatingToolbar, { TOOL_META } from '../editor/FloatingToolbar.vue'
import IconButton from '../../components/ui/IconButton.vue'
import RoundTimerBar from './RoundTimerBar.vue'
import GamePromptBanner from '../game/GamePromptBanner.vue'
import OpponentStatusChip from './OpponentStatusChip.vue'
import type { OpponentStatus } from './OpponentStatusChip.vue'
import SubmitButton from '../game/SubmitButton.vue'
import JudgingOverlay from '../game/JudgingOverlay.vue'
import ResultReveal from './ResultReveal.vue'
import type { DuelResult } from './ResultReveal.vue'

/** The square duel canvas (GAME.md §2). */
const GAME_CANVAS = 1080

// The poll cadence without a socket; pollCadence demotes it to WS_POLL_MS while the
// socket is live (docs/API.md §9.5).
const POLL_MS = 2000
const WS_POLL_MS = 15000

// Notices a half-open socket without a TCP timeout; the server's WS_READ_IDLE_TIMEOUT
// must clear it.
const WS_PING_MS = 25000

// Capped at the last entry; resets on a clean reconnect.
const WS_RECONNECT_BACKOFF_MS = [1000, 2000, 4000, 10000]

// Early enough to land before the server's deadline: a late submit is a 409 and a forfeit.
const AUTO_SUBMIT_MARGIN_MS = 3000

const shell = ref<{ canvasEl: HTMLDivElement | null } | null>(null)
let editor: Editor | null = null
let unsubscribe: (() => void) | null = null

// Konva can't read CSS variables (same wiring as DrawView).
const cursorRingColor = useThemeColor('primary')

const session = useSessionStore()
const gate = useAuthGate()
const createMatch = useCreateMatch()
const submitMatch = useSubmitMatch()
const router = useRouter()
const queryClient = useQueryClient()

function blankGameDocument(): Document {
    return {
        version: DOC_VERSION,
        width: GAME_CANVAS,
        height: GAME_CANVAS,
        background: null,
        layers: [{ id: newId(), name: 'Layer 1', visible: true, opacity: 1, strokes: [] }]
    }
}

const ui = reactive({
    activeTool: 'pen' as ToolId,
    color: DEFAULT_STYLE.color,
    strokeWidth: DEFAULT_STYLE.strokeWidth,
    fillEnabled: DEFAULT_STYLE.fill !== null,
    fill: DEFAULT_STYLE.fill ?? '#ffffff'
})

const canUndo = ref(false)
const canRedo = ref(false)
const zoom = ref(1)
const zoomPercent = computed(() => Math.round(zoom.value * 100))

function syncEditorState() {
    if (!editor) return
    canUndo.value = editor.canUndo()
    canRedo.value = editor.canRedo()
    zoom.value = editor.getZoom()
}

/**
 * A client view over GAME.md's match states: `connecting` (POST /matches), `waiting`
 * (roster filling, prompt redacted), `drawing`, `submitting` (transient), `judging`
 * (awaiting the opponent and the verdict), `done` and `error`.
 */
type Phase = 'connecting' | 'waiting' | 'drawing' | 'submitting' | 'judging' | 'done' | 'error'
const phase = ref<Phase>('connecting')

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

const pollCadence = ref(POLL_MS)

// Degraded, not broken: the poll keeps the round moving; only presence stops.
const wsReconnecting = ref(false)

// Best-effort presence; undefined until the first frame, and never load-bearing.
const opponentOnline = ref<boolean | undefined>(undefined)

// The deadline (epoch ms, null while waiting) and the server clock skew, re-anchored on
// every response (docs/NOTES.md "The round countdown is anchored on the server clock").
const deadlineMs = ref<number | null>(null)
let clockOffsetMs = 0

// Captured at the first deadline; drives only the progress bar.
const roundTotalSeconds = ref(0)

const remaining = ref(0)
const submitting = computed(() => phase.value === 'submitting')
const canSubmit = computed(() => phase.value === 'drawing')

const result = ref<DuelResult | null>(null)
const errorMsg = ref('')

// Object URLs for the rasters, revoked on reset and unmount.
let youImageUrl: string | null = null
let opponentImageUrl: string | null = null

// Cleared on reset and unmount.
let tick: number | null = null
const timeouts = new Set<number>()

// wsGeneration marks callbacks from a replaced or closed socket as stale, so a late close
// is not read as a drop.
let socket: MatchSocketHandle | null = null
let wsGeneration = 0
let wsReconnectAttempt = 0
let wsHeartbeat: number | null = null

function later(fn: () => void, ms: number): void {
    const id = window.setTimeout(() => {
        timeouts.delete(id)
        fn()
    }, ms)
    timeouts.add(id)
}

function stopCountdown(): void {
    if (tick !== null) {
        clearInterval(tick)
        tick = null
    }
}

function clearTimers(): void {
    stopCountdown()
    for (const id of timeouts) clearTimeout(id)
    timeouts.clear()
}

// Also runs right after each re-anchor, so the display never lags a second.
function tickRemaining(): void {
    if (deadlineMs.value === null) {
        remaining.value = 0
        return
    }
    remaining.value = Math.max(0, (deadlineMs.value - (Date.now() + clockOffsetMs)) / 1000)
}

function anchorClock(drawingDeadline: string | null, serverTime: string): void {
    clockOffsetMs = Date.parse(serverTime) - Date.now()
    const nextDeadlineMs = drawingDeadline !== null ? Date.parse(drawingDeadline) : null
    if (nextDeadlineMs !== null && roundTotalSeconds.value === 0) {
        roundTotalSeconds.value = Math.max(0, (nextDeadlineMs - (Date.now() + clockOffsetMs)) / 1000)
    }
    deadlineMs.value = nextDeadlineMs
    tickRemaining()
}

function startCountdown(): void {
    stopCountdown()
    tick = window.setInterval(() => {
        tickRemaining()
        if (phase.value !== 'drawing' || deadlineMs.value === null) return
        const remainingMsNow = deadlineMs.value - (Date.now() + clockOffsetMs)
        if (remainingMsNow <= AUTO_SUBMIT_MARGIN_MS) {
            stopCountdown()
            submit() // near the server cutoff — auto-submit whatever is on the canvas
        }
    }, 1000)
}

// A spent daily budget: the card offers the ladder instead of a doomed "Try again".
const exhausted = ref(false)

function toError(msg: string, spent = false): void {
    exhausted.value = spent
    errorMsg.value = msg
    phase.value = 'error'
    stopCountdown()
}

// A poll tick and the socket's 4001 close can both notice one dead session; this joins
// them into one recovery.
let recovering = false

// Signing back in restarts with a fresh match; declining leaves the retry card, whose
// "Try again" raises the gate again on the next 401.
async function recoverFromAuthError(): Promise<void> {
    if (recovering) return
    recovering = true
    // Stop the clock first, or the auto-submit fires into the dead session, loops back
    // here and parks the player in `submitting`.
    stopCountdown()
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
    anchorClock(m.drawingDeadline, m.serverTime)
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
    startCountdown()
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
    later(() => void pollTick(), pollCadence.value)
}

function stopHeartbeat(): void {
    if (wsHeartbeat !== null) {
        clearInterval(wsHeartbeat)
        wsHeartbeat = null
    }
}

// Bump the generation first, so this socket's late callbacks read as stale.
function closeSocket(): void {
    stopHeartbeat()
    wsGeneration++
    socket?.close()
    socket = null
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

// Through later(), so clearTimers() cancels a pending attempt.
function scheduleReconnect(): void {
    if (disposed || isTerminalPhase() || matchId === null) return
    wsReconnecting.value = true
    pollCadence.value = POLL_MS
    const id = matchId
    const step = Math.min(wsReconnectAttempt, WS_RECONNECT_BACKOFF_MS.length - 1)
    wsReconnectAttempt += 1
    later(() => {
        if (disposed || isTerminalPhase() || matchId !== id) return
        openSocket(id)
    }, WS_RECONNECT_BACKOFF_MS[step])
}

/** Open the match socket, replacing any existing one. */
function openSocket(id: string): void {
    closeSocket()
    const gen = ++wsGeneration
    socket = openMatchSocket(id, {
        onOpen: () => {
            if (disposed || gen !== wsGeneration) return
            wsReconnecting.value = false
            wsReconnectAttempt = 0
            pollCadence.value = WS_POLL_MS
            stopHeartbeat()
            wsHeartbeat = window.setInterval(() => socket?.ping(), WS_PING_MS)
        },
        onClose: (code) => {
            if (disposed || gen !== wsGeneration) return
            stopHeartbeat()
            pollCadence.value = POLL_MS
            if (code === 4001) {
                // Armed at the JWT exp (docs/API.md §9.1): the session is gone.
                recoverFromAuthError()
                return
            }
            scheduleReconnect()
        },
        onError: () => {
            if (disposed || gen !== wsGeneration) return
            // No detail here; the close event that follows carries the code.
            pollCadence.value = POLL_MS
        },
        onFrame: handleWsFrame
    })
}

async function captureYourRaster(): Promise<string | null> {
    if (!editor) return null
    try {
        const doc = editor.getDocument()
        // Advisory only: the judged raster is rendered server-side (docs/GAME.md §6).
        const blob = await editor.toPNG({ outWidth: doc.width, outHeight: doc.height, fit: 'contain' })
        return URL.createObjectURL(blob)
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
    wsReconnecting.value = false
    pollCadence.value = POLL_MS
    wsReconnectAttempt = 0
    closeSocket()
    deadlineMs.value = null
    clockOffsetMs = 0
    roundTotalSeconds.value = 0
    remaining.value = 0
    try {
        const m = await createMatch.mutateAsync()
        if (disposed) return
        matchId = m.id
        applyRoster(m)
        // Open now, so a waiting player hears the instant the opponent joins.
        openSocket(matchId)
        if (m.status === 'drawing') beginDrawing()
        else phase.value = 'waiting'
        scheduleNextPoll()
    } catch (err) {
        if (disposed) return
        handleError(err)
    }
}

async function submit(): Promise<void> {
    if (phase.value !== 'drawing' || matchId === null || !editor) return
    phase.value = 'submitting'
    stopCountdown()
    revokeYourRaster()
    youImageUrl = await captureYourRaster()
    if (disposed) return
    try {
        const res = await submitMatch.mutateAsync({ id: matchId, document: editor.getDocument() })
        if (disposed) return
        anchorClock(res.drawingDeadline, res.serverTime)
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
    stopCountdown()
    // Off the critical path; skipped for a forfeiter with no drawing.
    if (matchId !== null && opp?.drawingId) renderOpponentRaster(matchId, opp.userId)
}

function playAgain(): void {
    clearTimers()
    closeSocket()
    revokeYourRaster()
    revokeOpponentRaster()
    result.value = null
    editor?.loadDocument(blankGameDocument())
    syncEditorState()
    startMatch()
}

function viewLeaderboard(): void {
    router.push('/leaderboard')
}

function pickTool(id: ToolId) {
    ui.activeTool = id
    editor?.setTool(TOOLS[id])
}
function setColor(hex: string) {
    ui.color = hex
    editor?.setStyle({ color: hex })
}
function setWidth(width: number) {
    ui.strokeWidth = width
    editor?.setStyle({ strokeWidth: width })
}
function toggleFill(enabled: boolean) {
    ui.fillEnabled = enabled
    editor?.setStyle({ fill: enabled ? ui.fill : null })
}
function setFill(hex: string) {
    ui.fill = hex
    if (ui.fillEnabled) editor?.setStyle({ fill: hex })
}
function undo() {
    editor?.undo()
}
function redo() {
    editor?.redo()
}
function zoomIn() {
    editor?.zoomIn()
}
function zoomOut() {
    editor?.zoomOut()
}
function fitView() {
    editor?.fitToViewport()
}

const KEY_TO_TOOL = new Map<string, ToolId>(
    (Object.keys(TOOLS) as ToolId[]).map((id) => [TOOL_META[id].key.toLowerCase(), id])
)

function onKeydown(e: KeyboardEvent) {
    // The sign-in modal owns the keyboard: Ctrl+Enter would submit into the old session.
    if (gate.open) return
    const target = e.target as HTMLElement | null
    if (target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable)) {
        return
    }
    const key = e.key.toLowerCase()
    if (e.ctrlKey || e.metaKey) {
        if (key === 'enter') {
            e.preventDefault()
            submit()
        } else if (key === 'z' && !e.shiftKey) {
            e.preventDefault()
            editor?.undo()
        } else if ((key === 'z' && e.shiftKey) || key === 'y') {
            e.preventDefault()
            editor?.redo()
        } else if (key === '0') {
            e.preventDefault()
            editor?.fitToViewport()
        } else if (key === '=' || key === '+') {
            e.preventDefault()
            editor?.zoomIn()
        } else if (key === '-') {
            e.preventDefault()
            editor?.zoomOut()
        }
        return
    }
    if (e.altKey) return
    // Tool keys only while drawing.
    if (phase.value !== 'drawing') return
    const tool = KEY_TO_TOOL.get(key)
    if (tool) {
        e.preventDefault()
        pickTool(tool)
    }
}

onMounted(async () => {
    const container = shell.value?.canvasEl ?? null
    if (!container) return
    editor = new Editor(container, blankGameDocument())
    editor.setTool(TOOLS[ui.activeTool])
    editor.setStyle({ ...DEFAULT_STYLE })
    editor.setCursorColor(cursorRingColor.value || null)
    unsubscribe = editor.onChange(syncEditorState)
    syncEditorState()
    window.addEventListener('keydown', onKeydown)

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
    window.removeEventListener('keydown', onKeydown)
    clearTimers()
    closeSocket()
    revokeYourRaster()
    revokeOpponentRaster()
    unsubscribe?.()
    unsubscribe = null
    editor?.destroy()
    editor = null
})
</script>

<template>
    <EditorShell ref="shell" mode="play">
        <template #top-left>
            <!-- Display name or "Player 2", never a login. -->
            <OpponentStatusChip :name="opponent.name" :status="opponent.status" :online="opponentOnline" />
            <OriBadge
                v-if="wsReconnecting"
                content="reconnecting…"
                color="warn"
                variant="tonal"
                label="Reconnecting to the match"
            />
        </template>

        <template #top-center>
            <!-- Hidden until the deadline exists, rather than a misleading 0:00. -->
            <RoundTimerBar v-if="deadlineMs !== null" :remaining="remaining" :total="roundTotalSeconds" />
            <div class="play__prompt">
                <GamePromptBanner :prompt="prompt" :revealed="promptRevealed" />
            </div>
        </template>

        <!-- No drawer: SideMenu is /draw-specific.
             TODO(play-api): a play drawer (leave/rematch/profile). -->
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
            <OriSurface class="play__zoom" role="group" aria-label="Zoom">
                <IconButton icon="minus" label="Zoom out — Ctrl+-" @click="zoomOut" />
                <span class="play__zoom-value">{{ zoomPercent }}%</span>
                <IconButton icon="plus" label="Zoom in — Ctrl+=" @click="zoomIn" />
                <IconButton icon="fit" label="Fit — Ctrl+0" @click="fitView" />
            </OriSurface>
        </template>

        <template #overlay>
            <OriSurface v-if="phase === 'error'" class="play__notice" role="alert">
                <h2 class="play__notice-title">
                    {{ exhausted ? 'That’s your duels for today' : 'Can’t start the duel' }}
                </h2>
                <p class="play__notice-msg">{{ errorMsg }}</p>
                <OriButton
                    v-if="exhausted"
                    text="Leaderboard"
                    variant="outline"
                    radius="md"
                    :icon="icons.podium"
                    icon-position="left"
                    @click="viewLeaderboard"
                />
                <OriButton v-else text="Try again" variant="fill" color="primary" radius="md" @click="startMatch" />
            </OriSurface>
            <JudgingOverlay v-else-if="phase === 'judging' || phase === 'submitting'" :opponent-name="opponent.name" />
            <ResultReveal
                v-else-if="phase === 'done' && result"
                :result="result"
                @play-again="playAgain"
                @view-leaderboard="viewLeaderboard"
            />
        </template>
    </EditorShell>
</template>

<style scoped>
/* Clears the timer chip and the corner islands on a narrow phone. */
.play__prompt {
    padding-top: 2.5rem;
    pointer-events: none;
}

/* The shell's strip is pointer-events:none; the toolbar opts back in. */
.play__toolbar-item {
    pointer-events: auto;
}

/* Mirrors /draw's zoom island: island visuals, not shell layout. */
.play__zoom {
    display: flex;
    align-items: center;
    gap: 0;

    padding: var(--ori-size-gap_xs, 0.125rem) var(--ori-size-gap_sm, 0.25rem);
}

.play__zoom-value {
    min-width: 3.1rem;
    padding: 0.25rem;

    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_sm, 0.85rem);
    font-variant-numeric: tabular-nums;
    text-align: center;
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
