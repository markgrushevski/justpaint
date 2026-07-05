<script lang="ts" setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { OriButton } from '@oriui/vue'
import { Editor, TOOLS, DEFAULT_STYLE, newId } from '@justpaint/editor'
import type { ToolId, LayerView } from '@justpaint/editor'
import type { Document } from '@justpaint/document'
import { DEFAULT_BACKGROUND, DEFAULT_CANVAS, DOC_VERSION, LIMITS, parseDocument } from '@justpaint/document'
import { isAuthError, toApiError, useLoadLatestDrawing, useSaveDrawing, useSessionStore, useThemeStore } from '@core'
import FloatingToolbar, { TOOL_META } from '../components/FloatingToolbar.vue'
import LayersPanel from '../components/LayersPanel.vue'
import ShortcutsDialog from '../components/ShortcutsDialog.vue'
import SideMenu from '../components/SideMenu.vue'
import ToolIcon from '../components/icons/ToolIcon.vue'
import type { IconName } from '../components/icons/ToolIcon.vue'

const containerRef = ref<HTMLDivElement | null>(null)
let editor: Editor | null = null
let unsubscribe: (() => void) | null = null

type MessageSeverity = 'info' | 'success' | 'error'

/** Id of the drawing currently open (set after a successful save/load). */
const currentId = ref<string | null>(null)
const message = ref<{ text: string; severity: MessageSeverity } | null>(null)

// Server data goes through TanStack Query: save/load are mutations, so the view
// gets `isPending`/`error` + cache invalidation without hand-rolled busy flags.
const saveMutation = useSaveDrawing()
const loadMutation = useLoadLatestDrawing()
const busy = computed(() => saveMutation.isPending.value || loadMutation.isPending.value)

const ui = reactive({
    activeTool: 'pen' as ToolId,
    color: DEFAULT_STYLE.color,
    strokeWidth: DEFAULT_STYLE.strokeWidth,
    fillEnabled: DEFAULT_STYLE.fill !== null,
    fill: DEFAULT_STYLE.fill ?? '#ffffff'
})

// Editor-derived state, kept in sync via the editor's onChange subscription so
// Vue re-renders the toolbar (undo/redo enablement), the layers panel, and zoom.
const layers = ref<LayerView[]>([])
const activeLayerId = ref('')
const canUndo = ref(false)
const canRedo = ref(false)
const zoom = ref(1)
const zoomPercent = computed(() => Math.round(zoom.value * 100))
const MAX_LAYERS = LIMITS.maxLayers

const session = useSessionStore()
const theme = useThemeStore()

/* --- shell chrome state ---------------------------------------------- */

const menuOpen = ref(false)
const shortcutsOpen = ref(false)
// Layers start open where there's room, closed on small screens. Match the CSS
// reflow breakpoint (601px+ has room for the island).
const layersOpen = ref(window.innerWidth > 600) // oriui --ori-size-screen_xs (600px)

const THEME_ICON: Record<string, IconName> = { auto: 'monitor', light: 'sun', dark: 'moon' }
const themeIcon = computed(() => THEME_ICON[theme.mode] ?? 'monitor')
const themeTitle = computed(() => `Theme: ${theme.mode} (click to switch)`)

// Status messages self-dismiss; duration scales with severity so errors stay
// readable longer than successes: success 3.5s, info 5s, error 8s.
const MESSAGE_DURATION: Record<MessageSeverity, number> = {
    success: 3500,
    info: 5000,
    error: 8000
}
let messageTimer: ReturnType<typeof setTimeout> | null = null
watch(message, (m) => {
    if (messageTimer) clearTimeout(messageTimer)
    if (m) messageTimer = setTimeout(() => (message.value = null), MESSAGE_DURATION[m.severity])
})

function blankDocument(): Document {
    return {
        version: DOC_VERSION,
        width: DEFAULT_CANVAS.width,
        height: DEFAULT_CANVAS.height,
        background: DEFAULT_BACKGROUND,
        layers: [{ id: newId(), name: 'Layer 1', visible: true, opacity: 1, strokes: [] }]
    }
}

function syncEditorState() {
    if (!editor) return
    layers.value = editor.getLayers()
    activeLayerId.value = editor.getActiveLayerId()
    canUndo.value = editor.canUndo()
    canRedo.value = editor.canRedo()
    zoom.value = editor.getZoom()
}

