<script lang="ts" setup>
/**
 * The free editor (`/draw`): layers, file actions, and the AI features that need a
 * canvas without a clock (assist, "what did I draw?").
 */
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { OriButton, OriInput, OriMenu, OriSurface, OriToaster } from '@oriui/vue'
import { LIMITS } from '@justpaint/editor'
import ConfirmDialog from '../../components/ConfirmDialog.vue'
import ModeNav from '../../components/ModeNav.vue'
import ToolIcon from '../../components/icons/ToolIcon.vue'
import IconButton from '../../components/ui/IconButton.vue'
import EditorShell from '../editor/EditorShell.vue'
import FloatingToolbar from '../editor/FloatingToolbar.vue'
import ShortcutsDialog from '../editor/ShortcutsDialog.vue'
import ZoomControls from '../editor/ZoomControls.vue'
import { useBackdrop } from '../editor/useBackdrop'
import { useEditorHost } from '../editor/useEditorHost'
import { useLeaveGuard } from '../editor/useLeaveGuard'
import GuessResult from './GuessResult.vue'
import LayersPanel from './LayersPanel.vue'
import SaveDialog from './SaveDialog.vue'
import SideMenu from './SideMenu.vue'
import WelcomeOverlay from './WelcomeOverlay.vue'
import { useAssistPanel } from './useAssistPanel'
import { useCanvasCoords } from './useCanvasCoords'
import { fittedDocument, useDrawingFile } from './useDrawingFile'
import { useGatedActions } from './useGatedActions'
import { useGuessPanel } from './useGuessPanel'

const MAX_LAYERS = LIMITS.maxLayers

const AI_ITEMS = [
    { value: 'assist', label: 'Draw with AI' },
    { value: 'guess', label: 'Guess my drawing' }
]

const {
    shell,
    editor,
    ui,
    layers,
    activeLayerId,
    canUndo,
    canRedo,
    historyMark,
    zoomPercent,
    docWidth,
    docHeight,
    background,
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
    initialDocument: (canvas) => fittedDocument(canvas),
    // Ctrl+S would otherwise open the browser's own save dialog.
    commands: { s: () => file.save() },
    beforeToolKeys: onPlainKey
})
const canvas = () => shell.value?.canvasEl ?? null

const route = useRoute()
const router = useRouter()

const menuOpen = ref(false)
const shortcutsOpen = ref(false)
const layersOpen = ref(false)

// Shown by the Layers toggle while the panel is closed, since new strokes land there.
const activeLayerName = computed(() => layers.value.find((l) => l.id === activeLayerId.value)?.name ?? '')

const { toaster, gated, reportError } = useGatedActions({
    // The cheat-sheet's focus trap would fight the sign-in dialog.
    onSessionExpired: () => (shortcutsOpen.value = false)
})
const assist = reactive(useAssistPanel(editor, gated, reportError))
const guess = reactive(useGuessPanel(editor, isEmpty, gated, reportError))
const file = reactive(
    useDrawingFile({
        editor,
        canvas,
        isEmpty,
        historyMark,
        load,
        toPNG,
        gated,
        reportError,
        toaster,
        // The ghost and the guess describe the outgoing drawing.
        onReplace: () => {
            assist.clear()
            guess.invalidate()
        }
    })
)
const backdrop = reactive(useBackdrop(editor))
const { coords } = useCanvasCoords(editor, canvas)

const aiActive = computed(() => assist.open || guess.open)

function onAi(value: string) {
    if (value === 'assist' && !assist.open) assist.toggle()
    if (value === 'guess' && !guess.open) guess.toggle()
}

// The welcome is gone from the first touch of the canvas until the next visit. Panels
// that sit where it draws hide it while they are open.
const welcomeGone = ref(false)
const showWelcome = computed(
    () => !welcomeGone.value && isEmpty.value && !guess.open && !assist.open && !menuOpen.value && !layersOpen.value
)
watch(isEmpty, (empty) => {
    if (!empty) welcomeGone.value = true
})

function dismissWelcome() {
    welcomeGone.value = true
}

const {
    pending: leavePending,
    leave,
    stay
} = useLeaveGuard(() =>
    file.dirty
        ? {
              title: 'Leave without saving?',
              message: 'This drawing has changes that aren’t saved.',
              confirmText: 'Leave'
          }
        : null
)

