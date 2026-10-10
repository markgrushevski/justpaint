<script lang="ts" setup>
/**
 * The AI-judged duel (`/play`) on the shared EditorShell. `useDuel` runs the round; this
 * view owns the editor, the countdown, the draft and what the player reads.
 */
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useRouter } from 'vue-router'
import { OriBadge, OriButton } from '@oriui/vue'
import { useQueryClient } from '@tanstack/vue-query'
import { blankDocument, renderToPNG } from '@justpaint/editor'
import { useSessionStore, matches, icons, leaderboardKeys } from '@core'
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
import { clearDrafts, loadDraft, saveDraft } from './drafts'
import { opponentChip, toDuelResult } from './duel'
import type { DuelEnd, DuelPhase, DuelResult } from './duel'
import OpponentStatusChip from './OpponentStatusChip.vue'
import ResultReveal from './ResultReveal.vue'
import RoundTimerBar from './RoundTimerBar.vue'
import { useDuel } from './useDuel'
import { useRoundClock } from './useRoundClock'

// Early enough to land before the server's deadline, where a submit is a 409.
const AUTO_SUBMIT_MARGIN_MS = 1500

// How long the canvas rests before the draft is written.
const DRAFT_SAVE_MS = 1000

const session = useSessionStore()
const router = useRouter()
const queryClient = useQueryClient()

const blankGameDocument = () => blankDocument(GAME_CANVAS, GAME_CANVAS)

const duel = useDuel()
const state = duel.state
const phase = computed(() => state.value.phase)

const {
    shell,
    editor,
    ui,
    canUndo,
    canRedo,
    historyMark,
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
    load
} = useEditorHost({
    initialDocument: blankGameDocument,
    commands: { enter: () => submit() },
    // Tool keys only while drawing.
    beforeToolKeys: () => phase.value !== 'drawing' || leavePending.value !== null
})
useBackdrop(editor, { judged: true })

// The round runs on after a player leaves it (docs/GAME.md §4.1); leaving the queue
// cancels the open match.
const {
    pending: leavePending,
    leave,
    stay
} = useLeaveGuard(() => {
    if (phase.value === 'drawing' || phase.value === 'submitting')
        return {
            title: 'Leave the duel?',
            message:
                'The round keeps running. Come back before time runs out to finish, or your opponent wins if they submit.',
            confirmText: 'Leave'
        }
    if (phase.value === 'waiting')
        return {
            title: 'Leave the queue?',
            message: 'You’ll stop looking for an opponent.',
            confirmText: 'Leave'
        }
    return null
})

function confirmLeave(): void {
    duel.leaveQueue()
    leave()
}

// The canvas takes strokes only while they can still be sent.
const locked = computed(() => phase.value !== 'drawing')

const prompt = computed(() => state.value.prompt)
const promptRevealed = computed(() => prompt.value !== '')
// The prompt reads large until the first stroke.
const promptLarge = computed(() => promptRevealed.value && phase.value === 'drawing' && isEmpty.value)
// After a queue nobody joined there is no prompt to show and nobody to wait for.
const showBanner = computed(() => phase.value !== 'ended' || promptRevealed.value)

const chip = computed(() => opponentChip(state.value))
const opponentName = computed(() => chip.value?.name ?? state.value.opponent?.name ?? 'Player 2')

// The clock runs while the round does, for both players, and is armed only while drawing.
const TICKING = new Set<DuelPhase>(['drawing', 'submitting', 'submitted'])
const clock = useRoundClock({
    marginMs: AUTO_SUBMIT_MARGIN_MS,
    // A failed auto-submit retries each tick while time remains.
    armed: () => phase.value === 'drawing' && clock.remaining.value > 0 && !duel.signingIn.value,
    onDeadline: () => submit()
})
const { deadlineMs, totalSeconds: roundTotalSeconds, remaining } = clock
const showTimer = computed(() => deadlineMs.value !== null && TICKING.has(phase.value))

watch(
    () => state.value.clock,
    (c) => (c ? clock.anchor(c.drawingDeadline, c.serverTime) : clock.reset())
)

const canSubmit = computed(() => phase.value === 'drawing')

function submit(): void {
    const ed = editor.value
    const id = state.value.matchId
    if (!ed || id === null || phase.value !== 'drawing') return
    // A stroke still under the pointer when time runs out belongs to the drawing.
    ed.finishStroke()
    const doc = ed.getDocument()
    saveDraft(id, doc)
    duel.submit(doc)
}

// --- the draft: kept while drawing, restored on a resume, dropped when the match is over

let pendingDraft: { id: string; timer: number } | null = null

function cancelDraft(): void {
    if (pendingDraft) clearTimeout(pendingDraft.timer)
    pendingDraft = null
}

function flushDraft(): void {
    const pending = pendingDraft
    cancelDraft()
    if (pending && editor.value) saveDraft(pending.id, editor.value.getDocument())
}

watch(historyMark, () => {
    const id = state.value.matchId
    if (id === null || phase.value !== 'drawing') return
    cancelDraft()
    pendingDraft = { id, timer: window.setTimeout(flushDraft, DRAFT_SAVE_MS) }
})