onMounted(() => {
    void session.fetchMe() // restore an existing cookie session, if any
    if (!containerRef.value) return
    // The editor sizes its Konva stage to the container and fits the document
    // into it (a ResizeObserver keeps it fitted); it never CSS-transforms canvas.
    editor = new Editor(containerRef.value, parseDocument(blankDocument()))
    editor.setTool(TOOLS[ui.activeTool])
    editor.setStyle({ ...DEFAULT_STYLE })
    unsubscribe = editor.onChange(syncEditorState)
    syncEditorState()
    window.addEventListener('keydown', onKeydown)
})

onBeforeUnmount(() => {
    window.removeEventListener('keydown', onKeydown)
    if (messageTimer) clearTimeout(messageTimer)
    unsubscribe?.()
    unsubscribe = null
    // Destroy the Konva stage (removes it from Konva's module-global registry and
    // releases its <canvas> elements); merely dropping the ref would leak it.
    editor?.destroy()
    editor = null
})

/** Single-key tool bindings, derived from TOOL_META so key and hint can't drift. */
const KEY_TO_TOOL = new Map<string, ToolId>(
    (Object.keys(TOOLS) as ToolId[]).map((id) => [TOOL_META[id].key.toLowerCase(), id])
)

/**
 * Keyboard shortcuts (DECISIONS 2026-07-04): Ctrl/Cmd+Z/Y undo-redo, Ctrl/Cmd+
 * 0/+/- zoom, Ctrl/Cmd+S save, modifier-free B/E/L/R/O/T tool keys, and "?"
 * for the cheat-sheet. Skips form fields; single keys are also suppressed
 * while the side menu or the cheat-sheet is open (they own the keyboard then).
 */
