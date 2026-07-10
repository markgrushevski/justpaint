<script lang="ts">
/**
 * The legacy 8px checkerboard tiles (SVG data-URIs; 24-unit viewBox, 2×2 cells).
 * Theme-specific: translucent black cells on light, translucent white on dark.
 * Module scope: the two HTMLImageElements are built lazily on first use and
 * shared across mounts.
 */
const GRID_TILE_LIGHT =
    "data:image/svg+xml,%3csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' width='8' height='8'%3e%3crect x='12' y='0' width='12' height='12' fill='%230002'/%3e%3crect x='0' y='12' width='12' height='12' fill='%230002'/%3e%3c/svg%3e"
const GRID_TILE_DARK =
    "data:image/svg+xml,%3csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' width='8' height='8'%3e%3crect x='0' y='0' width='12' height='12' fill='%23fff2'/%3e%3crect x='12' y='12' width='12' height='12' fill='%23fff2'/%3e%3c/svg%3e"

let gridTileLightImg: HTMLImageElement | null = null
let gridTileDarkImg: HTMLImageElement | null = null

/** The checkerboard tile for the given theme, created (and loading) on demand. */
function gridTile(dark: boolean): HTMLImageElement {
    if (dark) {
        if (!gridTileDarkImg) {
            gridTileDarkImg = new Image()
            gridTileDarkImg.src = GRID_TILE_DARK
        }
        return gridTileDarkImg
    }
    if (!gridTileLightImg) {
        gridTileLightImg = new Image()
        gridTileLightImg.src = GRID_TILE_LIGHT
    }
    return gridTileLightImg
}
</script>

<script lang="ts" setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { OriSurface, OriToaster, useToast } from '@oriui/vue'
import { useThemeColor } from '@oriui/headless/vue'
import { Editor, TOOLS, DEFAULT_STYLE, newId } from '@justpaint/editor'
import type { ToolId, LayerView } from '@justpaint/editor'
import type { Document } from '@justpaint/document'
import { DEFAULT_CANVAS, DOC_VERSION, LIMITS, parseDocument } from '@justpaint/document'
import {
    copyImage,
    copyText,
    isAuthError,
    toApiError,
    useLoadLatestDrawing,
    useSaveDrawing,
    useSessionStore,
    useThemeStore
} from '@core'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import EmptyState from '../components/EmptyState.vue'
import FloatingToolbar, { TOOL_META } from '../components/FloatingToolbar.vue'
import LayersPanel from '../components/LayersPanel.vue'
import ShortcutsDialog from '../components/ShortcutsDialog.vue'
import SideMenu from '../components/SideMenu.vue'
import EditorShell from '../components/shell/EditorShell.vue'
import IconButton from '../components/ui/IconButton.vue'

// The shared layout skeleton (desk + Konva mount + floating regions). We read
// its exposed canvas mount element in onMounted and build the Editor into it.
const shell = ref<{ canvasEl: HTMLDivElement | null } | null>(null)
// The canvas mount element, captured once at mount: blankDocument sizes to it,
// and it owns the coords-readout pointer listeners we add/remove ourselves.
let canvasHost: HTMLDivElement | null = null
let editor: Editor | null = null
let unsubscribe: (() => void) | null = null

/** Id of the drawing currently open (set after a successful save/load). */
const currentId = ref<string | null>(null)

/** Drawing name — shown/renamed in the side menu, persisted with save. */
const DEFAULT_NAME = 'new art'
const drawingName = ref(DEFAULT_NAME)

