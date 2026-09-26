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
import { OriButton, OriInput, OriSurface, OriToaster, useToast } from '@oriui/vue'
import { useThemeColor } from '@oriui/headless/vue'
import { DEFAULT_CANVAS, DEFAULT_STYLE, DOC_VERSION, Editor, LIMITS, newId, TOOLS } from '@justpaint/editor'
import type { Document, DocSummary, LayerView, Op, ToolId } from '@justpaint/editor'
import {
    copyImage,
    copyText,
    isAuthError,
    isBudgetExhausted,
    isRateLimited,
    toApiError,
    useAssist,
    useAuthGate,
    useGuess,
    useLoadLatestDrawing,
    useSaveDrawing,
    useSessionStore,
    useThemeStore
} from '@core'
import type { Guess } from '@core'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import EmptyState from '../components/EmptyState.vue'
import FloatingToolbar, { TOOL_META } from '../components/FloatingToolbar.vue'
import GuessResult, { type GuessStatus } from '../components/GuessResult.vue'
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

/* --- AI assist (text drawing commands, docs/ASSIST.md) --------------- */

// The prompt panel (mounted in the shell's #top-center region) and its state.
// A returned batch is previewed as a GHOST inside the editor (previewOps) and is
// NOT in the document or history until Accept — so `pendingOps` is only a UI flag
// (input phase ⇄ accept/reject phase); the ghost lifecycle lives in the editor.
const assistMutation = useAssist()
const assistPending = computed(() => assistMutation.isPending.value)
const assistOpen = ref(false)
const assistPrompt = ref('')
const pendingOps = ref<Op[] | null>(null)
const assistNote = ref<string | null>(null)

/* --- AI guess (the AI reads the canvas back to you) ------------------- */

// The whole feature is one card in the shell's #overlay: the wait, the answer and
// every failure render there. `guessOpen` is what mounts it — the mutation's own
// `isPending` can't be, because the card has to OUTLIVE the request to show the
// answer it came back with.
const guessMutation = useGuess()
const guessOpen = ref(false)
/**
 * What the card shows, as ONE discriminant (PracticeView's `Phase` precedent).
 * `guessResult` / `guessError` are its payloads and `guessExhausted` its
 * orthogonal qualifier; `setGuessStatus` is the only thing that moves them, so a
 * status can never be read against a payload left over from the answer before it.
 *
 * It is NOT the same fact as `guessPending` below. That one is the transport's —
 * is a call in flight — and it is what guards SPENDING; this one is the card's.
 * They part company for exactly one window: the card dismissed mid-call, where
 * the status stays `pending` (the guess is already paid for, so re-opening lands
 * back on the wait) while nothing is mounted to show it.
 */
const guessStatus = ref<GuessStatus>('idle')
const guessPending = computed(() => guessMutation.isPending.value)
const guessResult = ref<Guess | null>(null)
// A failure lives in the card too, not in a toast — see `onGuessError`.
const guessError = ref('')
const guessExhausted = ref(false)
// Set when the DOCUMENT is replaced (New / Load) while a guess is in flight: the
// answer that lands afterwards describes a drawing nobody can see any more, so it
// is dropped rather than shown against the new canvas. Dismissing the card
// deliberately does NOT set it — that call is still about the canvas in front of
// you, and it has already been paid for.
let guessStale = false

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
// The active layer's name, surfaced next to the Layers toggle when the panel is
// closed — new strokes AND the eraser land on this layer (per-layer, like
// Photoshop), so it must be discoverable without opening the panel.
const activeLayerName = computed(() => layers.value.find((l) => l.id === activeLayerId.value)?.name ?? '')
// Widened: DEFAULT_CANVAS is `as const`, so a bare ref() would narrow to the literal.
const docWidth = ref<number>(DEFAULT_CANVAS.width)
const docHeight = ref<number>(DEFAULT_CANVAS.height)
const MAX_LAYERS = LIMITS.maxLayers

const session = useSessionStore()
const gate = useAuthGate()