// A new match id: a resume brings its draft back, anything else starts blank.
watch(
    () => state.value.matchId,
    (id) => {
        flushDraft()
        if (id !== null) clearDrafts(id)
        load((id !== null && loadDraft(id, GAME_CANVAS)) || blankGameDocument())
    }
)

// Over for good, unlike a sign-in the player declined or a refusal "Try again" can resume.
const MATCH_OVER = new Set<DuelEnd>(['queue-ended', 'nobody-submitted', 'left', 'gone'])

watch(phase, (p) => {
    if (TICKING.has(p)) clock.start()
    else clock.stop()
    if (p === 'done' || (p === 'ended' && state.value.end !== null && MATCH_OVER.has(state.value.end))) {
        cancelDraft()
        clearDrafts()
    }
})

function onPageHide(): void {
    const id = state.value.matchId
    if (id !== null && phase.value === 'drawing' && editor.value) saveDraft(id, editor.value.getDocument())
}

// --- the result

const result = shallowRef<DuelResult | null>(null)
// Both drawings as the server holds them (docs/API.md §8), so a second tab shows what
// was actually sent.
const images = ref<{ you: string | null; opponent: string | null }>({ you: null, opponent: null })
const imagesLoading = ref(false)
let imageUrls: string[] = []
let imageGeneration = 0

function clearImages(): void {
    imageGeneration += 1
    for (const url of imageUrls) URL.revokeObjectURL(url)
    imageUrls = []
    images.value = { you: null, opponent: null }
    imagesLoading.value = false
}

async function renderDrawing(matchId: string, userId: string | null): Promise<string | null> {
    if (userId === null) return null
    try {
        const doc = await matches.playerDrawing(matchId, userId)
        const blob = await renderToPNG(doc, { outWidth: doc.width, outHeight: doc.height, fit: 'contain' })
        return URL.createObjectURL(blob)
    } catch {
        return null
    }
}

async function loadImages(matchId: string, view: DuelResult): Promise<void> {
    clearImages()
    const gen = imageGeneration
    imagesLoading.value = true
    const [you, opponent] = await Promise.all([
        view.you.drew ? renderDrawing(matchId, view.you.userId) : null,
        view.opponent.drew ? renderDrawing(matchId, view.opponent.userId) : null
    ])
    const urls = [you, opponent].filter((url): url is string => url !== null)
    if (gen !== imageGeneration) {
        for (const url of urls) URL.revokeObjectURL(url)
        return
    }
    imageUrls = urls
    images.value = { you, opponent }
    imagesLoading.value = false
}

watch(
    () => state.value.result,
    (r) => {
        if (!r) {
            result.value = null
            clearImages()
            return
        }
        const view = toDuelResult(r, state.value.me, session.user?.rating ?? 1200)
        result.value = view
        // The session store only refreshes user.rating at login or restore.
        if (session.user && view.rating) session.user.rating = view.rating.after
        queryClient.invalidateQueries({ queryKey: leaderboardKeys.all })
        if (state.value.matchId !== null) loadImages(state.value.matchId, view)
    }
)

// --- what the player reads

const END_COPY: Record<DuelEnd, { title: string; message: string }> = {
    'queue-ended': { title: 'Nobody joined', message: 'No one was free to duel just now.' },
    left: { title: 'You left the queue', message: 'No one is waiting on you.' },
    'nobody-submitted': {
        title: 'Time ran out',
        message: 'Neither drawing was sent in time, so the round doesn’t count.'
    },
    auth: { title: 'Sign in to play', message: 'Duels are for signed-in players.' },
    budget: { title: 'That’s your duels for today', message: 'New ones unlock as the day rolls over.' },
    gone: { title: 'This duel is over', message: 'It can’t be opened any more.' },
    failed: { title: 'Something went wrong', message: 'The duel couldn’t go on.' }
}

const ending = computed(() => (state.value.end ? END_COPY[state.value.end] : null))
const endsInQueue = computed(() => state.value.end === 'queue-ended' || state.value.end === 'left')

// Polite and always mounted, so a change of text is read out.
const announcement = computed(() => {
    const s = state.value
    switch (s.phase) {
        case 'done':
            return result.value?.headline ?? ''
        case 'judging':
            return 'Both drawings are in. The judge is scoring them.'
        case 'submitted':
            return `Your drawing is in. Waiting for ${opponentName.value}.`
        case 'drawing':
            return s.opponent?.submitted ? `${opponentName.value} submitted their drawing.` : ''
        default:
            return ''
    }
})

function playAgain(): void {
    duel.start()
}

function practiceInstead(): void {
    duel.leaveQueue()
    router.push('/practice')
}

function viewLeaderboard(): void {
    router.push('/leaderboard')
}

onMounted(() => {
    window.addEventListener('pagehide', onPageHide)
    duel.begin()
})

onBeforeUnmount(() => {
    window.removeEventListener('pagehide', onPageHide)
    if (phase.value === 'drawing') onPageHide()
    cancelDraft()
    clock.stop()
    clearImages()
})
</script>

