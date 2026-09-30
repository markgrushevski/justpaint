<script lang="ts" setup>
/**
 * Single-player practice (`/practice`): /play's shell, canvas and judge without
 * matchmaking, the socket or a timer (docs/GAME.md §10). The server renders the raster
 * and scores it synchronously; the PNG captured here is only the result thumbnail.
 */
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { OriBadge, OriButton, OriSpinner, OriSurface } from '@oriui/vue'
import { useThemeColor } from '@oriui/headless/vue'
import { DEFAULT_STYLE, DOC_VERSION, Editor, newId, TOOLS } from '@justpaint/editor'
import type { Document, ToolId } from '@justpaint/editor'
import {
    icons,
    isAuthError,
    isBudgetExhausted,
    isRateLimited,
    toApiError,
    useAuthGate,
    usePracticePrompt,
    useSubmitPractice
} from '@core'
import type { PracticePrompt, PracticeRun } from '@core'
import EditorShell from '../editor/EditorShell.vue'
import FloatingToolbar, { TOOL_META } from '../editor/FloatingToolbar.vue'
import IconButton from '../../components/ui/IconButton.vue'
import GamePromptBanner from '../game/GamePromptBanner.vue'
import JudgingOverlay from '../game/JudgingOverlay.vue'
import PracticeResult from './PracticeResult.vue'
import SubmitButton from '../game/SubmitButton.vue'

// The duel's canvas (docs/GAME.md §2), so scores stay comparable.
const GAME_CANVAS = 1080

const shell = ref<{ canvasEl: HTMLDivElement | null } | null>(null)
let editor: Editor | null = null
let unsubscribe: (() => void) | null = null

// Konva can't read CSS variables (same wiring as DrawView).
const cursorRingColor = useThemeColor('primary')
watch(cursorRingColor, (color) => editor?.setCursorColor(color || null))

const gate = useAuthGate()
const router = useRouter()
const fetchPrompt = usePracticePrompt()
const submitRun = useSubmitPractice()

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

// Gates Submit: a blank canvas would spend a daily judge call on a 0.
const hasStrokes = ref(false)

function syncEditorState() {
    if (!editor) return
    canUndo.value = editor.canUndo()
    canRedo.value = editor.canRedo()
    zoom.value = editor.getZoom()
    hasStrokes.value = editor.getLayers().some((l) => l.strokeCount > 0)
}

/**
 * `loading` (a prompt, or the sign-in modal before it), `drawing`, `judging` (the submit
 * POST: a server render plus a model call, slow enough to be a state), `done`, and
 * `error` (no usable prompt).
 */
type Phase = 'loading' | 'drawing' | 'judging' | 'done' | 'error'
const phase = ref<Phase>('loading')

// Set true on unmount; every async continuation checks it before touching state.
let disposed = false

const prompt = ref<PracticePrompt | null>(null)
const run = ref<PracticeRun | null>(null)

const loadError = ref('')

// Not a phase: a failed submit loses nothing, so it is a notice over `drawing`.
const submitError = ref('')
// A spent daily budget drops "Try again". Not isRateLimited: the per-IP 429 clears in
// seconds (docs/API.md §3.1).
const submitExhausted = ref(false)

/** Object URL of the thumbnail captured at submit, revoked on reset and unmount. */
const drawingImage = ref<string | null>(null)

const submitting = computed(() => phase.value === 'judging')
const canSubmit = computed(() => phase.value === 'drawing' && hasStrokes.value && prompt.value !== null)
// Says why Submit is disabled.
const showEmptyHint = computed(() => phase.value === 'drawing' && !hasStrokes.value)

function revokeDrawingImage(): void {
    if (drawingImage.value) {
        URL.revokeObjectURL(drawingImage.value)
        drawingImage.value = null
    }
}

function toLoadError(msg: string): void {
    loadError.value = msg
    phase.value = 'error'
}

// The result card's thumbnail only; the judged raster is rendered server-side.
async function captureDrawing(): Promise<string | null> {
    if (!editor) return null
    try {
        const doc = editor.getDocument()
        const blob = await editor.toPNG({ outWidth: doc.width, outHeight: doc.height, fit: 'contain' })
        return URL.createObjectURL(blob)
    } catch {
        return null
    }
}

// Without a prompt there is nothing to draw, so a lapsed session signs in and refetches.
function handlePromptError(err: unknown): void {
    if (isAuthError(err)) {
        gate.ensure('Sign in to practice.').then((signedIn) => {
            if (!disposed) {
                if (signedIn) loadPrompt()
                else toLoadError('Sign in to practice.')
            }
        })
        return
    }
    toLoadError(toApiError(err)?.message ?? 'Could not get a prompt. Try again.')
}

