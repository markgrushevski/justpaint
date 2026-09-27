<script lang="ts" setup>
/**
 * PlayView — the AI-judged drawing duel (`/play`). Composes the same shared
 * EditorShell as /draw on the square duel canvas (docs/GAME.md §2), and runs
 * live against the async-duel API (docs/API.md §8): create/join, poll the
 * roster and verdict, submit the document. A WS socket (docs/API.md §9) pushes
 * the same transitions instantly and demotes — never removes — the poll loop,
 * so the round still runs with the socket dropped. The judged raster is
 * rendered server-side; the client PNG here is advisory only (docs/GAME.md §6).
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
import EditorShell from '../components/shell/EditorShell.vue'
import FloatingToolbar, { TOOL_META } from '../components/FloatingToolbar.vue'
import IconButton from '../components/ui/IconButton.vue'
import RoundTimerBar from '../components/game/RoundTimerBar.vue'
import GamePromptBanner from '../components/game/GamePromptBanner.vue'
import OpponentStatusChip from '../components/game/OpponentStatusChip.vue'
import type { OpponentStatus } from '../components/game/OpponentStatusChip.vue'
import SubmitButton from '../components/game/SubmitButton.vue'
import JudgingOverlay from '../components/game/JudgingOverlay.vue'
import ResultReveal from '../components/game/ResultReveal.vue'
import type { DuelResult } from '../components/game/ResultReveal.vue'

/** The canonical square duel canvas (GAME.md §2 — GAME_CANVAS = 1080×1080). */
const GAME_CANVAS = 1080

// Fast fallback cadence — fixed forever. `pollCadence` is what scheduleNextPoll
// actually reads, demoted while the socket is live and snapped back here on
// any disconnect (docs/API.md §9.5).
const POLL_MS = 2000

// Slow reconciliation while the WS socket is live — a belt-and-suspenders
// check in case a frame was ever missed.
const WS_POLL_MS = 15000

// Lets a dead/half-open socket be noticed without waiting on a TCP timeout.
const WS_PING_MS = 25000

// Capped at the last entry; resets to the first step on a clean reconnect.
const WS_RECONNECT_BACKOFF_MS = [1000, 2000, 4000, 10000]

// Fires before the server's authoritative deadline so the request can land in
// time — a late submit is rejected (409 round_expired) as a forfeit loss.
// Fixed, independent of however slow the poll has been demoted.
const AUTO_SUBMIT_MARGIN_MS = 3000

const shell = ref<{ canvasEl: HTMLDivElement | null } | null>(null)
let editor: Editor | null = null
let unsubscribe: (() => void) | null = null

