<script lang="ts" setup>
/**
 * PlayView — the AI-judged drawing duel (`/play`). It composes the SAME shared
 * EditorShell that /draw uses (DECISIONS 2026-07-04: one design, game chrome on
 * top), mounting the real Editor into the shell's exposed canvas element exactly
 * as DrawView does — but on the square 1080² duel canvas (GAME.md §2) and with
 * game chrome (round timer, prompt banner, opponent status, submit, judging +
 * result overlays) filling the regions instead of /draw's file/layer chrome.
 *
 * LIVE against the async-duel API (docs/API.md §8, docs/GAME.md): on mount it
 * creates/auto-joins a match (`POST /api/matches`), polls the roster until the
 * opponent joins, reveals the pinned prompt, and — on submit — POSTs the vector
 * document and polls the verdict (`GET /api/matches/:id/result`) until the judge
 * decides. Reads that drive the flow are polled directly (an ephemeral per-round
 * flow); create/submit go through TanStack mutations. WS push replaces the polling
 * later (docs/API.md §9, not-v1). The authoritative judged raster is rendered
 * server-side — the client PNG here is an advisory preview only (GAME.md §6).
 */
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { OriButton, OriSurface } from '@oriui/vue'
import { Editor, TOOLS, DEFAULT_STYLE, newId, renderToPNG } from '@justpaint/editor'
import type { ToolId } from '@justpaint/editor'
import type { Document } from '@justpaint/document'
import { DOC_VERSION, parseDocument } from '@justpaint/document'
import { useThemeColor } from '@oriui/headless/vue'
import { useSessionStore, useCreateMatch, useSubmitMatch, matches, isAuthError, toApiError } from '@core'
import type { Match, MatchResultDone } from '@core'
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

/** How often to poll the roster / verdict while a round is in flight. */
const POLL_MS = 2000

/**
 * Fire the auto-submit this far BEFORE the server-authoritative deadline, so the
 * request has a chance to land before the server's own cutoff. Fixed and
 * deliberately NOT derived from `POLL_MS` (docs/DESIGN-PHASE3-LIVE.md §2.9) — a
 * submit that arrives after the deadline is rejected (409 `round_expired`) and
 * self-inflicts a forfeit loss, so auto-submit fires a little early on purpose.
 */
const AUTO_SUBMIT_MARGIN_MS = 3000

/* --- editor mount seam (identical to DrawView) ------------------------- */

// The shared layout skeleton exposes its Konva mount element; we read it in
// onMounted and build the Editor into it — the SAME seam DrawView uses.
const shell = ref<{ canvasEl: HTMLDivElement | null } | null>(null)
let editor: Editor | null = null
let unsubscribe: (() => void) | null = null

// Konva can't read CSS custom properties, so the brush cursor ring gets the
// resolved --ori-color-primary through the oriui token bridge (re-resolves on
// theme flips) — exactly as DrawView wires it.
const cursorRingColor = useThemeColor('primary')

const session = useSessionStore()
const createMatch = useCreateMatch()
const submitMatch = useSubmitMatch()

/** A blank single-layer document at the square GAME canvas size. */
function blankGameDocument(): Document {
    return {
        version: DOC_VERSION,
        width: GAME_CANVAS,
        height: GAME_CANVAS,
        background: null,
        layers: [{ id: newId(), name: 'Layer 1', visible: true, opacity: 1, strokes: [] }]
    }
}

/* --- toolbar / zoom state (mirrors DrawView, minus file+layers) -------- */

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

/* --- match state machine (driven by the live API) ---------------------- */

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

// The opponent — a safe display label + coarse status (NEVER a login; GAME.md
// §4.2). Populated from the roster once they join; the flag is polled.
const opponent = reactive<{ name: string; status: OpponentStatus }>({
    name: 'Player 2',
    status: 'drawing'
})