// Back onto the canvas with a notice: the drawing and the prompt survive.
function handleSubmitError(err: unknown): void {
    phase.value = 'drawing'
    if (isAuthError(err)) {
        // Don't re-submit after sign-in: minutes may pass, and a submit spends a daily
        // judge call.
        submitExhausted.value = false
        submitError.value = 'Your session expired — sign in, then submit again.'
        gate.ensure('Sign in to have your drawing judged.').then((signedIn) => {
            if (disposed || !signedIn) return
            dismissSubmitError()
        })
        return
    }
    const api = toApiError(err)
    submitExhausted.value = isBudgetExhausted(err)
    if (submitExhausted.value) {
        submitError.value = api?.message ?? 'That is every scored drawing you get today.'
        return
    }
    // The server only says "too many requests"; say that waiting will help.
    submitError.value = isRateLimited(err)
        ? `${api?.message ?? 'Too many requests just now'} — try again in a moment.`
        : (api?.message ?? 'The judge could not be reached. Try again.')
}

async function loadPrompt(): Promise<void> {
    phase.value = 'loading'
    loadError.value = ''
    submitError.value = ''
    submitExhausted.value = false
    try {
        const next = await fetchPrompt.mutateAsync()
        if (disposed) return
        prompt.value = next
        phase.value = 'drawing'
    } catch (err) {
        if (disposed) return
        handlePromptError(err)
    }
}

async function submit(): Promise<void> {
    const target = prompt.value
    if (!canSubmit.value || !target || !editor) return
    // Snapshot before the await, so the thumbnail and the judged document match.
    const doc = editor.getDocument()
    phase.value = 'judging'
    submitError.value = ''
    submitExhausted.value = false
    revokeDrawingImage()
    drawingImage.value = await captureDrawing()
    if (disposed) return
    try {
        const scored = await submitRun.mutateAsync({ promptId: target.id, document: doc })
        if (disposed) return
        run.value = scored
        phase.value = 'done'
    } catch (err) {
        if (disposed) return
        handleSubmitError(err)
    }
}

function drawAgain(): void {
    revokeDrawingImage()
    run.value = null
    editor?.loadDocument(blankGameDocument())
    syncEditorState()
    phase.value = 'drawing'
}

function newPrompt(): void {
    revokeDrawingImage()
    run.value = null
    prompt.value = null
    editor?.loadDocument(blankGameDocument())
    syncEditorState()
    loadPrompt()
}

function dismissSubmitError(): void {
    submitError.value = ''
    submitExhausted.value = false
}

function playDuel(): void {
    router.push('/play')
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
    const signedIn = await gate.ensure('Sign in to practice.')
    if (disposed) return
    if (!signedIn) {
        toLoadError('Sign in to practice.')
        return
    }
    loadPrompt()
})

onBeforeUnmount(() => {
    disposed = true
    window.removeEventListener('keydown', onKeydown)
    revokeDrawingImage()
    unsubscribe?.()
    unsubscribe = null
    editor?.destroy()
    editor = null
})
</script>

