<script lang="ts" setup>
/**
 * PlayView — the AI-judged drawing duel (`/play`). It composes the SAME shared
 * EditorShell that /draw uses (DECISIONS 2026-07-04: one design, game chrome on
 * top), mounting the real Editor into the shell's exposed canvas element exactly
 * as DrawView does — but on the square 1080² duel canvas (GAME.md §2) and with
 * game chrome (round timer, prompt banner, opponent status, submit, judging +
 * result overlays) filling the regions instead of /draw's file/layer chrome.
 *
 * SCAFFOLD: the match loop is driven by LOCAL MOCK STATE (a small phase machine
 * + a ticking countdown), so /play is fully explorable with no server. Every
 * place a real create/poll/submit/result call will swap in is marked
 * `// TODO(play-api):`. The live /api/matches client is the NEXT unit.
 */
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { OriTooltip } from '@oriui/vue'
import { Editor, TOOLS, DEFAULT_STYLE, newId } from '@justpaint/editor'
import type { ToolId } from '@justpaint/editor'
import type { Document } from '@justpaint/document'
import { DOC_VERSION, parseDocument } from '@justpaint/document'
import { useThemeColor } from '@oriui/headless/vue'
import { useSessionStore } from '@core'
import EditorShell from '../components/shell/EditorShell.vue'
import FloatingToolbar, { TOOL_META } from '../components/FloatingToolbar.vue'
import ToolIcon from '../components/icons/ToolIcon.vue'
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

/** How long a round lasts (mock). TODO(play-api): comes from the match record. */
const ROUND_SECONDS = 90

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

/* --- mock match state machine ------------------------------------------ */

/**
 * Phases (superset of GAME.md's states for the client view):
 *  waiting    → roster filling; prompt redacted (GAME.md `open`)
 *  drawing    → prompt revealed; timer running (GAME.md `drawing`)
 *  submitting → submit clicked; capturing the raster (transient)
 *  judging    → rendered + awaiting the judge (GAME.md `judging`)
 *  done       → result revealed (GAME.md `done`)
 */
type Phase = 'waiting' | 'drawing' | 'submitting' | 'judging' | 'done'
const phase = ref<Phase>('waiting')

// TODO(play-api): the prompt is delivered by the server only once the match
// enters `drawing` (GAME.md §5 reveal timing) — do not ship it to the client
// while waiting. Hardcoded here for the scaffold.
const prompt = ref('a lighthouse at night')
const promptRevealed = computed(() => phase.value !== 'waiting')

// The opponent — a safe display label + coarse status (NEVER a login; GAME.md
// §4.2). TODO(play-api): comes from match_players / WS room events.
const opponent = reactive<{ name: string; status: OpponentStatus }>({
    name: 'Player 2',
    status: 'drawing'
})

const remaining = ref(ROUND_SECONDS)
const submitting = computed(() => phase.value === 'submitting')
const canSubmit = computed(() => phase.value === 'drawing')

const result = ref<DuelResult | null>(null)
// Object URL of the player's captured raster — revoked on reset/unmount.
let youImageUrl: string | null = null

// Timers we own and must clear on reset/unmount.
let tick: number | null = null
const timeouts = new Set<number>()

function later(fn: () => void, ms: number): void {
    const id = window.setTimeout(() => {
        timeouts.delete(id)
        fn()
    }, ms)
    timeouts.add(id)
}

function clearTimers(): void {
    if (tick !== null) {
        clearInterval(tick)
        tick = null
    }
    for (const id of timeouts) clearTimeout(id)
    timeouts.clear()
}

function startCountdown(): void {
    if (tick !== null) clearInterval(tick)
    tick = window.setInterval(() => {
        remaining.value = Math.max(0, remaining.value - 1)
        // TODO(play-api): the server is the authoritative clock; a real round
        // reconciles against match state rather than trusting this interval.
        if (remaining.value <= 0) {
            if (tick !== null) clearInterval(tick)
            tick = null
            void submit() // out of time — auto-submit whatever is on the canvas
        }
    }, 1000)
}

/** Enter the drawing phase: reveal the prompt, reset + start the clock. */
function beginDrawing(): void {
    phase.value = 'drawing'
    opponent.status = 'drawing'
    remaining.value = ROUND_SECONDS
    startCountdown()
    // Mock: the opponent locks in partway through so the status chip visibly
    // changes. TODO(play-api): driven by an "opponent submitted" room event.
    later(() => {
        if (phase.value === 'drawing') opponent.status = 'submitted'
    }, 6500)
}