/**
 * Server-anchored countdown state (docs/DESIGN-PHASE3-LIVE.md §2.9): `deadlineMs`
 * is the absolute round deadline in epoch ms (null while `open`/waiting — nothing
 * to count down to yet), and `clockOffsetMs` is the last-computed skew between the
 * server clock and this client's `Date.now()`. Both are re-anchored from every
 * roster/create/submit response (`anchorClock`), so the two duelists always count
 * down from the SAME server instant instead of each starting their own local timer
 * on first local sight of `drawing` (the old drift bug).
 */
const deadlineMs = ref<number | null>(null)
let clockOffsetMs = 0

/** The round's total length in seconds, captured once per round from the first
 *  sight of the deadline — drives only the progress-bar fraction; the countdown
 *  itself always reads the absolute deadline, so this has no bearing on
 *  correctness. Reset to 0 at the start of each new match. */
const roundTotalSeconds = ref(0)

const remaining = ref(0)
const submitting = computed(() => phase.value === 'submitting')
const canSubmit = computed(() => phase.value === 'drawing')

const result = ref<DuelResult | null>(null)
// Error surface for the `error` phase.
const errorMsg = ref('')
const needsAuth = ref(false)

// Object URL of the player's captured raster — revoked on reset/unmount.
let youImageUrl: string | null = null
// Object URL of the opponent's raster, rendered client-side from their fetched
// document on the reveal (GAME.md §6) — revoked on reset/unmount, like youImageUrl.
let opponentImageUrl: string | null = null

// Timers we own and must clear on reset/unmount (the countdown + poll ticks).
let tick: number | null = null
const timeouts = new Set<number>()

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

/** Recompute `remaining` (seconds, clamped >= 0) from the last-anchored deadline +
 *  clock offset. Called on every tick and immediately after every re-anchor so the
 *  display never waits a full second to reflect a fresh server response. */
function tickRemaining(): void {
    if (deadlineMs.value === null) {
        remaining.value = 0
        return
    }
    remaining.value = Math.max(0, (deadlineMs.value - (Date.now() + clockOffsetMs)) / 1000)
}

/**
 * Re-anchor the server-authoritative countdown from a fresh (deadline, serverTime)
 * pair — called from every roster/create/submit response. Both duelists compute
 * their countdown from the SAME server instant, re-anchored on every poll, which
 * structurally closes the old drift bug (docs/DESIGN-PHASE3-LIVE.md §2.9).
 */
