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
import { OriIcon, OriToaster, OriTooltip, useToast } from '@oriui/vue'
import { useThemeColor } from '@oriui/headless/vue'
import { Editor, TOOLS, DEFAULT_STYLE, newId } from '@justpaint/editor'
import type { ToolId, LayerView } from '@justpaint/editor'
import type { Document } from '@justpaint/document'
import { DEFAULT_CANVAS, DOC_VERSION, LIMITS, parseDocument } from '@justpaint/document'
import {
    copyImage,
    copyText,
    icons,
    isAuthError,
    toApiError,
    useLoadLatestDrawing,
    useSaveDrawing,
    useSessionStore,
    useThemeStore
} from '@core'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import FloatingToolbar, { TOOL_META } from '../components/FloatingToolbar.vue'
import LayersPanel from '../components/LayersPanel.vue'
import ShortcutsDialog from '../components/ShortcutsDialog.vue'
import SideMenu from '../components/SideMenu.vue'
import ToolIcon from '../components/icons/ToolIcon.vue'
import type { IconName } from '../components/icons/ToolIcon.vue'

const containerRef = ref<HTMLDivElement | null>(null)
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

const THEME_ICON: Record<string, IconName> = { auto: 'monitor', light: 'sun', dark: 'moon' }
const themeIcon = computed(() => THEME_ICON[theme.mode] ?? 'monitor')
const themeTitle = computed(() => `Theme: ${theme.mode} (click to switch)`)

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
    const el = containerRef.value
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
    if (!containerRef.value) return
    const container = containerRef.value
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
    containerRef.value?.removeEventListener('pointermove', onCanvasPointerMove)
    containerRef.value?.removeEventListener('pointerleave', onCanvasPointerLeave)
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
    <div class="draw">
        <!-- The Editor sizes its Konva stage to this full-bleed wrapper and fits
             the document into it (zoom/pan via the stage transform). Everything
             else floats above the canvas — the shared /draw+/play shell language. -->
        <div ref="containerRef" class="draw__canvas"></div>

        <!-- Top-left: brand -->
        <div class="draw__top-left">
            <span class="draw__brand jp-float">justpaint</span>
        </div>

        <!-- Top-left (phones only): undo/redo island — the toolbar hides its
             history group <=600px, so history keeps a one-tap home clear of
             the tool row. Hidden on desktop (the bar has its own). -->
        <div class="draw__history jp-float" role="group" aria-label="History">
            <button class="draw__history-btn" type="button" aria-label="Undo" :disabled="!canUndo" @click="undo">
                <ToolIcon name="undo" />
            </button>
            <button class="draw__history-btn" type="button" aria-label="Redo" :disabled="!canRedo" @click="redo">
                <ToolIcon name="redo" />
            </button>
        </div>

        <!-- Top-right corner: the menu toggler. Its OriTooltip wrapper carries the
             absolute corner pin (z-110 > menu z-100) so the same chip opens and
             closes it (the legacy pattern); the tooltip drops BELOW to stay
             on-screen at the top edge. -->
        <OriTooltip class="draw__menu-toggle-tip" placement="bottom" :content="menuOpen ? 'Close menu' : 'Open menu'">
            <button
                class="draw__menu-toggle jp-float"
                type="button"
                :aria-label="menuOpen ? 'Close menu' : 'Open menu'"
                :aria-expanded="menuOpen"
                @click="menuOpen = !menuOpen"
            >
                <OriIcon :icon="menuOpen ? icons.mdiMenuOpen : icons.mdiMenu" />
            </button>
        </OriTooltip>

        <!-- Top-right: panels, theme, file actions (left of the toggler). Every
             chip's tooltip drops BELOW (placement="bottom") so the bubble stays
             on-screen at the top edge (.draw has overflow:hidden, clipping any
             upward bubble). -->
        <div class="draw__top-right">
            <div class="draw__actions jp-float">
                <OriTooltip placement="bottom" content="Layers">
                    <button
                        class="draw__chip-inline"
                        :class="{ 'draw__chip-inline--active': layersOpen }"
                        type="button"
                        :aria-pressed="layersOpen"
                        aria-label="Toggle layers panel"
                        @click="layersOpen = !layersOpen"
                    >
                        <ToolIcon name="layers" />
                    </button>
                </OriTooltip>
                <OriTooltip placement="bottom" :content="themeTitle">
                    <button class="draw__chip-inline" type="button" :aria-label="themeTitle" @click="theme.cycle()">
                        <ToolIcon :name="themeIcon" />
                    </button>
                </OriTooltip>
                <!-- draw__chip-help rides the WRAPPER (not the button) so it's the
                     wrapper that display:none-s <=600px, leaving no empty flex gap. -->
                <OriTooltip class="draw__chip-help" placement="bottom" content="Keyboard shortcuts — ?">
                    <button
                        class="draw__chip-inline"
                        :class="{ 'draw__chip-inline--active': shortcutsOpen }"
                        type="button"
                        aria-label="Keyboard shortcuts — ?"
                        @click="shortcutsOpen = !shortcutsOpen"
                    >
                        <ToolIcon name="help" />
                    </button>
                </OriTooltip>
                <span class="draw__sep" aria-hidden="true"></span>
                <!-- File actions as icon chips on every breakpoint — the side
                     menu keeps the text duplicates. -->
                <OriTooltip placement="bottom" content="New drawing">
                    <button class="draw__chip-inline" type="button" aria-label="New drawing" @click="requestNew">
                        <OriIcon :icon="icons.mdiPlus" />
                    </button>
                </OriTooltip>
                <OriTooltip placement="bottom" content="Load">
                    <button
                        class="draw__chip-inline"
                        type="button"
                        aria-label="Load latest drawing"
                        :disabled="!session.isLoggedIn || busy"
                        :aria-busy="busy || undefined"
                        @click="load"
                    >
                        <OriIcon :icon="icons.mdiCloudDownloadOutline" />
                    </button>
                </OriTooltip>
                <OriTooltip placement="bottom" content="Save — Ctrl/⌘+S">
                    <button
                        class="draw__chip-inline draw__chip-inline--accent"
                        type="button"
                        aria-label="Save drawing"
                        :disabled="!session.isLoggedIn || busy"
                        :aria-busy="busy || undefined"
                        @click="save"
                    >
                        <OriIcon :icon="icons.mdiContentSaveOutline" />
                    </button>
                </OriTooltip>
                <OriTooltip placement="bottom" content="Export">
                    <button class="draw__chip-inline" type="button" aria-label="Export PNG" @click="exportPng">
                        <OriIcon :icon="icons.mdiDownload" />
                    </button>
                </OriTooltip>
            </div>
        </div>

        <!-- Transient status: the oriui toast queue (pushed via useToast()) -->
        <OriToaster position="top-center" />

        <!-- First-run hint: only on an empty canvas, until dismissed. -->
        <Transition name="toast">
            <p v-if="showHint" class="draw__hint jp-float">
                Pick a tool and draw. Press <kbd>?</kbd> for shortcuts.
                <button class="draw__hint-x" type="button" aria-label="Dismiss" @click="dismissHint">
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

        <!-- Bottom-right: zoom. Tooltips point UP (placement="top") — the island
             sits at the bottom edge. -->
        <div class="draw__zoom jp-float" role="group" aria-label="Zoom">
            <OriTooltip placement="top" content="Zoom out — Ctrl+-">
                <button class="draw__zoom-btn" type="button" aria-label="Zoom out" @click="zoomOut">
                    <ToolIcon name="minus" />
                </button>
            </OriTooltip>
            <span class="draw__zoom-value">{{ zoomPercent }}%</span>
            <OriTooltip placement="top" content="Zoom in — Ctrl+=">
                <button class="draw__zoom-btn" type="button" aria-label="Zoom in" @click="zoomIn">
                    <ToolIcon name="plus" />
                </button>
            </OriTooltip>
            <OriTooltip placement="top" content="Fit — Ctrl+0">
                <button class="draw__zoom-btn" type="button" aria-label="Fit to view" @click="fitView">
                    <ToolIcon name="fit" />
                </button>
            </OriTooltip>
        </div>

        <!-- Bottom-left: cursor document-coordinate readout (desktop only —
             hidden <=600px; no hover on touch). Shows where a stroke would land,
             mapped through the editor's own stage transform at any zoom/pan. -->
        <div v-if="coords" class="draw__coords jp-float">
            <span class="draw__coords-mark" aria-hidden="true">⌖</span>
            <span class="draw__coords-value">{{ Math.round(coords.x) }}, {{ Math.round(coords.y) }}</span>
        </div>

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
    </div>