// Konva can't read CSS custom properties; the cursor ring gets the resolved
// --ori-color-primary through the oriui token bridge (same wiring as DrawView).
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
 * Phases (a client view over GAME.md's match states):
 *  connecting → creating/auto-joining the match (POST /matches)
 *  waiting    → roster filling; prompt redacted (GAME.md `open`)
 *  drawing    → prompt revealed; timer running (GAME.md `drawing`)
 *  submitting → submit POST in flight (transient)
 *  judging    → submitted; awaiting the opponent + verdict (GAME.md `judging`)
 *  done       → result revealed (GAME.md `done`)
 *  error      → auth needed / network / unrecoverable
 */
type Phase = 'connecting' | 'waiting' | 'drawing' | 'submitting' | 'judging' | 'done' | 'error'
const phase = ref<Phase>('connecting')

// Set true on unmount; every async continuation checks it before touching state.
let disposed = false

// The live match id + my resolved user id (to pick "me" out of the roster).
let matchId: string | null = null
let myUserId = ''

// The pinned prompt (server-delivered; redacted until `drawing`). Empty until known.
const prompt = ref('')
const REVEAL_PHASES = new Set<Phase>(['drawing', 'submitting', 'judging', 'done'])
const promptRevealed = computed(() => prompt.value !== '' && REVEAL_PHASES.has(phase.value))

// The opponent — a safe display label + coarse status, never a login
// (docs/GAME.md §4.2). Populated from the roster once they join.
const opponent = reactive<{ name: string; status: OpponentStatus }>({
    name: 'Player 2',
    status: 'drawing'
})

// Starts at the fast POLL_MS fallback, demoted to WS_POLL_MS while the socket
// is live, snapped back on disconnect (docs/API.md §9.5).
const pollCadence = ref(POLL_MS)

// True while the socket is down and reconnecting — a "degraded, not broken"
// affordance; the poll fallback keeps the round moving either way.
const wsReconnecting = ref(false)

// Best-effort presence from opponent_connected/disconnected frames; undefined
// until the first one arrives. Never load-bearing for correctness.
const opponentOnline = ref<boolean | undefined>(undefined)

// Server-anchored countdown (docs/NOTES.md "/play — async-duel client"):
// deadlineMs is the absolute deadline in epoch ms (null while waiting);
// clockOffsetMs is the server/client clock skew. Both re-anchor on every
// roster/create/submit response so both duelists count down from the same
// server instant, instead of each starting a local timer on first sight of
// `drawing` (the old drift bug).
const deadlineMs = ref<number | null>(null)
let clockOffsetMs = 0

// Total round length in seconds, captured once from the first deadline sight —
// drives only the progress-bar fraction; the countdown itself reads the
// absolute deadline, so this never affects correctness.
const roundTotalSeconds = ref(0)

const remaining = ref(0)
const submitting = computed(() => phase.value === 'submitting')
const canSubmit = computed(() => phase.value === 'drawing')

const result = ref<DuelResult | null>(null)
const errorMsg = ref('')

// Object URLs for the captured rasters — revoked on reset/unmount.
let youImageUrl: string | null = null
let opponentImageUrl: string | null = null

// Timers owned here, cleared on reset/unmount (the countdown + poll ticks).
let tick: number | null = null
const timeouts = new Set<number>()

// wsGeneration invalidates callbacks from a socket we've already replaced or
// torn down, so a late close event from a superseded socket reads as stale
// instead of an unexpected drop.
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

// Called on every tick and immediately after every re-anchor so the display
// never waits a full second to reflect a fresh server response.
function tickRemaining(): void {
    if (deadlineMs.value === null) {
        remaining.value = 0
        return
    }
    remaining.value = Math.max(0, (deadlineMs.value - (Date.now() + clockOffsetMs)) / 1000)
}

// Re-anchors from a fresh (deadline, serverTime) pair on every roster/create/
// submit response (docs/NOTES.md "/play — async-duel client").
function anchorClock(drawingDeadline: string | null, serverTime: string): void {
    clockOffsetMs = Date.parse(serverTime) - Date.now()
    const nextDeadlineMs = drawingDeadline !== null ? Date.parse(drawingDeadline) : null
    if (nextDeadlineMs !== null && roundTotalSeconds.value === 0) {
        // First sight of the deadline this round; see roundTotalSeconds above.
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
            void submit() // near the server cutoff — auto-submit whatever is on the canvas
        }
    }, 1000)
}

// A refusal the player can't retry (a spent daily budget). The card drops
// "Try again" for it and offers the ladder instead — a retry guaranteed to
// fail until tomorrow is a worse dead end than saying so.
const exhausted = ref(false)

function toError(msg: string, spent = false): void {
    exhausted.value = spent
    errorMsg.value = msg
    phase.value = 'error'
    stopCountdown()
}

// A poll tick and the WS's 4001 close can both notice the same dead session
// within moments of each other. This sentinel joins them into one recovery
// instead of racing two independent startMatch() calls.
let recovering = false

// Recovers through the one shared gate instead of a dead-end inline form:
// signing back in restarts with a fresh match; declining falls back to the
// retry card, whose "Try again" re-raises this same gate on the next 401.
async function recoverFromAuthError(): Promise<void> {
    if (recovering) return
    recovering = true
    // Stop the clock before awaiting a human — left running, the auto-submit
    // fires into the dead session and its 401 loops back here, parking the
    // player in `submitting` forever.
    stopCountdown()
    try {
        const signedIn = await gate.ensure('Sign in to play a duel.')
        if (disposed) return
        if (signedIn && session.user) {
            // The visitor may have signed back in as a different account;
            // myUserId decides who is "me" throughout the roster and result.
            myUserId = session.user.id
            void startMatch()
        } else {
            toError('Sign in to play a duel.')
        }
    } finally {
        recovering = false
    }
}

function handleError(err: unknown): void {
    if (isAuthError(err)) {
        void recoverFromAuthError()
        return
    }
    // isBudgetExhausted, not isRateLimited: the per-IP write tier also answers
    // 429 (docs/API.md §3.1) but clears in seconds, so treating every 429 as a
    // spent day would take away a retry that would have worked.
    toError(toApiError(err)?.message ?? 'Something went wrong. Try again.', isBudgetExhausted(err))
}

// Makes applyRoster/applyResult monotonic: a slower in-flight response can
// never regress a terminal UI, and a re-delivered verdict is a no-op.
// startMatch always moves phase to `connecting` first, implicitly resetting this.
function isTerminalPhase(): boolean {
    return phase.value === 'done' || phase.value === 'error'
}