function anchorClock(drawingDeadline: string | null, serverTime: string): void {
    clockOffsetMs = Date.parse(serverTime) - Date.now()
    const nextDeadlineMs = drawingDeadline !== null ? Date.parse(drawingDeadline) : null
    if (nextDeadlineMs !== null && roundTotalSeconds.value === 0) {
        // First sight of the deadline this round — capture the total for the
        // progress-bar fraction only (see roundTotalSeconds's doc comment).
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

/** Move to the terminal error phase (auth = show a sign-in path instead of retry). */
function toError(msg: string, auth = false): void {
    errorMsg.value = msg
    needsAuth.value = auth
    phase.value = 'error'
    stopCountdown()
}

/** Map any thrown API error onto the error phase (auth → sign-in prompt). */
function handleError(err: unknown): void {
    if (isAuthError(err)) {
        toError('Sign in to play a duel.', true)
        return
    }
    toError(toApiError(err)?.message ?? 'Something went wrong. Try again.')
}

/** True once a terminal phase (`done`, or abandoned/error) has been reached for
 *  the current match. Makes `applyRoster`/`applyResult` monotonic: a later,
 *  slower in-flight response must never regress a terminal UI, and a re-delivered
 *  verdict is a no-op (docs/DESIGN-PHASE3-LIVE.md §2.9). Implicitly reset by
 *  `startMatch`, which always moves `phase` to `connecting` before a new round's
 *  first response can arrive. */
function isTerminalPhase(): boolean {
    return phase.value === 'done' || phase.value === 'error'
}

/** Reflect the roster into the opponent chip + the revealed prompt, and re-anchor
 *  the server countdown from this response. A no-op once a terminal phase has
 *  been reached (monotonic — see `isTerminalPhase`). */
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

/** Enter the drawing phase: reveal the prompt and start the server-anchored clock
 *  (the deadline itself was already captured by the `applyRoster` call that
 *  triggered this transition). */
function beginDrawing(): void {
    phase.value = 'drawing'
    startCountdown()
}

/**
 * The single poll loop. It reschedules itself every POLL_MS until a terminal phase
 * (done/error) — spanning waiting → drawing → (submitting) → judging — so there is
 * exactly one loop for the whole round. `submitting` falls through untouched (it
 * just waits for the next tick, by which point the phase is `judging`).
 */
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
    later(() => void pollTick(), POLL_MS)
}

/** Capture the player's drawing as a PNG object URL (advisory preview only). */
async function captureYourRaster(): Promise<string | null> {
    if (!editor) return null
    try {
        const doc = editor.getDocument()
        // Client PNG is advisory (GAME.md §6) — the authoritative judged raster is
        // rendered server-side. This is only for the reveal preview.
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

/**
 * Reveal the opponent's canvas: fetch their submitted document (authorized by
 * match membership + `done`, since the ownership-scoped drawings route 404s a
 * non-owner) and render it to a PNG object URL client-side — the SAME renderer
 * that captured your own raster, so both sides are uniform (GAME.md §6; no object
 * storage). Non-fatal: on any failure the reveal keeps its "No preview" fallback.
 */
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

/** Create or auto-join a match and start the poll loop. */
async function startMatch(): Promise<void> {
    phase.value = 'connecting'
    errorMsg.value = ''
    needsAuth.value = false
    prompt.value = ''
    opponent.name = 'Player 2'
    opponent.status = 'drawing'
    // Fresh round: clear the previous round's server-anchored clock so it doesn't
    // leak into this one (roundTotalSeconds's "first sight" capture in particular).
    deadlineMs.value = null
    clockOffsetMs = 0
    roundTotalSeconds.value = 0
    remaining.value = 0
    try {
        const m = await createMatch.mutateAsync()
        if (disposed) return
        matchId = m.id
        applyRoster(m)
        // Auto-joined an existing open match ⇒ the round is already live; otherwise
        // we opened one and wait for an opponent (prompt stays redacted).
        if (m.status === 'drawing') beginDrawing()
        else phase.value = 'waiting'
        scheduleNextPoll()
    } catch (err) {
        if (disposed) return
        handleError(err)
    }
}

/** Submit the current drawing, then hand off to the verdict poll. */
async function submit(): Promise<void> {
    if (phase.value !== 'drawing' || matchId === null || !editor) return
    phase.value = 'submitting'
    stopCountdown()
    // Snapshot the advisory preview before we hand the document to the server.
    revokeYourRaster()
    youImageUrl = await captureYourRaster()
    if (disposed) return
    try {
        const res = await submitMatch.mutateAsync({ id: matchId, document: editor.getDocument() })
        if (disposed) return
        anchorClock(res.drawingDeadline, res.serverTime)
        // Recorded (202). The live poll loop picks up the `judging` branch and
        // polls the verdict; the opponent status resolves from there.
        opponent.status = 'judging'
        phase.value = 'judging'
    } catch (err) {
        if (disposed) return
        // A 409 (e.g. round_expired) means the submission is already recorded / the
        // match moved on — proceed to poll the verdict rather than erroring the
        // player out (docs/DESIGN-PHASE3-LIVE.md §2.4/§2.9).
        if (toApiError(err)?.status === 409) {
            phase.value = 'judging'
            return
        }
        handleError(err)
    }
}

/** Map the decided server verdict into the reveal shape (GAME.md §7.1). Idempotent
 *  and monotonic: a no-op once a terminal phase is already reached, so a
 *  re-delivered verdict can't clobber the async-patched opponent image
 *  (docs/DESIGN-PHASE3-LIVE.md §2.9). */
function applyResult(r: MatchResultDone): void {
    if (isTerminalPhase()) return
    const me = r.players.find((p) => p.userId === myUserId)
    const opp = r.players.find((p) => p.userId !== myUserId)
    const before = me?.ratingBefore ?? session.user?.rating ?? 1200
    const after = me?.ratingAfter ?? before
    result.value = {
        // Judge scores are 0..1; the reveal bar is 0..100. Both are null server-side
        // on a forfeit (no judge ran); the reveal hides the score row/bar itself via
        // `resolution` rather than showing a misleading 0%.
        you: { score: (me?.score ?? 0) * 100, image: youImageUrl },
        opponent: {
            name: opp?.displayName ?? opponent.name,
            score: (opp?.score ?? 0) * 100,
            // Rendered client-side from the opponent's fetched document below (no
            // object storage); null until it resolves → "No preview" placeholder.
            image: null
        },
        winner: r.winnerUserId === null ? 'tie' : r.winnerUserId === myUserId ? 'you' : 'opponent',
        reason: r.reason ?? '',
        resolution: r.resolution,
        eloDelta: after - before,
        ratingBefore: before
    }
    phase.value = 'done'
    stopCountdown()
    // Fetch + render the opponent's canvas off the critical path; it patches into
    // result.opponent.image when ready (or silently leaves the placeholder). Skipped
    // when the opponent has no drawing at all (the forfeiter on a forfeit loss) —
    // there is nothing to fetch, so don't fire a request that can only fail.
    if (matchId !== null && opp?.drawingId) void renderOpponentRaster(matchId, opp.userId)
}

/** Reset the canvas and create a fresh match. */
function playAgain(): void {
    clearTimers()
    revokeYourRaster()
    revokeOpponentRaster()
    result.value = null
    editor?.loadDocument(parseDocument(blankGameDocument()))
    syncEditorState()
    void startMatch()
}

/* --- toolbar handlers (mirror DrawView) -------------------------------- */

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

/* --- keyboard shortcuts (lean — tools, history, zoom, submit) ---------- */

const KEY_TO_TOOL = new Map<string, ToolId>(
    (Object.keys(TOOLS) as ToolId[]).map((id) => [TOOL_META[id].key.toLowerCase(), id])
)

function onKeydown(e: KeyboardEvent) {
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

/* --- lifecycle --------------------------------------------------------- */

onMounted(async () => {
    const container = shell.value?.canvasEl ?? null
    if (!container) return
    // The editor sizes its Konva stage to the container and fits the 1080²
    // document into it; a ResizeObserver keeps it fitted (never CSS-transforms).
    editor = new Editor(container, parseDocument(blankGameDocument()))
    editor.setTool(TOOLS[ui.activeTool])
    editor.setStyle({ ...DEFAULT_STYLE })
    editor.setCursorColor(cursorRingColor.value || null)
    unsubscribe = editor.onChange(syncEditorState)
    syncEditorState()
    window.addEventListener('keydown', onKeydown)

    // A duel is auth-required. Restore an existing cookie session, then create or
    // auto-join a match; an anonymous visitor gets the sign-in path.
    await session.fetchMe()
    if (disposed) return
    if (!session.isLoggedIn || !session.user) {
        toError('Sign in to play a duel.', true)
        return
    }
    myUserId = session.user.id
    void startMatch()
})

onBeforeUnmount(() => {
    disposed = true
    window.removeEventListener('keydown', onKeydown)
    clearTimers()
    revokeYourRaster()
    revokeOpponentRaster()
    unsubscribe?.()
    unsubscribe = null
    editor?.destroy()
    editor = null
})
</script>

<template>
    <!-- The SAME shared editor shell as /draw — desk + Konva mount + floating
         regions — now in play mode, with game chrome filling the slots. -->
    <EditorShell ref="shell" mode="play">
        <!-- Top-left: who you're dueling (display name / "Player 2", never a login). -->
        <template #top-left>
            <OpponentStatusChip :name="opponent.name" :status="opponent.status" />
        </template>

        <!-- Top-center: the round timer pinned to the very top edge, above the
             prompt reveal. The wrapper clears the fixed timer clock chip. -->
        <template #top-center>
            <!-- Hidden until the deadline is stamped (open/waiting has none yet) —
                 shows the waiting state instead of a misleading 0:00 countdown. -->
            <RoundTimerBar v-if="deadlineMs !== null" :remaining="remaining" :total="roundTotalSeconds" />
            <div class="play__prompt">
                <GamePromptBanner :prompt="prompt" :revealed="promptRevealed" />
            </div>
        </template>

        <!-- Top-right: the one accent action (replaces /draw's Save). No drawer
             for now — SideMenu is /draw-specific (file/canvas), so the hamburger
             is omitted. TODO(play-api): a play-specific drawer (leave/rematch/
             profile) can slot in here later. -->
        <template #top-right>
            <SubmitButton :disabled="!canSubmit" :loading="submitting" @submit="submit" />
        </template>

        <!-- Bottom-center: the SAME floating toolbar as /draw (opts back into
             pointer events over the shell's pass-through strip). -->
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

        <!-- Bottom-right: zoom island (mirrors DrawView's). -->
        <template #bottom-right>
            <OriSurface class="play__zoom" role="group" aria-label="Zoom">
                <IconButton icon="minus" label="Zoom out — Ctrl+-" @click="zoomOut" />
                <span class="play__zoom-value">{{ zoomPercent }}%</span>
                <IconButton icon="plus" label="Zoom in — Ctrl+=" @click="zoomIn" />
                <IconButton icon="fit" label="Fit — Ctrl+0" @click="fitView" />
            </OriSurface>
        </template>

        <!-- Overlay: one card per terminal/pending phase — error, judging, result. -->
        <template #overlay>
            <OriSurface v-if="phase === 'error'" class="play__notice" role="alert">
                <h2 class="play__notice-title">{{ needsAuth ? 'Sign in to duel' : 'Can’t start the duel' }}</h2>
                <p class="play__notice-msg">{{ errorMsg }}</p>
                <RouterLink v-if="needsAuth" class="play__notice-link" to="/draw">
                    Go to the draw page to sign in →
                </RouterLink>
                <OriButton v-else text="Try again" variant="fill" color="primary" radius="md" @click="startMatch" />
            </OriSurface>
            <JudgingOverlay v-else-if="phase === 'judging' || phase === 'submitting'" :opponent-name="opponent.name" />
            <ResultReveal v-else-if="phase === 'done' && result" :result="result" @play-again="playAgain" />
        </template>
    </EditorShell>
</template>

<style scoped>
/* Drop the prompt banner below the fixed RoundTimerBar clock chip AND below the
   top-corner islands (opponent chip / submit) so nothing collides on a narrow
   phone where all three top zones crowd the centre. The shell's top-center
   region is pointer-events:none; the banner stays that way (drawing passes
   through), so nothing here re-enables events. */
.play__prompt {
    padding-top: 2.5rem;
    pointer-events: none;
}

/* The shell's bottom-center strip is pointer-events:none — the toolbar opts back
   in so drawing passes through the empty flanks either side of it. */
.play__toolbar-item {
    pointer-events: auto;
}

/* Zoom island — same compact chrome as /draw's (the shell's bottom-right region
   positions it). Mirrored (not shared) since it's island visuals, not shell
   layout. */
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

.play__notice-link {
    color: var(--ori-color-primary);

    font-size: var(--ori-font-size_sm, 0.9rem);
    font-weight: 700;
    text-decoration: none;
}

.play__notice-link:hover {
    text-decoration: underline;
}
</style>