onMounted(() => {
    canvas()?.addEventListener('pointerdown', dismissWelcome, { once: true })
    // The gallery opens a drawing as /draw?id=…; the id leaves the URL once taken. Open
    // only after the replace settles: every navigation releases the sign-in modal
    // (main.ts), so a modal raised before it would close at once.
    const id = route.query.id
    if (typeof id === 'string' && id) router.replace({ query: {} }).then(() => file.open(id))
})

// The side menu is non-modal, so it does not suppress single keys; only modal overlays do.
function onPlainKey(e: KeyboardEvent): boolean {
    // The menu first, since its own Esc never fires while focus is on the canvas; then
    // the cheat-sheet, then the guess card.
    if (e.key === 'Escape') {
        if (menuOpen.value) menuOpen.value = false
        else if (shortcutsOpen.value) shortcutsOpen.value = false
        else if (guess.open) guess.dismiss()
        return true
    }
    // Desktop only: phones have no keyboard to need the cheat-sheet.
    if (e.key === '?') {
        if (window.innerWidth <= 600 || file.confirmOpen || file.nameOpen || leavePending.value !== null) return true
        e.preventDefault()
        shortcutsOpen.value = !shortcutsOpen.value
        return true
    }
    // Every modal overlay must be listed here, or tool hotkeys fire underneath it.
    return shortcutsOpen.value || file.confirmOpen || file.nameOpen || leavePending.value !== null
}

function addLayer() {
    editor.value?.addLayer()
}
function selectLayer(id: string) {
    editor.value?.setActiveLayer(id)
}
function removeLayer(id: string) {
    editor.value?.removeLayer(id)
}
function moveLayer(id: string, toIndex: number) {
    editor.value?.moveLayer(id, toIndex)
}
function toggleLayerVisible(id: string, visible: boolean) {
    editor.value?.setLayerVisible(id, visible)
}
function setLayerOpacity(id: string, opacity: number) {
    editor.value?.setLayerOpacity(id, opacity)
}
function renameLayer(id: string, name: string) {
    editor.value?.renameLayer(id, name)
}
function setBackground(color: string | null) {
    editor.value?.setBackground(color)
}

// Tear down a pending AI ghost; the editor host destroys the stage after this.
onBeforeUnmount(() => assist.clear())
</script>

