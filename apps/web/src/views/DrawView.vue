<script lang="ts" setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { OriButton } from '@oriui/vue'
import { Editor, TOOLS, DEFAULT_STYLE, newId } from '@justpaint/editor'
import type { ToolId, LayerView } from '@justpaint/editor'
import type { Document } from '@justpaint/document'
import { DEFAULT_BACKGROUND, DEFAULT_CANVAS, DOC_VERSION, LIMITS, parseDocument } from '@justpaint/document'
import { isAuthError, toApiError, useLoadLatestDrawing, useSaveDrawing, useSessionStore, useThemeStore } from '@core'
import FloatingToolbar from '../components/FloatingToolbar.vue'
import LayersPanel from '../components/LayersPanel.vue'
import SideMenu from '../components/SideMenu.vue'
import ToolIcon from '../components/icons/ToolIcon.vue'
import type { IconName } from '../components/icons/ToolIcon.vue'

const containerRef = ref<HTMLDivElement | null>(null)
let editor: Editor | null = null
let unsubscribe: (() => void) | null = null

/** Id of the drawing currently open (set after a successful save/load). */
const currentId = ref<string | null>(null)
const message = ref<string | null>(null)

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
// Layers start open where there's room, closed on small screens.
const layersOpen = ref(window.innerWidth > 900)

const THEME_ICON: Record<string, IconName> = { auto: 'monitor', light: 'sun', dark: 'moon' }
const themeIcon = computed(() => THEME_ICON[theme.mode] ?? 'monitor')
const themeTitle = computed(() => `Theme: ${theme.mode} (click to switch)`)

// Status messages self-dismiss; errors linger a little longer than successes.
let messageTimer: ReturnType<typeof setTimeout> | null = null
watch(message, (m) => {
    if (messageTimer) clearTimeout(messageTimer)
    if (m) messageTimer = setTimeout(() => (message.value = null), 6000)
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

/** Keyboard shortcuts: undo/redo + zoom (Ctrl/Cmd + 0 fit, +/- zoom). Skips form fields. */
function onKeydown(e: KeyboardEvent) {
    const target = e.target as HTMLElement | null
    if (target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable)) {
        return
    }
    if (!(e.ctrlKey || e.metaKey)) return
    const key = e.key.toLowerCase()
    if (key === 'z' && !e.shiftKey) {
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
        message.value = `Sign in to ${action} (menu ☰).`
        return
    }
    const api = toApiError(err)
    message.value = api ? `Could not ${action}: ${api.message}` : `Could not ${action} (is the server running?).`
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
                message.value = existing ? 'Saved.' : `Saved as ${meta.id}.`
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
                message.value = 'No saved drawings yet.'
                return
            }
            // full.document is already validated by drawings.get (parseDocument).
            editor?.loadDocument(full.document)
            currentId.value = full.id
            message.value = `Loaded ${full.id}.`
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
                <span class="draw__sep" aria-hidden="true"></span>
                <OriButton size="sm" variant="outline" @click="clearCanvas">New</OriButton>
                <OriButton size="sm" variant="outline" :loading="busy" @click="load">Load</OriButton>
                <OriButton size="sm" variant="fill" :loading="busy" @click="save">Save</OriButton>
                <OriButton size="sm" variant="outline" @click="exportPng">Export</OriButton>
            </div>
        </div>

        <!-- Transient status -->
        <Transition name="toast">
            <p v-if="message" class="draw__message jp-float" role="status">{{ message }}</p>
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
            <OriButton size="sm" variant="text" aria-label="Zoom out" @click="zoomOut">−</OriButton>
            <button class="draw__zoom-value" title="Fit to view — Ctrl+0" @click="fitView">{{ zoomPercent }}%</button>
            <OriButton size="sm" variant="text" aria-label="Zoom in" @click="zoomIn">+</OriButton>
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

        <SideMenu :open="menuOpen" @close="menuOpen = false" />
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
    top: var(--ori-size-gap_md, 0.75rem);
    z-index: 10;

    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_sm, 0.5rem);
}

.draw__top-left {
    left: var(--ori-size-gap_md, 0.75rem);
}

.draw__top-right {
    right: var(--ori-size-gap_md, 0.75rem);
    max-width: calc(100vw - 8rem);
}

.draw__chip {
    display: grid;
    place-items: center;

    width: 2.6rem;
    height: 2.6rem;
    padding: 0;

    color: var(--ori-color-on-surface);
    font-size: 1.05rem;
    cursor: pointer;
}

.draw__chip:hover {
    background-color: color-mix(in srgb, var(--ori-color-primary) 10%, var(--ori-color-surface));
}

.draw__brand {
    padding: 0.45rem 0.8rem;

    font-weight: 700;
    color: var(--ori-color-primary);
    letter-spacing: -0.01em;
}

.draw__actions {
    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_sm, 0.375rem);
    flex-wrap: wrap;

    padding: 0.3rem 0.45rem;
}

.draw__chip-inline {
    display: grid;
    place-items: center;

    width: 2rem;
    height: 2rem;
    padding: 0;

    border: none;
    border-radius: var(--ori-size-radius_md, 8px);
    background: transparent;
    color: var(--ori-color-on-surface);

    font-size: 0.95rem;
    cursor: pointer;
}

.draw__chip-inline:hover {
    background-color: color-mix(in srgb, var(--ori-color-primary) 12%, transparent);
}

.draw__chip-inline--active {
    background-color: color-mix(in srgb, var(--ori-color-primary) 18%, transparent);
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
    top: var(--ori-size-gap_md, 0.75rem);
    left: 50%;
    z-index: 12;
    transform: translateX(-50%);

    max-width: min(34rem, 80vw);
    margin: 0;
    padding: 0.5rem 0.9rem;

    color: var(--ori-color-on-surface);
    font-size: var(--ori-font-size_sm, 0.875rem);
}

.draw__toolbar {
    position: absolute;
    bottom: var(--ori-size-gap_lg, 1rem);
    left: 50%;
    z-index: 10;
    transform: translateX(-50%);
}

.draw__zoom {
    position: absolute;
    right: var(--ori-size-gap_md, 0.75rem);
    bottom: var(--ori-size-gap_lg, 1rem);
    z-index: 10;

    display: flex;
    align-items: center;
    gap: 0;

    padding: 0.2rem 0.3rem;
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
    right: var(--ori-size-gap_md, 0.75rem);
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
        inset: auto var(--ori-size-gap_sm, 0.5rem) 5rem var(--ori-size-gap_sm, 0.5rem);

        width: auto;
    }

    .draw__toolbar {
        bottom: var(--ori-size-gap_sm, 0.5rem);
    }

    .draw__zoom {
        display: none; /* pinch/buttons later; zoom hotkeys still work */
    }

    .draw__brand {
        display: none;
    }
}
</style>