// A no-op once a terminal phase is reached (monotonic — see isTerminalPhase).
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

// The deadline itself was already captured by the applyRoster call that
// triggered this transition.
function beginDrawing(): void {
    phase.value = 'drawing'
    startCountdown()
}

// The one poll loop for the whole round: reschedules itself until a terminal
// phase, spanning waiting -> drawing -> (submitting) -> judging. `submitting`
// falls through untouched; by the next tick the phase is `judging`.
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

// Bumps wsGeneration first so any callback still in flight from this socket
// is recognized as stale and ignored, rather than triggering a reconnect.
function closeSocket(): void {
    stopHeartbeat()
    wsGeneration++
    socket?.close()
    socket = null
}

// A thin adapter: dispatches into the same poll-loop handlers, no new state
// machine. `match_state` mirrors pollTick's waiting/drawing branch so the
// waiting player reacts the instant the opponent joins (docs/API.md §9.2).
// Bails once a terminal phase is reached, so a frame can never regress it.
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

// Scheduled via the shared later()/timeouts bookkeeping, so clearTimers()
// (already called by playAgain/unmount) cancels a pending attempt for free.
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

/** Open the live match socket (or replace an existing one). Frames dispatch
 *  into `handleWsFrame`; the poll loop is only ever demoted, never stopped. */
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
                // The backend arms this at the JWT exp (docs/API.md §9.1): the
                // session itself is gone, so recover through the auth gate
                // instead of reconnecting.
                void recoverFromAuthError()
                return
            }
            scheduleReconnect()
        },
        onError: () => {
            if (disposed || gen !== wsGeneration) return
            // The DOM error event carries no detail; the close event that
            // follows has the real code, so just fall back and let onClose decide.
            pollCadence.value = POLL_MS
        },
        onFrame: handleWsFrame
    })
}

async function captureYourRaster(): Promise<string | null> {
    if (!editor) return null
    try {
        const doc = editor.getDocument()
        // Advisory only (docs/GAME.md §6) — the judged raster is server-side.
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

// Fetches the opponent's document (membership-gated; the ownership-scoped
// drawings route 404s a non-owner) and renders it client-side with the same
// renderer used for your own raster — uniform, no object storage (docs/GAME.md
// §6). Non-fatal: any failure keeps the "No preview" fallback.
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
        // Leave the placeholder; the reveal already shows scores + your canvas.
    }
}

async function startMatch(): Promise<void> {
    phase.value = 'connecting'
    errorMsg.value = ''
    prompt.value = ''
    opponent.name = 'Player 2'
    opponent.status = 'drawing'
    opponentOnline.value = undefined
    // startMatch can be re-entered directly (the error overlay's "Try again"),
    // not only via playAgain, so reset all WS/clock bookkeeping for a fresh round.
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
        // Open the socket now, so a still-waiting player gets the match_state
        // push the instant the opponent joins (docs/API.md §9.2).
        openSocket(matchId)
        // Auto-joined an existing open match means the round is already live;
        // otherwise wait for an opponent with the prompt still redacted.
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
        // A 409 (round_expired) means the submission is already recorded or the
        // match moved on, so poll the verdict instead of erroring out
        // (docs/API.md §8, submit).
        if (toApiError(err)?.status === 409) {
            phase.value = 'judging'
            return
        }
        handleError(err)
    }
}