<template>
    <EditorShell ref="shell" mode="draw">
        <template #top-left>
            <ModeNav :collapse-below="720" />
        </template>

        <!-- Flips from input to accept/reject while a proposal is pending. -->
        <template #top-center>
            <OriSurface v-if="assist.open" class="draw__assist" role="group" aria-label="AI assist">
                <!-- The AI menu can be off-screen on narrow widths. -->
                <div class="draw__assist-head">
                    <span class="draw__assist-title">Draw with AI</span>
                    <IconButton icon="close" label="Close AI assist" placement="bottom" @click="assist.toggle" />
                </div>
                <template v-if="assist.pendingOps">
                    <p v-if="assist.note" class="draw__assist-note">{{ assist.note }}</p>
                    <div class="draw__assist-actions">
                        <OriButton
                            variant="solid"
                            radius="md"
                            :label="isEmpty ? 'Accept' : 'Add on top'"
                            fluid
                            @click="assist.accept('add')"
                        />
                        <OriButton
                            v-if="!isEmpty"
                            variant="outline"
                            radius="md"
                            label="Replace drawing"
                            fluid
                            @click="assist.accept('replace')"
                        />
                        <OriButton variant="outline" radius="md" label="Reject" fluid @click="assist.reject" />
                    </div>
                </template>
                <template v-else>
                    <div class="draw__assist-row">
                        <OriInput
                            v-model="assist.prompt"
                            class="draw__assist-input"
                            aria-label="Describe what to draw"
                            placeholder="Describe what to draw…"
                            :disabled="assist.pending"
                            @keydown.enter="assist.submit"
                        />
                        <OriButton
                            variant="solid"
                            radius="md"
                            label="Draw"
                            :loading="assist.pending"
                            :disabled="!assist.prompt.trim() || assist.pending"
                            @click="assist.submit"
                        />
                    </div>
                </template>
            </OriSurface>
        </template>

        <template #top-right>
            <OriSurface class="draw__actions">
                <IconButton
                    icon="layers"
                    label="Layers"
                    placement="bottom"
                    :pressed="layersOpen"
                    @click="layersOpen = !layersOpen"
                />
                <!-- Only worth the room once there is a second layer to be on. -->
                <button
                    v-if="!layersOpen && layers.length > 1"
                    class="draw__active-layer"
                    type="button"
                    :aria-label="`Active layer: ${activeLayerName}. Open layers panel`"
                    :title="`Active layer: ${activeLayerName} — click to open layers`"
                    @click="layersOpen = true"
                >
                    {{ activeLayerName }}
                </button>
                <OriMenu :items="AI_ITEMS" placement="bottom-end" @select="onAi">
                    <template #trigger="{ props: trigger }">
                        <OriButton
                            v-bind="trigger"
                            class="draw__ai"
                            variant="text"
                            :color="aiActive ? 'primary' : 'surface'"
                            :active="aiActive"
                            radius="md"
                        >
                            <ToolIcon name="assist" />
                            <span>AI</span>
                        </OriButton>
                    </template>
                    <template #item="{ item }">
                        <span class="draw__ai-item">
                            <ToolIcon :name="item.value === 'assist' ? 'assist' : 'guess'" />
                            {{ item.label }}
                        </span>
                    </template>
                </OriMenu>
            </OriSurface>
            <!-- Loud only while there is something to lose. -->
            <OriButton
                class="draw__save"
                label="Save"
                :variant="file.dirty ? 'solid' : 'soft'"
                color="primary"
                radius="md"
                :loading="file.busy"
                @click="file.save"
            />
        </template>

        <template #bottom-center>
            <FloatingToolbar
                class="draw__toolbar-item"
                :active-tool="ui.activeTool"
                :color="ui.color"
                :stroke-width="ui.strokeWidth"
                :fill-enabled="ui.fillEnabled"
                :fill="ui.fill"
                :can-undo="canUndo"
                :can-redo="canRedo"
                ink-view
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

        <template #bottom-left>
            <OriSurface v-if="coords" class="draw__coords">
                <span class="draw__coords-mark" aria-hidden="true">⌖</span>
                <span class="draw__coords-value">{{ Math.round(coords.x) }}, {{ Math.round(coords.y) }}</span>
            </OriSurface>
        </template>

        <template #overlay>
            <OriToaster position="top-center" align="center" />

            <Transition name="jp-fade">
                <WelcomeOverlay v-if="showWelcome" @start="dismissWelcome" />
            </Transition>

            <Transition name="jp-pop">
                <GuessResult
                    v-if="guess.open"
                    class="draw__guess"
                    :status="guess.status"
                    :label="guess.result?.label ?? null"
                    :confidence="guess.result?.confidence ?? 0"
                    :alternatives="guess.result?.alternatives ?? []"
                    :error="guess.error"
                    :exhausted="guess.exhausted"
                    :can-retry="!isEmpty"
                    @again="guess.request"
                    @dismiss="guess.dismiss"
                />
            </Transition>

            <ShortcutsDialog :open="shortcutsOpen" @close="shortcutsOpen = false" />

            <ConfirmDialog
                :open="file.confirmOpen"
                title="Clear the canvas?"
                message="This starts a new drawing and can't be undone."
                confirm-text="Clear"
                cancel-text="Cancel"
                discard
                @confirm="file.confirmNew"
                @cancel="file.cancelNew"
            />

            <SaveDialog :open="file.nameOpen" :initial="file.name" @save="file.confirmName" @cancel="file.cancelName" />

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

        <!-- Self-teleports to body; non-modal, canvas stays live. -->
        <template #drawer>
            <SideMenu
                :open="menuOpen"
                :busy="file.busy"
                :title="file.savedName"
                :backdrop-grid="backdrop.grid"
                :canvas-width="docWidth"
                :canvas-height="docHeight"
                :background="background"
                @close="menuOpen = false"
                @new-drawing="file.requestNew"
                @save="file.save"
                @export-png="file.exportPng"
                @copy-text="file.copyJson"
                @copy-image="file.copyPng"
                @shortcuts="shortcutsOpen = true"
                @toggle-grid="backdrop.setGrid"
                @apply-canvas-size="file.applyCanvasSize"
                @set-background="setBackground"
            />
        </template>

        <!-- Self-positioned chrome in the shell's default slot. The toggle sits above the
             menu panel, so the same chip closes it. -->
        <OriSurface class="draw__menu-toggle">
            <IconButton
                :icon="menuOpen ? 'close' : 'menu'"
                :label="menuOpen ? 'Close menu' : 'Open menu'"
                placement="bottom"
                :pressed="menuOpen"
                @click="menuOpen = !menuOpen"
            />
        </OriSurface>

        <!-- Phones only: the toolbar hides its history group <=600px. -->
        <OriSurface class="draw__history" role="group" aria-label="History">
            <IconButton icon="undo" label="Undo" :disabled="!canUndo" @click="undo" />
            <IconButton icon="redo" label="Redo" :disabled="!canRedo" @click="redo" />
        </OriSurface>

        <!-- Scrim behind the mobile layers bottom sheet (display:none >600px) -->
        <div v-if="layersOpen" class="draw__layers-scrim" @click="layersOpen = false"></div>

        <!-- Layers: a dropdown under the top-right island (desktop), a bottom sheet (phones) -->
        <div v-show="layersOpen" class="draw__layers">
            <LayersPanel
                :layers="layers"
                :active-layer-id="activeLayerId"
                :can-add="layers.length < MAX_LAYERS"
                @add="addLayer"
                @select="selectLayer"
                @remove="removeLayer"
                @move="moveLayer"
                @toggle-visible="toggleLayerVisible"
                @set-opacity="setLayerOpacity"
                @rename="renameLayer"
                @close="layersOpen = false"
            />
        </div>
    </EditorShell>