// A DIFFERENT account signed in (a session lapsed mid-visit and someone else
// took over the tab): the open drawing belongs to the previous one, so forget
// its id. Saving would otherwise PUT a row this user does not own, and an
// ownership-scoped query answers 404 — surfacing as "Could not save: not
// found", which is a lie about what happened.
watch(
    () => session.user?.id,
    (now, before) => {
        if (before && now && now !== before) currentId.value = null
    }
)
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
// stroke flips isEmpty false and the hint disappears on its own. It also stands
// down while the guess card is up: both are centred in the same `#overlay` slot,
// and now that the guess trigger works on a blank canvas (to say so in words)
// they can want that slot at the same moment. The card was asked for; the hint
// was not, so the hint yields.
const showHint = computed(() => !hintDismissed.value && isEmpty.value && !guessOpen.value)
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
    editor = new Editor(container, blankDocument())
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
    // Tear down any pending AI ghost before the stage is destroyed below.
    clearAssistProposal()
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
    // The sign-in modal owns the keyboard while it is up. Without this the
    // Ctrl-branch below still fired underneath it: Ctrl+Z mutated the canvas
    // behind an opaque backdrop, and Ctrl+S queued a SECOND save on the same
    // modal, which on a first save meant two `POST /api/drawings` and two rows.
    if (gate.open) return
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
    // canvas where its own panel-scoped Esc never fires — then the cheat-sheet,
    // then the guess card. The card comes last because it is the least insistent
    // of the three (no backdrop, no focus trap, and the canvas stays live under
    // it), but it IS floating chrome over the drawing, so Esc has to reach it.
    if (e.key === 'Escape') {
        if (menuOpen.value) menuOpen.value = false
        else if (shortcutsOpen.value) shortcutsOpen.value = false
        else if (guessOpen.value) dismissGuess()
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
    // A pending AI proposal references the outgoing document — drop the ghost
    // before the fresh blank doc replaces it, and the AI's guess with it (it
    // describes the drawing that is about to be thrown away).
    clearAssistProposal()
    invalidateGuess()
    editor.loadDocument(blankDocument(w, h))
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

// Deliberately UNgated: the name is a local ref until someone saves, and
// `save()` is where the session is actually needed. Interrupting a text edit
// with a sign-in modal would ask for a session to change a string in memory.
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
            duration: TOAST_ERROR
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
            duration: TOAST_ERROR
        })
    }
}

/**
 * Ask the gate, but only ever ONCE at a time.
 *
 * A gated action waits on a HUMAN, and `busy` — the mutation's own pending flag
 * — does not go true until the mutation actually starts, which is after that
 * wait. So a second trigger landing in the window before the modal is up (the
 * gate first awaits the store's cookie restore) queues a SECOND waiter, and one
 * sign-in then resolves both: the same canvas saved twice, as two rows. Once the
 * dialog is up the rest of the page is inert and this cannot happen — it is
 * exactly the gap before that which needs closing.
 */
let awaitingGate = false

async function gated(reason: string): Promise<boolean> {
    if (awaitingGate) return false
    awaitingGate = true
    try {
        return await gate.ensure(reason)
    } finally {
        awaitingGate = false
    }
}

function reportError(err: unknown, action: string) {
    if (isAuthError(err)) {
        // The transport has already forgotten the dead session, so just ask for
        // a new one. Deliberately NOT re-firing the action afterwards: minutes
        // may have passed, the canvas may have moved on, and the visitor may
        // sign in as someone else entirely — replaying their old click then
        // saves something they never asked to save. Their canvas is intact and
        // the button is right there. Close the cheat-sheet first: its focus trap
        // would fight the incoming dialog.
        shortcutsOpen.value = false
        void gated(`Your session expired — sign in to ${action}.`)
        return
    }
    const api = toApiError(err)
    toaster.error({
        text: api ? `Could not ${action}: ${api.message}` : `Could not ${action} (is the server running?).`,
        duration: TOAST_ERROR
    })
}

async function save() {
    if (!editor || busy.value) return
    if (!(await gated('Sign in to save your drawing.'))) return
    // Re-check: the modal can stay up for minutes, and browser Back unmounts
    // this view and nulls `editor` underneath us. TypeScript keeps the
    // narrowing above across the await, so only this can catch it.
    if (!editor) return
    const existing = currentId.value
    saveMutation.mutate(
        { id: existing ?? undefined, document: editor.getDocument(), name: drawingName.value },
        {
            onSuccess: (meta) => {
                currentId.value = meta.id
                toaster.success({
                    text: existing ? 'Saved.' : `Saved as ${meta.id}.`,
                    duration: TOAST_SUCCESS
                })
            },
            onError: (err) => reportError(err, 'save')
        }
    )
}