</template>

<style scoped>
.draw {
    position: relative;
    height: 100%;
    overflow: hidden;

    /* Letterbox around the fitted document (the view-only backdrop paints the
       "paper" behind the transparent document; this fills the margins, like a
       canvas on a desk). The "desk" sits one step off the paper so the document
       edge reads at any zoom — never the paper's/background's own color. */
    background-color: var(--jp-desk, #e9ebef);
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

/* Shifted left of the corner menu toggler (gap + chip + gap). Max width =
   viewport minus the toggler offset minus a left breathing gap; on very narrow
   phones (~<=360px) the chips wrap to a second row inside the island. */
.draw__top-right {
    right: calc(var(--ori-size-gap_md, 0.5rem) * 2 + var(--ori-size-action_md, 2.75rem));
    max-width: calc(100vw - (var(--ori-size-gap_md, 0.5rem) * 3 + var(--ori-size-action_md, 2.75rem)));
}

/* The menu toggler's OriTooltip wrapper — pinned above the 400px drawer (z-110 >
   the menu's z-100) so the toggler stays clickable, flipping to a "close" glyph,
   while the menu is open. The pin lives on the wrapper (not the button) so the
   tooltip bubble anchors to the button; unlayered, so it beats .ori-tooltip's
   own layered position:relative. */
.draw__menu-toggle-tip {
    position: absolute;
    top: var(--ori-size-gap_md, 0.5rem);
    right: var(--ori-size-gap_md, 0.5rem);
    z-index: 110;
}

.draw__menu-toggle {
    display: grid;
    place-items: center;

    width: var(--ori-size-action_md, 2.75rem);
    /* Match the actions island's outer height (chip + its vertical padding +
       the 1px jp-float borders; border-box, so the border must be added) so the
       toggler reads as the rightmost element of the same top row. */
    height: calc(var(--ori-size-action_md, 2.75rem) + 2 * var(--ori-size-gap_xs, 0.125rem) + 2px);
    padding: 0;

    color: var(--ori-color-on-surface);
    cursor: pointer;
}

.draw__menu-toggle:hover {
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

.draw__chip-inline:disabled {
    opacity: 0.35;
    cursor: default;
}

.draw__chip-inline:hover:not(:disabled) {
    background-color: var(--jp-hover-bg, color-mix(in srgb, var(--ori-color-primary) 12%, transparent));
}

/* Save — the one filled (primary) chip, inheriting the old fill-button role.
   Hover deliberately stays primary; disabled dims like every other control. */
.draw__chip-inline--accent {
    background-color: var(--ori-color-primary);
    color: var(--ori-color-on-primary);
}

.draw__chip-inline--accent:hover:not(:disabled) {
    background-color: var(--ori-color-primary);
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

/* Full-width centering strip (NOT left:50% + translate): an absolutely
   positioned box with a left offset gets shrink-to-fit against the REMAINING
   half of the viewport, which squeezed the bar to min-content and wrapped it
   on phones. The strip itself must not eat canvas events. */
.draw__toolbar {
    position: absolute;
    bottom: var(--ori-size-gap_lg, 0.75rem);
    left: 0;
    right: 0;
    z-index: 10;

    display: flex;
    justify-content: center;

    pointer-events: none;
}

.draw__toolbar > * {
    pointer-events: auto;
}

/* First-run hint — sits above the toolbar, clear of it. */
.draw__hint {
    position: absolute;
    bottom: 5rem;
    left: 50%;
    z-index: 11;
    transform: translateX(-50%);

    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_sm, 0.25rem);

    max-width: min(30rem, 90vw);
    margin: 0;
    padding: 0.4rem 0.8rem;

    color: var(--ori-color-on-surface);
    font-size: var(--ori-font-size_sm, 0.875rem);
}

.draw__hint kbd {
    padding: 0.05rem 0.35rem;

    border: 1px solid var(--ori-color-outline, rgb(0 0 0 / 12%));
    border-radius: var(--ori-size-radius_sm, 4px);
    background-color: var(--jp-neutral-hover-bg, color-mix(in srgb, var(--ori-color-on-surface) 8%, transparent));

    font-family: var(--ori-font-family_mono, ui-monospace, monospace);
    font-size: 0.8em;
}

/* Bare glyph dismiss — matches the shell's other transparent icon buttons. */
.draw__hint-x {
    display: grid;
    place-items: center;
    flex-shrink: 0;

    width: 1.4rem;
    height: 1.4rem;
    padding: 0;

    border: none;
    border-radius: var(--ori-size-radius_md, 8px);
    background: transparent;
    color: var(--ori-color-on-surface);

    cursor: pointer;
}

.draw__hint-x:hover {
    background-color: var(--jp-neutral-hover-bg, color-mix(in srgb, var(--ori-color-on-surface) 8%, transparent));
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

/* Compact square zoom/history glyphs — hand-rolled to match the chip chrome;
   OriButton only renders a fixed square when given an `icon` prop. */
.draw__zoom-btn,
.draw__history-btn {
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

.draw__zoom-btn:disabled,
.draw__history-btn:disabled {
    opacity: 0.35;
    cursor: default;
}

.draw__zoom-btn:hover:not(:disabled),
.draw__history-btn:hover:not(:disabled) {
    background-color: var(--jp-hover-bg, color-mix(in srgb, var(--ori-color-primary) 12%, transparent));
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

/* Cursor document-coordinate readout — bottom-left, desktop only. That corner is
   free: the history group lives in the bar on desktop (its top-left island is
   <=600px only), and the readout is hidden on touch (no hover to drive it). A
   passive readout: pointer-events:none so it never intercepts canvas drawing. */
.draw__coords {
    position: absolute;
    left: var(--ori-size-gap_md, 0.5rem);
    bottom: var(--ori-size-gap_lg, 0.75rem);
    z-index: 10;

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
    right: var(--ori-size-gap_md, 0.5rem);
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

    .draw__toolbar {
        bottom: var(--ori-size-gap_sm, 0.25rem);
    }

    /* Zoom tucks into the bottom-right above the one-row toolbar (strip offset
       0.25rem + bar ~2.83rem: 0.35rem*2 padding + 2rem tools + border —
       4.25rem clears it with ~1.2rem of air), keeping the top corners for the
       history island (left) and the actions row (right). */
    .draw__zoom {
        right: var(--ori-size-gap_sm, 0.25rem);
        bottom: 4.25rem;
    }

    /* Raise the hint clear of the toolbar AND the relocated zoom island (its
       top edge sits ~6.9rem up: 4.25rem + 2.25rem chip + padding + border). */
    .draw__hint {
        bottom: 7.25rem;
    }

    .draw__brand {
        display: none;
    }

    /* Undo/redo island on — the toolbar hides its own history group here. */
    .draw__history {
        display: flex;
    }

    /* The shortcuts chip goes on phones (no hardware keyboard); the file-action
       icon chips stay — the side menu keeps their text duplicates. The class now
       rides the OriTooltip wrapper, so display:none drops the whole tip (no gap). */
    .draw__chip-help {
        display: none;
    }

    /* No hover on touch — the coordinate readout has nothing to track. */
    .draw__coords {
        display: none;
    }
}
</style>