// Transient status goes through the oriui toast queue (rendered by the single
// <OriToaster> below); duration scales with severity so errors stay readable
// longer than successes.
const toaster = useToast()
const TOAST_SUCCESS = 3500
const TOAST_INFO = 5000
const TOAST_ERROR = 8000

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
// Vue re-renders the toolbar (undo/redo enablement), the layers panel, zoom,
// and the side menu's canvas-size fields.
const layers = ref<LayerView[]>([])
const activeLayerId = ref('')
const canUndo = ref(false)
const canRedo = ref(false)
const zoom = ref(1)
const zoomPercent = computed(() => Math.round(zoom.value * 100))
// Widened: DEFAULT_CANVAS is `as const`, so a bare ref() would narrow to the literal.
const docWidth = ref<number>(DEFAULT_CANVAS.width)
const docHeight = ref<number>(DEFAULT_CANVAS.height)
const MAX_LAYERS = LIMITS.maxLayers

const session = useSessionStore()
const theme = useThemeStore()

// Konva canvas cannot read CSS custom properties, so the brush-size cursor ring
// gets `--ori-color-primary` RESOLVED through the @oriui/headless token bridge:
// '' until mounted, then the computed color, re-resolving on every theme flip
// (the store toggles `.ori-theme_dark` on <html>; `auto` OS flips are covered
// too). The editor stays token-agnostic — it only ever sees the color string.
const cursorRingColor = useThemeColor('primary')
watch(cursorRingColor, (color) => editor?.setCursorColor(color || null))

/* --- shell chrome state ---------------------------------------------- */

const menuOpen = ref(false)
const shortcutsOpen = ref(false)
// Layers start open where there's room, closed on small screens. Match the CSS
// reflow breakpoint (601px+ has room for the island).
const layersOpen = ref(window.innerWidth > 600) // oriui --ori-size-screen_xs (600px)

// True when nothing is drawn yet (no strokes in any layer). Drives the
// first-run hint and the "New"/apply-size confirm skip.
const isEmpty = computed(() => layers.value.every((l) => l.strokeCount === 0))

/* --- first-run onboarding hint --------------------------------------- */

const HINT_KEY = 'jp.hintDismissed'
const hintDismissed = ref(false)
// Show only on an empty canvas for users who haven't dismissed it; the first
// stroke flips isEmpty false and the hint disappears on its own.
const showHint = computed(() => !hintDismissed.value && isEmpty.value)
function dismissHint() {
    hintDismissed.value = true
    try {
        localStorage.setItem(HINT_KEY, '1')
    } catch {
        /* private mode / storage disabled — the hint just won't persist */
    }
}

/* --- confirm before "New" / apply-size wipes the canvas -------------- */

const confirmNewOpen = ref(false)
/** Size for a pending confirm — set when Apply-size hits a non-empty canvas. */
const pendingSize = ref<{ w: number; h: number } | null>(null)

// Clearing resets history (irreversible), so confirm only when there's work to
// lose; an already-empty canvas clears straight away.
function requestNew() {
    pendingSize.value = null
    if (isEmpty.value) {
        clearCanvas()
    } else {
        confirmNewOpen.value = true
    }
}

/** Apply-size from the menu = "New at this size" — same confirm-if-dirty flow. */
function onApplyCanvasSize(w: number, h: number) {
    if (isEmpty.value) {
        clearCanvas(w, h)
    } else {
        pendingSize.value = { w, h }
        confirmNewOpen.value = true
    }
}

function onConfirmNew() {
    const size = pendingSize.value
    pendingSize.value = null
    clearCanvas(size?.w, size?.h)
    confirmNewOpen.value = false
}

function onCancelNew() {
    pendingSize.value = null
    confirmNewOpen.value = false
}

/** Clamp a canvas dimension to the document's integer [1, maxCanvasDimension] domain. */
function clampDim(n: number): number {
    return Math.min(LIMITS.maxCanvasDimension, Math.max(1, Math.round(n)))
}

/**
 * A blank single-layer document. Unsized, it matches the current viewport (the
 * canvas fills the screen on a fresh /draw), falling back to DEFAULT_CANVAS
 * before layout. Background is null — the transparent document lets the
 * view-only backdrop below (paper / checkerboard) show through.
 */