</template>

<style scoped>
.draw__menu-toggle {
    position: absolute;
    top: var(--ori-size-gap_md, 0.5rem);
    right: var(--ori-size-gap_md, 0.5rem);
    z-index: 110;

    padding: var(--ori-size-gap_xs, 0.125rem);
}

.draw__actions {
    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_xs, 0.125rem);

    padding: var(--ori-size-gap_xs, 0.125rem) var(--ori-size-gap_sm, 0.25rem);
}

.draw__ai {
    gap: var(--ori-size-gap_sm, 0.25rem);

    font-weight: 700;
}

.draw__ai-item {
    display: inline-flex;
    align-items: center;
    gap: var(--ori-size-gap_md, 0.5rem);
}

.draw__save {
    align-self: stretch;
}

/* Neutral structural hover only, never a brand-role mix (docs/DESIGN-SYSTEM.md §1). */
.draw__active-layer {
    max-width: 8rem;
    padding: 0.15rem 0.4rem;

    border: none;
    border-radius: var(--ori-size-radius_sm, 4px);
    background: transparent;
    color: var(--ori-color-on-surface);

    font-family: inherit;
    font-size: var(--ori-font-size_sm, 0.85rem);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;

    cursor: pointer;
}

.draw__active-layer:hover {
    background-color: var(--jp-neutral-hover-bg, color-mix(in srgb, var(--ori-color-on-surface) 8%, transparent));
}

/* The shell's strips are pointer-events:none, so chrome in them opts back in. */
.draw__toolbar-item {
    pointer-events: auto;
}

/* Clamped so it never spills past a phone's viewport. */
.draw__assist {
    pointer-events: auto;

    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_sm, 0.25rem);

    width: min(30rem, calc(100vw - 1.5rem));
    padding: var(--ori-size-gap_sm, 0.25rem);
}

.draw__assist-row {
    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_sm, 0.25rem);
}

.draw__assist-input {
    flex: 1;
    min-width: 0;
}

.draw__assist-actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--ori-size-gap_sm, 0.25rem);
}

.draw__assist-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--ori-size-gap_sm, 0.25rem);
}

.draw__assist-title {
    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_sm, 0.85rem);
    font-weight: 600;
}

.draw__assist-note {
    margin: 0;
    padding: var(--ori-size-gap_xs, 0.125rem) var(--ori-size-gap_sm, 0.25rem);

    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_sm, 0.85rem);
}

/* The card sizes itself; /draw only places it. On a small screen the lift clears the
   ~4rem toolbar. */