async function load() {
    if (!editor || busy.value) return
    if (!(await gated('Sign in to load your drawing.'))) return
    if (!editor) return
    loadMutation.mutate(undefined, {
        onSuccess: (full) => {
            if (!full) {
                toaster.info({ text: 'No saved drawings yet.', duration: TOAST_INFO })
                return
            }
            editor?.loadDocument(full.document)
            // A pending AI proposal references the OLD document's layers — drop the
            // ghost before the incoming doc replaces it, and the guess with it (it
            // describes the drawing that was just replaced).
            clearAssistProposal()
            invalidateGuess()
            currentId.value = full.id
            drawingName.value = full.name
            toaster.success({ text: `Loaded ${full.id}.`, duration: TOAST_SUCCESS })
        },
        onError: (err) => reportError(err, 'load')
    })
}

/* --- AI assist handlers ---------------------------------------------- */

/**
 * The minimal doc summary the endpoint receives (docs/ASSIST.md §4): canvas size
 * + the layer inventory (id/name/strokeCount), never point paths. Phase A stops
 * here — no per-stroke bbox/style.
 */
function buildDocSummary(): DocSummary {
    const doc = editor!.getDocument()
    return {
        canvas: { width: doc.width, height: doc.height },
        layers: doc.layers.map((l) => ({ id: l.id, name: l.name, strokeCount: l.strokes.length }))
    }
}

/** Discard any pending proposal + its ghost so a stale batch never survives a doc swap. */
function clearAssistProposal() {
    if (pendingOps.value) editor?.rejectOps()
    pendingOps.value = null
    assistNote.value = null
}

/** Toggle the prompt panel; closing while previewing discards the ghost. */
function toggleAssist() {
    assistOpen.value = !assistOpen.value
    if (!assistOpen.value) clearAssistProposal()
}

async function submitAssist() {
    if (!editor) return
    const prompt = assistPrompt.value.trim()
    // Mirror the submit button's own disabled guard (Enter can reach here too).
    if (!prompt || assistPending.value || pendingOps.value) return
    if (!(await gated('Sign in to use assist.'))) return
    if (!editor) return
    const targetLayerId = editor.getActiveLayerId() || undefined
    assistMutation.mutate(
        { prompt, docSummary: buildDocSummary(), targetLayerId },
        {
            onSuccess: (r) => {
                editor?.previewOps(r.ops)
                pendingOps.value = r.ops
                // The note is shown inline in the panel (.draw__assist-note); no
                // toast — a top-center toast would land over the panel itself.
                assistNote.value = r.note ?? null
            },
            onError: (err) => reportError(err, 'use assist')
        }
    )
}

/** Commit the previewed batch as one composite command (one Ctrl+Z undoes it all). */
function acceptAssist() {
    editor?.acceptOps()
    pendingOps.value = null
    assistNote.value = null
    assistPrompt.value = '' // reset the panel to the input phase for the next prompt
}

/** Discard the preview — nothing enters the document or history. */
function rejectAssist() {
    editor?.rejectOps()
    pendingOps.value = null
    assistNote.value = null
}

/* --- AI guess handlers ------------------------------------------------ */

/** Move the card to `status` and drop every payload with it — the ONE place the
 *  four pieces of card state change together. */
function setGuessStatus(status: GuessStatus) {
    guessStatus.value = status
    guessResult.value = null
    guessError.value = ''
    guessExhausted.value = false
}

/** Close the card and forget the answer, so the next ask starts clean. The
 *  document-swap teardown is `invalidateGuess` below, which goes further. */
function dismissGuess() {
    guessOpen.value = false
    // A call still in flight keeps its `pending` status: the card goes away, but
    // the guess is already paid for, so re-opening must land back on the wait
    // rather than on a blank `idle` (and `onSuccess` still has somewhere to put
    // the answer). Everything else resets.
    if (!guessPending.value) setGuessStatus('idle')
}

/** A guess describes the document it was asked about — drop it (and disown any
 *  call still in flight) whenever that document is replaced. Called alongside
 *  `clearAssistProposal`, for the same reason the ghost goes: it is about a
 *  canvas that no longer exists. */
function invalidateGuess() {
    guessStale = true
    guessOpen.value = false
    // Unlike a plain dismiss this resets an IN-FLIGHT call's status too: the
    // answer it is about to return has just been disowned, so leaving `pending`
    // behind would let the trigger re-open onto a wait that can never resolve.
    setGuessStatus('idle')
}