<template>
    <EditorShell ref="shell" mode="play" :class="{ 'play--locked': locked }">
        <template #top-left>
            <!-- Two rows, so the opponent never reaches the prompt centered on the first. -->
            <div class="play__top-left">
                <ModeNav :collapse-below="1200" />
                <div v-if="chip || state.offline" class="play__opponent">
                    <!-- Display name or "Player 2", never a login. -->
                    <OpponentStatusChip v-if="chip" :name="chip.name" :status="chip.status" />
                    <OriBadge
                        v-if="state.offline"
                        content="reconnecting…"
                        color="warning"
                        variant="soft"
                        aria-label="Reconnecting to the match"
                    />
                </div>
            </div>
        </template>

        <template #top-center>
            <!-- Hidden until the deadline exists, rather than a misleading 0:00. -->
            <RoundTimerBar v-if="showTimer" :remaining="remaining" :total="roundTotalSeconds" />
            <div v-if="showBanner" class="play__prompt">
                <GamePromptBanner :prompt="prompt" :revealed="promptRevealed" :large="promptLarge" />
            </div>
        </template>

        <!-- No drawer: SideMenu is /draw-specific; leaving goes through ModeNav. -->
        <template #top-right>
            <div class="play__submit">
                <SubmitButton :disabled="!canSubmit" :loading="phase === 'submitting'" @submit="submit" />
                <OriBadge
                    v-if="state.submitFailed && phase === 'drawing'"
                    role="alert"
                    content="Couldn’t send — retry"
                    color="danger"
                    variant="soft"
                />
            </div>
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
            <IslandSurface v-if="phase === 'waiting'" class="play__notice" elevation="md">
                <p class="play__notice-msg">The prompt appears when someone joins.</p>
                <OriButton label="Practice instead" variant="outline" radius="md" @click="practiceInstead" />
            </IslandSurface>
            <IslandSurface v-else-if="phase === 'ended' && ending" class="play__notice" role="alert" elevation="lg">
                <h2 class="play__notice-title">{{ ending.title }}</h2>
                <p class="play__notice-msg">{{ ending.message }}</p>
                <div class="play__notice-actions">
                    <template v-if="state.end === 'budget'">
                        <OriButton
                            label="Leaderboard"
                            variant="outline"
                            radius="md"
                            :icon="icons.podium"
                            icon-position="start"
                            @click="viewLeaderboard"
                        />
                    </template>
                    <template v-else-if="state.end === 'auth'">
                        <OriButton label="Sign in" variant="solid" color="primary" radius="md" @click="duel.begin()" />
                    </template>
                    <template v-else-if="endsInQueue">
                        <OriButton label="Queue again" variant="solid" color="primary" radius="md" @click="playAgain" />
                        <OriButton label="Practice instead" variant="outline" radius="md" @click="practiceInstead" />
                    </template>
                    <OriButton
                        v-else
                        :label="state.end === 'failed' ? 'Try again' : 'Play again'"
                        variant="solid"
                        color="primary"
                        radius="md"
                        @click="playAgain"
                    />
                </div>
            </IslandSurface>
            <JudgingOverlay
                v-else-if="phase === 'submitted' || phase === 'judging'"
                :opponent-name="opponentName"
                :waiting-for="phase === 'submitted' ? opponentName : undefined"
            />
            <ResultReveal
                v-else-if="phase === 'done' && result"
                :result="result"
                :images="images"
                :loading="imagesLoading"
                @play-again="playAgain"
                @view-leaderboard="viewLeaderboard"
            />

            <ConfirmDialog
                :open="leavePending !== null"
                :title="leavePending?.title ?? ''"
                :message="leavePending?.message"
                :confirm-text="leavePending?.confirmText"
                cancel-text="Stay"
                discard
                @confirm="confirmLeave"
                @cancel="stay"
            />
        </template>

        <p class="jp-sr-only" role="status">{{ announcement }}</p>
    </EditorShell>
</template>

<style scoped>
.play__top-left {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: var(--ori-size-gap_md, 0.5rem);
}

.play__opponent {
    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_md, 0.5rem);
}

/* Clears the timer chip and the corner islands on a narrow phone. */
.play__prompt {
    padding-top: 2.5rem;
    pointer-events: none;
}

.play__submit {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: var(--ori-size-gap_sm, 0.25rem);
}

/* The shell's strip is pointer-events:none; the toolbar opts back in. */
.play__toolbar-item {
    pointer-events: auto;
}

/* Strokes outside a live round would be lost or, worse, sent; a drag there must not
   select the notice text either. */
.play--locked {
    user-select: none;
}

.play--locked :deep(.shell__canvas) {
    pointer-events: none;
}

/* Opts back into pointer events inside the shell's passive overlay. */
.play__notice {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--ori-size-gap_sm, 0.25rem);

    width: min(92vw, 24rem);
    padding: var(--ori-size-gap_xl, 1rem);

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
    margin: 0 0 var(--ori-size-gap_lg, 0.75rem);

    font-size: var(--ori-font-size_sm, 0.9rem);
    opacity: var(--jp-dim, 0.8);
}

.play__notice-actions {
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    gap: var(--ori-size-gap_md, 0.5rem);
}
</style>