// Idempotent and monotonic (docs/GAME.md §7.1): a no-op once a terminal phase
// is reached, so a re-delivered verdict can't clobber the opponent image
// patched in later.
function applyResult(r: MatchResultDone): void {
    if (isTerminalPhase()) return
    const me = r.players.find((p) => p.userId === myUserId)
    const opp = r.players.find((p) => p.userId !== myUserId)
    const before = me?.ratingBefore ?? session.user?.rating ?? 1200
    const after = me?.ratingAfter ?? before
    result.value = {
        // Judge scores are 0..1, the reveal bar is 0..100. Both are null when
        // the judge never ran (forfeit or aborted); `resolution` hides the
        // score row itself rather than showing a misleading 0%.
        you: { score: (me?.score ?? 0) * 100, image: youImageUrl },
        opponent: {
            name: opp?.displayName ?? opponent.name,
            score: (opp?.score ?? 0) * 100,
            // Rendered client-side below (no object storage); null until it
            // resolves shows the "No preview" placeholder.
            image: null
        },
        // Read the server's own isTie rather than re-deriving it from a null
        // winner: a null winner also means "nobody was scored" on an aborted
        // round, which the server already distinguishes (docs/GAME.md §3).
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
    // The session store only updates user.rating on boot restore or login, so
    // without this the SideMenu/leaderboard would show the pre-match rating.
    if (session.user && me) session.user.rating = after
    void queryClient.invalidateQueries({ queryKey: leaderboardKeys.all })
    phase.value = 'done'
    stopCountdown()
    // Off the critical path; patches into result.opponent.image when ready, or
    // silently keeps the placeholder. Skipped for a forfeiter with no drawing.
    if (matchId !== null && opp?.drawingId) void renderOpponentRaster(matchId, opp.userId)
}

function playAgain(): void {
    clearTimers()
    closeSocket()
    revokeYourRaster()
    revokeOpponentRaster()
    result.value = null
    editor?.loadDocument(blankGameDocument())
    syncEditorState()
    void startMatch()
}

// ResultReveal is presentational; the view owns navigation.
function viewLeaderboard(): void {
    void router.push('/leaderboard')
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
    // The sign-in modal owns the keyboard while it is up — including Ctrl+Enter,
    // which would otherwise submit the round into the session being replaced.
    if (gate.open) return
    const target = e.target as HTMLElement | null
    if (target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable)) {
        return
    }
    const key = e.key.toLowerCase()
    if (e.ctrlKey || e.metaKey) {
        if (key === 'enter') {
            e.preventDefault()
            void submit()
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
    // Only bind tool keys while actually drawing (not during judging/result).
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
    // The editor sizes its Konva stage to the container and fits the 1080²
    // document into it; a ResizeObserver keeps it fitted (never CSS-transforms).
    editor = new Editor(container, blankGameDocument())
    editor.setTool(TOOLS[ui.activeTool])
    editor.setStyle({ ...DEFAULT_STYLE })
    editor.setCursorColor(cursorRingColor.value || null)
    unsubscribe = editor.onChange(syncEditorState)
    syncEditorState()
    window.addEventListener('keydown', onKeydown)

    // A duel is auth-required. `ensure` waits for the store's cookie restore,
    // then raises the one shared sign-in modal; signing in resumes straight
    // into the duel.
    const signedIn = await gate.ensure('Sign in to play a duel.')
    if (disposed) return
    if (!signedIn) {
        // "Try again" re-attempts the match, which re-raises this same gate on
        // the next 401 — always a way back in.
        toError('Sign in to play a duel.')
        return
    }
    if (!session.user) return // gate only resolves true once a session exists
    myUserId = session.user.id
    void startMatch()
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
    <!-- The same shared shell as /draw, now in play mode with game chrome
         filling the slots. -->
    <EditorShell ref="shell" mode="play">
        <template #top-left>
            <!-- Display name or "Player 2", never a login. -->
            <OpponentStatusChip :name="opponent.name" :status="opponent.status" :online="opponentOnline" />
            <!-- Degraded, not broken: the poll fallback keeps the round moving
                 while the socket reconnects, but presence updates quietly stop. -->
            <OriBadge
                v-if="wsReconnecting"
                content="reconnecting…"
                color="warn"
                variant="tonal"
                label="Reconnecting to the match"
            />
        </template>

        <template #top-center>
            <!-- Hidden until the deadline is stamped, to show the waiting state
                 instead of a misleading 0:00 countdown. -->
            <RoundTimerBar v-if="deadlineMs !== null" :remaining="remaining" :total="roundTotalSeconds" />
            <div class="play__prompt">
                <GamePromptBanner :prompt="prompt" :revealed="promptRevealed" />
            </div>
        </template>

        <!-- No drawer for now — SideMenu is /draw-specific (file/canvas).
             TODO(play-api): a play-specific drawer (leave/rematch/profile). -->
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
/* Clears the fixed RoundTimerBar clock chip and the top-corner islands so
   nothing collides on a narrow phone where all three top zones crowd the
   centre. Stays pointer-events:none like the region itself. */
.play__prompt {
    padding-top: 2.5rem;
    pointer-events: none;
}

/* The shell's bottom-center strip is pointer-events:none — the toolbar opts back
   in so drawing passes through the empty flanks either side of it. */
.play__toolbar-item {
    pointer-events: auto;
}

/* Same compact chrome as /draw's; mirrored, not shared, since it's island
   visuals, not shell layout. */
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

/* Error / sign-in card — centred in the shell's pointer-events:none overlay, so
   it opts back in. Same OriSurface chrome as the other overlays. */
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
