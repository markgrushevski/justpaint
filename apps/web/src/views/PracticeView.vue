<script lang="ts" setup>
/**
 * PracticeView — single-player practice (`/practice`), the duel's little sibling:
 * the same EditorShell, toolbar, judged canvas and judge as /play, minus
 * matchmaking, the opponent chip, the WS socket and the reveal (docs/GAME.md §10).
 *
 * No round timer: a duel counts down because two players wait on each other;
 * practice has nobody to be fair to. The prompt is never redacted either.
 *
 * Flow: fetch a prompt, draw, submit — the server renders the raster and asks
 * a vision model synchronously for a score plus feedback. The PNG captured at
 * submit is advisory only, the thumbnail beside the feedback, never what was
 * judged (docs/GAME.md §6).
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
import EditorShell from '../components/shell/EditorShell.vue'
import FloatingToolbar, { TOOL_META } from '../components/FloatingToolbar.vue'
import IconButton from '../components/ui/IconButton.vue'
import GamePromptBanner from '../components/game/GamePromptBanner.vue'
import JudgingOverlay from '../components/game/JudgingOverlay.vue'
import PracticeResult from '../components/game/PracticeResult.vue'
import SubmitButton from '../components/game/SubmitButton.vue'

// Practice draws on the same 1080² canvas a duel does (docs/GAME.md §2),
// judged the same way, so scores stay comparable.
const GAME_CANVAS = 1080

const shell = ref<{ canvasEl: HTMLDivElement | null } | null>(null)
let editor: Editor | null = null
let unsubscribe: (() => void) | null = null

// Konva can't read CSS custom properties; the cursor ring gets the resolved
// --ori-color-primary through the oriui token bridge, kept right across a
// theme flip mid-session.
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

// Gates Submit: a blank canvas scores 0 and burns one of a finite number of
// daily judge calls to tell the player what they already know.
const hasStrokes = ref(false)

function syncEditorState() {
    if (!editor) return
    canUndo.value = editor.canUndo()
    canRedo.value = editor.canRedo()
    zoom.value = editor.getZoom()
    hasStrokes.value = editor.getLayers().some((l) => l.strokeCount > 0)
}

/**
 * Phases:
 *  loading  → waiting on a prompt (or on the sign-in modal in front of it)
 *  drawing  → prompt on screen, canvas live
 *  judging  → the submit POST is in flight; slow by nature (server render + a
 *             vision model, in-request), so this is a real state, not a spinner
 *             on a button
 *  done     → the score + feedback card
 *  error    → there is no usable prompt, so there is nothing to draw
 */
type Phase = 'loading' | 'drawing' | 'judging' | 'done' | 'error'
const phase = ref<Phase>('loading')

// Set true on unmount; every async continuation checks it before touching state.
let disposed = false

const prompt = ref<PracticePrompt | null>(null)
const run = ref<PracticeRun | null>(null)

const loadError = ref('')

// Not a phase: unlike a duel, nothing is lost when the judge can't be
// reached — the drawing and prompt are both still there — so this is a
// dismissible notice over a live `drawing` phase, not an error screen.
const submitError = ref('')
// A refusal the player can't retry: a spent daily budget. The notice drops
// "Try again" for it. isBudgetExhausted, not isRateLimited: the per-IP write
// tier also answers 429 (docs/API.md §3.1) but clears in seconds, so treating
// every 429 as a spent day would take away a retry that would have worked.
const submitExhausted = ref(false)

/** Object URL of the advisory raster captured at submit — revoked on reset/unmount. */
const drawingImage = ref<string | null>(null)

const submitting = computed(() => phase.value === 'judging')
const canSubmit = computed(() => phase.value === 'drawing' && hasStrokes.value && prompt.value !== null)
// A disabled button that never says why is the frustrating half of the guard.
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

// Nothing to draw is terminal until the player retries, so a lapsed session
// recovers through the shared gate and resumes into a fresh fetch.
function handlePromptError(err: unknown): void {
    if (isAuthError(err)) {
        void (async () => {
            const signedIn = await gate.ensure('Sign in to practice.')
            if (disposed) return
            if (signedIn) void loadPrompt()
            else toLoadError('Sign in to practice.')
        })()
        return
    }
    toLoadError(toApiError(err)?.message ?? 'Could not get a prompt. Try again.')
}