/**
 * The canvas emptying under the card is a document swap in everything but name.
 * `requestGuess` only checks `isEmpty` when it FIRES, so undoing back to a blank
 * page used to leave an answer about a drawing that no longer exists — and it
 * would have let `EmptyState`'s first-run hint share the `#overlay` slot with it,
 * the collision the template below asserts is impossible.
 *
 * It disowns rather than merely closes, because with the trigger now always
 * enabled a lingering answer would re-open on a blank canvas instead of saying
 * "draw something first". A redo that brings the strokes back costs a new guess,
 * which is the same price a dismiss has always carried.
 */
watch(isEmpty, (empty) => {
    if (empty) invalidateGuess()
})

/**
 * A failed guess stays INSIDE the card — deliberately NOT `reportError`'s red
 * toast. Two guesses a day is the whole budget, so "that was your last one" is an
 * ordinary, expected outcome, and a toast identical to the one a crashed server
 * gets would read it as a failure on the visitor's part.
 *
 * Which refusal it is decides whether the retry survives, and `429 rate_limited`
 * is TWO refusals wearing one code (docs/API.md §3.1). `isBudgetExhausted` — a
 * 429 with no `Retry-After` — is the daily cap, and only that one drops the
 * retry. The per-IP write tier (burst 30, one token per 2s, shared with saves and
 * matches, and easy to trip from behind a NAT) also answers 429, but it clears in
 * seconds, so it keeps its retry and says so; calling that one "your allowance
 * for today" took away the single action that would have worked.
 *
 * A lapsed session is the one thing that does not belong in the card: it is not a
 * verdict about the drawing at all. It goes back to the shared `reportError`,
 * which raises the ONE sign-in gate and deliberately does not re-fire the request
 * (minutes can pass behind that modal, and re-asking would silently spend another
 * of the two). The card closes rather than sitting there pending behind a dialog.
 */
function onGuessError(err: unknown) {
    if (guessStale) return
    if (isAuthError(err)) {
        dismissGuess()
        reportError(err, 'guess your drawing')
        return
    }
    // Status first: it clears whatever the card was holding, then the payload for
    // this failure goes in beside it.
    setGuessStatus('error')
    const api = toApiError(err)
    if (isBudgetExhausted(err)) {
        guessExhausted.value = true
        guessError.value = api?.message ?? 'That is every AI guess you get today.'
        return
    }
    // The server's own sentence for a throttle is just "too many requests"; the
    // part that matters to the visitor is that waiting works, which only this
    // side knows to say (the header gives seconds, not a promise worth printing).
    guessError.value = isRateLimited(err)
        ? `${api?.message ?? 'Too many requests just now'} — try again in a moment.`
        : (api?.message ?? 'The AI could not be reached. Try again.')
}

/**
 * Ask the AI what is on the canvas. Unlike assist this sends the WHOLE document
 * (the server has to render it before it can look at it), and unlike save it is
 * capped at a couple of calls a day — so every guard here exists to stop one of
 * those being spent on nothing: a blank canvas, a double click while a call is in
 * flight, or a view that went away behind the sign-in modal.
 */
async function requestGuess() {
    if (!editor || guessPending.value) return
    // The empty-canvas guard still stops the call; what changed is that it now
    // ANSWERS. The trigger used to carry the reason in a disabled button's
    // tooltip, which a phone cannot show and a keyboard cannot reach, so the card
    // — already the single home for every outcome — says it instead.
    if (isEmpty.value) {
        setGuessStatus('idle')
        guessOpen.value = true
        return
    }
    if (!(await gated('Sign in to have the AI guess your drawing.'))) return
    // Re-check across the await: the modal can stay up for minutes, and browser
    // Back unmounts this view and nulls `editor` underneath us (as in `save()`).
    if (!editor) return
    guessStale = false
    // Open the card BEFORE firing, so the several-second wait has somewhere to live.
    setGuessStatus('pending')
    guessOpen.value = true
    guessMutation.mutate(editor.getDocument(), {
        onSuccess: (result) => {
            if (guessStale) return
            setGuessStatus('answered')
            guessResult.value = result
        },
        onError: onGuessError
    })
}

/**
 * The island trigger is a TOGGLE, not a fire button: with the card already up a
 * second click hides it instead of spending another of the day's calls. Re-asking
 * is the card's own "Guess again", where the cost is in front of you.
 */