function blankDocument(w?: number, h?: number): Document {
    const el = canvasHost
    const width = clampDim(w ?? (el && el.clientWidth > 0 ? el.clientWidth : DEFAULT_CANVAS.width))
    const height = clampDim(h ?? (el && el.clientHeight > 0 ? el.clientHeight : DEFAULT_CANVAS.height))
    return {
        version: DOC_VERSION,
        width,
        height,
        background: null,
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
    const doc = editor.getDocument()
    docWidth.value = doc.width
    docHeight.value = doc.height
}

/* --- canvas backdrop (paper / checkerboard, a persisted view pref) ---- */

const BACKDROP_KEY = 'jp.backdropGrid'
const backdropGrid = ref(false)

/**
 * Push the current backdrop pref into the editor: the checkerboard pattern when
 * the grid is on (waiting for the tile image to decode on first use), else the
 * theme "paper" (white/black) behind the transparent document. View-only — the
 * editor guarantees it can never leak into exports or the judged raster.
 */
async function applyBackdrop() {
    if (!editor) return
    if (!backdropGrid.value) {
        editor.setCanvasBackdrop({ type: 'color', color: theme.isDark ? '#000000' : '#ffffff' })
        return
    }
    const img = gridTile(theme.isDark)
    if (!img.complete) {
        try {
            await img.decode()
        } catch {
            return // a data-URI that fails to decode won't succeed on retry
        }
        // Async gap: re-check the pref/theme still want THIS tile before applying.
        if (!editor || !backdropGrid.value || img !== gridTile(theme.isDark)) return
    }
    editor.setCanvasBackdrop({ type: 'pattern', image: img })
}

// Theme flips and grid toggles both re-apply (the tiles are theme-specific).
watch([() => theme.isDark, backdropGrid], () => void applyBackdrop())

function onToggleGrid(on: boolean) {
    backdropGrid.value = on
    try {
        localStorage.setItem(BACKDROP_KEY, on ? '1' : '0')
    } catch {
        /* private mode / storage disabled — the pref just won't persist */
    }
}

/* --- cursor document-coordinate readout (desktop) -------------------- */

// A subtle bottom-left readout of the pointer's DOCUMENT coordinates, mapped
// through the editor's own stage transform (so it matches where a stroke would
// land at any zoom/pan). Null = hidden: pointer off-canvas, over chrome, or touch
// (the chip is display:none <=600px and toDocumentCoords returns null off-stage).
const coords = ref<{ x: number; y: number } | null>(null)
// rAF throttle: pointermove fires far faster than we need to repaint — coalesce
// to one read per frame off the latest client position instead of per raw move.
let coordsRaf = 0
let lastPointer: { x: number; y: number } | null = null

function onCanvasPointerMove(e: PointerEvent) {
    lastPointer = { x: e.clientX, y: e.clientY }
    if (coordsRaf) return
    coordsRaf = requestAnimationFrame(() => {
        coordsRaf = 0
        if (!editor || !lastPointer) return
        coords.value = editor.toDocumentCoords(lastPointer.x, lastPointer.y)
    })
}

function onCanvasPointerLeave() {
    if (coordsRaf) {
        cancelAnimationFrame(coordsRaf)
        coordsRaf = 0
    }
    lastPointer = null
    coords.value = null
}

onMounted(() => {
    void session.fetchMe() // restore an existing cookie session, if any
    try {
        hintDismissed.value = localStorage.getItem(HINT_KEY) === '1'
        backdropGrid.value = localStorage.getItem(BACKDROP_KEY) === '1'
    } catch {
        /* private mode / storage disabled — defaults (hint on, paper backdrop) */
    }
    // The shell exposes its Konva mount element; build the Editor into it.
    const container = shell.value?.canvasEl ?? null
    if (!container) return
    canvasHost = container
    // The editor sizes its Konva stage to the container and fits the document
    // into it (a ResizeObserver keeps it fitted); it never CSS-transforms canvas.
    editor = new Editor(container, parseDocument(blankDocument()))
    editor.setTool(TOOLS[ui.activeTool])
    editor.setStyle({ ...DEFAULT_STYLE })
    // useThemeColor resolves in ITS mounted hook (registered before this one),
    // so the value is usually ready here; the watch covers late/changed values.
    editor.setCursorColor(cursorRingColor.value || null)
    void applyBackdrop()
    unsubscribe = editor.onChange(syncEditorState)
    syncEditorState()
    window.addEventListener('keydown', onKeydown)
    // Desktop cursor-coordinate readout: a container-level pointermove drives it
    // (the chip is hidden <=600px; the listener is harmless on touch).
    container.addEventListener('pointermove', onCanvasPointerMove)
    container.addEventListener('pointerleave', onCanvasPointerLeave)
})

onBeforeUnmount(() => {
    window.removeEventListener('keydown', onKeydown)
    // Drop the coords readout listeners + any pending frame.
    if (coordsRaf) cancelAnimationFrame(coordsRaf)
    canvasHost?.removeEventListener('pointermove', onCanvasPointerMove)
    canvasHost?.removeEventListener('pointerleave', onCanvasPointerLeave)
    canvasHost = null
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
 * for the cheat-sheet. Skips form fields and contenteditable (the menu's title
 * rename). The side menu is NON-MODAL — the canvas stays interactive behind
 * it — so it does NOT suppress single keys; only the modal overlays do.
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
    // Esc: the menu first — it's non-modal, so focus may still sit on the
    // canvas where its own panel-scoped Esc never fires — then the cheat-sheet.
    if (e.key === 'Escape') {
        if (menuOpen.value) menuOpen.value = false
        else if (shortcutsOpen.value) shortcutsOpen.value = false
        return
    }
    // "?" toggles the cheat-sheet — desktop only (the chip is hidden <=600px).
    if (e.key === '?') {
        if (window.innerWidth <= 600) return
        if (confirmNewOpen.value) return
        e.preventDefault()
        shortcutsOpen.value = !shortcutsOpen.value
        return
    }
    // Every MODAL overlay must be listed here, or its single-key tool hotkeys
    // (B/E/L/R/O/T) leak to this window listener and fire underneath it. The
    // non-modal side menu deliberately is not — drawing under it is a feature.
    if (shortcutsOpen.value || confirmNewOpen.value) return
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

function clearCanvas(w?: number, h?: number) {
    if (!editor) return
    // Validate the freshly built blank doc before loading (loadDocument does
    // not validate). parseDocument throws DocumentValidationError on bad input.
    editor.loadDocument(parseDocument(blankDocument(w, h)))
    currentId.value = null
    drawingName.value = DEFAULT_NAME
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

/* --- menu handlers ---------------------------------------------------- */

function onRename(name: string) {
    drawingName.value = name.trim() || DEFAULT_NAME
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

/** Copy the raw vector document (JSON) — the menu's "Copy as text". */
async function copyDocJson() {
    if (!editor) return
    try {
        await copyText(JSON.stringify(editor.getDocument()))
        toaster.success({ text: 'Copied document JSON', duration: TOAST_SUCCESS })
    } catch (err) {
        toaster.error({
            text: err instanceof Error ? err.message : 'Could not copy to the clipboard.',
            duration: TOAST_ERROR,
            closable: true
        })
    }
}

/** Copy a rendered PNG at the document's own size — the menu's "Copy as image". */
async function copyPngToClipboard() {
    if (!editor) return
    try {
        const doc = editor.getDocument()
        const blob = await editor.toPNG({ outWidth: doc.width, outHeight: doc.height, fit: 'contain' })
        await copyImage(blob)
        toaster.success({ text: 'Copied image', duration: TOAST_SUCCESS })
    } catch (err) {
        toaster.error({
            text: err instanceof Error ? err.message : 'Could not copy the image.',
            duration: TOAST_ERROR,
            closable: true
        })
    }
}

function reportError(err: unknown, action: string) {
    if (isAuthError(err)) {
        // Open the drawer so the sign-in form is one glance away. Close the
        // cheat-sheet first: its focus trap would fight the incoming drawer.
        shortcutsOpen.value = false
        menuOpen.value = true
        toaster.error({ text: `Sign in from the menu to ${action}.`, duration: TOAST_ERROR, closable: true })
        return
    }
    const api = toApiError(err)
    toaster.error({
        text: api ? `Could not ${action}: ${api.message}` : `Could not ${action} (is the server running?).`,
        duration: TOAST_ERROR,
        closable: true
    })
}

function save() {
    if (!editor || busy.value) return
    const existing = currentId.value
    saveMutation.mutate(
        { id: existing ?? undefined, document: editor.getDocument(), name: drawingName.value },
        {
            onSuccess: (meta) => {
                currentId.value = meta.id
                toaster.success({ text: existing ? 'Saved.' : `Saved as ${meta.id}.`, duration: TOAST_SUCCESS })
            },
            onError: (err) => reportError(err, 'save')
        }
    )
}

function load() {
    if (!editor || busy.value) return
    loadMutation.mutate(undefined, {
        onSuccess: (full) => {
            if (!full) {
                toaster.info({ text: 'No saved drawings yet.', duration: TOAST_INFO })
                return
            }
            // full.document is already validated by drawings.get (parseDocument).
            editor?.loadDocument(full.document)
            currentId.value = full.id
            drawingName.value = full.name
            toaster.success({ text: `Loaded ${full.id}.`, duration: TOAST_SUCCESS })
        },
        onError: (err) => reportError(err, 'load')
    })
}
</script>

<template>
    <!-- The shared editor shell owns the desk/letterbox surface, the Konva canvas
         mount, and the floating-region layout; /draw fills the regions with its
         chrome. /play will compose the SAME shell (one design, game chrome on top). -->
    <EditorShell ref="shell" mode="draw">
        <!-- Top-left: help + layers island -->
        <template #top-left>
            <OriSurface class="draw__actions">
                <IconButton
                    class="draw__help-btn"
                    icon="help"
                    label="Keyboard shortcuts — ?"
                    placement="bottom"
                    :active="shortcutsOpen"
                    @click="shortcutsOpen = !shortcutsOpen"
                />
                <IconButton
                    icon="layers"
                    label="Toggle layers panel"
                    placement="bottom"
                    :active="layersOpen"
                    @click="layersOpen = !layersOpen"
                />
            </OriSurface>
        </template>

        <!-- Bottom-center: the floating toolbar. The shell's centering strip is
             pointer-events:none; the bar opts back in so drawing passes through
             the empty flanks either side of it. -->
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
                @pick-tool="pickTool"
                @set-color="setColor"
                @set-width="setWidth"
                @toggle-fill="toggleFill"
                @set-fill="setFill"
                @undo="undo"
                @redo="redo"
            />
        </template>

        <!-- Bottom-right: zoom. Tooltips point UP (placement="top") — the island
             sits at the bottom edge. -->
        <template #bottom-right>
            <OriSurface class="draw__zoom" role="group" aria-label="Zoom">
                <IconButton icon="minus" label="Zoom out — Ctrl+-" @click="zoomOut" />
                <span class="draw__zoom-value">{{ zoomPercent }}%</span>
                <IconButton icon="plus" label="Zoom in — Ctrl+=" @click="zoomIn" />
                <IconButton icon="fit" label="Fit — Ctrl+0" @click="fitView" />
            </OriSurface>
        </template>

        <!-- Bottom-left: cursor document-coordinate readout (desktop only —
             hidden <=600px; no hover on touch). Shows where a stroke would land,
             mapped through the editor's own stage transform at any zoom/pan. -->
        <template #bottom-left>
            <OriSurface v-if="coords" class="draw__coords">
                <span class="draw__coords-mark" aria-hidden="true">⌖</span>
                <span class="draw__coords-value">{{ Math.round(coords.x) }}, {{ Math.round(coords.y) }}</span>
            </OriSurface>
        </template>

        <!-- Centered overlay layer: the toast queue, the first-run empty-state
             card, and the modal dialogs. All but the card teleport to body /
             manage their own stacking; the card opts back into pointer events. -->
        <template #overlay>
            <!-- Transient status: the oriui toast queue (pushed via useToast()) -->
            <OriToaster position="top-center" />

            <!-- First-run empty state: a welcome card centered on a blank canvas,
                 only until dismissed or the first stroke lands. The shell overlay
                 lets pointer events pass THROUGH so drawing around the card still
                 works — only the card (pointer-events:auto) is interactive. -->
            <Transition name="jp-pop">
                <EmptyState
                    v-if="showHint"
                    class="draw__empty"
                    :signed-in="session.isLoggedIn"
                    @dismiss="dismissHint"
                    @sign-in="menuOpen = true"
                    @shortcuts="shortcutsOpen = true"
                />
            </Transition>

            <ShortcutsDialog :open="shortcutsOpen" @close="shortcutsOpen = false" />

            <ConfirmDialog
                :open="confirmNewOpen"
                title="Clear the canvas?"
                message="This starts a new drawing and can't be undone."
                confirm-text="Clear"
                cancel-text="Cancel"
                danger
                @confirm="onConfirmNew"
                @cancel="onCancelNew"
            />
        </template>

        <!-- Side drawer (self-teleports to body; non-modal, canvas stays live). -->
        <template #drawer>
            <SideMenu
                :open="menuOpen"
                :busy="busy"
                :can-rename="session.isLoggedIn"
                :title="drawingName"
                :backdrop-grid="backdropGrid"
                :canvas-width="docWidth"
                :canvas-height="docHeight"
                @close="menuOpen = false"
                @new-drawing="requestNew"
                @load="load"
                @save="save"
                @export-png="exportPng"
                @copy-text="copyDocJson"
                @copy-image="copyPngToClipboard"
                @rename="onRename"
                @toggle-grid="onToggleGrid"
                @apply-canvas-size="onApplyCanvasSize"
            />
        </template>

        <!-- Free-floating /draw chrome (self-positioned, into the shell's default
             slot as direct children of the non-stacking-context root). -->

        <!-- Top-right corner: the menu toggler. The OriSurface wrapper carries the
             absolute corner pin (z-110 > drawer z-100) so the same chip opens and
             closes it; the tooltip drops BELOW to stay on-screen at the top edge. -->
        <OriSurface class="draw__menu-toggle">
            <IconButton
                :icon="menuOpen ? 'close' : 'menu'"
                :label="menuOpen ? 'Close menu' : 'Open menu'"
                placement="bottom"
                :active="menuOpen"
                @click="menuOpen = !menuOpen"
            />
        </OriSurface>

        <!-- Top-left (phones only): undo/redo island — the toolbar hides its
             history group <=600px, so history keeps a one-tap home clear of the
             tool row. Hidden on desktop (the bar has its own). -->
        <OriSurface class="draw__history" role="group" aria-label="History">
            <IconButton icon="undo" label="Undo" :disabled="!canUndo" @click="undo" />
            <IconButton icon="redo" label="Redo" :disabled="!canRedo" @click="redo" />
        </OriSurface>

        <!-- Scrim behind the mobile layers bottom sheet (display:none >600px) -->
        <div v-if="layersOpen" class="draw__layers-scrim" @click="layersOpen = false"></div>

        <!-- Layers: a dropdown under the actions island (desktop), bottom sheet (phones) -->
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
/* The desk/letterbox surface, the Konva canvas mount, and the floating-region
   POSITIONING all live in EditorShell now (the shared /draw+/play skeleton).
   What remains here is /draw's own chrome: island visuals + the self-positioned
   extras (menu toggler, mobile history, layers panel + scrim). */

/* --- floating chrome -------------------------------------------------- */

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
    gap: var(--ori-size-gap_sm, 0.25rem);
    flex-wrap: wrap;

    padding: var(--ori-size-gap_xs, 0.125rem) var(--ori-size-gap_sm, 0.25rem);
}

/* EditorShell's bottom-center strip is pointer-events:none (its empty flanks
   never eat canvas events) — the toolbar opts back in so it stays interactive. */
.draw__toolbar-item {
    pointer-events: auto;
}

/* First-run empty state — the centered welcome card. EditorShell's overlay layer
   is full-bleed but pointer-events:none so it never blocks drawing; only the
   card (pointer-events:auto) is interactive. */
.draw__empty {
    pointer-events: auto;
}

/* The card fades + settles in place (a straight fade/scale — NOT the toast's
   horizontal slide, which reads wrong on a centered welcome card). */
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

/* The bottom-right zoom island — EditorShell's bottom-right region positions it. */
.draw__zoom {
    display: flex;
    align-items: center;
    gap: 0;

    padding: var(--ori-size-gap_xs, 0.125rem) var(--ori-size-gap_sm, 0.25rem);
}

/* Display-only readout — fit-to-view moved to its own explicit chip. */
.draw__zoom-value {
    min-width: 3.1rem;
    padding: 0.25rem;

    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_sm, 0.85rem);
    font-variant-numeric: tabular-nums;
    text-align: center;
}

/* Cursor document-coordinate readout — the bottom-left corner (desktop only;
   hidden <=600px, no hover on touch). EditorShell's bottom-left region positions
   it and is itself pointer-events:none; the chip stays a passive readout, so it
   never intercepts canvas drawing at any zoom/pan. */
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

/* Mobile-only history island (top-left) — undo/redo keep a one-tap home when
   the toolbar hides its own history group <=600px. display:none here so it can
   never show on desktop; the media block flips it on. */
.draw__history {
    position: absolute;

    /* SECOND row top-left: the top row belongs to the actions island + toggler,
       which can stretch across a narrow phone — same-row placement overlapped
       them (verified at 405px). 3.125rem = the 50px top-row island height. */
    top: calc(var(--ori-size-gap_md, 0.5rem) * 2 + 3.125rem);
    left: var(--ori-size-gap_md, 0.5rem);
    z-index: 10;

    display: none;
    align-items: center;
    gap: 0;

    padding: var(--ori-size-gap_xs, 0.125rem) var(--ori-size-gap_sm, 0.25rem);
}

/* Layers — a dropdown hanging under the actions island (desktop). The wrapper
   stretches the panel to its clamped height so the panel's own list scrolls. */
.draw__layers {
    position: absolute;
    top: calc(
        var(--ori-size-gap_md, 0.5rem) + var(--ori-size-action_md, 2.75rem) + var(--ori-size-gap_sm, 0.25rem) + 0.35rem
    );
    left: var(--ori-size-gap_md, 0.5rem);
    z-index: 9;

    display: flex;
    align-items: stretch;

    width: 16rem;
    max-height: calc(100dvh - 10rem);
}

/* Scrim behind the mobile layers sheet — display:none here so it can never
   show on desktop; the <=600px media query turns it on. */
.draw__layers-scrim {
    position: fixed;
    inset: 0;
    z-index: 59;

    display: none;

    background-color: rgb(0 0 0 / 35%);
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
    .draw__layers-scrim {
        display: block;
    }

    /* The layers island becomes a full-width bottom sheet over the scrim. The
       wrapper carries the sheet chrome (top radius + surface) so the panel's
       own rounded bottom corners can't notch the screen edge. */
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

    /* Undo/redo island on — the toolbar hides its own history group here. */
    .draw__history {
        display: flex;
    }

    /* The Help button goes on phones (no hardware keyboard); Layers stays in the
       same island. The class rides the IconButton directly, so display:none
       drops the whole control (no empty flex gap). */
    .draw__help-btn {
        display: none;
    }

    /* No hover on touch — the coordinate readout has nothing to track. */
    .draw__coords {
        display: none;
    }
}
</style>