// Drops back onto the canvas with a notice, never an error screen: the
// drawing and the prompt both survive.
function handleSubmitError(err: unknown): void {
    phase.value = 'drawing'
    if (isAuthError(err)) {
        // Don't re-submit once signed back in: minutes can pass behind the
        // modal, a submit spends one of a finite number of daily judge calls,
        // and the drawing is untouched with the Submit button right there.
        submitExhausted.value = false
        submitError.value = 'Your session expired — sign in, then submit again.'
        void (async () => {
            const signedIn = await gate.ensure('Sign in to have your drawing judged.')
            if (disposed || !signedIn) return
            dismissSubmitError()
        })()
        return
    }
    const api = toApiError(err)
    submitExhausted.value = isBudgetExhausted(err)
    if (submitExhausted.value) {
        submitError.value = api?.message ?? 'That is every scored drawing you get today.'
        return
    }
    // A transient throttle clears in seconds, so say that waiting will help —
    // the server's own wording for it is just "too many requests".
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
    // Snapshot before the capture await, so the thumbnail and the judged
    // document are the same instant of the canvas.
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
    void loadPrompt()
}

function dismissSubmitError(): void {
    submitError.value = ''
    submitExhausted.value = false
}

function playDuel(): void {
    void router.push('/play')
}

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
    // which would otherwise submit into the session being replaced.
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

    // Practice is judged and recorded, so it needs a session. `ensure` waits
    // for the store's cookie restore, then raises the one shared sign-in
    // modal; signing in drops straight into a prompt.
    const signedIn = await gate.ensure('Sign in to practice.')
    if (disposed) return
    if (!signedIn) {
        // "Try again" re-fetches, which re-raises this same gate on the next
        // 401 — always a way back in.
        toLoadError('Sign in to practice.')
        return
    }
    void loadPrompt()
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
    <!-- Same shared shell as /draw and /play. `mode="play"` is a layout choice,
         not a claim to be a duel: the variant with no drawer toggler, so the
         top-right corner belongs to Submit. -->
    <EditorShell ref="shell" mode="play">
        <template #top-left>
            <!-- Shell is pixel-identical to /play; without this there's no way
                 to tell a practice run from a duel. -->
            <OriBadge content="Practice" color="primary" variant="tonal" label="Practice mode" />
        </template>

        <!-- Banner mounts only once a prompt exists — its unrevealed state is
             duel copy ("Waiting for opponent…") and would be a lie here. The
             hint sits under the prompt, not the bottom-left readout corner,
             because the toolbar's full-width strip overlaps that corner on a phone. -->
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

        <!-- The submit notice comes last because it rides on top of a live
             `drawing` phase, not a terminal state of its own. -->
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
                    <!-- The drawing is untouched either way, so "Keep drawing" is
                         always offered; a spent budget drops the retry instead. -->
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
/* Stays pointer-events:none like the region itself. With no round timer above
   it, the banner sits right at the top edge — /play's 2.5rem offset exists to
   clear its clock chip, which would just be dead space here. */
.practice__prompt {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--ori-size-gap_sm, 0.25rem);

    pointer-events: none;
}

/* The shell's bottom-center strip is pointer-events:none — the toolbar opts back
   in so drawing passes through the empty flanks either side of it. */
.practice__toolbar-item {
    pointer-events: auto;
}

/* Quiet on purpose: no surface chrome, so it reads as a note on the desk
   rather than another island. */
.practice__hint {
    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_xs, 0.75rem);

    /* Only ever shown on an empty canvas, so the ground under it is always
       paper or desk, never a stroke; 0.7 keeps it past WCAG AA on both. */
    opacity: 0.7;
    pointer-events: none;
    user-select: none;
}

/* Same compact chrome as /draw and /play; mirrored, not shared, since it's
   island visuals, not shell layout. */
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

/* Prompt-fetch spinner + the notice cards — centred in the shell's
   pointer-events:none overlay, so each opts back in. */
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

/* The banner is a centered box that grows with the prompt (up to 34rem), so its
   right edge reaches under the Submit button on a narrow viewport — measured
   overlap at 375px, a long prompt still colliding at ~768px. Breakpoint = 900px:
   at 900px a full-width banner clears Submit by ~60px, at 768px it does not. */
@media (width <= 900px) {
    .practice__prompt {
        padding-top: calc(var(--ori-size-action_md, 2.75rem) + var(--ori-size-gap_sm, 0.25rem));
    }
}
</style>