<template>
    <!-- `mode="play"` is the layout without a drawer toggle, so Submit gets the corner. -->
    <EditorShell ref="shell" mode="play">
        <template #top-left>
            <!-- The shell is identical to /play; this tells a practice run apart. -->
            <OriBadge content="Practice" color="primary" variant="tonal" label="Practice mode" />
        </template>

        <!-- The banner waits for a prompt: its unrevealed state is duel copy. The hint
             sits here because the toolbar covers the bottom-left corner on a phone. -->
        <template #top-center>
            <div class="practice__prompt">
                <GamePromptBanner v-if="prompt" :prompt="prompt.text" revealed solo />
                <span v-if="showEmptyHint" class="practice__hint" role="status">Draw something to submit it</span>
            </div>
        </template>

        <template #top-right>
            <SubmitButton solo :disabled="!canSubmit" :loading="submitting" @submit="submit" />
        </template>

        <template #bottom-center>
            <FloatingToolbar
                class="practice__toolbar-item"
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
            <OriSurface class="practice__zoom" role="group" aria-label="Zoom">
                <IconButton icon="minus" label="Zoom out — Ctrl+-" @click="zoomOut" />
                <span class="practice__zoom-value">{{ zoomPercent }}%</span>
                <IconButton icon="plus" label="Zoom in — Ctrl+=" @click="zoomIn" />
                <IconButton icon="fit" label="Fit — Ctrl+0" @click="fitView" />
            </OriSurface>
        </template>

        <!-- The submit notice is last: it rides over a live `drawing` phase. -->
        <template #overlay>
            <OriSurface v-if="phase === 'error'" class="practice__notice" role="alert">
                <h2 class="practice__notice-title">Nothing to draw yet</h2>
                <p class="practice__notice-msg">{{ loadError }}</p>
                <OriButton text="Try again" variant="fill" color="primary" radius="md" @click="loadPrompt" />
            </OriSurface>

            <OriSurface v-else-if="phase === 'loading'" class="practice__loading" role="status">
                <OriSpinner size="lg" color="primary" />
                <span class="practice__loading-text">Finding you something to draw…</span>
            </OriSurface>

            <JudgingOverlay v-else-if="phase === 'judging'" solo />

            <PracticeResult
                v-else-if="phase === 'done' && run"
                :score="run.score"
                :feedback="run.feedback"
                :prompt="run.prompt.text"
                :image="drawingImage"
                @draw-again="drawAgain"
                @new-prompt="newPrompt"
                @play-duel="playDuel"
            />

            <OriSurface v-else-if="submitError" class="practice__notice" role="alert">
                <h2 class="practice__notice-title">
                    {{ submitExhausted ? 'That’s your judging for today' : 'The judge didn’t answer' }}
                </h2>
                <p class="practice__notice-msg">{{ submitError }}</p>
                <div class="practice__notice-actions">
                    <!-- The drawing is untouched, so "Keep drawing" is always offered. -->
                    <OriButton
                        v-if="!submitExhausted"
                        class="practice__notice-action"
                        text="Try again"
                        variant="fill"
                        color="primary"
                        radius="md"
                        fluid
                        @click="submit"
                    />
                    <OriButton
                        v-else
                        class="practice__notice-action"
                        text="Leaderboard"
                        variant="outline"
                        color="surface"
                        radius="md"
                        fluid
                        :icon="icons.podium"
                        icon-position="left"
                        @click="viewLeaderboard"
                    />
                    <OriButton
                        class="practice__notice-action"
                        text="Keep drawing"
                        variant="outline"
                        color="surface"
                        radius="md"
                        fluid
                        @click="dismissSubmitError"
                    />
                </div>
            </OriSurface>
        </template>
    </EditorShell>
</template>

<style scoped>
/* Passive like its region; with no timer above it, no /play-style offset. */
.practice__prompt {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--ori-size-gap_sm, 0.25rem);

    pointer-events: none;
}

/* The shell's strip is pointer-events:none; the toolbar opts back in. */
.practice__toolbar-item {
    pointer-events: auto;
}

/* No surface chrome: a note on the desk, not another island. */
.practice__hint {
    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_xs, 0.75rem);

    /* Shown only over paper or desk, where 0.7 still passes WCAG AA. */
    opacity: 0.7;
    pointer-events: none;
    user-select: none;
}

/* Mirrors /draw's zoom island: island visuals, not shell layout. */
.practice__zoom {
    display: flex;
    align-items: center;
    gap: 0;

    padding: var(--ori-size-gap_xs, 0.125rem) var(--ori-size-gap_sm, 0.25rem);
}

.practice__zoom-value {
    min-width: 3.1rem;
    padding: 0.25rem;

    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_sm, 0.85rem);
    font-variant-numeric: tabular-nums;
    text-align: center;
}

/* The spinner and the notices opt back into pointer events in the passive overlay. */
.practice__loading {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--ori-size-gap_sm, 0.25rem);

    padding: var(--ori-size-gap_lg, 0.75rem) var(--ori-size-gap_xl, 1rem);

    pointer-events: auto;
}

.practice__loading-text {
    font-size: var(--ori-font-size_sm, 0.9rem);
    opacity: 0.8;
}

.practice__notice {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--ori-size-gap_sm, 0.25rem);

    width: min(92vw, 24rem);
    padding: var(--ori-size-gap_lg, 0.75rem) var(--ori-size-gap_xl, 1rem) var(--ori-size-gap_xl, 1rem);

    pointer-events: auto;
    text-align: center;
}

.practice__notice-title {
    margin: 0;

    font-size: var(--ori-font-size_lg, 1.15rem);
    font-weight: 800;
    letter-spacing: -0.01em;
}

.practice__notice-msg {
    margin: 0 0 var(--ori-size-gap_sm, 0.25rem);

    font-size: var(--ori-font-size_sm, 0.9rem);
    overflow-wrap: anywhere;
    opacity: 0.8;
}

.practice__notice-actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--ori-size-gap_sm, 0.25rem);

    width: 100%;
}

.practice__notice-action {
    flex: 1 1 8rem;
}

/* The banner grows with the prompt (up to 34rem) and reaches under Submit on narrow
   viewports; from 900px up a full-width banner clears it. */
@media (width <= 900px) {
    .practice__prompt {
        padding-top: calc(var(--ori-size-action_md, 2.75rem) + var(--ori-size-gap_sm, 0.25rem));
    }
}
</style>
