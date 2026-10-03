<script lang="ts" setup>
/**
 * Single-player practice (`/practice`): /play's shell, canvas and judge without
 * matchmaking, the socket or a timer (docs/GAME.md §10). The server renders the raster
 * and scores it synchronously; the PNG captured here is only the result thumbnail.
 */
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { OriButton, OriSpinner } from '@oriui/vue'
import { blankDocument } from '@justpaint/editor'
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
import ConfirmDialog from '../../components/ConfirmDialog.vue'
import ModeNav from '../../components/ModeNav.vue'
import IslandSurface from '../../components/ui/IslandSurface.vue'
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
import PracticeResult from './PracticeResult.vue'

const gate = useAuthGate()
const router = useRouter()
const fetchPrompt = usePracticePrompt()
const submitRun = useSubmitPractice()

const blankGameDocument = () => blankDocument(GAME_CANVAS, GAME_CANVAS)

const {
    shell,
    editor,
    ui,
    canUndo,
    canRedo,
    zoomPercent,
    isEmpty,
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
// A blank canvas would spend a daily judge call on a 0.
const canSubmit = computed(() => phase.value === 'drawing' && !isEmpty.value && prompt.value !== null)
// Says why Submit is disabled.
const showEmptyHint = computed(() => phase.value === 'drawing' && isEmpty.value)

// Nothing keeps an unsubmitted practice drawing, and a run being judged has already
// spent one of the day's scored drawings.
const {
    pending: leavePending,
    leave,
    stay
} = useLeaveGuard(() => {
    if (phase.value === 'drawing' && !isEmpty.value)
        return {
            title: 'Leave practice?',
            message: 'This drawing isn’t kept unless you submit it.',
            confirmText: 'Leave'
        }
    if (phase.value === 'judging')
        return {
            title: 'Leave before the score?',
            message: 'The judge is still scoring this drawing. It counts toward today’s limit either way.',
            confirmText: 'Leave'
        }
    return null
})

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
    try {
        const blob = await toPNG()
        return blob && URL.createObjectURL(blob)
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
    if (!canSubmit.value || !target || !editor.value) return
    // Snapshot before the await, so the thumbnail and the judged document match.
    const doc = editor.value.getDocument()
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
    load(blankGameDocument())
    phase.value = 'drawing'
}

function newPrompt(): void {
    revokeDrawingImage()
    run.value = null
    prompt.value = null
    load(blankGameDocument())
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

onMounted(async () => {
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
    revokeDrawingImage()
})
</script>

<template>
    <!-- `mode="play"` is the layout without a drawer toggle, so Submit gets the corner. -->
    <EditorShell ref="shell" mode="play">
        <template #top-left>
            <ModeNav :collapse-below="1200" />
        </template>

        <!-- The banner waits for a prompt: its unrevealed state is duel copy. The hint
             sits here because the toolbar covers the bottom-left corner on a phone. -->
        <template #top-center>
            <div class="practice__prompt">
                <GamePromptBanner v-if="prompt" :prompt="prompt.text" revealed solo :large="showEmptyHint" />
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
            <ZoomControls :percent="zoomPercent" @zoom-in="zoomIn" @zoom-out="zoomOut" @fit="fitView" />
        </template>

        <!-- The submit notice is last: it rides over a live `drawing` phase. -->
        <template #overlay>
            <IslandSurface v-if="phase === 'error'" class="practice__notice" role="alert" elevation="lg">
                <h2 class="practice__notice-title">Nothing to draw yet</h2>
                <p class="practice__notice-msg">{{ loadError }}</p>
                <OriButton label="Try again" variant="solid" color="primary" radius="md" @click="loadPrompt" />
            </IslandSurface>

            <IslandSurface v-else-if="phase === 'loading'" class="practice__loading" role="status" elevation="md">
                <OriSpinner size="lg" color="primary" />
                <span class="practice__loading-text">Finding you something to draw…</span>
            </IslandSurface>

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

            <IslandSurface v-else-if="submitError" class="practice__notice" role="alert" elevation="lg">
                <h2 class="practice__notice-title">
                    {{ submitExhausted ? 'That’s your judging for today' : 'The judge didn’t answer' }}
                </h2>
                <p class="practice__notice-msg">{{ submitError }}</p>
                <div class="practice__notice-actions">
                    <!-- The drawing is untouched, so "Keep drawing" is always offered. -->
                    <OriButton
                        v-if="!submitExhausted"
                        class="practice__notice-action"
                        label="Try again"
                        variant="solid"
                        color="primary"
                        radius="md"
                        fluid
                        @click="submit"
                    />
                    <OriButton
                        v-else
                        class="practice__notice-action"
                        label="Leaderboard"
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
                        label="Keep drawing"
                        variant="outline"
                        color="surface"
                        radius="md"
                        fluid
                        @click="dismissSubmitError"
                    />
                </div>
            </IslandSurface>

            <ConfirmDialog
                :open="leavePending !== null"
                :title="leavePending?.title ?? ''"
                :message="leavePending?.message"
                :confirm-text="leavePending?.confirmText"
                cancel-text="Stay"
                discard
                @confirm="leave"
                @cancel="stay"
            />
        </template>
    </EditorShell>
</template>

<style scoped>
/* Passive like its region; with no timer above it, no /play-style offset. The gap clears
   the dealt card's tilted corner. */
.practice__prompt {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--ori-size-gap_lg, 0.75rem);

    pointer-events: none;
}

/* The shell's strip is pointer-events:none; the toolbar opts back in. */
.practice__toolbar-item {
    pointer-events: auto;
}

/* On its own surface chip: the sheet under it is white in both themes, so bare
   theme ink would vanish in the dark one. */
.practice__hint {
    padding: var(--ori-size-gap_xs, 0.125rem) var(--ori-size-gap_md, 0.5rem);

    border-radius: var(--ori-size-radius_sm, 4px);
    background-color: var(--ori-color-surface);
    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_xs, 0.75rem);

    pointer-events: none;
    user-select: none;
}

/* The spinner and the notices opt back into pointer events in the passive overlay. */
.practice__loading {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--ori-size-gap_md, 0.5rem);

    padding: var(--ori-size-gap_lg, 0.75rem) var(--ori-size-gap_xl, 1rem);

    pointer-events: auto;
}

.practice__loading-text {
    font-size: var(--ori-font-size_sm, 0.9rem);
    opacity: var(--jp-dim, 0.8);
}

.practice__notice {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--ori-size-gap_sm, 0.25rem);

    width: min(92vw, 24rem);
    padding: var(--ori-size-gap_xl, 1rem);

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
    margin: 0 0 var(--ori-size-gap_lg, 0.75rem);

    font-size: var(--ori-font-size_sm, 0.9rem);
    overflow-wrap: anywhere;
    opacity: var(--jp-dim, 0.8);
}

.practice__notice-actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--ori-size-gap_md, 0.5rem);

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