function toggleGuess() {
    if (guessOpen.value) {
        dismissGuess()
        return
    }
    // Closed with a call still running, or with an answer that landed after it was
    // closed: that call is already spent, so re-open onto it rather than paying
    // for a second. A read guess resets the status on dismiss, so anything but
    // `idle` here is a guess nobody has actually seen yet.
    if (guessStatus.value !== 'idle') {
        guessOpen.value = true
        return
    }
    void requestGuess()
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
                    :pressed="shortcutsOpen"
                    @click="shortcutsOpen = !shortcutsOpen"
                />
                <IconButton
                    icon="layers"
                    label="Toggle layers panel"
                    placement="bottom"
                    :pressed="layersOpen"
                    @click="layersOpen = !layersOpen"
                />
                <IconButton
                    icon="assist"
                    label="AI assist — describe what to draw"
                    placement="bottom"
                    :pressed="assistOpen"
                    @click="toggleAssist"
                />
                <!-- The mirror of assist: assist draws what you say, this says what
                     you drew. It lives in THIS island rather than the top-center
                     strip because the assist panel already owns that slot (and drops
                     to its own row <=1050px), so a second panel would fight it; the
                     answer lands in the overlay card instead.

                     Deliberately NOT disabled on a blank canvas. A disabled button
                     takes no focus, and oriui's tooltip needs hover or focus-within,
                     so on a phone the reason for the dimming had nowhere to appear —
                     it was a faint eye that would not respond and would not explain.
                     The guard is still there (`requestGuess` spends nothing on an
                     empty page); it just answers in the card now. -->
                <IconButton
                    icon="guess"
                    label="Guess my drawing — ask the AI what it sees"
                    placement="bottom"
                    :pressed="guessOpen"
                    @click="toggleGuess"
                />
                <!-- Which layer new strokes / the eraser land on — shown only when the
                     panel is closed (open, the panel highlights the active row itself).
                     Click opens the panel so it doubles as an affordance. -->
                <button
                    v-if="!layersOpen"
                    class="draw__active-layer"
                    type="button"
                    :aria-label="`Active layer: ${activeLayerName}. Open layers panel`"
                    :title="`Active layer: ${activeLayerName} — click to open layers`"
                    @click="layersOpen = true"
                >
                    {{ activeLayerName }}
                </button>
            </OriSurface>
        </template>

        <!-- Top-center: the AI-assist prompt panel (toggled from the actions
             island). The shell's centering strip is pointer-events:none; the panel
             opts back in. Returned ops render as a ghost inside the editor and only
             land on Accept — the panel flips from the input to the accept/reject
             phase while a proposal is pending. -->
        <template #top-center>
            <OriSurface v-if="assistOpen" class="draw__assist" role="group" aria-label="AI assist">
                <!-- Header + explicit close: the toggle in the actions island can be
                     off-screen on narrow widths, so the panel is always dismissible
                     from within (calls the same toggleAssist). -->
                <div class="draw__assist-head">
                    <span class="draw__assist-title">AI assist</span>
                    <IconButton icon="close" label="Close AI assist" placement="bottom" @click="toggleAssist" />
                </div>
                <template v-if="pendingOps">
                    <p v-if="assistNote" class="draw__assist-note">{{ assistNote }}</p>
                    <div class="draw__assist-actions">
                        <OriButton variant="fill" radius="md" text="Accept" fluid @click="acceptAssist" />
                        <OriButton variant="outline" radius="md" text="Reject" fluid @click="rejectAssist" />
                    </div>
                </template>
                <template v-else>
                    <div class="draw__assist-row">
                        <OriInput
                            v-model="assistPrompt"
                            class="draw__assist-input"
                            aria-label="Describe what to draw"
                            placeholder="Describe what to draw…"
                            :disabled="assistPending"
                            @keydown.enter="submitAssist"
                        />
                        <OriButton
                            variant="fill"
                            radius="md"
                            text="Draw"
                            :loading="assistPending"
                            :disabled="!assistPrompt.trim() || assistPending"
                            @click="submitAssist"
                        />
                    </div>
                </template>
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
            <OriToaster position="top-center" align="center" />

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
                    @sign-in="void gated('Sign in to save and load your drawings.')"
                    @shortcuts="shortcutsOpen = true"
                />
            </Transition>

            <!-- The AI's reading of the canvas — mounted on demand, and the one
                 surface the whole feature has: the wait, the answer, every failure
                 and "draw something first" all land here. It can never share the
                 overlay with the empty-state card above, because `showHint` stands
                 down for exactly as long as this is open. -->
            <Transition name="jp-pop">
                <GuessResult
                    v-if="guessOpen"
                    class="draw__guess"
                    :status="guessStatus"
                    :label="guessResult?.label ?? null"
                    :confidence="guessResult?.confidence ?? 0"
                    :alternatives="guessResult?.alternatives ?? []"
                    :error="guessError"
                    :exhausted="guessExhausted"
                    :can-retry="!isEmpty"
                    @again="requestGuess"
                    @dismiss="dismissGuess"
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
                :pressed="menuOpen"
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

/* The active-layer chip beside the Layers toggle (shown while the panel is
   closed). A quiet text button — neutral structural hover only (DESIGN-SYSTEM
   §1), never a brand-role mix; the global focus-visible ring covers keyboard. */
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

/* EditorShell's bottom-center strip is pointer-events:none (its empty flanks
   never eat canvas events) — the toolbar opts back in so it stays interactive. */
.draw__toolbar-item {
    pointer-events: auto;
}

/* The AI-assist prompt panel in the top-center strip. Like the toolbar, the
   strip is pointer-events:none, so the panel opts back in. Clamped so it never
   spills past the viewport on a phone. */
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

/* First-run empty state — the centered welcome card. EditorShell's overlay layer
   is full-bleed but pointer-events:none so it never blocks drawing; only the
   card (pointer-events:auto) is interactive. */
.draw__empty {
    pointer-events: auto;
}

/* The AI-guess card owns its own box (width, scroll, pointer-events) — what is
   /draw's business is WHERE it sits, which is the small-screen rule below.

   The lift is declared here and applied there so the number has a name at the
   point it is explained: the overlay CENTERS its children and the toolbar owns
   the bottom ~4rem of a small screen, so a centred card can reach it. The margin
   is part of the centred box, so this 5rem buys a ~2.5rem rise — enough that the
   tools stay tappable while the answer is up, which matters because the whole
   point is to go back and draw more. */
.draw__guess {
    --jp-guess-lift: 5rem;
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

/* Tablet/phone: the assist panel leaves the shared top row so its opaque surface
   can never cover the top-left actions island (which holds the assist toggle —
   its only external close affordance) or the top-right menu toggle. It drops to
   its OWN row and goes full-width, mirroring how .draw__history avoids the same
   collision. Positioned within the (pointer-events:none) top-center region, so
   the wider box never eats canvas events; z stays region-level (below the
   z-100 drawer / z-110 toggler).

   Breakpoint = 1050px, not 768px: the panel is a CENTERED ~30rem box, so its left
   edge reaches the (chip-widened) actions island until the viewport is wide enough
   — measured overlap persists to ~1050px (verified at 800px). */
@media (width <= 1050px) {
    .draw__assist {
        position: absolute;
        /* Region top is already offset by gap_md, so +gap_md lands the panel on
           the same visual row as .draw__history (2*gap_md + island height). */
        top: calc(var(--ori-size-gap_md, 0.5rem) + 3.125rem);
        left: var(--ori-size-gap_md, 0.5rem);
        right: var(--ori-size-gap_md, 0.5rem);

        width: auto;
    }
}

/* The guess card's lift, keyed on BOTH axes because the constraint it solves is
   VERTICAL — the toolbar sits at the bottom edge and the card is centred above it.
   A width-only rule missed the case that needs it most: at 667x375 (a phone in
   landscape) the width query never fires, and a full answer there (label +
   certainty + two runner-ups, the tallest this card gets) leaves single-digit
   pixels above the toolbar. Measured with the lift on: the card ends at 231 and
   the bar starts at 309. 500px of height is where a centred card and the toolbar
   begin competing for the same screen. */
@media (width <= 600px), (height <= 500px) {
    .draw__guess {
        margin-bottom: var(--jp-guess-lift);
    }
}

@media (width <= 600px) {
    /* The mobile history island claims that second row (top-left), so the assist
       panel drops one row further to clear it. */
    .draw__assist {
        top: calc(var(--ori-size-gap_md, 0.5rem) * 2 + 3.125rem * 2);
    }

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