.draw__guess {
    --jp-guess-lift: 5rem;
}

/* The welcome leaves with a plain fade: it is a whole layer, not a card. */
.jp-fade-leave-active {
    transition: opacity 0.25s ease-out;
}

.jp-fade-leave-to {
    opacity: 0;
}

@media (prefers-reduced-motion: reduce) {
    .jp-fade-leave-active {
        transition: none;
    }
}

/* A plain fade and scale; the toast's slide reads wrong on a centered card. */
.jp-pop-enter-active,
.jp-pop-leave-active {
    transition:
        opacity 0.18s ease-out,
        transform 0.18s ease-out;
}

.jp-pop-enter-from,
.jp-pop-leave-to {
    opacity: 0;
    transform: scale(0.96);
}

/* A passive readout that never intercepts drawing. */
.draw__coords {
    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_xs, 0.125rem);

    padding: 0.2rem 0.5rem;

    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_xs, 0.75rem);
    font-variant-numeric: tabular-nums;

    opacity: 0.7;
    pointer-events: none;
    user-select: none;
}

.draw__coords-mark {
    font-size: 0.9em;
    opacity: 0.8;
}

/* Hidden by default; the <=600px block shows it. */
.draw__history {
    position: absolute;

    /* The second row: the top row's islands can span a narrow phone. 3.125rem is the
       50px top-row island height. */
    top: calc(var(--ori-size-gap_md, 0.5rem) * 2 + 3.125rem);
    left: var(--ori-size-gap_md, 0.5rem);
    /* Under the top row, so the mode menu drops over it. */
    z-index: 9;

    display: none;
    align-items: center;
    gap: 0;

    padding: var(--ori-size-gap_xs, 0.125rem) var(--ori-size-gap_sm, 0.25rem);
}

/* The wrapper stretches the panel to its clamped height so its own list scrolls. */
.draw__layers {
    position: absolute;
    top: calc(
        var(--ori-size-gap_md, 0.5rem) + var(--ori-size-action_md, 2.75rem) + var(--ori-size-gap_sm, 0.25rem) + 0.35rem
    );
    right: var(--ori-size-gap_md, 0.5rem);
    z-index: 9;

    display: flex;
    align-items: stretch;

    width: 16rem;
    max-height: calc(100dvh - 10rem);
}

/* Hidden by default; the <=600px block shows it. */
.draw__layers-scrim {
    position: fixed;
    inset: 0;
    z-index: 59;

    display: none;

    background-color: rgb(0 0 0 / 35%);
}

/* Below ~1050px the centered 30rem assist panel reaches the actions island, which
   holds its only outside close, so it drops to its own full-width row. */
@media (width <= 1050px) {
    .draw__assist {
        position: absolute;
        /* The region is already offset by gap_md: this is .draw__history's row. */
        top: calc(var(--ori-size-gap_md, 0.5rem) + 3.125rem);
        left: var(--ori-size-gap_md, 0.5rem);
        right: var(--ori-size-gap_md, 0.5rem);

        width: auto;
    }
}

/* Height too: on a landscape phone a full answer ends pixels above the toolbar. */
@media (width <= 600px), (height <= 500px) {
    .draw__guess {
        margin-bottom: var(--jp-guess-lift);
    }
}

@media (width <= 600px) {
    /* The history island takes the second row, so the assist panel drops one more. */
    .draw__assist {
        top: calc(var(--ori-size-gap_md, 0.5rem) * 2 + 3.125rem * 2);
    }

    .draw__layers-scrim {
        display: block;
    }

    /* A full-width bottom sheet. The wrapper carries the sheet chrome, so the panel's
       rounded bottom corners can't notch the screen edge. */
    .draw__layers {
        position: fixed;
        inset: auto 0 0;
        z-index: 60;

        width: 100%;
        max-height: 60dvh;
        overflow: hidden;

        border-radius: var(--ori-size-radius_lg, 12px) var(--ori-size-radius_lg, 12px) 0 0;
        background-color: var(--ori-color-surface);
    }

    .draw__history {
        display: flex;
    }

    /* The menu holds Save on a phone, where the top row has no room for it. */
    .draw__save {
        display: none;
    }

    /* No hover on touch, so nothing to track. */
    .draw__coords {
        display: none;
    }
}
</style>