function onKeydown(e: KeyboardEvent) {
    const target = e.target as HTMLElement | null
    if (target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable)) {
        return
    }
    const key = e.key.toLowerCase()
    if (e.ctrlKey || e.metaKey) {
        if (key === 'z' && !e.shiftKey) {
            e.preventDefault()
            editor?.undo()
        } else if ((key === 'z' && e.shiftKey) || key === 'y') {
            e.preventDefault()
            editor?.redo()
        } else if (key === 's') {
            e.preventDefault() // the browser's own save dialog
            save()
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
    // Esc closes the cheat-sheet even when focus never entered it (the SideMenu
    // handles its own Esc — focus moves into its panel on open).
    if (e.key === 'Escape') {
        if (shortcutsOpen.value) shortcutsOpen.value = false
        return
    }
    // While an overlay is open, single keys belong to it — except "?", which
    // still toggles the cheat-sheet closed.
    if (e.key === '?') {
        if (menuOpen.value) return
        e.preventDefault()
        shortcutsOpen.value = !shortcutsOpen.value
        return
    }
    if (menuOpen.value || shortcutsOpen.value) return
    const tool = KEY_TO_TOOL.get(key)
    if (tool) {
        e.preventDefault()
        pickTool(tool)
    }
}

/* --- toolbar handlers ------------------------------------------------ */

function pickTool(id: ToolId) {
    ui.activeTool = id
    // setTool takes a Tool OBJECT, not the id string.
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

/* --- zoom handlers --------------------------------------------------- */

function zoomIn() {
    editor?.zoomIn()
}
function zoomOut() {
    editor?.zoomOut()
}
function fitView() {
    editor?.fitToViewport()
}

function clearCanvas() {
    if (!editor) return
    // Validate the freshly built blank doc before loading (loadDocument does
    // not validate). parseDocument throws DocumentValidationError on bad input.
    editor.loadDocument(parseDocument(blankDocument()))
    currentId.value = null
    message.value = null
}

/* --- layers panel handlers ------------------------------------------ */

function addLayer() {
    editor?.addLayer()
}
function selectLayer(id: string) {
    editor?.setActiveLayer(id)
}
function removeLayer(id: string) {
    editor?.removeLayer(id)
}
function moveLayer(id: string, toIndex: number) {
    editor?.moveLayer(id, toIndex)
}
function toggleLayerVisible(id: string, visible: boolean) {
    editor?.setLayerVisible(id, visible)
}
function setLayerOpacity(id: string, opacity: number) {
    editor?.setLayerOpacity(id, opacity)
}
function renameLayer(id: string, name: string) {
    editor?.renameLayer(id, name)
}

async function exportPng() {
    if (!editor) return
    const doc = editor.getDocument()
    const blob = await editor.toPNG({ outWidth: doc.width, outHeight: doc.height, fit: 'contain' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `justpaint-${Date.now()}.png`
    a.click()
    // Defer the revoke a tick: revoking in the same tick as click() can abort
    // the download in some browsers before the navigation resolves.
    setTimeout(() => URL.revokeObjectURL(url), 0)
}

function reportError(err: unknown, action: string) {
    if (isAuthError(err)) {
        // Open the drawer so the sign-in form is one glance away. Close the
        // cheat-sheet first: both teleport to body at the same z-index with
        // independent focus traps, so stacking them fights over focus.
        shortcutsOpen.value = false
        menuOpen.value = true
        message.value = { text: `Sign in from the menu (top-left) to ${action}.`, severity: 'error' }
        return
    }
    const api = toApiError(err)
    message.value = {
        text: api ? `Could not ${action}: ${api.message}` : `Could not ${action} (is the server running?).`,
        severity: 'error'
    }
}

function save() {
    if (!editor || busy.value) return
    message.value = null
    const existing = currentId.value
    saveMutation.mutate(
        { id: existing ?? undefined, document: editor.getDocument() },
        {
            onSuccess: (meta) => {
                currentId.value = meta.id
                message.value = { text: existing ? 'Saved.' : `Saved as ${meta.id}.`, severity: 'success' }
            },
            onError: (err) => reportError(err, 'save')
        }
    )
}

function load() {
    if (!editor || busy.value) return
    message.value = null
    loadMutation.mutate(undefined, {
        onSuccess: (full) => {
            if (!full) {
                message.value = { text: 'No saved drawings yet.', severity: 'info' }
                return
            }
            // full.document is already validated by drawings.get (parseDocument).
            editor?.loadDocument(full.document)
            currentId.value = full.id
            message.value = { text: `Loaded ${full.id}.`, severity: 'success' }
        },
        onError: (err) => reportError(err, 'load')
    })
}
</script>

<template>
    <div class="draw">
        <!-- The Editor sizes its Konva stage to this full-bleed wrapper and fits
             the document into it (zoom/pan via the stage transform). Everything
             else floats above the canvas — the shared /draw+/play shell language. -->
        <div ref="containerRef" class="draw__canvas"></div>

        <!-- Top-left: menu + brand -->
        <div class="draw__top-left">
            <button class="draw__chip jp-float" type="button" aria-label="Open menu" @click="menuOpen = true">
                <ToolIcon name="menu" />
            </button>
            <span class="draw__brand jp-float">justpaint</span>
        </div>

        <!-- Top-right: panels, theme, file actions -->
        <div class="draw__top-right">
            <div class="draw__actions jp-float">
                <button
                    class="draw__chip-inline"
                    :class="{ 'draw__chip-inline--active': layersOpen }"
                    type="button"
                    :aria-pressed="layersOpen"
                    aria-label="Toggle layers panel"
                    title="Layers"
                    @click="layersOpen = !layersOpen"
                >
                    <ToolIcon name="layers" />
                </button>
                <button
                    class="draw__chip-inline"
                    type="button"
                    :aria-label="themeTitle"
                    :title="themeTitle"
                    @click="theme.cycle()"
                >
                    <ToolIcon :name="themeIcon" />
                </button>
                <button
                    class="draw__chip-inline"
                    :class="{ 'draw__chip-inline--active': shortcutsOpen }"
                    type="button"
                    aria-label="Keyboard shortcuts — ?"
                    title="Keyboard shortcuts — ?"
                    @click="shortcutsOpen = !shortcutsOpen"
                >
                    <ToolIcon name="help" />
                </button>
                <span class="draw__sep" aria-hidden="true"></span>
                <OriButton
                    class="draw__action--desktop"
                    text="New"
                    variant="outline"
                    radius="md"
                    @click="clearCanvas"
                />
                <OriButton
                    class="draw__action--desktop"
                    text="Load"
                    variant="outline"
                    radius="md"
                    :loading="busy"
                    @click="load"
                />
                <OriButton text="Save" variant="fill" radius="md" :loading="busy" @click="save" />
                <OriButton
                    class="draw__action--desktop"
                    text="Export"
                    variant="outline"
                    radius="md"
                    @click="exportPng"
                />
            </div>
        </div>

        <!-- Transient status -->
        <Transition name="toast">
            <p
                v-if="message"
                class="draw__message jp-float"
                :class="{ 'draw__message--error': message.severity === 'error' }"
                :role="message.severity === 'error' ? 'alert' : 'status'"
            >
                <span class="draw__message-text">{{ message.text }}</span>
                <button class="draw__message-dismiss" type="button" aria-label="Dismiss" @click="message = null">
                    <ToolIcon name="close" />
                </button>
            </p>
        </Transition>

        <!-- Bottom-center: the floating toolbar -->
        <div class="draw__toolbar">
            <FloatingToolbar
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
        </div>

        <!-- Bottom-right: zoom -->
        <div class="draw__zoom jp-float" role="group" aria-label="Zoom">
            <button
                class="draw__zoom-btn"
                type="button"
                aria-label="Zoom out"
                title="Zoom out — Ctrl+-"
                @click="zoomOut"
            >
                <ToolIcon name="minus" />
            </button>
            <button
                class="draw__zoom-value"
                type="button"
                :aria-label="`Fit to view (currently ${zoomPercent}%)`"
                title="Fit to view — Ctrl+0"
                @click="fitView"
            >
                {{ zoomPercent }}%
            </button>
            <button class="draw__zoom-btn" type="button" aria-label="Zoom in" title="Zoom in — Ctrl+=" @click="zoomIn">
                <ToolIcon name="plus" />
            </button>
        </div>

        <!-- Right: layers island -->
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

        <SideMenu
            :open="menuOpen"
            :busy="busy"
            @close="menuOpen = false"
            @new-drawing="clearCanvas"
            @load="load"
            @save="save"
            @export-png="exportPng"
        />

        <ShortcutsDialog :open="shortcutsOpen" @close="shortcutsOpen = false" />
    </div>
</template>

<style scoped>
.draw {
    position: relative;
    height: 100%;
    overflow: hidden;

    /* Letterbox around the fitted document (its own white background shows the
       "paper"; this fills the margins, like a canvas on a desk). */
    background-color: var(--ori-color-background);
}

.draw__canvas {
    position: absolute;
    inset: 0;
}

/* --- floating chrome -------------------------------------------------- */

.draw__top-left,
.draw__top-right {
    position: absolute;
    top: var(--ori-size-gap_md, 0.5rem);
    z-index: 10;

    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_sm, 0.25rem);
}

.draw__top-left {
    left: var(--ori-size-gap_md, 0.5rem);
}

.draw__top-right {
    right: var(--ori-size-gap_md, 0.5rem);
    max-width: calc(100vw - 8rem);
}

.draw__chip {
    display: grid;
    place-items: center;

    width: var(--jp-control-lg, 2.4rem);
    height: var(--jp-control-lg, 2.4rem);
    padding: 0;

    color: var(--ori-color-on-surface);
    font-size: 1.05rem;
    cursor: pointer;
}

.draw__chip:hover {
    background-color: var(--jp-hover-bg, color-mix(in srgb, var(--ori-color-primary) 12%, transparent));
}

.draw__brand {
    padding: var(--ori-size-gap_sm, 0.25rem) var(--ori-size-gap_md, 0.5rem);

    font-weight: 700;
    color: var(--ori-color-primary);
    letter-spacing: -0.01em;
}

.draw__actions {
    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_sm, 0.25rem);
    flex-wrap: wrap;

    padding: var(--ori-size-gap_xs, 0.125rem) var(--ori-size-gap_sm, 0.25rem);
}

.draw__chip-inline {
    display: grid;
    place-items: center;

    width: var(--ori-size-action_md, 2.75rem);
    height: var(--ori-size-action_md, 2.75rem);
    padding: 0;

    border: none;
    border-radius: var(--ori-size-radius_md, 8px);
    background: transparent;
    color: var(--ori-color-on-surface);

    font-size: 0.95rem;
    cursor: pointer;
}

.draw__chip-inline:hover {
    background-color: var(--jp-hover-bg, color-mix(in srgb, var(--ori-color-primary) 12%, transparent));
}

.draw__chip-inline--active {
    background-color: var(--jp-selected-bg, color-mix(in srgb, var(--ori-color-primary) 18%, transparent));
    color: var(--ori-color-primary);
}

.draw__sep {
    align-self: stretch;
    width: 1px;
    margin: 0.25rem 0.15rem;
    background-color: var(--ori-color-outline, rgb(0 0 0 / 12%));
}

.draw__message {
    position: absolute;
    top: calc(var(--ori-size-gap_md, 0.5rem) + 3.25rem);
    left: 50%;
    z-index: 12;
    transform: translateX(-50%);

    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_sm, 0.25rem);

    max-width: min(34rem, 80vw);
    margin: 0;
    padding: 0.5rem 0.9rem;

    color: var(--ori-color-on-surface);
    font-size: var(--ori-font-size_sm, 0.875rem);
}

.draw__message--error {
    border-inline-start: 3px solid var(--ori-color-danger);
}

.draw__message-text {
    flex: 1;
}

/* Bare glyph dismiss — matches the other transparent icon buttons in the shell. */
.draw__message-dismiss {
    display: grid;
    place-items: center;
    flex-shrink: 0;

    width: 1.5rem;
    height: 1.5rem;
    padding: 0;

    border: none;
    border-radius: var(--ori-size-radius_md, 8px);
    background: transparent;
    color: var(--ori-color-on-surface);

    cursor: pointer;
}

.draw__message-dismiss:hover {
    background-color: var(--jp-hover-bg, color-mix(in srgb, var(--ori-color-primary) 12%, transparent));
}

.draw__toolbar {
    position: absolute;
    bottom: var(--ori-size-gap_lg, 0.75rem);
    left: 50%;
    z-index: 10;
    transform: translateX(-50%);
}

.draw__zoom {
    position: absolute;
    right: var(--ori-size-gap_md, 0.5rem);
    bottom: var(--ori-size-gap_lg, 0.75rem);
    z-index: 10;

    display: flex;
    align-items: center;
    gap: 0;

    padding: var(--ori-size-gap_xs, 0.125rem) var(--ori-size-gap_sm, 0.25rem);
}

/* Compact square zoom glyphs — hand-rolled to match the chip chrome; OriButton
   only renders a fixed square when given an `icon` prop. */
.draw__zoom-btn {
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

.draw__zoom-btn:hover {
    background-color: var(--jp-hover-bg, color-mix(in srgb, var(--ori-color-primary) 12%, transparent));
}

.draw__zoom-value {
    min-width: 3.1rem;
    padding: 0.25rem;

    border: none;
    background: none;
    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_sm, 0.85rem);
    font-variant-numeric: tabular-nums;
    text-align: center;
    cursor: pointer;
}

.draw__layers {
    position: absolute;
    top: 4.1rem;
    right: var(--ori-size-gap_md, 0.5rem);
    bottom: 4.6rem;
    z-index: 9;

    display: flex;
    align-items: flex-start;

    width: 15.5rem;
}

.toast-enter-active,
.toast-leave-active {
    transition:
        opacity 180ms ease,
        transform 180ms ease;
}

.toast-enter-from,
.toast-leave-to {
    opacity: 0;
    transform: translateX(-50%) translateY(-0.4rem);
}

/* --- small screens ----------------------------------------------------- */

@media (width <= 600px) {
    /* The layers island becomes a bottom sheet above the toolbar. */
    .draw__layers {
        inset: auto var(--ori-size-gap_sm, 0.25rem) 5rem var(--ori-size-gap_sm, 0.25rem);

        width: auto;
    }

    .draw__toolbar {
        bottom: var(--ori-size-gap_sm, 0.25rem);
    }

    .draw__zoom {
        display: none; /* pinch/buttons later; zoom hotkeys still work */
    }

    .draw__brand {
        display: none;
    }

    /* New/Load/Export live in the side menu on phones — only Save + the two
       chips stay in the top-right island (it was wrapping over the canvas). */
    .draw__action--desktop {
        display: none;
    }
}
</style>