/** Capture the player's drawing as a PNG object URL (advisory preview only). */
async function captureYourRaster(): Promise<string | null> {
    if (!editor) return null
    try {
        const doc = editor.getDocument()
        // Client PNG is advisory (GAME.md §6) — the authoritative judged raster
        // is rendered server-side. This is only for the reveal preview.
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

async function submit(): Promise<void> {
    if (phase.value !== 'drawing') return
    phase.value = 'submitting'
    if (tick !== null) {
        clearInterval(tick)
        tick = null
    }
    // TODO(play-api): POST /api/matches/:id/submit with the vector document.
    // The server validates (1080² check), persists, renders the authoritative
    // raster, and flips the match to `judging` when the last slot is stamped.
    revokeYourRaster()
    youImageUrl = await captureYourRaster()

    phase.value = 'judging'
    opponent.status = 'judging'
    // TODO(play-api): poll GET /api/matches/:id (or await a WS `result` event)
    // instead of this fixed delay; then map the server result into DuelResult.
    later(finishJudging, 2200)
}

function finishJudging(): void {
    // TODO(play-api): replace this mock with the real judged result — scores,
    // winner (mapped from the judge's positional A/B per GAME.md §7.1), reason,
    // and the Elo delta computed server-side.
    result.value = {
        you: { score: 78, image: youImageUrl },
        opponent: { name: opponent.name, score: 64, image: null },
        winner: 'you',
        reason: 'Your lighthouse reads clearly against the night sky — a stronger silhouette and better contrast than the opponent’s.',
        eloDelta: 15,
        ratingBefore: 1200
    }
    phase.value = 'done'
}

function playAgain(): void {
    // TODO(play-api): create/join a fresh match instead of resetting locally.
    clearTimers()
    revokeYourRaster()
    result.value = null
    editor?.loadDocument(parseDocument(blankGameDocument()))
    syncEditorState()
    beginDrawing()
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
    if (phase.value !== 'drawing' && phase.value !== 'waiting') return
    const tool = KEY_TO_TOOL.get(key)
    if (tool) {
        e.preventDefault()
        pickTool(tool)
    }
}

/* --- lifecycle --------------------------------------------------------- */

onMounted(() => {
    void session.fetchMe() // restore an existing cookie session, if any
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

    // Kick off the mock reveal: brief "waiting for opponent", then the roster
    // fills and the prompt is revealed. TODO(play-api): replace with create +
    // poll/subscribe until the match enters `drawing`.
    later(beginDrawing, 1400)
})

onBeforeUnmount(() => {
    window.removeEventListener('keydown', onKeydown)
    clearTimers()
    revokeYourRaster()
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
            <RoundTimerBar :remaining="remaining" :total="ROUND_SECONDS" />
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
            <div class="play__zoom jp-float" role="group" aria-label="Zoom">
                <OriTooltip placement="top" content="Zoom out — Ctrl+-">
                    <button class="play__zoom-btn" type="button" aria-label="Zoom out" @click="zoomOut">
                        <ToolIcon name="minus" />
                    </button>
                </OriTooltip>
                <span class="play__zoom-value">{{ zoomPercent }}%</span>
                <OriTooltip placement="top" content="Zoom in — Ctrl+=">
                    <button class="play__zoom-btn" type="button" aria-label="Zoom in" @click="zoomIn">
                        <ToolIcon name="plus" />
                    </button>
                </OriTooltip>
                <OriTooltip placement="top" content="Fit — Ctrl+0">
                    <button class="play__zoom-btn" type="button" aria-label="Fit to view" @click="fitView">
                        <ToolIcon name="fit" />
                    </button>
                </OriTooltip>
            </div>
        </template>

        <!-- Overlay: judging skeleton, then the result reveal — one per phase. -->
        <template #overlay>
            <JudgingOverlay v-if="phase === 'judging' || phase === 'submitting'" :opponent-name="opponent.name" />
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

.play__zoom-btn {
    display: grid;
    place-items: center;

    width: var(--jp-control-sm, 2.25rem);
    height: var(--jp-control-sm, 2.25rem);
    padding: 0;

    border: none;
    border-radius: var(--ori-size-radius_md, 8px);
    background: transparent;
    color: var(--ori-color-on-surface);

    font-size: 1.05rem;
    cursor: pointer;
}

.play__zoom-btn:hover {
    background-color: var(--jp-hover-bg, color-mix(in srgb, var(--ori-color-primary) 12%, transparent));
}

.play__zoom-value {
    min-width: 3.1rem;
    padding: 0.25rem;

    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_sm, 0.85rem);
    font-variant-numeric: tabular-nums;
    text-align: center;
}
</style>
